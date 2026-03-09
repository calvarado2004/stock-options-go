# Stock Options Project

## Legal Disclaimer

This project is provided **as-is** for educational and informational use. It is not
financial advice, investment advice, or a recommendation to buy/sell securities.
All outputs can be wrong or incomplete and must be independently validated.

## Project Scope

Design and implement a backend API that retrieves and stores five years of historical stock price data for a given ticker symbol, for example `PSTG`.

## Functional Objectives

### Historical Data Retrieval

- The API accepts a stock ticker symbol.
- It retrieves approximately the last five years of daily market data from an external source.
- Minimum stored fields:
  - trading date
  - open price
  - high price
  - low price
  - close price
  - adjusted close price (if available)
  - trading volume

### Persistent Storage

- Retrieved stock data is stored in a local database.
- Persistence avoids unnecessary repeated external calls for previously ingested data.
- The system queries local DB as primary source after ingestion.

### Forecasting (In-House)

- Forecast calculations are performed locally in project code (`pkg/forecast`).
- No external site/service is used for forecast calculations.
- Current implementation: linear regression over yearly average prices.
- Output targets:
  - projected average for remainder of current year
  - projected average for next calendar year
  - projected average for year after next

## Expected Behavior

- If local data for a ticker is sufficiently up to date, the system should use DB data instead of re-fetching full history.
- Expose endpoints for:
  - triggering/refreshing ingestion
  - retrieving stored historical data
  - retrieving forecast results

## API Endpoints

- `POST /ingest?ticker=SYMBOL`
- `GET /data?ticker=SYMBOL&start_date=YYYY-MM-DD&end_date=YYYY-MM-DD`
- `GET /forecast?ticker=SYMBOL`
- `GET /analysis?ticker=SYMBOL`
- `GET /healthz`

## Current Implementation Notes

- Historical providers (`pkg/data`):
  - Alpha Vantage (optional, via `ALPHA_API_KEY`)
  - Yahoo Finance (no API key in current implementation)
  - Stooq fallback
- Alpha Vantage is one possible provider among several supported sources
- Provider calls are serial (no parallel provider fan-out)
- Ingest is cache-first by default; use `refresh=true` to force external refresh
- Provider HTTP calls use retry/backoff for transient errors (`429`, `5xx`) with pacing delay
- Storage (`pkg/storage`): configurable DB driver via GORM
  - local fallback: SQLite
  - container/K8s baseline: PostgreSQL service
- Forecast engine (`pkg/forecast`): local linear regression forecaster
- Advanced analytics (`pkg/forecast`):
  - Monte Carlo simulation bands
  - AR(1)-style return forecast
  - DuPont analysis enriched from SEC companyfacts (when available)
  - Rule-based signal: `BUY` / `HOLD` / `SELL` with reasons/confidence

### Kubernetes Hardened Baseline

- Backend and DB use `envFrom` Kubernetes Secrets
- PostgreSQL bootstrap uses an init script mounted from Secret at `/docker-entrypoint-initdb.d`
- Backend has startup/readiness/liveness probes on `GET /healthz`
- DB has startup/readiness/liveness probes using `pg_isready`

## Non-Goals / Constraints

- This phase is limited to basic, explainable linear regression.
- High-accuracy financial prediction is not a goal.
- Advanced time-series/ML models are out of scope for now.

## Build Commands

```bash
go build -o stock-options .
```

```bash
go build -gcflags="-N -l" -o stock-options-debug .
```

```bash
go run main.go
```

## Test Commands

Run all tests:

```bash
go test ./...
```

Coverage report:

```bash
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out
```

Run key packages:

```bash
go test ./pkg/api ./pkg/storage ./pkg/forecast ./pkg/data ./pkg/model
```

Run live PSTG integration test:

```bash
RUN_REAL_INTEGRATION=1 go test -v ./pkg/data -run TestPSTGRealDataIntegration
```

## Project Conventions

### Go Project Structure
- `main.go` at root
- packages under `/pkg`
- files formatted with `gofmt`
- concise lowercase package names

### Code Style
- Follow Effective Go
- camelCase for variables/functions
- PascalCase for exported names
- documentation comments for public APIs

### Testing Conventions
- test files `*_test.go`
- tests start with `Test`
- prefer table-driven tests where useful
- mock external dependencies via interfaces for testability
