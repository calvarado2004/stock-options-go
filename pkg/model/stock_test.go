package model

import (
	"testing"
	"time"
)

func TestStockData(t *testing.T) {
	// Create a sample stock data entry
	now := time.Now()
	data := StockData{
		Ticker:        "TEST",
		TradingDate:   now,
		OpenPrice:     100.50,
		HighPrice:     102.75,
		LowPrice:      99.25,
		ClosePrice:    101.30,
		Volume:        1000000,
	}

	if data.Ticker != "TEST" {
		t.Errorf("Expected ticker TEST, got %s", data.Ticker)
	}

	if data.OpenPrice != 100.50 {
		t.Errorf("Expected open price 100.50, got %.2f", data.OpenPrice)
	}

	if data.Volume != 1000000 {
		t.Errorf("Expected volume 1000000, got %d", data.Volume)
	}
}