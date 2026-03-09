# Example Usage

## Start Server

```bash
go run main.go
```

Server starts on `http://localhost:8080`.

## 1) Ingest and Forecast

```bash
curl -X POST "http://localhost:8080/ingest?ticker=PSTG"
```

What this does:
- retrieves missing historical data for ~last 5 years
- stores/upserts rows into SQLite
- computes forecast locally in `pkg/forecast`
- stores forecast in DB

## 2) Query Historical Data

```bash
curl "http://localhost:8080/data?ticker=PSTG&start_date=2021-01-01&end_date=2026-03-08"
```

## 3) Query Forecast

```bash
curl "http://localhost:8080/forecast?ticker=PSTG"
```

## 4) Query Advanced Analysis (Local)

```bash
curl "http://localhost:8080/analysis?ticker=PSTG"
```

This returns local analytics computed in-house:
- Monte Carlo distribution bands
- AR(1) expected path
- DuPont (when SEC fundamentals are available)
- base rule-based `BUY/HOLD/SELL` signal

## 5) Query Advanced Analysis + Optional External ML Enrichment

One-shot ML enrichment:

```bash
curl -X POST "http://localhost:8080/ml-analysis?ticker=PSTG"
```

Or by query parameter on analysis endpoint:

```bash
curl "http://localhost:8080/analysis?ticker=PSTG&include_ml=true"
```

Expected external ML response contract (from your ML service):

```json
{
  "provider": "my-ml-service",
  "model": "ensemble-v1",
  "status": "ok",
  "recommendation": {
    "action": "BUY",
    "confidence": "High",
    "score_delta": 2,
    "rationale": [
      "Neural model trend is positive",
      "Sentiment score improved"
    ]
  }
}
```

## Environment

Historical data provider key (optional):

```bash
export ALPHA_API_KEY="your_alpha_vantage_key"
```

Alpha Vantage is one possible provider, not a requirement. Without `ALPHA_API_KEY`, the
service can still ingest from Yahoo Finance and Stooq.

SEC fundamentals (recommended for DuPont):

```bash
export SEC_USER_AGENT="stock-forecast-terminal/1.0 (you@example.com)"
```

External ML bridge (all optional):

```bash
export ML_SERVICE_URL="http://your-ml-service:9000"
export ML_SERVICE_PUSH_PATH="/ingest"
export ML_SERVICE_API_KEY="your-token"
export ML_SERVICE_TIMEOUT_MS="10000"
```

## Main Files

- `pkg/api/router.go`: endpoint handlers and ingestion orchestration
- `pkg/data/provider.go`: external historical-data retrieval
- `pkg/forecast/linear.go`: local linear regression forecast engine
- `pkg/forecast/advanced.go`: Monte Carlo, AR(1), signal rules
- `pkg/ml/client.go`: external ML bridge client
- `pkg/storage/database.go`: persistence, upsert, forecast persistence
- `pkg/model/stock.go`: data models
