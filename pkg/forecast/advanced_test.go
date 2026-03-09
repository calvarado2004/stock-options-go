package forecast

import (
	"testing"

	"stock-options/pkg/model"
)

func TestEvaluateTradeSignalBuy(t *testing.T) {
	signal := EvaluateTradeSignal(
		100,
		model.MonteCarloAnalysis{
			Points: []model.MonteCarloPoint{
				{HorizonDays: 252, P10: 90, P50: 122, P90: 150},
			},
		},
		model.AR1Analysis{ExpectedPrice30D: 103},
		model.DuPontAnalysis{Available: true, ReturnOnEquity: 0.18, NetProfitMargin: 0.12},
	)
	if signal.Action != "BUY" {
		t.Fatalf("expected BUY, got %s", signal.Action)
	}
}

func TestEvaluateTradeSignalSell(t *testing.T) {
	signal := EvaluateTradeSignal(
		100,
		model.MonteCarloAnalysis{
			Points: []model.MonteCarloPoint{
				{HorizonDays: 252, P10: 55, P50: 92, P90: 130},
			},
		},
		model.AR1Analysis{ExpectedPrice30D: 95},
		model.DuPontAnalysis{Available: true, ReturnOnEquity: 0.03, NetProfitMargin: 0.02},
	)
	if signal.Action != "SELL" {
		t.Fatalf("expected SELL, got %s", signal.Action)
	}
}
