# TrendRadar

TrendRadar is a research platform for studying medium-term stock-market trends and evaluating transparent investment strategies. It is not investment advice and does not place brokerage orders.

The project is being implemented in the order defined by [the specification](docs/specification.md). Implemented foundations include a Go API, a Vue 3 + TypeScript frontend, PostgreSQL migrations, an OpenAPI-generated contract, NYSE calendar helpers, technical indicators, cross-sectional scoring, fixture market data, a deterministic backtest engine, and statistical-validation building blocks.

## Requirements

- Go 1.26+
- Node.js 22.22.2+ or 24+
- Podman with `podman compose`
- PostgreSQL 18 (or start it with Podman Compose)

## Local setup

1. Copy `.env.example` to `.env` and adjust local values as needed.
2. Install frontend dependencies once: `cd frontend && npm ci && cd ..`
3. Start the complete local stack from the repository root:

   ```sh
   make dev
   ```

   This starts PostgreSQL, waits for it to become ready, applies migrations, and runs the API, scheduled worker, and frontend. It does not import synthetic or real market data. Press `Ctrl+C` to stop the local processes. The PostgreSQL container remains running.

   To start or stop only PostgreSQL, use `make db-up` or `make db-down`.

4. In a second terminal, import real daily data for Nestlé, SPY and USD/CHF:

   ```sh
   make import-yahoo
   ```

   This downloads up to ten years of daily history using Yahoo Finance's public chart endpoint, then stores an explicitly survivorship-biased snapshot. The endpoint is unofficial, may change or be rate-limited, and its terms/licensing suitability are not guaranteed. Use it only for local/private research under the provider's applicable terms. The instrument catalog lets you open Nestlé's CHF price history even though CHF securities are excluded from the current USD-only rankings and backtests. With only SPY as a USD symbol, the current import does not provide a broad investable universe for meaningful rankings.

The API listens on `127.0.0.1:8080` by default. The frontend runs on `127.0.0.1:5173` and proxies `/api` to the API server.

Health endpoints:

- `GET http://127.0.0.1:8080/api/v1/health/live` checks that the process is alive.
- `GET http://127.0.0.1:8080/api/v1/health/ready` checks PostgreSQL connectivity.

## Configuration

See `.env.example` for the supported local environment values. `MAX_CAPITAL_CHF` defaults to the compiled ceiling of CHF 5,000 and may be lowered; values above CHF 5,000 are rejected at startup. API binding is restricted to loopback addresses.

The Yahoo Finance import currently needs no credentials. It is an unofficial source and should only be used for local/private research subject to Yahoo's applicable terms. The normal `make dev` startup does not import data. To load synthetic development data explicitly, use `make fixture-import`; do not treat fixture-derived results as real market data.

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
- `backend/internal/marketdata`: price validation, fixtures, the Yahoo Finance chart adapter, provider contracts and snapshot hashing.
- `backend/internal/repository`: PostgreSQL market-data imports and stock reads.
- `backend/internal/backtest`, `backend/internal/metrics`, and `backend/internal/validation`: decimal-ledger simulation, costs/corporate actions, baseline strategies, and statistical performance reporting.
- `backend/internal/scheduler`: persisted job queue, crash-recoverable leases, daily market-calendar scheduling, ranking, alert, and data-quality jobs.
- `backend/internal/api`: generated OpenAPI server routes for health, stocks, rankings, strategy setup, asynchronous backtests, alerts, and job status.
- `backend/migrations`: embedded transactional PostgreSQL migrations with up/down support.
- `frontend`: Vue, TypeScript, Vue Router, Pinia, and PrimeVue starter.
- `contracts/openapi.yaml`: spec-first HTTP contract.
- `backend/migrations`: versioned PostgreSQL migrations.

## Current limitations

The Yahoo Finance chart adapter provides daily prices for Nestlé (`NESN.SW`) and SPY, plus USD/CHF rates; this is an unofficial endpoint, not a licensed production feed. Delisted-security coverage, broker costs, fractional-share policy, and a CHF risk-free series remain open decisions. Imported universes and all resulting reports remain marked survivorship-biased. The current ranking and backtest calculations use USD-denominated instruments only; Nestlé is visible in the instrument catalog and its chart is denominated in CHF. The REST API can create strategies, queue and retrieve historical backtests, expose equity/trades/statistics, acknowledge alerts, and report job status. The frontend includes the dashboard, stock explorer and details, strategy/backtest workflow, and alerts. Start `cmd/worker --schedule` to process persisted jobs. See [backtesting assumptions and integration boundaries](docs/backtesting-assumptions.md).

Additional reference documentation: [architecture](docs/architecture.md), [strategy and scoring](docs/strategy.md), [backtesting and validation](docs/backtesting.md), [data sources](docs/data-sources.md), and [Swiss investor assumptions](docs/swiss-investor-notes.md).

## Important disclaimer

TrendRadar is a research tool, not investment advice. The application must never manage more than CHF 5,000 of contributed capital. Historical results are not a guarantee of future performance.
