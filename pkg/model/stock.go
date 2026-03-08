package model

import "time"

// StockData represents historical stock data for a single trading day
type StockData struct {
	ID            uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Ticker        string    `json:"ticker" gorm:"index;uniqueIndex:idx_ticker_date"`
	TradingDate   time.Time `json:"trading_date" gorm:"uniqueIndex:idx_ticker_date"`
	OpenPrice     float64   `json:"open_price"`
	HighPrice     float64   `json:"high_price"`
	LowPrice      float64   `json:"low_price"`
	ClosePrice    float64   `json:"close_price"`
	AdjustedClose *float64  `json:"adjusted_close,omitempty"`
	Volume        int64     `json:"volume"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ForecastResult represents the forecasted average stock values by year
type ForecastResult struct {
	Ticker                       string    `json:"ticker" gorm:"primaryKey"`
	CurrentYear                  int       `json:"current_year"`
	RemainingMonths              int       `json:"remaining_months"`
	CurrentYearRemainingForecast float64   `json:"current_year_remaining_forecast"`
	NextYear                     int       `json:"next_year"`
	NextYearForecast             float64   `json:"next_year_forecast"`
	YearAfterNext                int       `json:"year_after_next"`
	YearAfterNextForecast        float64   `json:"year_after_next_forecast"`
	RegressionSlope              float64   `json:"regression_slope"`
	RegressionIntercept          float64   `json:"regression_intercept"`
	GeneratedAt                  time.Time `json:"generated_at"`
}
