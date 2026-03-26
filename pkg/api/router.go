package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"stock-options/pkg/data"
	"stock-options/pkg/forecast"
	"stock-options/pkg/ml"
	"stock-options/pkg/model"
	"stock-options/pkg/storage"

	"github.com/gorilla/mux"
)

type Router struct {
	db                 *storage.Database
	dataProvider       data.StockDataProvider
	secFundamentalsCli *data.SECFundamentalsClient
	externalMLClient   ml.Client
	mlMu               sync.RWMutex
	mlJobs             map[string]*mlJobState
}

type mlJobState struct {
	Ticker      string
	JobID       string
	Status      string
	Message     string
	SubmittedAt time.Time
	UpdatedAt   time.Time
}

type mlAnalysisResponse struct {
	Ticker   string                  `json:"ticker"`
	JobID    string                  `json:"job_id,omitempty"`
	Status   string                  `json:"status"`
	Message  string                  `json:"message,omitempty"`
	Analysis *model.AdvancedAnalysis `json:"analysis,omitempty"`
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

	return &Router{
		db:                 db,
		dataProvider:       dataProvider,
		secFundamentalsCli: data.NewSECFundamentalsClient(os.Getenv("SEC_USER_AGENT")),
		externalMLClient:   ml.NewHTTPClientFromEnv(),
		mlJobs:             make(map[string]*mlJobState),
	}
}

func NewRouterWithDependencies(db *storage.Database, provider data.StockDataProvider) *Router {
	return &Router{
		db:           db,
		dataProvider: provider,
		mlJobs:       make(map[string]*mlJobState),
	}
}

func NewRouterWithDependenciesAndClients(db *storage.Database, provider data.StockDataProvider, sec *data.SECFundamentalsClient, externalML ml.Client) *Router {
	return &Router{
		db:                 db,
		dataProvider:       provider,
		secFundamentalsCli: sec,
		externalMLClient:   externalML,
		mlJobs:             make(map[string]*mlJobState),
	}
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	router := mux.NewRouter()

	router.HandleFunc("/ingest", r.ingestHandler).Methods("POST")
	router.HandleFunc("/data", r.dataHandler).Methods("GET")
	router.HandleFunc("/forecast", r.forecastHandler).Methods("GET")
	router.HandleFunc("/analysis", r.analysisHandler).Methods("GET")
	router.HandleFunc("/ml-analysis", r.mlAnalysisHandler).Methods("POST")
	router.HandleFunc("/ml-analysis-status", r.mlAnalysisStatusHandler).Methods("GET")
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

	endDate := time.Now().UTC().Truncate(24 * time.Hour)
	startDate := endDate.AddDate(-5, 0, 0)

	latestTradingDate, hasData, err := r.db.GetLatestTradingDate(ticker)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to inspect existing data for %s: %v", ticker, err), http.StatusInternalServerError)
		return
	}

	usingCachedData := false
	fetchStart := startDate
	if hasData {
		latestTradingDate = latestTradingDate.UTC().Truncate(24 * time.Hour)
		nextMissingDate := latestTradingDate.AddDate(0, 0, 1)
		if nextMissingDate.After(endDate) {
			usingCachedData = true
		} else {
			// Keep cache as primary source, but backfill only the missing gap
			// between the latest stored trading date and today.
			fetchStart = nextMissingDate
		}
		if forceRefresh && fetchStart.Before(startDate) {
			fetchStart = startDate
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

func (r *Router) analysisHandler(w http.ResponseWriter, req *http.Request) {
	ticker := strings.ToUpper(strings.TrimSpace(req.URL.Query().Get("ticker")))
	if ticker == "" {
		http.Error(w, "Ticker parameter is required", http.StatusBadRequest)
		return
	}

	analysis, err := r.buildLocalAnalysis(ticker)
	if err != nil {
		http.Error(w, "No advanced analysis available for this ticker", http.StatusNotFound)
		return
	}
	if parseBool(req.URL.Query().Get("include_ml")) {
		if insight, pending, msg := r.getOrRequestExternalML(req.Context(), ticker, analysis); insight != nil {
			analysis.ExternalML = insight
			applyExternalRecommendation(&analysis.Signal, insight)
		} else if pending {
			analysis.ExternalML = &model.ExternalMLInsight{
				Status:      "pending",
				Message:     msg,
				RequestedAt: time.Now().UTC(),
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(analysis); err != nil {
		http.Error(w, fmt.Sprintf("Failed to write response: %v", err), http.StatusInternalServerError)
	}
}

func (r *Router) mlAnalysisHandler(w http.ResponseWriter, req *http.Request) {
	ticker := strings.ToUpper(strings.TrimSpace(req.URL.Query().Get("ticker")))
	if ticker == "" {
		http.Error(w, "Ticker parameter is required", http.StatusBadRequest)
		return
	}
	if r.externalMLClient == nil {
		http.Error(w, "External ML service is not configured", http.StatusServiceUnavailable)
		return
	}

	analysis, err := r.buildLocalAnalysis(ticker)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to build ML analysis for %s: %v", ticker, err), http.StatusBadGateway)
		return
	}

	forecastResult, err := r.getForecastForML(ticker)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load forecast for %s: %v", ticker, err), http.StatusBadGateway)
		return
	}
	submit, err := r.externalMLClient.SubmitAnalysisAndForecast(req.Context(), ticker, forecastResult, analysis)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to submit ML analysis for %s: %v", ticker, err), http.StatusBadGateway)
		return
	}

	resp := mlAnalysisResponse{
		Ticker:  ticker,
		Status:  strings.ToLower(strings.TrimSpace(submit.Status)),
		Message: strings.TrimSpace(submit.Message),
	}
	if resp.Status == "" {
		resp.Status = "queued"
	}

	if submit.Insight != nil {
		analysis.ExternalML = submit.Insight
		applyExternalRecommendation(&analysis.Signal, submit.Insight)
		resp.Status = "completed"
		resp.Analysis = analysis
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, fmt.Sprintf("Failed to write response: %v", err), http.StatusInternalServerError)
		}
		return
	}

	resp.JobID = strings.TrimSpace(submit.JobID)
	r.setMLJob(ticker, resp.JobID, resp.Status, resp.Message)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, fmt.Sprintf("Failed to write response: %v", err), http.StatusInternalServerError)
	}
}

