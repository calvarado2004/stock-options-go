package data

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"stock-options/pkg/model"
)

// StockDataProvider interface for different stock data providers
type StockDataProvider interface {
	GetHistoricalData(ticker string, startDate time.Time, endDate time.Time) ([]model.StockData, string, error)
}

// AlphaVantageProvider is a multi-provider historical data client.
type AlphaVantageProvider struct {
	AlphaAvangeApiKey string
	BaseURL           string
	Client            *http.Client
}

func NewAlphaVantageProvider(alphaAvangeApiKey string) *AlphaVantageProvider {
	return &AlphaVantageProvider{
		AlphaAvangeApiKey: alphaAvangeApiKey,
		BaseURL:           "https://www.alphavantage.co/query",
		Client:            &http.Client{Timeout: 20 * time.Second},
	}
}

// GetHistoricalData retrieves historical data using one of multiple providers.
func (p *AlphaVantageProvider) GetHistoricalData(ticker string, startDate time.Time, endDate time.Time) ([]model.StockData, string, error) {
	if strings.TrimSpace(ticker) == "" {
		return nil, "", fmt.Errorf("ticker cannot be empty")
	}
	if p.AlphaAvangeApiKey != "" {
		alphaData, err := p.getHistoricalDataFromAlphaVantage(ticker, startDate, endDate)
		if err == nil && len(alphaData) > 0 {
			return alphaData, "alphavantage", nil
		}
	}

	yahooData, yahooErr := p.getHistoricalDataFromYahoo(ticker, startDate, endDate)
	if yahooErr == nil && len(yahooData) > 0 {
		return yahooData, "yahoo", nil
	}

	stooqData, stooqErr := p.getHistoricalDataFromStooq(ticker, startDate, endDate)
	if stooqErr == nil && len(stooqData) > 0 {
		return stooqData, "stooq", nil
	}

	if yahooErr != nil {
		return nil, "", fmt.Errorf("failed to fetch data for %s: yahoo=%v stooq=%v", ticker, yahooErr, stooqErr)
	}
	return nil, "", fmt.Errorf("failed to fetch data for %s: stooq=%v", ticker, stooqErr)
}

func (p *AlphaVantageProvider) getHistoricalDataFromYahoo(ticker string, startDate time.Time, endDate time.Time) ([]model.StockData, error) {
	period1 := startDate.UTC().Unix()
	period2 := endDate.UTC().Add(24 * time.Hour).Unix()
	query := url.Values{}
	query.Set("interval", "1d")
	query.Set("period1", strconv.FormatInt(period1, 10))
	query.Set("period2", strconv.FormatInt(period2, 10))
	query.Set("events", "history")
	query.Set("includeAdjustedClose", "true")

	endpoint := fmt.Sprintf(
		"https://query1.finance.yahoo.com/v8/finance/chart/%s?%s",
		url.PathEscape(strings.ToUpper(strings.TrimSpace(ticker))),
		query.Encode(),
	)
	resp, err := p.Client.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("yahoo request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("yahoo returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed reading yahoo body: %w", err)
	}

	type yahooResponse struct {
		Chart struct {
			Result []struct {
				Timestamp  []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Open   []interface{} `json:"open"`
						High   []interface{} `json:"high"`
						Low    []interface{} `json:"low"`
						Close  []interface{} `json:"close"`
						Volume []interface{} `json:"volume"`
					} `json:"quote"`
					Adjclose []struct {
						Adjclose []interface{} `json:"adjclose"`
					} `json:"adjclose"`
				} `json:"indicators"`
			} `json:"result"`
			Error interface{} `json:"error"`
		} `json:"chart"`
	}

	var parsed yahooResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed parsing yahoo payload: %w", err)
	}
	if parsed.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo error: %v", parsed.Chart.Error)
	}
	if len(parsed.Chart.Result) == 0 {
		return nil, fmt.Errorf("yahoo returned no results")
	}

	result := parsed.Chart.Result[0]
	if len(result.Timestamp) == 0 || len(result.Indicators.Quote) == 0 {
		return nil, fmt.Errorf("yahoo payload missing series")
	}
	quote := result.Indicators.Quote[0]
	var adj []interface{}
	if len(result.Indicators.Adjclose) > 0 {
		adj = result.Indicators.Adjclose[0].Adjclose
	}

	rows := make([]model.StockData, 0, len(result.Timestamp))
	for i, ts := range result.Timestamp {
		date := time.Unix(ts, 0).UTC().Truncate(24 * time.Hour)
		if date.Before(startDate) || date.After(endDate) {
			continue
		}
		open, ok := toFloat64At(quote.Open, i)
		if !ok {
			continue
		}
		high, ok := toFloat64At(quote.High, i)
		if !ok {
			continue
		}
		low, ok := toFloat64At(quote.Low, i)
		if !ok {
			continue
		}
		closePrice, ok := toFloat64At(quote.Close, i)
		if !ok {
			continue
		}
		volume := toInt64At(quote.Volume, i)

		var adjustedClose *float64
		if len(adj) > i {
			if adjValue, ok := toFloat64(adj[i]); ok {
				adjustedClose = &adjValue
			}
		}

		rows = append(rows, model.StockData{
			Ticker:        strings.ToUpper(ticker),
			TradingDate:   date,
			OpenPrice:     open,
			HighPrice:     high,
			LowPrice:      low,
			ClosePrice:    closePrice,
			AdjustedClose: adjustedClose,
			Volume:        volume,
		})
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no rows returned for %s from yahoo", ticker)
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].TradingDate.Before(rows[j].TradingDate)
	})
	return rows, nil
}

