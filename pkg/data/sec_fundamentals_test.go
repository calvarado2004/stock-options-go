package data

import (
	"testing"
	"time"
)

func TestExtractAnnualUSDFacts(t *testing.T) {
	payload := map[string]interface{}{
		"facts": map[string]interface{}{
			"us-gaap": map[string]interface{}{
				"Revenues": map[string]interface{}{
					"units": map[string]interface{}{
						"USD": []interface{}{
							map[string]interface{}{"form": "10-K", "fp": "FY", "end": "2024-09-30", "val": 1000.0},
							map[string]interface{}{"form": "10-K", "fp": "FY", "end": "2023-09-30", "val": 900.0},
						},
					},
				},
			},
		},
	}

	latest, prev, periodEnd, ok := extractAnnualUSDFacts(payload, []string{"Revenues"})
	if !ok {
		t.Fatal("expected fact extraction to succeed")
	}
	if latest != 1000.0 {
		t.Fatalf("expected latest 1000, got %f", latest)
	}
	if prev != 900.0 {
		t.Fatalf("expected prev 900, got %f", prev)
	}
	expected, _ := time.Parse("2006-01-02", "2024-09-30")
	if !periodEnd.Equal(expected.UTC()) {
		t.Fatalf("expected period end %s, got %s", expected.UTC(), periodEnd)
	}
}

func TestExtractAnnualUSDFactsIgnoresQuarterly(t *testing.T) {
	payload := map[string]interface{}{
		"facts": map[string]interface{}{
			"us-gaap": map[string]interface{}{
				"NetIncomeLoss": map[string]interface{}{
					"units": map[string]interface{}{
						"USD": []interface{}{
							map[string]interface{}{"form": "10-Q", "fp": "Q3", "end": "2024-09-30", "val": 100.0},
						},
					},
				},
			},
		},
	}

	_, _, _, ok := extractAnnualUSDFacts(payload, []string{"NetIncomeLoss"})
	if ok {
		t.Fatal("expected quarterly-only data to be ignored")
	}
}
