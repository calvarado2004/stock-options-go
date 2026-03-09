package ml

import "testing"

func TestParseExternalMLInsight(t *testing.T) {
	raw := map[string]interface{}{
		"provider": "remote-ml",
		"model":    "nn-v2",
		"status":   "ok",
		"recommendation": map[string]interface{}{
			"action":      "buy",
			"confidence":  "High",
			"score_delta": float64(2),
			"rationale":   []interface{}{"Momentum positive", "Sentiment positive"},
		},
	}
	insight := parseExternalMLInsight(raw)
	if insight.Provider != "remote-ml" {
		t.Fatalf("expected provider remote-ml, got %q", insight.Provider)
	}
	if insight.Recommendation.Action != "BUY" {
		t.Fatalf("expected BUY action, got %q", insight.Recommendation.Action)
	}
	if insight.Recommendation.ScoreDelta != 2 {
		t.Fatalf("expected score delta 2, got %d", insight.Recommendation.ScoreDelta)
	}
	if len(insight.Recommendation.Rationale) != 2 {
		t.Fatalf("expected 2 rationale items, got %d", len(insight.Recommendation.Rationale))
	}
}

func TestNormalizeAction(t *testing.T) {
	if got := normalizeAction("sell"); got != "SELL" {
		t.Fatalf("expected SELL, got %q", got)
	}
	if got := normalizeAction("unknown"); got != "" {
		t.Fatalf("expected empty action, got %q", got)
	}
}
