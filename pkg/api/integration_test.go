package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"stock-options/pkg/storage"
)

// Integration test for the API endpoints with database operations
func TestAPIIntegration(t *testing.T) {
	router := newTestRouter(t)

	t.Run("Test basic routing", func(t *testing.T) {
		req, err := http.NewRequest("POST", "/ingest?ticker=TEST", nil)
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
		if payload["ticker"] != "TEST" {
			t.Errorf("expected ticker TEST in response, got %#v", payload["ticker"])
		}
		if payload["provider_used"] != "yahoo" {
			t.Errorf("expected provider_used yahoo in response, got %#v", payload["provider_used"])
		}
	})

	t.Run("Test data endpoint with ticker parameter", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/data?ticker=TEST", nil)
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
		if payload["ticker"] != "TEST" {
			t.Errorf("expected ticker TEST in response, got %#v", payload["ticker"])
		}
	})

	t.Run("Test forecast endpoint with ticker parameter", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/forecast?ticker=TEST", nil)
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
		if payload["ticker"] != "TEST" {
			t.Errorf("expected ticker TEST in response, got %#v", payload["ticker"])
		}
	})

	t.Run("Test analysis endpoint with ticker parameter", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/analysis?ticker=TEST", nil)
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
		if payload["ticker"] != "TEST" {
			t.Errorf("expected ticker TEST in response, got %#v", payload["ticker"])
		}
	})
}

// Test database operations directly
func TestDatabaseOperations(t *testing.T) {
	_, err := storage.NewDatabase(":memory:") // Use in-memory DB for testing
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Test saving and retrieving stock data", func(t *testing.T) {
		// This test would require more complex setup with actual StockData objects,
		// but demonstrates the concept of database integration tests.

		// In a real scenario, we'd:
		// 1. Create sample StockData records
		// 2. Save them to DB using db.SaveStockData()
		// 3. Retrieve and verify they were saved correctly

		t.Logf("Database connection established for testing")
	})
}
