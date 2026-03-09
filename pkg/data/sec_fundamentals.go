package data

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"stock-options/pkg/model"
)

type SECFundamentalsClient struct {
	Client    *http.Client
	UserAgent string

	mu          sync.RWMutex
	tickerToCIK map[string]string
}

func NewSECFundamentalsClient(userAgent string) *SECFundamentalsClient {
	if strings.TrimSpace(userAgent) == "" {
		userAgent = "stock-options-go/1.0 (contact: admin@example.com)"
	}
	return &SECFundamentalsClient{
		Client:      &http.Client{Timeout: 20 * time.Second},
		UserAgent:   userAgent,
		tickerToCIK: map[string]string{},
	}
}

func (c *SECFundamentalsClient) GetDuPontAnalysis(ticker string) (*model.DuPontAnalysis, error) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if ticker == "" {
		return nil, fmt.Errorf("ticker is required")
	}
	if c.Client == nil {
		c.Client = &http.Client{Timeout: 20 * time.Second}
	}

	cik, err := c.lookupCIK(ticker)
	if err != nil {
		return nil, err
	}

	body, err := c.getURL(fmt.Sprintf("https://data.sec.gov/api/xbrl/companyfacts/CIK%s.json", cik))
	if err != nil {
		return nil, err
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse sec companyfacts payload: %w", err)
	}

	netIncomeLatest, _, periodEndIncome, okNI := extractAnnualUSDFacts(payload, []string{
		"NetIncomeLoss",
		"ProfitLoss",
	})
	revenueLatest, _, periodEndRevenue, okRev := extractAnnualUSDFacts(payload, []string{
		"Revenues",
		"RevenueFromContractWithCustomerExcludingAssessedTax",
		"SalesRevenueNet",
	})
	assetsLatest, assetsPrev, periodEndAssets, okAssets := extractAnnualUSDFacts(payload, []string{
		"Assets",
	})
	equityLatest, equityPrev, periodEndEquity, okEquity := extractAnnualUSDFacts(payload, []string{
		"StockholdersEquity",
		"StockholdersEquityIncludingPortionAttributableToNoncontrollingInterest",
	})

	if !okNI || !okRev || !okAssets || !okEquity {
		return &model.DuPontAnalysis{
			Available: false,
			Message:   "SEC facts missing one or more DuPont inputs for this ticker.",
			Source:    "sec-companyfacts",
		}, nil
	}

	avgAssets := assetsLatest
	if assetsPrev > 0 {
		avgAssets = (assetsLatest + assetsPrev) / 2.0
	}
	avgEquity := equityLatest
	if equityPrev > 0 {
		avgEquity = (equityLatest + equityPrev) / 2.0
	}
	if revenueLatest == 0 || avgAssets == 0 || avgEquity == 0 {
		return &model.DuPontAnalysis{
			Available: false,
			Message:   "SEC facts contain zero values that prevent DuPont ratio computation.",
			Source:    "sec-companyfacts",
		}, nil
	}

	netMargin := netIncomeLatest / revenueLatest
	assetTurnover := revenueLatest / avgAssets
	equityMultiplier := avgAssets / avgEquity
	roe := netMargin * assetTurnover * equityMultiplier

	periodEnd := maxTime(periodEndIncome, periodEndRevenue, periodEndAssets, periodEndEquity)

	return &model.DuPontAnalysis{
		Available:        true,
		Message:          "Computed from SEC companyfacts annual filings.",
		Source:           "sec-companyfacts",
		PeriodEnd:        periodEnd,
		NetProfitMargin:  netMargin,
		AssetTurnover:    assetTurnover,
		EquityMultiplier: equityMultiplier,
		ReturnOnEquity:   roe,
		NetIncome:        netIncomeLatest,
		Revenue:          revenueLatest,
		AverageAssets:    avgAssets,
		AverageEquity:    avgEquity,
	}, nil
}

