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
		Signal: buildTradeSignal(currentPrice, mc, ar1, model.DuPontAnalysis{}),
	}, nil
}

func EvaluateTradeSignal(currentPrice float64, mc model.MonteCarloAnalysis, ar1 model.AR1Analysis, dupont model.DuPontAnalysis) model.TradeSignal {
	return buildTradeSignal(currentPrice, mc, ar1, dupont)
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

func buildTradeSignal(currentPrice float64, mc model.MonteCarloAnalysis, ar1 model.AR1Analysis, dupont model.DuPontAnalysis) model.TradeSignal {
	score := 0
	reasons := make([]string, 0, 6)

	var p50_12m, p10_12m float64
	for _, pt := range mc.Points {
		if pt.HorizonDays == 252 {
			p50_12m = pt.P50
			p10_12m = pt.P10
			break
		}
	}
	if p50_12m == 0 && len(mc.Points) > 0 {
		last := mc.Points[len(mc.Points)-1]
		p50_12m = last.P50
		p10_12m = last.P10
	}

	upside12m := safePctChange(currentPrice, p50_12m)
	downside12m := safePctChange(currentPrice, p10_12m)
	ar1_30d := safePctChange(currentPrice, ar1.ExpectedPrice30D)

	// Growth and risk thresholds (rule-based heuristic).
	if upside12m >= 0.12 {
		score += 2
		reasons = append(reasons, fmt.Sprintf("Monte Carlo median 12M upside is strong (%.1f%%).", upside12m*100))
	} else if upside12m >= 0.04 {
		score++
		reasons = append(reasons, fmt.Sprintf("Monte Carlo median 12M upside is positive (%.1f%%).", upside12m*100))
	} else if upside12m <= -0.05 {
		score -= 2
		reasons = append(reasons, fmt.Sprintf("Monte Carlo median 12M implies downside (%.1f%%).", upside12m*100))
	}

	if downside12m <= -0.35 {
		score -= 2
		reasons = append(reasons, fmt.Sprintf("Monte Carlo P10 indicates deep downside risk (%.1f%%).", downside12m*100))
	} else if downside12m <= -0.25 {
		score--
		reasons = append(reasons, fmt.Sprintf("Monte Carlo P10 downside risk is elevated (%.1f%%).", downside12m*100))
	} else if downside12m >= -0.15 {
		score++
		reasons = append(reasons, fmt.Sprintf("Monte Carlo downside band is contained (%.1f%% at P10).", downside12m*100))
	}

	if ar1_30d >= 0.02 {
		score++
		reasons = append(reasons, fmt.Sprintf("AR(1) 30-day expected return is positive (%.1f%%).", ar1_30d*100))
	} else if ar1_30d <= -0.03 {
		score--
		reasons = append(reasons, fmt.Sprintf("AR(1) 30-day expected return is negative (%.1f%%).", ar1_30d*100))
	}

	if dupont.Available {
		if dupont.ReturnOnEquity >= 0.15 {
			score += 2
			reasons = append(reasons, fmt.Sprintf("DuPont ROE is strong (%.1f%%).", dupont.ReturnOnEquity*100))
		} else if dupont.ReturnOnEquity >= 0.08 {
			score++
			reasons = append(reasons, fmt.Sprintf("DuPont ROE is acceptable (%.1f%%).", dupont.ReturnOnEquity*100))
		} else if dupont.ReturnOnEquity < 0.05 {
			score -= 2
			reasons = append(reasons, fmt.Sprintf("DuPont ROE is weak (%.1f%%).", dupont.ReturnOnEquity*100))
		}
		if dupont.NetProfitMargin < 0.03 {
			score--
			reasons = append(reasons, fmt.Sprintf("Net margin is thin (%.1f%%).", dupont.NetProfitMargin*100))
		}
	}

	action := "HOLD"
	if score >= 3 {
		action = "BUY"
	} else if score <= -2 {
		action = "SELL"
	}

	confidence := "Low"
	if absInt(score) >= 4 {
		confidence = "High"
	} else if absInt(score) >= 2 {
		confidence = "Medium"
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "Insufficient directional evidence; maintaining neutral stance.")
	}

	return model.TradeSignal{
		Action:      action,
		Confidence:  confidence,
		Score:       score,
		Reasons:     reasons,
		Disclaimer:  "Educational signal only, not financial advice.",
		GeneratedBy: "rule-based-v1",
	}
}

func safePctChange(base float64, value float64) float64 {
	if base == 0 {
		return 0
	}
	return (value / base) - 1.0
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
