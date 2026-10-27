# Data sources and coverage

## Provider status

The collector supports two explicit import modes:

- `go run ./cmd/collector --yahoo` fetches up to ten years of daily data for Nestlé (`NESN.SW`, CHF), SPY (USD), and USD/CHF (`USDCHF=X`) from Yahoo Finance's public chart endpoint. Run it from `backend` after configuring the database; `make import-yahoo` is the repository-root shortcut.
- `go run ./cmd/collector --fixture` imports deterministic synthetic data for tests/development only. It is not run by `make dev`; `make fixture-import` is the explicit shortcut.

The Yahoo chart endpoint is unofficial, may change or be rate-limited, and neither availability nor licensing suitability is guaranteed. Use it only for local/private research under applicable provider terms. Its small current-symbol set does not include delisted securities or historical point-in-time constituents; all resulting snapshots and reports remain marked survivorship-biased. This is not a production data feed.

The provider contracts are:

- `marketdata.MarketDataProvider`: symbol listings, historical prices, and corporate actions.
- `marketdata.FXProvider`: daily currency-pair rates over a requested date range.

Both use context-aware calls and carry provider identity; price and FX records also carry data-version metadata. The Yahoo import currently requires no credentials and is a manual collector command, not a scheduled collection job. Credentials for any future provider should be supplied through local configuration or a secret store, never committed to the repository.

## Required data

The application stores:

- Stock metadata and symbol history.
- Daily OHLCV and adjusted close, including currency, provider, and data version.
- Corporate actions: splits, dividends, delistings, symbol changes, and mergers.
- Dated FX rates; backtests require USD/CHF rates for execution and valuation.
- Universe membership with effective dates for point-in-time research.

Imported batches are checked for required identifiers, positive prices and FX rates, valid OHLC relationships, non-negative volume, date ordering/identity constraints, supported corporate-action types, and positive split ratios. Reimports are idempotent by database keys. A snapshot records row counts, date range, provider summary, survivorship status, and a SHA-256 content hash of canonicalized batch data.

## Data quality and interpretation

The database preserves provider and data-version identifiers so conflicting versions can be distinguished. The system does not fabricate missing prices or FX rates. A backtest that needs unavailable aligned data fails explicitly.

The Yahoo adapter imports adjusted closes, dividends, splits, and USD/CHF closes. Rankings and backtests currently accept USD-denominated securities only; CHF-denominated Nestlé is retained and displayed in CHF but excluded from those calculations. For credible historical evaluation, a production integration must document licensing, adjusted-price policy, corporate-action coverage, FX methodology, point-in-time universe coverage, and delisting treatment. The limited Yahoo import cannot remove the survivorship warning.

Open provider, broker, universe, fractional-share, and risk-free-rate decisions are listed in [the specification](./specification.md#23-open-decisions-to-be-resolved-before-or-during-milestone-2).
