# Data sources and coverage

## Provider status

The collector supports explicit import modes:

- `go run ./cmd/collector --yahoo` fetches up to ten years of daily data for Nestlé (`NESN.SW`), Novartis (`NOVN.SW`), Roche (`ROG.SW`), SPY, and USD/CHF (`USDCHF=X`) from Yahoo Finance's public chart endpoint. Run it from `backend` after configuring the database; `make import-yahoo` is the repository-root shortcut. This is a fixed initial ticker list, not a Yahoo-wide catalog.
- EODHD provides a broader, global active-common-stock catalog one exchange at a time and end-of-day history for selected listings. Set `EODHD_API_TOKEN` locally, then use `make import-eodhd-exchanges`, `make import-eodhd-catalog EXCHANGE=SW`, and `make import-eodhd-history EXCHANGE=SW SYMBOL=NOVN`. `SW` identifies the SIX Swiss Exchange. The history command defaults to the latest year and accepts optional `FROM` and `TO` dates.
- `go run ./cmd/collector --fixture` imports deterministic synthetic data for tests/development only. It is not run by `make dev`; `make fixture-import` is the explicit shortcut.

The Yahoo chart endpoint is unofficial, may change or be rate-limited, and neither availability nor licensing suitability is guaranteed. Use it only for local/private research under applicable provider terms. EODHD's current free plan is limited to 20 API calls per day and the past year of end-of-day history; a selected-symbol history import uses four calls (exchange catalog, prices, dividends, and splits). Its exchange catalog can be loaded incrementally, but the free quota is not suitable for obtaining prices for every global listing. Neither source in this setup provides delisted-security or historical point-in-time constituent coverage; imported market snapshots and research remain marked survivorship-biased.

The provider contracts are:

- `marketdata.MarketDataProvider`: symbol listings, historical prices, and corporate actions.
- `marketdata.FXProvider`: daily currency-pair rates over a requested date range.

Both use context-aware calls and carry provider identity; price and FX records also carry data-version metadata. Both imports are manual collector commands, not scheduled collection jobs. Yahoo requires no credentials in this adapter. EODHD requires an API token in local configuration; never commit credentials to the repository.

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

The Yahoo adapter imports adjusted closes, dividends, splits, and USD/CHF closes. The EODHD adapter imports adjusted daily closes, dividends, splits, and active common-stock metadata for selected securities. Rankings and backtests currently accept USD-denominated securities only; CHF-denominated Swiss stocks are retained and displayed in CHF but excluded from those calculations. For credible historical evaluation, a production integration must document licensing, adjusted-price policy, corporate-action coverage, FX methodology, point-in-time universe coverage, and delisting treatment. Neither source removes the survivorship warning.

Open provider, broker, universe, fractional-share, and risk-free-rate decisions are listed in [the specification](./specification.md#23-open-decisions-to-be-resolved-before-or-during-milestone-2).
