package backtest

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/angelaschereraiza/trend_radar/backend/internal/strategy"
)

func testConfig() Config {
	strategyConfig := strategy.DefaultConfig()
	strategyConfig.InitialCapitalCHF = decimal.NewFromInt(1000)
	strategyConfig.MaxPositions = 1
	strategyConfig.RankBuffer = 1
	strategyConfig.MaxHoldingMonths = 12
	strategyConfig.MinTradeValueCHF = decimal.Zero
	strategyConfig.FractionalShares = true
	return Config{
		Strategy: strategyConfig,
		Costs: CostConfig{
			CommissionBPS: decimal.Zero, CommissionMin: decimal.Zero,
			FXSpreadBPS: decimal.Zero, StampDutyBPS: decimal.Zero,
			SlippageBPS: decimal.Zero, WithholdingRate: decimal.Zero,
		},
	}
}

func testDays() []MarketDay {
	decision := time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC)
	return []MarketDay{
		{
			Date:         time.Date(2024, time.January, 3, 0, 0, 0, 0, time.UTC),
			DecisionDate: decision, Rebalance: true,
			Rankings:       []strategy.Candidate{{Symbol: "AAA", Rank: 1, Score: 100, Eligible: true}},
			OpenPricesUSD:  map[string]decimal.Decimal{"AAA": decimal.NewFromInt(10)},
			ClosePricesUSD: map[string]decimal.Decimal{"AAA": decimal.NewFromInt(10)},
			USDCHF:         decimal.NewFromInt(1), BenchmarkOpenUSD: decimal.NewFromInt(10),
			BenchmarkCloseUSD: decimal.NewFromInt(10),
		},
		{
			Date:           time.Date(2024, time.January, 4, 0, 0, 0, 0, time.UTC),
			OpenPricesUSD:  map[string]decimal.Decimal{"AAA": decimal.NewFromInt(11)},
			ClosePricesUSD: map[string]decimal.Decimal{"AAA": decimal.NewFromInt(11)},
			USDCHF:         decimal.NewFromInt(1), BenchmarkCloseUSD: decimal.NewFromInt(11),
		},
	}
}

func TestGoldenBacktestReference(t *testing.T) {
	result, err := Run(testDays(), testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) != 1 || !result.Fills[0].Quantity.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("fills = %+v; expected purchase of 100 shares", result.Fills)
	}
	if !result.Equity[1].PortfolioCHF.Equal(decimal.NewFromInt(1100)) {
		t.Fatalf("final portfolio = %s, want CHF 1100", result.Equity[1].PortfolioCHF)
	}
	if !result.Equity[1].BenchmarkCHF.Equal(decimal.NewFromInt(1100)) {
		t.Fatalf("benchmark = %s, want CHF 1100", result.Equity[1].BenchmarkCHF)
	}
	if len(result.Ledger) != 6 {
		t.Fatalf("ledger entries = %d, want initial capital and five trade entries", len(result.Ledger))
	}
}

func TestRunRejectsFutureSignalAndCapitalAboveCeiling(t *testing.T) {
	days := testDays()
	days[0].DecisionDate = days[0].Date
	if _, err := Run(days, testConfig()); err == nil {
		t.Fatal("backtest accepted same-day decision/execution")
	}
	config := testConfig()
	config.Strategy.InitialCapitalCHF = decimal.NewFromInt(5001)
	if _, err := Run(testDays(), config); err == nil {
		t.Fatal("backtest accepted capital above ceiling")
	}
}

func TestCostsReducePortfolioValueAndCashNeverGoesNegative(t *testing.T) {
	config := testConfig()
	config.Costs = DefaultCosts()
	result, err := Run(testDays(), config)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Equity[0].PortfolioCHF.LessThan(decimal.NewFromInt(1000)) {
		t.Fatalf("costs did not reduce equity: %s", result.Equity[0].PortfolioCHF)
	}
	for _, point := range result.Equity {
		if point.CashCHF.IsNegative() {
			t.Fatalf("negative cash: %s", point.CashCHF)
		}
	}
}