func (p *AlphaVantageProvider) getHistoricalDataFromAlphaVantage(ticker string, startDate time.Time, endDate time.Time) ([]model.StockData, error) {

	query := url.Values{}
	query.Set("function", "TIME_SERIES_DAILY_ADJUSTED")
	query.Set("symbol", strings.ToUpper(ticker))
	query.Set("apikey", p.AlphaAvangeApiKey)
	query.Set("outputsize", "full")

	avURL := fmt.Sprintf("%s?%s", p.BaseURL, query.Encode())
	resp, err := p.Client.Get(avURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch data from Alpha Vantage: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("alpha vantage returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var apiResponse map[string]interface{}
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		log.Printf("Error unmarshaling API response for ticker %s: %v\nRaw data: %s", ticker, err, string(body))
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	if note, ok := apiResponse["Note"].(string); ok && note != "" {
		return nil, fmt.Errorf("alpha vantage notice: %s", note)
	}
	if info, ok := apiResponse["Information"].(string); ok && info != "" {
		return nil, fmt.Errorf("alpha vantage information: %s", info)
	}
	if msg, ok := apiResponse["Error Message"].(string); ok && msg != "" {
		return nil, fmt.Errorf("alpha vantage error: %s", msg)
	}

	timeSeriesRaw, ok := apiResponse["Time Series (Daily)"]
	if !ok {
		log.Printf("No Time Series data found for ticker %s in Alpha Vantage response\nRaw data: %s", ticker, string(body))
		return nil, fmt.Errorf("no daily time series returned for %s", ticker)
	}

	tsMap, ok := timeSeriesRaw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected time series format for %s", ticker)
	}

	stockData := make([]model.StockData, 0, len(tsMap))
	for dateStr, values := range tsMap {
		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}
		if date.Before(startDate) || date.After(endDate) {
			continue
		}

		valueMap, ok := values.(map[string]interface{})
		if !ok {
			continue
		}

		openPrice, ok := valueMap["1. open"].(string)
		if !ok {
			continue
		}
		highPrice, ok := valueMap["2. high"].(string)
		if !ok {
			continue
		}
		lowPrice, ok := valueMap["3. low"].(string)
		if !ok {
			continue
		}
		closePrice, ok := valueMap["4. close"].(string)
		if !ok {
			continue
		}
		volume, ok := valueMap["6. volume"].(string)
		if !ok {
			continue
		}

		var adjClose *float64
		if adjustedCloseRaw, ok := valueMap["5. adjusted close"]; ok {
			if adjustedCloseStr, ok := adjustedCloseRaw.(string); ok {
				f, err := parseFloat(adjustedCloseStr)
				if err == nil {
					adjClose = &f
				}
			}
		}

		stockData = append(stockData, model.StockData{
			Ticker:        strings.ToUpper(ticker),
			TradingDate:   date.UTC(),
			OpenPrice:     mustParseFloat(openPrice),
			HighPrice:     mustParseFloat(highPrice),
			LowPrice:      mustParseFloat(lowPrice),
			ClosePrice:    mustParseFloat(closePrice),
			AdjustedClose: adjClose,
			Volume:        mustParseInt(volume),
		})
	}

	sort.Slice(stockData, func(i, j int) bool {
		return stockData[i].TradingDate.Before(stockData[j].TradingDate)
	})
	if len(stockData) == 0 {
		return nil, fmt.Errorf("no rows returned for %s", ticker)
	}
	return stockData, nil
}

