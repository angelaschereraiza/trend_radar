# Architecture

## Components and responsibilities

- `backend/cmd/api` starts the HTTP API and wires configuration, PostgreSQL, and request handlers.
- `backend/cmd/collector` imports explicit local fixtures, Yahoo Finance daily bars for the configured ticker list, or EODHD exchange catalogs and selected-symbol daily bars.
- `backend/cmd/migrate` applies or rolls back the embedded PostgreSQL migrations.
- `backend/cmd/worker` runs persisted jobs, either on demand or on the market schedule.
- `backend/internal/api` contains HTTP mapping and orchestration. The generated handler types are derived from `contracts/openapi.yaml`; do not edit `generated.gen.go` by hand.
- `backend/internal/repository` contains PostgreSQL reads and writes for market data, rankings, backtests, validation, alerts, and jobs.
- `backend/internal/calendar`, `indicators`, `scoring`, `strategy`, `backtest`, `metrics`, `stats`, and `validation` hold deterministic domain and analytics logic outside the HTTP and persistence layers.
- `backend/internal/scheduler` determines due jobs; `backend/internal/worker` executes supported job operations.
- `frontend` is a Vue 3 and TypeScript single-page application. Its API client communicates with the backend through `/api`.

## Data flow

1. A collector supplies symbols, daily OHLCV bars, corporate actions, and FX rates as a `marketdata.ImportBatch`.
2. The batch is validated, assigned provider/version metadata, persisted idempotently, and summarized by a content-hashed snapshot. Imported limited universes are marked survivorship-biased.
3. Ranking jobs read stored prices, calculate indicators and cross-sectional factor scores, and persist dated rankings.
4. The API reads stored stocks, rankings, alerts, jobs, and backtest results for the frontend.
5. A backtest request pins a data snapshot, universe, strategy configuration, and dataset role. The runner simulates the request, persists the result, and exposes status, equity, trades, and metrics.
6. Scheduled and manual jobs share the persisted `job_runs` queue. Leases allow interrupted work to be recovered without relying on process-local locks.

The market-data provider and FX provider contracts are defined in `backend/internal/marketdata/types.go`. Yahoo Finance is used only by the explicit collector command and does not provide a complete ticker catalog in this integration. EODHD catalog and selected-history imports use a local API token and are subject to free-plan quotas. CHF-denominated securities are excluded from the current USD-only ranking and backtest queries.

## Persistence

Migrations in `backend/migrations` define the PostgreSQL schema. Important table groups are:

- Reference and input data: `stocks`, `symbol_history`, `stock_prices`, `fx_rates`, `corporate_actions`, `universes`, `market_universe_memberships`, and `data_snapshots`.
- Analytics: `indicators`, `strategy_definitions`, `stock_scores`, and `factor_correlations`.
- Research: `backtest_runs`, `backtest_orders`, `backtest_fills`, `backtest_cash_ledger`, `backtest_positions`, `backtest_equity`, `experiment_trials`, and `holdout_access_log`.
- Platform: `job_runs` and `alerts`.

Prices and FX rates retain provider and data-version identifiers. Backtest results refer to the snapshot and universe used, and record the dataset role and survivorship flag. Monetary ledger and fill amounts use PostgreSQL `NUMERIC`; the Go simulation uses decimal arithmetic for money.

## Boundaries and operational limitations

- The API binds to loopback by default and has no authentication; it must not be exposed publicly without adding authentication and transport security.
- Scheduled market/FX collection, point-in-time delisted-security coverage, and integration tests against a disposable PostgreSQL service remain unimplemented or unresolved.
- Fixture-based results must be treated as survivorship-biased research output, not as evidence of future returns.
- The application has no brokerage order execution.

For local commands and configuration, see the [README](../README.md). For persisted entities and constraints, see the versioned SQL migrations in `../backend/migrations/`.
