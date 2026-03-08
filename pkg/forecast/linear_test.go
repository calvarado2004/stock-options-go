package forecast

import (
	"testing"
	"time"

	"stock-options/pkg/model"
)

func TestLinearRegressionForecast(t *testing.T) {
	f := LinearRegressionForecaster{}
	now := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)

	data := []model.StockData{
		{Ticker: "TEST", TradingDate: time.Date(2022, 1, 10, 0, 0, 0, 0, time.UTC), ClosePrice: 100},
		{Ticker: "TEST", TradingDate: time.Date(2022, 6, 10, 0, 0, 0, 0, time.UTC), ClosePrice: 100},
		{Ticker: "TEST", TradingDate: time.Date(2023, 1, 10, 0, 0, 0, 0, time.UTC), ClosePrice: 110},
		{Ticker: "TEST", TradingDate: time.Date(2023, 6, 10, 0, 0, 0, 0, time.UTC), ClosePrice: 110},
		{Ticker: "TEST", TradingDate: time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC), ClosePrice: 120},
		{Ticker: "TEST", TradingDate: time.Date(2024, 6, 10, 0, 0, 0, 0, time.UTC), ClosePrice: 120},
		{Ticker: "TEST", TradingDate: time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC), ClosePrice: 130},
		{Ticker: "TEST", TradingDate: time.Date(2025, 6, 10, 0, 0, 0, 0, time.UTC), ClosePrice: 130},
		{Ticker: "TEST", TradingDate: time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC), ClosePrice: 140},
	}

	forecast, err := f.Forecast("TEST", data, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if forecast.CurrentYear != 2026 {
		t.Fatalf("expected current year 2026, got %d", forecast.CurrentYear)
	}
	if forecast.NextYear != 2027 || forecast.YearAfterNext != 2028 {
		t.Fatalf("unexpected forecast years: %d/%d", forecast.NextYear, forecast.YearAfterNext)
	}
	if forecast.NextYearForecast <= 0 || forecast.YearAfterNextForecast <= 0 {
		t.Fatalf("expected positive forecast values, got %f and %f", forecast.NextYearForecast, forecast.YearAfterNextForecast)
	}
}

func TestLinearRegressionForecastErrors(t *testing.T) {
	f := LinearRegressionForecaster{}
	now := time.Now().UTC()

	if _, err := f.Forecast("TEST", nil, now); err == nil {
		t.Fatal("expected error for empty historical data")
	}

	data := []model.StockData{
		{Ticker: "TEST", TradingDate: time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC), ClosePrice: 100},
		{Ticker: "TEST", TradingDate: time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), ClosePrice: 101},
	}
	if _, err := f.Forecast("TEST", data, now); err == nil {
		t.Fatal("expected error for insufficient yearly points")
	}
}