func (p *AlphaVantageProvider) getHistoricalDataFromStooq(ticker string, startDate time.Time, endDate time.Time) ([]model.StockData, error) {
	symbol := strings.ToLower(strings.TrimSpace(ticker))
	candidates := []string{
		fmt.Sprintf("%s.us", symbol),
		symbol,
	}

	var lastErr error
	for _, candidate := range candidates {
		query := url.Values{}
		query.Set("s", candidate)
		query.Set("i", "d")
		downloadURL := fmt.Sprintf("https://stooq.com/q/d/l/?%s", query.Encode())

		resp, err := p.Client.Get(downloadURL)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("stooq returned status %d", resp.StatusCode)
			continue
		}

		reader := csv.NewReader(strings.NewReader(string(body)))
		records, err := reader.ReadAll()
		if err != nil {
			lastErr = err
			continue
		}
		if len(records) < 2 {
			lastErr = fmt.Errorf("no rows returned from stooq")
			continue
		}

		result := make([]model.StockData, 0, len(records)-1)
		for i := 1; i < len(records); i++ {
			row := records[i]
			if len(row) < 6 {
				continue
			}
			date, err := time.Parse("2006-01-02", row[0])
			if err != nil {
				continue
			}
			if date.Before(startDate) || date.After(endDate) {
				continue
			}
			openPrice, err := strconv.ParseFloat(row[1], 64)
			if err != nil {
				continue
			}
			highPrice, err := strconv.ParseFloat(row[2], 64)
			if err != nil {
				continue
			}
			lowPrice, err := strconv.ParseFloat(row[3], 64)
			if err != nil {
				continue
			}
			closePrice, err := strconv.ParseFloat(row[4], 64)
			if err != nil {
				continue
			}
			volume, err := strconv.ParseInt(row[5], 10, 64)
			if err != nil {
				volume = 0
			}

			result = append(result, model.StockData{
				Ticker:      strings.ToUpper(ticker),
				TradingDate: date.UTC(),
				OpenPrice:   openPrice,
				HighPrice:   highPrice,
				LowPrice:    lowPrice,
				ClosePrice:  closePrice,
				Volume:      volume,
			})
		}
		if len(result) == 0 {
			lastErr = fmt.Errorf("no rows in range for %s", ticker)
			continue
		}

		sort.Slice(result, func(i, j int) bool {
			return result[i].TradingDate.Before(result[j].TradingDate)
		})
		return result, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no provider succeeded")
	}
	return nil, fmt.Errorf("failed to fetch data for %s: %w", ticker, lastErr)
}

// Helper functions for parsing API responses
func mustParseFloat(s string) float64 {
	f, err := parseFloat(s)
	if err != nil {
		log.Printf("Failed to parse float: %s", s)
		return 0.0
	}
	return f
}

func mustParseInt(s string) int64 {
	i, err := parseInt(s)
	if err != nil {
		log.Printf("Failed to parse int: %s", s)
		return 0
	}
	return i
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

func parseInt(s string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
}

func toFloat64At(values []interface{}, idx int) (float64, bool) {
	if len(values) <= idx {
		return 0, false
	}
	return toFloat64(values[idx])
}

func toFloat64(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int64:
		return float64(v), true
	case int:
		return float64(v), true
	default:
		return 0, false
	}
}

func toInt64At(values []interface{}, idx int) int64 {
	if len(values) <= idx {
		return 0
	}
	value, ok := toFloat64(values[idx])
	if !ok {
		return 0
	}
	return int64(value)
}
