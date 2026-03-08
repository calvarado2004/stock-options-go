# Stock Options API

Backend API that retrieves and stores historical stock data and performs in-house forecasting.

## What It Does

- Ingests approximately 5 years of daily historical stock data for a ticker (e.g., `PSTG`)
- Stores data in local SQLite (`stock_data.db`) with upsert semantics
- Reuses cached data when local data is sufficiently up to date
- Performs local linear regression forecasting (no external forecasting service)
- Exposes REST endpoints for ingestion, historical data query, and forecast retrieval

## Architecture

```text
HTTP API (pkg/api)
  -> Data Provider (pkg/data)
      - Alpha Vantage (optional, key-based) via `ALPHA_API_KEY`
      - Yahoo Finance (no API key in current implementation)
      - Stooq CSV fallback
  -> Storage (pkg/storage)
      - SQLite via GORM
  -> Forecast Engine (pkg/forecast)
      - LinearRegressionForecaster (local calculations)
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
└── stock_data.db    # SQLite DB (created at runtime)
```

## Requirements

- Go 1.24+
- Optional: `ALPHA_API_KEY` (used only for Alpha Vantage)

Alpha Vantage is only one possible provider. The service supports multiple providers and
can ingest without an Alpha key by using Yahoo/Stooq.

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

### `POST /ingest?ticker=SYMBOL`

Ingests missing data (or uses cache) and generates/stores forecast.

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

Returns stored historical rows from SQLite.

### `GET /forecast?ticker=SYMBOL`

Returns persisted forecast for the ticker.

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
- db container (SQLite volume + file init)
- backend API: `http://localhost:8080`
- frontend UI: `http://localhost:5173`

Optional custom ports:

```bash
BACKEND_PORT=18080 FRONTEND_PORT=15173 docker compose up -d --build
```

Persistent DB is stored in Docker volume `stock_data` and shared between `db` and `backend`.

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

Apply:

```bash
kubectl apply -f k8s/db-statefulset.yaml
kubectl apply -f k8s/backend-deployment.yaml
kubectl apply -f k8s/frontend-deployment.yaml
```

Images referenced by the deployments:
- `calvarado2004/stock-forecast-backend:latest`
- `calvarado2004/stock-forecast-frontend:latest`

Optional Alpha key secret:

```bash
kubectl create secret generic stock-forecast-secrets \
  --from-literal=alpha_api_key=\"<your-alpha-key>\"
```

## License

This project is licensed under the MIT License. See `LICENSE`.
