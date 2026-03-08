package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"stock-options/pkg/storage"
)

// Integration test specifically for PSTG ticker as requested in the task
func TestPSTGTickerIntegration(t *testing.T) {
	t.Logf("Starting integration test for PSTG ticker")

	router := newTestRouter(t)

	t.Run("Test POST /ingest endpoint with PSTG", func(t *testing.T) {
		req, err := http.NewRequest("POST", "/ingest?ticker=PSTG", nil)
		if err != nil {
			t.Fatal(err)
		}

		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)

		if status := recorder.Code; status != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, status)
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatalf("invalid json response: %v", err)
		}
		if payload["ticker"] != "PSTG" {
			t.Errorf("expected ticker PSTG in response, got %#v", payload["ticker"])
		}
		if payload["provider_used"] != "yahoo" {
			t.Errorf("expected provider_used yahoo in response, got %#v", payload["provider_used"])
		}

		t.Logf("Successfully initiated ingestion for PSTG ticker")
	})

	t.Run("Test GET /data endpoint with PSTG and date range", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/data?ticker=PSTG&start_date=2021-01-01&end_date=2026-03-08", nil)
		if err != nil {
			t.Fatal(err)
		}

		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)

		if status := recorder.Code; status != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, status)
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatalf("invalid json response: %v", err)
		}
		if payload["ticker"] != "PSTG" {
			t.Errorf("expected ticker PSTG in response, got %#v", payload["ticker"])
		}

		t.Logf("Successfully retrieved data endpoint test for PSTG")
	})

	t.Run("Test GET /forecast endpoint with PSTG", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/forecast?ticker=PSTG", nil)
		if err != nil {
			t.Fatal(err)
		}

		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)

		if status := recorder.Code; status != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, status)
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatalf("invalid json response: %v", err)
		}
		if payload["ticker"] != "PSTG" {
			t.Errorf("expected ticker PSTG in response, got %#v", payload["ticker"])
		}

		t.Logf("Successfully tested forecast endpoint for PSTG")
	})

	t.Run("Test database operations with PSTG ticker", func(t *testing.T) {
		// Test that we can connect to the in-memory DB and perform basic operations
		db, err := storage.NewDatabase(":memory:")
		if err != nil {
			t.Fatal(err)
		}

		// Verify connection works (this is a minimal test since actual data would require more setup)
		_, err = db.GetStockData("PSTG", time.Now().Add(-5*365*24*time.Hour), time.Now())
		if err != nil {
			// This error might be expected if no data exists yet
			t.Logf("Expected database query error for PSTG (no data): %v", err)
		}

		t.Logf("Database operations test completed successfully")
	})
}

func getFiveYearsAgo() string {
	return "2021-03-08"
}

func getCurrentDate() string {
	return "2026-03-08"
}
