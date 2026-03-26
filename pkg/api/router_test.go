package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"stock-options/pkg/ml"
	"stock-options/pkg/model"
	"stock-options/pkg/storage"
)

type mockProvider struct{}

func (m *mockProvider) GetHistoricalData(ticker string, startDate time.Time, endDate time.Time) ([]model.StockData, string, error) {
	if ticker == "ERR" {
		return nil, "", fmt.Errorf("provider failure")
	}

	baseDate := time.Date(2023, 7, 3, 0, 0, 0, 0, time.UTC)
	rows := make([]model.StockData, 0, 520)
	for i := 0; i < 520; i++ {
		d := baseDate.AddDate(0, 0, i)
		price := 100.0 + float64(i)*0.2
		adj := price * 1.001
		rows = append(rows, model.StockData{
			Ticker:        ticker,
			TradingDate:   d,
			OpenPrice:     price - 0.5,
			HighPrice:     price + 0.8,
			LowPrice:      price - 0.9,
			ClosePrice:    price,
			AdjustedClose: &adj,
			Volume:        1000 + int64(i*5),
		})
	}
	return rows, "yahoo", nil
}

func newTestRouter(t *testing.T) *Router {
	t.Helper()
	db, err := storage.NewDatabase(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	return NewRouterWithDependencies(db, &mockProvider{})
}

func TestRouter(t *testing.T) {
	router := newTestRouter(t)

	req, err := http.NewRequest("POST", "/ingest?ticker=TEST", nil)
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if status := recorder.Code; status != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, status)
	}
}

func TestHealthEndpoint(t *testing.T) {
	router := newTestRouter(t)

	req, err := http.NewRequest("GET", "/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if status := recorder.Code; status != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, status)
	}
}

type failProvider struct{}

func (m *failProvider) GetHistoricalData(ticker string, startDate time.Time, endDate time.Time) ([]model.StockData, string, error) {
	return nil, "", fmt.Errorf("yahoo returned status 429")
}

