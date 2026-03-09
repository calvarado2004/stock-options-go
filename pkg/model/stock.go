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

type MonteCarloPoint struct {
	HorizonDays int     `json:"horizon_days"`
	MeanPrice   float64 `json:"mean_price"`
	P10         float64 `json:"p10"`
	P50         float64 `json:"p50"`
	P90         float64 `json:"p90"`
}

type MonteCarloAnalysis struct {
	Paths            int               `json:"paths"`
	TradingDays      int               `json:"trading_days"`
	StartPrice       float64           `json:"start_price"`
	DriftAnnual      float64           `json:"drift_annual"`
	VolatilityAnnual float64           `json:"volatility_annual"`
	Points           []MonteCarloPoint `json:"points"`
}

type AR1Analysis struct {
	Phi              float64 `json:"phi"`
	Intercept        float64 `json:"intercept"`
	Sigma            float64 `json:"sigma"`
	LastReturn       float64 `json:"last_return"`
	ForecastReturn1D float64 `json:"forecast_return_1d"`
	ExpectedPrice30D float64 `json:"expected_price_30d"`
}

type DuPontAnalysis struct {
	Available bool   `json:"available"`
	Message   string `json:"message"`
}

type AdvancedAnalysis struct {
	Ticker       string             `json:"ticker"`
	GeneratedAt  time.Time          `json:"generated_at"`
	CurrentPrice float64            `json:"current_price"`
	MonteCarlo   MonteCarloAnalysis `json:"monte_carlo"`
	AR1          AR1Analysis        `json:"ar1"`
	DuPont       DuPontAnalysis     `json:"dupont"`
}