func (c *SECFundamentalsClient) lookupCIK(ticker string) (string, error) {
	c.mu.RLock()
	if cik, ok := c.tickerToCIK[ticker]; ok {
		c.mu.RUnlock()
		return cik, nil
	}
	c.mu.RUnlock()

	body, err := c.getURL("https://www.sec.gov/files/company_tickers.json")
	if err != nil {
		return "", fmt.Errorf("failed to fetch sec ticker list: %w", err)
	}

	var payload map[string]struct {
		CIKStr int    `json:"cik_str"`
		Ticker string `json:"ticker"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("failed parsing sec ticker list: %w", err)
	}

	tmp := make(map[string]string, len(payload))
	for _, row := range payload {
		t := strings.ToUpper(strings.TrimSpace(row.Ticker))
		if t == "" || row.CIKStr == 0 {
			continue
		}
		tmp[t] = fmt.Sprintf("%010d", row.CIKStr)
	}

	c.mu.Lock()
	c.tickerToCIK = tmp
	c.mu.Unlock()

	if cik, ok := tmp[ticker]; ok {
		return cik, nil
	}
	return "", fmt.Errorf("ticker %s not found in sec ticker mapping", ticker)
}

func (c *SECFundamentalsClient) getURL(u string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sec returned status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return b, nil
}

type secFactEntry struct {
	Val  float64
	End  time.Time
	Form string
	FP   string
}

func extractAnnualUSDFacts(payload map[string]interface{}, concepts []string) (latest float64, prev float64, periodEnd time.Time, ok bool) {
	usGaap, ok := digMap(payload, "facts", "us-gaap")
	if !ok {
		return 0, 0, time.Time{}, false
	}

	for _, concept := range concepts {
		conceptMap, ok := asMap(usGaap[concept])
		if !ok {
			continue
		}
		units, ok := asMap(conceptMap["units"])
		if !ok {
			continue
		}
		unitSlice, ok := asSlice(units["USD"])
		if !ok {
			continue
		}

		entries := make([]secFactEntry, 0, len(unitSlice))
		for _, raw := range unitSlice {
			m, ok := asMap(raw)
			if !ok {
				continue
			}
			form := strings.ToUpper(strings.TrimSpace(asString(m["form"])))
			fp := strings.ToUpper(strings.TrimSpace(asString(m["fp"])))
			if form != "10-K" && form != "20-F" && form != "40-F" {
				continue
			}
			if fp != "" && fp != "FY" {
				continue
			}
			endStr := asString(m["end"])
			end, err := time.Parse("2006-01-02", endStr)
			if err != nil {
				continue
			}
			val, ok := asFloat64(m["val"])
			if !ok {
				continue
			}
			entries = append(entries, secFactEntry{Val: val, End: end.UTC(), Form: form, FP: fp})
		}

		if len(entries) == 0 {
			continue
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].End.After(entries[j].End) })
		latest = entries[0].Val
		periodEnd = entries[0].End
		if len(entries) > 1 {
			prev = entries[1].Val
		}
		return latest, prev, periodEnd, true
	}

	return 0, 0, time.Time{}, false
}

func digMap(m map[string]interface{}, keys ...string) (map[string]interface{}, bool) {
	cur := m
	for i, k := range keys {
		v, ok := cur[k]
		if !ok {
			return nil, false
		}
		if i == len(keys)-1 {
			return asMap(v)
		}
		next, ok := asMap(v)
		if !ok {
			return nil, false
		}
		cur = next
	}
	return nil, false
}

func asMap(v interface{}) (map[string]interface{}, bool) {
	m, ok := v.(map[string]interface{})
	return m, ok
}

func asSlice(v interface{}) ([]interface{}, bool) {
	s, ok := v.([]interface{})
	return s, ok
}

func asString(v interface{}) string {
	s, _ := v.(string)
	return s
}

func asFloat64(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err == nil {
			return f, true
		}
	}
	return 0, false
}

func maxTime(times ...time.Time) time.Time {
	var best time.Time
	for _, t := range times {
		if t.After(best) {
			best = t
		}
	}
	return best
}
