# Notes for Swiss investors

This document records implementation assumptions for research only. It is not tax, legal, or investment advice. Verify current rules and the chosen broker's terms before interpreting any result.

## CHF accounting and USD assets

The portfolio's base currency is CHF and the hard maximum contributed capital is CHF 5,000. USD security values are converted using the supplied USD/CHF rate; execution and portfolio valuation require the relevant dated rate. A missing rate is an error, not a reason to reuse a prior observation. Backtest reports and benchmark equity are expressed in CHF.

Stock-detail display of a historical CHF equivalent is not yet implemented. The frontend currently presents USD prices; this is a known limitation until an appropriate historical FX display endpoint and conversion behavior are added.

## Trading-cost placeholders

The current default cost model uses:

- Commission: 5 basis points per fill, with a CHF 1 minimum.
- FX spread: 25 basis points per fill.
- Stamp duty: 30 basis points per fill.
- Slippage: 10 basis points per fill.

These are not a broker quote. Small-account results are especially sensitive to minimum commissions, FX conversion fees, spreads, and minimum order sizes. Replace and validate the model against the actual broker's fee schedule and the intended trading arrangement before relying on backtest comparisons.

## Taxes and corporate actions

The simulator's default dividend withholding rate is 15%, representing a simplifying assumption for eligible US dividends with valid documentation. It does not model an individual's full tax circumstances, reclaim process, wealth taxes, or other tax obligations.

Stamp-duty applicability and the rate depend on the investor, instrument, transaction, and broker arrangement; the 30 bps default is a placeholder requiring verification. Splits, dividends, delistings, and supported cash proceeds are simulated as described in [backtesting assumptions](./backtesting-assumptions.md). Unsupported stock-for-stock mergers fail rather than being assigned fabricated cash value.

## Limits

The capital ceiling is an application constraint, not advice about a suitable investment amount. Historical results remain exposed to provider coverage, survivorship, execution, FX, tax, and model uncertainty. No live trading or brokerage order submission is supported.
