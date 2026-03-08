package storage

import (
	"os"
	"testing"
	"time"

	"stock-options/pkg/model"
)

func TestDatabase(t *testing.T) {
	// Create a temporary database for testing
	db, err := NewDatabase("test.db")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	t.Cleanup(func() {
		os.Remove("test.db")
	})

	// Test saving and retrieving data
	testData := []model.StockData{
		{
			Ticker:      "TEST",
			TradingDate: time.Now(),
			OpenPrice:   100.50,
			HighPrice:   102.75,
			LowPrice:    99.25,
			ClosePrice:  101.30,
			Volume:      1000000,
		},
	}

	err = db.SaveStockData(testData)
	if err != nil {
		t.Fatalf("Failed to save stock data: %v", err)
	}

	data, err := db.GetStockData("TEST", time.Now().Add(-365*24*time.Hour), time.Now())
	if err != nil {
		t.Fatalf("Failed to get stock data: %v", err)
	}

	if len(data) == 0 {
		t.Error("Expected to find saved stock data")
	}

}

func TestGenerateForecastPersistsResult(t *testing.T) {
	db, err := NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("Failed to create in-memory database: %v", err)
	}

	testData := []model.StockData{
		{Ticker: "TEST", TradingDate: time.Date(2022, 1, 10, 0, 0, 0, 0, time.UTC), ClosePrice: 100, Volume: 1000},
		{Ticker: "TEST", TradingDate: time.Date(2023, 1, 10, 0, 0, 0, 0, time.UTC), ClosePrice: 110, Volume: 1000},
		{Ticker: "TEST", TradingDate: time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC), ClosePrice: 120, Volume: 1000},
		{Ticker: "TEST", TradingDate: time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC), ClosePrice: 130, Volume: 1000},
	}
	if err := db.SaveStockData(testData); err != nil {
		t.Fatalf("Failed to save stock data: %v", err)
	}

	forecast, err := db.GenerateForecast("TEST")
	if err != nil {
		t.Fatalf("Failed to generate forecast: %v", err)
	}
	if forecast.Ticker != "TEST" {
		t.Fatalf("Expected ticker TEST, got %s", forecast.Ticker)
	}

	stored, err := db.GetForecastResult("TEST")
	if err != nil {
		t.Fatalf("Failed to retrieve stored forecast: %v", err)
	}
	if stored.NextYearForecast == 0 {
		t.Fatalf("Expected non-zero next year forecast")
	}
}
