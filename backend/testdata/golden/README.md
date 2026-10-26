# Golden backtest reference

The fixture represents CHF 1,000 in cash, one fully affordable AAA position, no costs, and a benchmark with identical execution prices.

1. On 2024-01-03, the signal from the prior session ranks AAA first. The engine buys `1,000 / (USD 10 × USD/CHF 1) = 100` shares.
2. Cash after the fill is CHF 0 and invested value is CHF 1,000.
3. On 2024-01-04, the closing price is USD 11 and FX remains 1. Portfolio value is `100 × USD 11 × 1 = CHF 1,100`.
4. The passive benchmark also owns 100 units and finishes at CHF 1,100.

The test asserts the exact share quantity, final portfolio and benchmark values, and the initial-capital plus five-entry trade ledger.
