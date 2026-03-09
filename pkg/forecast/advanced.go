package forecast

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"

	"stock-options/pkg/model"
)

func AnalyzeAdvanced(ticker string, historical []model.StockData, now time.Time) (*model.AdvancedAnalysis, error) {
	if len(historical) < 30 {
		return nil, fmt.Errorf("insufficient historical data for advanced analysis for %s", ticker)
	}

	prices := make([]float64, 0, len(historical))
	for _, row := range historical {
		price := row.ClosePrice
		if row.AdjustedClose != nil {
			price = *row.AdjustedClose
		}
		if price > 0 {
			prices = append(prices, price)
		}
	}
	if len(prices) < 30 {
		return nil, fmt.Errorf("insufficient valid prices for advanced analysis for %s", ticker)
	}

	returns := make([]float64, 0, len(prices)-1)
	for i := 1; i < len(prices); i++ {
		r := math.Log(prices[i] / prices[i-1])
		if !math.IsNaN(r) && !math.IsInf(r, 0) {
			returns = append(returns, r)
		}
	}
	if len(returns) < 20 {
		return nil, fmt.Errorf("insufficient returns for advanced analysis for %s", ticker)
	}

	muDaily, sigmaDaily := meanStd(returns)
	driftAnnual := muDaily * 252
	volAnnual := sigmaDaily * math.Sqrt(252)

	currentPrice := prices[len(prices)-1]
	mc := monteCarloAnalysis(currentPrice, driftAnnual, volAnnual, now)
	ar1 := ar1Analysis(returns, currentPrice)

	return &model.AdvancedAnalysis{
		Ticker:       ticker,
		GeneratedAt:  now.UTC(),
		CurrentPrice: currentPrice,
		MonteCarlo:   mc,
		AR1:          ar1,
		DuPont: model.DuPontAnalysis{
			Available: false,
			Message:   "DuPont requires financial statement inputs (net margin, asset turnover, equity multiplier).",
		},
	}, nil
}

func monteCarloAnalysis(startPrice float64, driftAnnual float64, volAnnual float64, now time.Time) model.MonteCarloAnalysis {
	paths := 300
	tradingDays := 252
	horizons := []int{21, 63, 126, 252}
	dt := 1.0 / 252.0
	mu := driftAnnual
	sigma := volAnnual

	rng := rand.New(rand.NewSource(now.UnixNano()))
	priceAt := make(map[int][]float64, len(horizons))
	for _, h := range horizons {
		priceAt[h] = make([]float64, 0, paths)
	}

	for p := 0; p < paths; p++ {
		price := startPrice
		for d := 1; d <= tradingDays; d++ {
			z := rng.NormFloat64()
			price = price * math.Exp((mu-0.5*sigma*sigma)*dt+sigma*math.Sqrt(dt)*z)
			if arr, ok := priceAt[d]; ok {
				priceAt[d] = append(arr, price)
			}
		}
	}

	points := make([]model.MonteCarloPoint, 0, len(horizons))
	for _, h := range horizons {
		dist := priceAt[h]
		sort.Float64s(dist)
		points = append(points, model.MonteCarloPoint{
			HorizonDays: h,
			MeanPrice:   mean(dist),
			P10:         percentileSorted(dist, 0.10),
			P50:         percentileSorted(dist, 0.50),
			P90:         percentileSorted(dist, 0.90),
		})
	}

	return model.MonteCarloAnalysis{
		Paths:            paths,
		TradingDays:      tradingDays,
		StartPrice:       startPrice,
		DriftAnnual:      driftAnnual,
		VolatilityAnnual: volAnnual,
		Points:           points,
	}
}

func ar1Analysis(returns []float64, currentPrice float64) model.AR1Analysis {
	if len(returns) < 3 {
		return model.AR1Analysis{}
	}
	x := make([]float64, 0, len(returns)-1)
	y := make([]float64, 0, len(returns)-1)
	for i := 1; i < len(returns); i++ {
		x = append(x, returns[i-1])
		y = append(y, returns[i])
	}

	phi, intercept := linearRegression(x, y)
	lastReturn := returns[len(returns)-1]
	forecast1D := intercept + phi*lastReturn

	var sigma float64
	for i := range x {
		e := y[i] - (intercept + phi*x[i])
		sigma += e * e
	}
	sigma = math.Sqrt(sigma / float64(len(x)))

	expectedPrice30D := currentPrice * math.Exp(30*forecast1D)
	return model.AR1Analysis{
		Phi:              phi,
		Intercept:        intercept,
		Sigma:            sigma,
		LastReturn:       lastReturn,
		ForecastReturn1D: forecast1D,
		ExpectedPrice30D: expectedPrice30D,
	}
}

func meanStd(values []float64) (float64, float64) {
	m := mean(values)
	if len(values) < 2 {
		return m, 0
	}
	var ss float64
	for _, v := range values {
		d := v - m
		ss += d * d
	}
	return m, math.Sqrt(ss / float64(len(values)-1))
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var s float64
	for _, v := range values {
		s += v
	}
	return s / float64(len(values))
}

func percentileSorted(sortedValues []float64, p float64) float64 {
	if len(sortedValues) == 0 {
		return 0
	}
	if p <= 0 {
		return sortedValues[0]
	}
	if p >= 1 {
		return sortedValues[len(sortedValues)-1]
	}
	pos := p * float64(len(sortedValues)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sortedValues[lo]
	}
	w := pos - float64(lo)
	return sortedValues[lo]*(1-w) + sortedValues[hi]*w
}