func (r *Router) mlAnalysisStatusHandler(w http.ResponseWriter, req *http.Request) {
	ticker := strings.ToUpper(strings.TrimSpace(req.URL.Query().Get("ticker")))
	if ticker == "" {
		http.Error(w, "Ticker parameter is required", http.StatusBadRequest)
		return
	}
	if r.externalMLClient == nil {
		http.Error(w, "External ML service is not configured", http.StatusServiceUnavailable)
		return
	}

	state, ok := r.getMLJob(ticker)
	if !ok || strings.TrimSpace(state.JobID) == "" {
		http.Error(w, "No pending ML job for this ticker", http.StatusNotFound)
		return
	}

	statusResult, err := r.externalMLClient.GetJobStatus(req.Context(), state.JobID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to poll ML status for %s: %v", ticker, err), http.StatusBadGateway)
		return
	}

	resp := mlAnalysisResponse{
		Ticker:  ticker,
		JobID:   state.JobID,
		Status:  statusResult.Status,
		Message: statusResult.Message,
	}

	if statusResult.Insight != nil {
		analysis, buildErr := r.buildLocalAnalysis(ticker)
		if buildErr != nil {
			http.Error(w, fmt.Sprintf("Failed to rebuild analysis for %s: %v", ticker, buildErr), http.StatusBadGateway)
			return
		}
		analysis.ExternalML = statusResult.Insight
		applyExternalRecommendation(&analysis.Signal, statusResult.Insight)
		resp.Status = "completed"
		resp.Analysis = analysis
		r.setMLJob(ticker, state.JobID, "completed", statusResult.Message)
	} else {
		r.setMLJob(ticker, state.JobID, resp.Status, resp.Message)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, fmt.Sprintf("Failed to write response: %v", err), http.StatusInternalServerError)
	}
}

func (r *Router) buildLocalAnalysis(ticker string) (*model.AdvancedAnalysis, error) {
	analysis, err := r.db.GenerateAdvancedAnalysis(ticker)
	if err != nil {
		return nil, err
	}
	if r.secFundamentalsCli != nil {
		if dupont, dupontErr := r.secFundamentalsCli.GetDuPontAnalysis(ticker); dupontErr == nil && dupont != nil {
			analysis.DuPont = *dupont
			analysis.Signal = forecast.EvaluateTradeSignal(analysis.CurrentPrice, analysis.MonteCarlo, analysis.AR1, analysis.DuPont)
		}
	}
	return analysis, nil
}