func TestIngestFallsBackToCacheWhenProviderFails(t *testing.T) {
	db, err := storage.NewDatabase(":memory:")
	if err != nil {
		t.Fatal(err)
	}

	adj := 101.0
	seed := []model.StockData{
		{
			Ticker:        "PSTG",
			TradingDate:   time.Now().UTC().AddDate(-1, 0, 0).Truncate(24 * time.Hour),
			OpenPrice:     100,
			HighPrice:     103,
			LowPrice:      99,
			ClosePrice:    102,
			AdjustedClose: &adj,
			Volume:        2000,
		},
		{
			Ticker:      "PSTG",
			TradingDate: time.Now().UTC().AddDate(0, 0, -7).Truncate(24 * time.Hour),
			OpenPrice:   98,
			HighPrice:   101,
			LowPrice:    96,
			ClosePrice:  99,
			Volume:      1800,
		},
	}
	if err := db.SaveStockData(seed); err != nil {
		t.Fatalf("failed to seed data: %v", err)
	}

	router := NewRouterWithDependencies(db, &failProvider{})
	req, err := http.NewRequest("POST", "/ingest?ticker=PSTG", nil)
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if status := recorder.Code; status != http.StatusOK {
		t.Fatalf("Expected status %d, got %d; body=%s", http.StatusOK, status, recorder.Body.String())
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if payload["provider_used"] != "cache" {
		t.Fatalf("expected provider_used cache, got %#v", payload["provider_used"])
	}
	if payload["using_cached_data"] != true {
		t.Fatalf("expected using_cached_data true, got %#v", payload["using_cached_data"])
	}
}

type countingProvider struct {
	calls     int
	lastStart time.Time
	lastEnd   time.Time
}

func buildSyntheticSeries(ticker string, start time.Time, days int) []model.StockData {
	rows := make([]model.StockData, 0, days)
	for i := 0; i < days; i++ {
		d := start.AddDate(0, 0, i).UTC().Truncate(24 * time.Hour)
		price := 50.0 + float64(i)*0.12
		adj := price * 1.001
		rows = append(rows, model.StockData{
			Ticker:        ticker,
			TradingDate:   d,
			OpenPrice:     price - 0.5,
			HighPrice:     price + 0.8,
			LowPrice:      price - 0.9,
			ClosePrice:    price,
			AdjustedClose: &adj,
			Volume:        1000 + int64(i),
		})
	}
	return rows
}

func (m *countingProvider) GetHistoricalData(ticker string, startDate time.Time, endDate time.Time) ([]model.StockData, string, error) {
	m.calls++
	m.lastStart = startDate
	m.lastEnd = endDate

	base := startDate.UTC().Truncate(24 * time.Hour)
	if base.After(endDate) {
		return []model.StockData{}, "yahoo", nil
	}

	rows := make([]model.StockData, 0, 5)
	for i := 0; i < 5; i++ {
		d := base.AddDate(0, 0, i)
		if d.After(endDate) {
			break
		}
		price := 70.0 + float64(i)
		rows = append(rows, model.StockData{
			Ticker:      ticker,
			TradingDate: d,
			OpenPrice:   price - 1,
			HighPrice:   price + 1,
			LowPrice:    price - 2,
			ClosePrice:  price,
			Volume:      1000 + int64(i),
		})
	}
	return rows, "yahoo", nil
}

func TestIngestUsesCacheWhenAlreadyCurrentWithoutCallingProvider(t *testing.T) {
	db, err := storage.NewDatabase(":memory:")
	if err != nil {
		t.Fatal(err)
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	seed := buildSyntheticSeries("PSTG", today.AddDate(-2, 0, -40), 771)
	if err := db.SaveStockData(seed); err != nil {
		t.Fatalf("failed to seed data: %v", err)
	}

	cp := &countingProvider{}
	router := NewRouterWithDependencies(db, cp)
	req, err := http.NewRequest("POST", "/ingest?ticker=PSTG", nil)
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if status := recorder.Code; status != http.StatusOK {
		t.Fatalf("Expected status %d, got %d; body=%s", http.StatusOK, status, recorder.Body.String())
	}
	if cp.calls != 0 {
		t.Fatalf("expected provider calls 0, got %d", cp.calls)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if payload["provider_used"] != "cache" {
		t.Fatalf("expected provider_used cache, got %#v", payload["provider_used"])
	}
	if payload["using_cached_data"] != true {
		t.Fatalf("expected using_cached_data true, got %#v", payload["using_cached_data"])
	}
}

func TestIngestFetchesOnlyMissingDatesWhenCacheIsStale(t *testing.T) {
	db, err := storage.NewDatabase(":memory:")
	if err != nil {
		t.Fatal(err)
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	staleLatest := today.AddDate(0, 0, -3)
	seed := buildSyntheticSeries("PSTG", today.AddDate(-2, 0, -40), 768)
	if err := db.SaveStockData(seed); err != nil {
		t.Fatalf("failed to seed data: %v", err)
	}

	cp := &countingProvider{}
	router := NewRouterWithDependencies(db, cp)
	req, err := http.NewRequest("POST", "/ingest?ticker=PSTG", nil)
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if status := recorder.Code; status != http.StatusOK {
		t.Fatalf("Expected status %d, got %d; body=%s", http.StatusOK, status, recorder.Body.String())
	}
	if cp.calls != 1 {
		t.Fatalf("expected provider calls 1, got %d", cp.calls)
	}

	expectedStart := staleLatest.AddDate(0, 0, 1)
	if !cp.lastStart.Equal(expectedStart) {
		t.Fatalf("expected incremental fetch start %s, got %s", expectedStart.Format("2006-01-02"), cp.lastStart.Format("2006-01-02"))
	}
	if !cp.lastEnd.Equal(today) {
		t.Fatalf("expected incremental fetch end %s, got %s", today.Format("2006-01-02"), cp.lastEnd.Format("2006-01-02"))
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if payload["provider_used"] != "yahoo" {
		t.Fatalf("expected provider_used yahoo, got %#v", payload["provider_used"])
	}
	if payload["using_cached_data"] != false {
		t.Fatalf("expected using_cached_data false, got %#v", payload["using_cached_data"])
	}
}

type mockExternalMLClient struct {
	async bool
}

func (m *mockExternalMLClient) SubmitAnalysisAndForecast(ctx context.Context, ticker string, forecast *model.ForecastResult, analysis *model.AdvancedAnalysis) (*ml.SubmitResult, error) {
	if m.async {
		return &ml.SubmitResult{
			JobID:   "job-123",
			Status:  "queued",
			Message: "queued",
		}, nil
	}
	return &ml.SubmitResult{
		Status: "completed",
		Insight: &model.ExternalMLInsight{
			Provider: "test-ml",
			Status:   "ok",
			Recommendation: model.ExternalMLRecommendation{
				Action:     "BUY",
				Confidence: "High",
				ScoreDelta: 2,
				Rationale:  []string{"Neural model trend is positive"},
			},
		},
	}, nil
}

func (m *mockExternalMLClient) GetJobStatus(ctx context.Context, jobID string) (*ml.JobStatusResult, error) {
	if m.async {
		return &ml.JobStatusResult{
			JobID:  jobID,
			Status: "completed",
			Insight: &model.ExternalMLInsight{
				Provider: "test-ml",
				Status:   "ok",
				Recommendation: model.ExternalMLRecommendation{
					Action:     "BUY",
					Confidence: "High",
					ScoreDelta: 2,
					Rationale:  []string{"Neural model trend is positive"},
				},
			},
		}, nil
	}
	return &ml.JobStatusResult{JobID: jobID, Status: "running"}, nil
}

func TestMLAnalysisEndpoint(t *testing.T) {
	db, err := storage.NewDatabase(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouterWithDependenciesAndClients(db, &mockProvider{}, nil, &mockExternalMLClient{})

	ingestReq := httptest.NewRequest(http.MethodPost, "/ingest?ticker=TEST", nil)
	ingestRec := httptest.NewRecorder()
	router.ServeHTTP(ingestRec, ingestReq)
	if ingestRec.Code != http.StatusOK {
		t.Fatalf("ingest failed with status=%d body=%s", ingestRec.Code, ingestRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/ml-analysis?ticker=TEST", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload mlAnalysisResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if payload.Analysis == nil || payload.Analysis.ExternalML == nil || payload.Analysis.ExternalML.Provider != "test-ml" {
		t.Fatalf("expected external ml payload, got %+v", payload.Analysis)
	}
	if payload.Analysis.Signal.Action != "BUY" {
		t.Fatalf("expected BUY action from external recommendation, got %s", payload.Analysis.Signal.Action)
	}
	if payload.Analysis.Signal.GeneratedBy == "" {
		t.Fatalf("expected generated_by to be set")
	}
}

func TestMLAnalysisAsyncFlow(t *testing.T) {
	db, err := storage.NewDatabase(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouterWithDependenciesAndClients(db, &mockProvider{}, nil, &mockExternalMLClient{async: true})

	ingestReq := httptest.NewRequest(http.MethodPost, "/ingest?ticker=TEST", nil)
	ingestRec := httptest.NewRecorder()
	router.ServeHTTP(ingestRec, ingestReq)
	if ingestRec.Code != http.StatusOK {
		t.Fatalf("ingest failed with status=%d body=%s", ingestRec.Code, ingestRec.Body.String())
	}

	startReq := httptest.NewRequest(http.MethodPost, "/ml-analysis?ticker=TEST", nil)
	startRec := httptest.NewRecorder()
	router.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", startRec.Code, startRec.Body.String())
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/ml-analysis-status?ticker=TEST", nil)
	statusRec := httptest.NewRecorder()
	router.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", statusRec.Code, statusRec.Body.String())
	}

	var payload mlAnalysisResponse
	if err := json.Unmarshal(statusRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if payload.Status != "completed" {
		t.Fatalf("expected completed, got %q", payload.Status)
	}
	if payload.Analysis == nil || payload.Analysis.ExternalML == nil {
		t.Fatalf("expected analysis with external_ml in status response")
	}
}

func TestAnalysisIncludeMLQuery(t *testing.T) {
	db, err := storage.NewDatabase(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouterWithDependenciesAndClients(db, &mockProvider{}, nil, &mockExternalMLClient{})

	ingestReq := httptest.NewRequest(http.MethodPost, "/ingest?ticker=TEST", nil)
	ingestRec := httptest.NewRecorder()
	router.ServeHTTP(ingestRec, ingestReq)
	if ingestRec.Code != http.StatusOK {
		t.Fatalf("ingest failed with status=%d body=%s", ingestRec.Code, ingestRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/analysis?ticker=TEST&include_ml=true", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var analysis model.AdvancedAnalysis
	if err := json.Unmarshal(rec.Body.Bytes(), &analysis); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if analysis.ExternalML == nil {
		t.Fatalf("expected external_ml insight in response")
	}
}
