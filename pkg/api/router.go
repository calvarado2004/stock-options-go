package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"stock-options/pkg/data"
	"stock-options/pkg/model"
	"stock-options/pkg/storage"

	"github.com/gorilla/mux"
)

type Router struct {
	db           *storage.Database
	dataProvider data.StockDataProvider
}

type ingestResponse struct {
	Ticker             string                `json:"ticker"`
	StartDate          string                `json:"start_date"`
	EndDate            string                `json:"end_date"`
	FetchedRecordCount int                   `json:"fetched_record_count"`
	UsingCachedData    bool                  `json:"using_cached_data"`
	ProviderUsed       string                `json:"provider_used"`
	Forecast           *model.ForecastResult `json:"forecast"`
}

func NewRouter() *Router {
	dbDriver := os.Getenv("DB_DRIVER")
	dbDSN := os.Getenv("DB_DSN")
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "stock_data.db"
	}
	db, err := storage.NewDatabaseWithConfig(dbDriver, dbDSN, dbPath)
	if err != nil {
		panic(err)
	}

	alphaAvangeApiKey := os.Getenv("ALPHA_API_KEY")
	dataProvider := data.NewAlphaVantageProvider(alphaAvangeApiKey)

	return NewRouterWithDependencies(db, dataProvider)
}

func NewRouterWithDependencies(db *storage.Database, provider data.StockDataProvider) *Router {
	return &Router{
		db:           db,
		dataProvider: provider,
	}
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	router := mux.NewRouter()

	router.HandleFunc("/ingest", r.ingestHandler).Methods("POST")
	router.HandleFunc("/data", r.dataHandler).Methods("GET")
	router.HandleFunc("/forecast", r.forecastHandler).Methods("GET")
	router.HandleFunc("/healthz", r.healthHandler).Methods("GET")

	router.ServeHTTP(w, req)
}

func (r *Router) ingestHandler(w http.ResponseWriter, req *http.Request) {
	ticker := strings.ToUpper(strings.TrimSpace(req.URL.Query().Get("ticker")))
	if ticker == "" {
		http.Error(w, "Ticker parameter is required", http.StatusBadRequest)
		return
	}
	forceRefresh := strings.EqualFold(strings.TrimSpace(req.URL.Query().Get("refresh")), "true") ||
		strings.TrimSpace(req.URL.Query().Get("refresh")) == "1"

	endDate := time.Now().UTC()
	startDate := endDate.AddDate(-5, 0, 0)

	latestTradingDate, hasData, err := r.db.GetLatestTradingDate(ticker)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to inspect existing data for %s: %v", ticker, err), http.StatusInternalServerError)
		return
	}

	usingCachedData := false
	fetchStart := startDate
	if hasData {
		// Cache-first strategy: if we already have data, prefer local DB unless
		// caller explicitly requests refresh=true.
		if !forceRefresh {
			usingCachedData = true
		} else {
			fetchStart = latestTradingDate.AddDate(0, 0, 1)
		}
	}

	fetchedCount := 0
	providerUsed := "cache"
	if !usingCachedData {
		historicalData, source, fetchErr := r.dataProvider.GetHistoricalData(ticker, fetchStart, endDate)
		if fetchErr != nil {
			if hasData {
				// Providers can be temporarily unavailable or rate-limited.
				// If we already have local history, continue with cached data.
				usingCachedData = true
				providerUsed = "cache"
				log.Printf("{\"ticker\":\"%s\",\"provider_used\":\"cache\",\"fallback_reason\":%q}", ticker, fetchErr.Error())
			} else {
				http.Error(w, fmt.Sprintf("Failed to fetch data for %s: %v", ticker, fetchErr), http.StatusBadGateway)
				return
			}
		} else {
			providerUsed = source
			fetchedCount = len(historicalData)

			if fetchedCount > 0 {
				if err := r.db.SaveStockData(historicalData); err != nil {
					http.Error(w, fmt.Sprintf("Failed to store data for %s: %v", ticker, err), http.StatusInternalServerError)
					return
				}
			}
		}
	}

	hasStoredData, err := r.db.HasStockData(ticker)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to validate stored data for %s: %v", ticker, err), http.StatusInternalServerError)
		return
	}
	if !hasStoredData {
		http.Error(w, fmt.Sprintf("No historical data stored for %s after ingestion attempt", ticker), http.StatusBadGateway)
		return
	}

	forecast, err := r.db.GenerateForecast(ticker)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate forecast for %s: %v", ticker, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	resp := ingestResponse{
		Ticker:             ticker,
		StartDate:          startDate.Format("2006-01-02"),
		EndDate:            endDate.Format("2006-01-02"),
		FetchedRecordCount: fetchedCount,
		UsingCachedData:    usingCachedData,
		ProviderUsed:       providerUsed,
		Forecast:           forecast,
	}
	log.Printf("{\"ticker\":\"%s\",\"provider_used\":\"%s\",\"using_cached_data\":%t,\"fetched_record_count\":%d}", ticker, providerUsed, usingCachedData, fetchedCount)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, fmt.Sprintf("Failed to write response: %v", err), http.StatusInternalServerError)
	}
}

func (r *Router) dataHandler(w http.ResponseWriter, req *http.Request) {
	ticker := strings.ToUpper(strings.TrimSpace(req.URL.Query().Get("ticker")))
	if ticker == "" {
		http.Error(w, "Ticker parameter is required", http.StatusBadRequest)
		return
	}

	startDateStr := req.URL.Query().Get("start_date")
	endDateStr := req.URL.Query().Get("end_date")

	var startDate, endDate time.Time

	if startDateStr != "" {
		var err error
		startDate, err = time.Parse("2006-01-02", startDateStr)
		if err != nil {
			http.Error(w, "Invalid start date format. Use YYYY-MM-DD.", http.StatusBadRequest)
			return
		}
	} else {
		startDate = time.Now().UTC().AddDate(-5, 0, 0)
	}

	if endDateStr != "" {
		var err error
		endDate, err = time.Parse("2006-01-02", endDateStr)
		if err != nil {
			http.Error(w, "Invalid end date format. Use YYYY-MM-DD.", http.StatusBadRequest)
			return
		}
	} else {
		endDate = time.Now().UTC()
	}

	data, err := r.db.GetStockData(ticker, startDate, endDate)
	if err != nil {
		http.Error(w, "Failed to retrieve data", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	resp := map[string]interface{}{
		"ticker":     ticker,
		"start_date": startDate.Format("2006-01-02"),
		"end_date":   endDate.Format("2006-01-02"),
		"data_count": len(data),
		"data":       data,
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, fmt.Sprintf("Failed to write response: %v", err), http.StatusInternalServerError)
	}
}

func (r *Router) forecastHandler(w http.ResponseWriter, req *http.Request) {
	ticker := strings.ToUpper(strings.TrimSpace(req.URL.Query().Get("ticker")))
	if ticker == "" {
		http.Error(w, "Ticker parameter is required", http.StatusBadRequest)
		return
	}

	forecast, err := r.db.GetForecastResult(ticker)
	if err != nil {
		http.Error(w, "No forecast data available for this ticker", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(forecast); err != nil {
		http.Error(w, fmt.Sprintf("Failed to write response: %v", err), http.StatusInternalServerError)
	}
}

func (r *Router) healthHandler(w http.ResponseWriter, req *http.Request) {
	if err := r.db.Ping(); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		http.Error(w, fmt.Sprintf("Failed to write response: %v", err), http.StatusInternalServerError)
	}
}