func (r *Router) getOrRequestExternalML(ctx context.Context, ticker string, analysis *model.AdvancedAnalysis) (*model.ExternalMLInsight, bool, string) {
	if r.externalMLClient == nil {
		return nil, false, ""
	}
	if state, ok := r.getMLJob(ticker); ok && strings.TrimSpace(state.JobID) != "" {
		statusResult, err := r.externalMLClient.GetJobStatus(ctx, state.JobID)
		if err != nil {
			return &model.ExternalMLInsight{
				Status:      "error",
				Message:     err.Error(),
				RequestedAt: time.Now().UTC(),
			}, false, err.Error()
		}
		if statusResult.Insight != nil {
			r.setMLJob(ticker, state.JobID, "completed", statusResult.Message)
			return statusResult.Insight, false, statusResult.Message
		}
		r.setMLJob(ticker, state.JobID, statusResult.Status, statusResult.Message)
		return nil, true, pendingMessage(state.JobID, statusResult.Status, statusResult.Message)
	}

	forecastResult, err := r.getForecastForML(ticker)
	if err != nil {
		return &model.ExternalMLInsight{
			Status:      "error",
			Message:     err.Error(),
			RequestedAt: time.Now().UTC(),
		}, false, err.Error()
	}
	submit, err := r.externalMLClient.SubmitAnalysisAndForecast(ctx, ticker, forecastResult, analysis)
	if err != nil {
		return &model.ExternalMLInsight{
			Status:      "error",
			Message:     err.Error(),
			RequestedAt: time.Now().UTC(),
		}, false, err.Error()
	}
	if submit.Insight != nil {
		return submit.Insight, false, submit.Message
	}
	jobID := strings.TrimSpace(submit.JobID)
	status := strings.ToLower(strings.TrimSpace(submit.Status))
	if status == "" {
		status = "queued"
	}
	r.setMLJob(ticker, jobID, status, submit.Message)
	return nil, true, pendingMessage(jobID, status, submit.Message)
}

func pendingMessage(jobID, status, message string) string {
	msg := strings.TrimSpace(message)
	if msg != "" {
		return msg
	}
	if strings.TrimSpace(jobID) != "" {
		return fmt.Sprintf("External ML job %s is %s", jobID, status)
	}
	return "External ML job is running"
}

func (r *Router) getForecastForML(ticker string) (*model.ForecastResult, error) {
	forecastResult, err := r.db.GetForecastResult(ticker)
	if err == nil {
		return forecastResult, nil
	}
	forecastResult, err = r.db.GenerateForecast(ticker)
	if err != nil {
		return nil, fmt.Errorf("unable to load forecast for external ML request: %w", err)
	}
	return forecastResult, nil
}

func (r *Router) getMLJob(ticker string) (*mlJobState, bool) {
	r.mlMu.RLock()
	defer r.mlMu.RUnlock()
	state, ok := r.mlJobs[ticker]
	if !ok {
		return nil, false
	}
	cpy := *state
	return &cpy, true
}

func (r *Router) setMLJob(ticker, jobID, status, message string) {
	r.mlMu.Lock()
	defer r.mlMu.Unlock()
	now := time.Now().UTC()
	state, ok := r.mlJobs[ticker]
	if !ok {
		r.mlJobs[ticker] = &mlJobState{
			Ticker:      ticker,
			JobID:       strings.TrimSpace(jobID),
			Status:      strings.ToLower(strings.TrimSpace(status)),
			Message:     strings.TrimSpace(message),
			SubmittedAt: now,
			UpdatedAt:   now,
		}
		return
	}
	if strings.TrimSpace(jobID) != "" {
		state.JobID = strings.TrimSpace(jobID)
	}
	state.Status = strings.ToLower(strings.TrimSpace(status))
	state.Message = strings.TrimSpace(message)
	state.UpdatedAt = now
}

func applyExternalRecommendation(signal *model.TradeSignal, insight *model.ExternalMLInsight) {
	if signal == nil || insight == nil {
		return
	}
	rec := insight.Recommendation
	if rec.Action != "" {
		signal.Action = rec.Action
	}
	if rec.Confidence != "" {
		signal.Confidence = rec.Confidence
	}
	signal.Score += rec.ScoreDelta
	for _, reason := range rec.Rationale {
		trimmed := strings.TrimSpace(reason)
		if trimmed != "" {
			signal.Reasons = append(signal.Reasons, "External ML: "+trimmed)
		}
	}
	if strings.TrimSpace(signal.GeneratedBy) == "" {
		signal.GeneratedBy = "rule-based-v1+external-ml"
		return
	}
	if !strings.Contains(signal.GeneratedBy, "external-ml") {
		signal.GeneratedBy = signal.GeneratedBy + "+external-ml"
	}
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
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
