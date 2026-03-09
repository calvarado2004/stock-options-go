package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"stock-options/pkg/model"
	"stock-options/pkg/storage"
)

type mockProvider struct{}

func (m *mockProvider) GetHistoricalData(ticker string, startDate time.Time, endDate time.Time) ([]model.StockData, string, error) {
	if ticker == "ERR" {
		return nil, "", fmt.Errorf("provider failure")
	}

	adj := 101.5
	return []model.StockData{
		{
			Ticker:        ticker,
			TradingDate:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			OpenPrice:     100,
			HighPrice:     102,
			LowPrice:      99,
			ClosePrice:    101,
			AdjustedClose: &adj,
			Volume:        1000,
		},
		{
			Ticker:      ticker,
			TradingDate: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			OpenPrice:   110,
			HighPrice:   112,
			LowPrice:    108,
			ClosePrice:  111,
			Volume:      1200,
		},
	}, "yahoo", nil
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
	calls int
}

func (m *countingProvider) GetHistoricalData(ticker string, startDate time.Time, endDate time.Time) ([]model.StockData, string, error) {
	m.calls++
	return nil, "", fmt.Errorf("should not be called")
}

func TestIngestUsesCacheFirstWithoutCallingProvider(t *testing.T) {
	db, err := storage.NewDatabase(":memory:")
	if err != nil {
		t.Fatal(err)
	}

	adj := 101.0
	seed := []model.StockData{
		{
			Ticker:        "PSTG",
			TradingDate:   time.Now().UTC().AddDate(-3, 0, 0).Truncate(24 * time.Hour),
			OpenPrice:     50,
			HighPrice:     55,
			LowPrice:      49,
			ClosePrice:    53,
			AdjustedClose: &adj,
			Volume:        1000,
		},
		{
			Ticker:      "PSTG",
			TradingDate: time.Now().UTC().AddDate(0, 0, -10).Truncate(24 * time.Hour),
			OpenPrice:   60,
			HighPrice:   62,
			LowPrice:    58,
			ClosePrice:  61,
			Volume:      2000,
		},
	}
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
