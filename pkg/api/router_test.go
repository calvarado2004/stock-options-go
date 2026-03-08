package api

import (
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
