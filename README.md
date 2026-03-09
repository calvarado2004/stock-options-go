# Stock Options API

Backend API that retrieves and stores historical stock data and performs in-house forecasting.

## Legal Disclaimer

This project is provided **as-is** for educational and informational purposes only.
It is **not** investment, legal, accounting, or tax advice, and it is **not** a recommendation,
offer, or solicitation to buy or sell any security. Forecasts, analytics, and signals can be
wrong or incomplete. You are solely responsible for any decisions made using this software.
Consult a licensed financial advisor before making investment decisions.

## What It Does

- Ingests approximately 5 years of daily historical stock data for a ticker (e.g., `PSTG`)
- Stores data in a relational DB with upsert semantics
- Reuses cached data when local data is sufficiently up to date
- Falls back to cached data if external providers are temporarily unavailable/rate-limited
- Performs local linear regression forecasting (no external forecasting service)
- Exposes REST endpoints for ingestion, historical data query, and forecast retrieval
- Exposes advanced analytics endpoint (Monte Carlo + AR(1) + DuPont placeholder)
  - DuPont now enriched from SEC companyfacts when available

## Architecture

```text
HTTP API (pkg/api)
  -> Data Provider (pkg/data)
      - Alpha Vantage (optional, key-based) via `ALPHA_API_KEY`
      - Yahoo Finance (no API key in current implementation)
      - Stooq CSV fallback
      - SEC fundamentals client for DuPont inputs (ticker->CIK + companyfacts)
  -> Storage (pkg/storage)
      - GORM with configurable driver (`sqlite` local fallback, `postgres` for service-backed DB)
  -> Forecast Engine (pkg/forecast)
      - LinearRegressionForecaster (local calculations)
      - Advanced analysis (Monte Carlo, AR(1), DuPont)
```

## Project Structure

```text
.
├── main.go
├── README.md
├── CLAUDE.md
├── example_usage.md
├── pkg/
│   ├── api/         # Router and handlers
│   ├── data/        # Historical data providers
│   ├── forecast/    # Local forecasting engine(s)
│   ├── model/       # GORM/data models
│   └── storage/     # Persistence and forecast orchestration
└── stock_data.db    # local SQLite fallback (when DB_DRIVER=sqlite)
```

## Requirements

- Go 1.24+
- Optional: `ALPHA_API_KEY` (used only for Alpha Vantage)

Alpha Vantage is only one possible provider. The service supports multiple providers and
can ingest without an Alpha key by using Yahoo/Stooq.

Provider behavior:
- Requests are attempted serially (no parallel fan-out): Alpha Vantage (if key) -> Yahoo -> Stooq
- Cache-first ingestion by default (avoids external calls when local ticker data exists)
- HTTP retry/backoff for transient failures (`429`, `5xx`)
- Small per-provider pacing delay between requests

DuPont fundamentals source:
- SEC `company_tickers.json` for ticker->CIK mapping
- SEC `companyfacts` XBRL API for annual net income, revenue, assets, equity
- Optional env: `SEC_USER_AGENT` (recommended by SEC API policy)

## Run

```bash
go run main.go
```

Default port is `8080` (or set `PORT`).

## Frontend (React)

The frontend app lives in `web/` and provides:
- interactive price chart with hover/crosshair tooltips
- SMA 20 / SMA 50 overlays
- volume subplot
- annual regression and projections
- range selectors (`1Y`, `3Y`, `5Y`, `ALL`)
- PNG and PDF export

Run locally:

```bash
cd web
npm install
npm run dev
```

Open `http://localhost:5173`.

## API

### `POST /ingest?ticker=SYMBOL[&refresh=true]`

Ingests missing data (or uses cache) and generates/stores forecast.
If live fetch fails but local history exists, ingestion still succeeds from cache.
Default behavior is cache-first: if the ticker already exists in DB, it will use local
data and avoid external provider calls.
Use `refresh=true` to force an external fetch attempt for newer rows.

Example response:

```json
{
  "ticker": "PSTG",
  "start_date": "2021-03-08",
  "end_date": "2026-03-08",
  "fetched_record_count": 1250,
  "using_cached_data": false,
  "provider_used": "yahoo",
  "forecast": {
    "ticker": "PSTG",
    "current_year": 2026,
    "remaining_months": 9,
    "current_year_remaining_forecast": 58.2,
    "next_year": 2027,
    "next_year_forecast": 61.1,
    "year_after_next": 2028,
    "year_after_next_forecast": 64.0,
    "regression_slope": 2.9,
    "regression_intercept": -5798.0,
    "generated_at": "2026-03-08T16:00:00Z"
  }
}
```

### `GET /data?ticker=SYMBOL&start_date=YYYY-MM-DD&end_date=YYYY-MM-DD`

Returns stored historical rows from configured DB.

### `GET /forecast?ticker=SYMBOL`

Returns persisted forecast for the ticker.

### `GET /analysis?ticker=SYMBOL`

Returns in-house advanced analytics computed from stored history:
- Monte Carlo price distribution (P10/P50/P90 horizons)
- AR(1)-style return model with 30-day expected price
- DuPont decomposition from SEC annual companyfacts when available
- Rule-based `signal` (`BUY` / `HOLD` / `SELL`) with confidence and rationale

Example (trimmed):

