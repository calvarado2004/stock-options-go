package forecast

import (
	"fmt"
	"math"
	"sort"
	"time"

	"stock-options/pkg/model"
)

// Forecaster defines forecast engines that use historical stock data.
type Forecaster interface {
	Forecast(ticker string, historical []model.StockData, now time.Time) (*model.ForecastResult, error)
}

// LinearRegressionForecaster projects yearly averages with linear regression.
type LinearRegressionForecaster struct{}

// Forecast calculates current-year remainder and next two year forecasts from local historical data.
func (f LinearRegressionForecaster) Forecast(ticker string, historical []model.StockData, now time.Time) (*model.ForecastResult, error) {
	if len(historical) == 0 {
		return nil, fmt.Errorf("no historical data for ticker %s", ticker)
	}

	yearSum := map[int]float64{}
	yearCount := map[int]int{}
	for _, row := range historical {
		year := row.TradingDate.Year()
		price := row.ClosePrice
		if row.AdjustedClose != nil {
			price = *row.AdjustedClose
		}
		yearSum[year] += price
		yearCount[year]++
	}
	if len(yearSum) < 2 {
		return nil, fmt.Errorf("insufficient yearly points for regression for ticker %s", ticker)
	}

	years := make([]int, 0, len(yearSum))
	for year := range yearSum {
		years = append(years, year)
	}
	sort.Ints(years)

	x := make([]float64, 0, len(years))
	y := make([]float64, 0, len(years))
	for _, year := range years {
		x = append(x, float64(year))
		y = append(y, yearSum[year]/float64(yearCount[year]))
	}

	slope, intercept := linearRegression(x, y)
	predict := func(year int) float64 {
		return intercept + slope*float64(year)
	}

	now = now.UTC()
	currentYear := now.Year()
	nextYear := currentYear + 1
	yearAfterNext := currentYear + 2

	elapsedMonths := int(now.Month())
	remainingMonths := 12 - elapsedMonths

	currentYearYTD := averagePriceForYearUpTo(historical, currentYear, elapsedMonths)
	currentYearFullForecast := predict(currentYear)
	currentRemainingForecast := currentYearFullForecast
	if remainingMonths > 0 && !math.IsNaN(currentYearYTD) {
		currentRemainingForecast = ((currentYearFullForecast * 12) - (currentYearYTD * float64(elapsedMonths))) / float64(remainingMonths)
	}

	return &model.ForecastResult{
		Ticker:                       ticker,
		CurrentYear:                  currentYear,
		RemainingMonths:              remainingMonths,
		CurrentYearRemainingForecast: currentRemainingForecast,
		NextYear:                     nextYear,
		NextYearForecast:             predict(nextYear),
		YearAfterNext:                yearAfterNext,
		YearAfterNextForecast:        predict(yearAfterNext),
		RegressionSlope:              slope,
		RegressionIntercept:          intercept,
		GeneratedAt:                  now,
	}, nil
}

func linearRegression(x []float64, y []float64) (float64, float64) {
	n := float64(len(x))
	var sumX, sumY, sumXY, sumXX float64
	for i := range x {
		sumX += x[i]
		sumY += y[i]
		sumXY += x[i] * y[i]
		sumXX += x[i] * x[i]
	}

	denominator := (n * sumXX) - (sumX * sumX)
	if denominator == 0 {
		return 0, sumY / n
	}
	slope := ((n * sumXY) - (sumX * sumY)) / denominator
	intercept := (sumY - slope*sumX) / n
	return slope, intercept
}

func averagePriceForYearUpTo(data []model.StockData, year int, monthInclusive int) float64 {
	var sum float64
	var count int
	for _, row := range data {
		if row.TradingDate.Year() != year {
			continue
		}
		if int(row.TradingDate.Month()) > monthInclusive {
			continue
		}
		price := row.ClosePrice
		if row.AdjustedClose != nil {
			price = *row.AdjustedClose
		}
		sum += price
		count++
	}
	if count == 0 {
		return math.NaN()
	}
	return sum / float64(count)
}
