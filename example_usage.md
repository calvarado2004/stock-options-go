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

## Environment

Optional API key:

```bash
export ALPHA_API_KEY="your_alpha_vantage_key"
```

Alpha Vantage is one possible provider, not a requirement. Without `ALPHA_API_KEY`, the
service can still ingest from Yahoo Finance and Stooq.

## Main Files

- `pkg/api/router.go`: endpoint handlers and ingestion orchestration
- `pkg/data/provider.go`: external historical-data retrieval
- `pkg/forecast/linear.go`: local linear regression forecast engine
- `pkg/storage/database.go`: persistence, upsert, forecast persistence
- `pkg/model/stock.go`: data models