```json
{
  "ticker": "PSTG",
  "current_price": 63.14,
  "monte_carlo": {
    "paths": 300,
    "drift_annual": 0.18,
    "volatility_annual": 0.34,
    "points": [
      { "horizon_days": 21, "p10": 57.4, "p50": 63.8, "p90": 70.9 }
    ]
  },
  "ar1": {
    "forecast_return_1d": 0.0012,
    "expected_price_30d": 65.5
  },
  "dupont": {
    "available": true,
    "source": "sec-companyfacts",
    "net_profit_margin": 0.123,
    "asset_turnover": 0.78,
    "equity_multiplier": 2.11,
    "return_on_equity": 0.202
  },
  "signal": {
    "action": "BUY",
    "confidence": "Medium",
    "score": 3,
    "reasons": [
      "Monte Carlo median 12M upside is strong (12.0%)."
    ],
    "disclaimer": "Educational signal only, not financial advice.",
    "generated_by": "rule-based-v1"
  }
}
```

### Signal Thresholds (rule-based-v1)

The recommendation engine combines Monte Carlo, AR(1), and DuPont:

- Monte Carlo 12M median upside:
  - `>= +12%` => positive (strong)
  - `<= -5%` => negative
- Monte Carlo 12M downside (P10):
  - `<= -35%` => strong risk penalty
  - `<= -25%` => risk penalty
  - `>= -15%` => risk bonus
- AR(1) 30D expected return:
  - `>= +2%` => positive
  - `<= -3%` => negative
- DuPont (if available):
  - ROE `>= 15%` strong positive
  - ROE `8-15%` mild positive
  - ROE `< 5%` negative
  - Net margin `< 3%` additional negative

Decision:
- score `>= 3` => `BUY`
- score `<= -2` => `SELL`
- otherwise => `HOLD`

## Database Schema

### `stock_data`

- `id` (PK)
- `ticker` (indexed, composite unique with `trading_date`)
- `trading_date` (composite unique with `ticker`)
- `open_price`, `high_price`, `low_price`, `close_price`
- `adjusted_close` (nullable)
- `volume`
- `created_at`, `updated_at`

### `forecast_results`

- `ticker` (PK)
- `current_year`
- `remaining_months`
- `current_year_remaining_forecast`
- `next_year`, `next_year_forecast`
- `year_after_next`, `year_after_next_forecast`
- `regression_slope`, `regression_intercept`
- `generated_at`

## Forecasting (Local)

`pkg/forecast/linear.go` implements:

- yearly average aggregation from local historical rows
- ordinary least squares linear regression
- projected average for:
  - remainder of current year
  - next calendar year
  - following calendar year

This module is internal and designed to be extended with additional models later.

## Test Commands

Run all tests:

```bash
go test ./...
```

Coverage report:

```bash
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out
```

Specific packages:

```bash
go test ./pkg/api ./pkg/storage ./pkg/forecast ./pkg/data ./pkg/model
```

Live provider integration test (real PSTG retrieval):

```bash
RUN_REAL_INTEGRATION=1 go test -v ./pkg/data -run TestPSTGRealDataIntegration
```

Notes:
- This test requires network access.
- It may use `alphavantage`, `yahoo`, or `stooq` depending on availability and config.

## Docker

Build and run the baseline container stack (`frontend + backend + db`):

```bash
docker compose up -d --build
```

Services:
- db container (PostgreSQL service)
- backend API: `http://localhost:8080`
- frontend UI: `http://localhost:5173`

Frontend container uses `API_UPSTREAM` to route `/api/*` to backend.
Defaults:
- Docker Compose: `backend:8080`
- Kubernetes: `stock-forecast-backend:8080`

Optional custom ports:

```bash
BACKEND_PORT=18080 FRONTEND_PORT=15173 docker compose up -d --build
```

Persistent DB is stored in Docker volume `stock_pg_data`.

Stop:

```bash
docker compose down
```

Docker-based integration test flow:

```bash
make test-integration-docker
```

## Docker Image Publish

Build and push Linux x86_64 images:

```bash
docker buildx build --platform linux/amd64 -f Dockerfile.backend -t calvarado2004/stock-forecast-backend:latest --push .
docker buildx build --platform linux/amd64 -f web/Dockerfile -t calvarado2004/stock-forecast-frontend:latest --push ./web
```

If push is denied, authenticate first:

```bash
docker login
```

## Kubernetes Baseline

Kubernetes manifests are in `k8s/` and include:
- DB StatefulSet with PVC (`RWO`, `10Gi`, StorageClass `px-csi-db`)
- Backend Deployment + Service
- Frontend Deployment + Service
- Secret templates for DB/backend env and DB init SQL shell

Create secrets first:

```bash
cp k8s/secrets.example.yaml k8s/secrets.yaml
# edit k8s/secrets.yaml with real passwords and ALPHA_API_KEY
kubectl apply -f k8s/secrets.yaml
```

Apply workloads:

```bash
kubectl apply -f k8s/db-statefulset.yaml
kubectl apply -f k8s/backend-deployment.yaml
kubectl apply -f k8s/frontend-deployment.yaml
```

Images referenced by the deployments:
- `calvarado2004/stock-forecast-backend:latest`
- `calvarado2004/stock-forecast-frontend:latest`

Hardening details:
- Backend and DB both use `envFrom` Secrets.
- DB bootstraps app role/database from Secret mounted at `/docker-entrypoint-initdb.d`.
- Backend exposes `/healthz` and has startup/readiness/liveness probes.

## License

This project is licensed under the MIT License. See `LICENSE`.
