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
	Available        bool      `json:"available"`
	Message          string    `json:"message"`
	Source           string    `json:"source,omitempty"`
	PeriodEnd        time.Time `json:"period_end,omitempty"`
	NetProfitMargin  float64   `json:"net_profit_margin,omitempty"`
	AssetTurnover    float64   `json:"asset_turnover,omitempty"`
	EquityMultiplier float64   `json:"equity_multiplier,omitempty"`
	ReturnOnEquity   float64   `json:"return_on_equity,omitempty"`
	NetIncome        float64   `json:"net_income,omitempty"`
	Revenue          float64   `json:"revenue,omitempty"`
	AverageAssets    float64   `json:"average_assets,omitempty"`
	AverageEquity    float64   `json:"average_equity,omitempty"`
}

type AdvancedAnalysis struct {
	Ticker       string             `json:"ticker"`
	GeneratedAt  time.Time          `json:"generated_at"`
	CurrentPrice float64            `json:"current_price"`
	MonteCarlo   MonteCarloAnalysis `json:"monte_carlo"`
	AR1          AR1Analysis        `json:"ar1"`
	DuPont       DuPontAnalysis     `json:"dupont"`
	Signal       TradeSignal        `json:"signal"`
	ExternalML   *ExternalMLInsight `json:"external_ml,omitempty"`
}

type TradeSignal struct {
	Action      string   `json:"action"` // BUY | HOLD | SELL
	Confidence  string   `json:"confidence"`
	Score       int      `json:"score"`
	Reasons     []string `json:"reasons"`
	Disclaimer  string   `json:"disclaimer"`
	GeneratedBy string   `json:"generated_by"`
}

type ExternalMLRecommendation struct {
	Action     string   `json:"action,omitempty"` // BUY | HOLD | SELL
	Confidence string   `json:"confidence,omitempty"`
	ScoreDelta int      `json:"score_delta,omitempty"`
	Rationale  []string `json:"rationale,omitempty"`
}

type ExternalMLInsight struct {
	Provider       string                   `json:"provider,omitempty"`
	Model          string                   `json:"model,omitempty"`
	Status         string                   `json:"status"`
	Message        string                   `json:"message,omitempty"`
	RequestedAt    time.Time                `json:"requested_at"`
	ReceivedAt     time.Time                `json:"received_at,omitempty"`
	Recommendation ExternalMLRecommendation `json:"recommendation,omitempty"`
	Raw            map[string]interface{}   `json:"raw,omitempty"`
}
