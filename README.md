# TrendRadar

TrendRadar is a research platform for studying medium-term stock-market trends and evaluating transparent investment strategies. It is not investment advice and does not place brokerage orders.

The project is being implemented in the order defined by [the specification](docs/specification.md). Implemented foundations include a Go API, a Vue 3 + TypeScript frontend, PostgreSQL migrations, an OpenAPI-generated contract, NYSE calendar helpers, technical indicators, cross-sectional scoring, fixture market data, a deterministic backtest engine, and statistical-validation building blocks.

## Requirements

- Go 1.26+
- Node.js 22.22.2+ or 24+
- Docker Compose or Podman Compose
- PostgreSQL 18 (or start it with Compose)

## Local setup

1. Copy `.env.example` to `.env` and adjust local values as needed.
2. Start PostgreSQL: `docker compose up -d postgres`
3. Apply the database migrations:

   ```sh
   set -a
   . ./.env
   set +a
   cd backend && go run ./cmd/migrate up
   ```

4. Import the deterministic fixture data: `go run ./cmd/collector --fixture` (from `backend`)
5. Start the API: `go run ./cmd/api` (from `backend`)
6. In another terminal, start the scheduled worker: `go run ./cmd/worker --schedule` (from `backend`)
7. In another terminal, start the frontend: `cd frontend && npm ci && npm run dev`

The API listens on `127.0.0.1:8080` by default. The frontend runs on `127.0.0.1:5173` and proxies `/api` to the API server.

Health endpoints:

- `GET http://127.0.0.1:8080/api/v1/health/live` checks that the process is alive.
- `GET http://127.0.0.1:8080/api/v1/health/ready` checks PostgreSQL connectivity.

## Configuration

See `.env.example` for the supported local environment values. `MAX_CAPITAL_CHF` defaults to the compiled ceiling of CHF 5,000 and may be lowered; values above CHF 5,000 are rejected at startup. API binding is restricted to loopback addresses.

Never put real provider credentials in the repository. No real market data or FX provider has been selected; fixture import is local/test-only and carries the survivorship-biased flag.

## Development commands

From the repository root:

```sh
make generate
make test
make lint
make build
```

Go commands can also be run from `backend`:

```sh
go test ./...
go test -race ./...
go vet ./...
```

Frontend commands can be run from `frontend`: `npm run dev`, `npm run test`, `npm run typecheck`, `npm run lint`, and `npm run build`.

## Architecture

- `backend/cmd/api`: API process entry point and graceful shutdown.
- `backend/internal/config`: environment loading and validation.
- `backend/internal/domain`: shared domain constraints.
- `backend/internal/calendar`: NYSE holiday, trading-day and early-close helpers for 1990–2035.
- `backend/internal/indicators`: pure Go technical indicator functions.
- `backend/internal/scoring`: winsorized cross-sectional percentile ranking.
- `backend/internal/marketdata`: price validation, fixture data, provider contracts and snapshot hashing.
- `backend/internal/repository`: PostgreSQL market-data imports and stock reads.
- `backend/internal/backtest`, `backend/internal/metrics`, and `backend/internal/validation`: decimal-ledger simulation, costs/corporate actions, baseline strategies, and statistical performance reporting.
- `backend/internal/scheduler`: persisted job queue, crash-recoverable leases, daily market-calendar scheduling, ranking, alert, and data-quality jobs.
- `backend/internal/api`: generated OpenAPI server routes for health, stocks, rankings, strategy setup, asynchronous backtests, alerts, and job status.
- `backend/migrations`: embedded transactional PostgreSQL migrations with up/down support.
- `frontend`: Vue, TypeScript, Vue Router, Pinia, and PrimeVue starter.
- `contracts/openapi.yaml`: spec-first HTTP contract.
- `backend/migrations`: versioned PostgreSQL migrations.

## Current limitations

The production market-data provider, delisted-security coverage, broker costs, fractional-share policy, FX source, and CHF risk-free series remain open decisions. Market data currently comes only from deterministic local fixtures; fixture universes and all resulting reports remain marked survivorship-biased. The REST API can create strategies, queue and retrieve historical backtests, expose equity/trades/statistics, acknowledge alerts, and report job status. The frontend includes the dashboard, stock explorer and details, strategy/backtest workflow, and alerts. Start `cmd/worker --schedule` to process persisted jobs. Market-data and FX collection jobs intentionally fail with an explicit “provider is not configured” message until a licensed provider is selected; rankings, alert generation and data-quality checks operate on imported data. See [backtesting assumptions and integration boundaries](docs/backtesting-assumptions.md).

## Important disclaimer

TrendRadar is a research tool, not investment advice. The application must never manage more than CHF 5,000 of contributed capital. Historical results are not a guarantee of future performance.
