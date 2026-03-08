package data

import (
	"os"
	"testing"
	"time"
)

// TestPSTGRealDataIntegration validates real provider data retrieval for PSTG.
// Run with: RUN_REAL_INTEGRATION=1 go test -v ./pkg/data -run TestPSTGRealDataIntegration
func TestPSTGRealDataIntegration(t *testing.T) {
	if os.Getenv("RUN_REAL_INTEGRATION") != "1" {
		t.Skip("set RUN_REAL_INTEGRATION=1 to run live provider integration test")
	}

	provider := NewAlphaVantageProvider(os.Getenv("ALPHA_API_KEY"))
	endDate := time.Now().UTC()
	startDate := endDate.AddDate(-5, 0, 0)

	rows, source, err := provider.GetHistoricalData("PSTG", startDate, endDate)
	if err != nil {
		t.Fatalf("real provider fetch failed: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected at least one PSTG historical row")
	}
	if source == "" {
		t.Fatal("expected non-empty provider source")
	}
}
