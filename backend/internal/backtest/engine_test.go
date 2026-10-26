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
			RankingsAsOfDate: decision,
			Rankings:         []strategy.Candidate{{Symbol: "AAA", Rank: 1, Score: 100, Eligible: true}},
			OpenPricesUSD:    map[string]decimal.Decimal{"AAA": decimal.NewFromInt(10)},
			ClosePricesUSD:   map[string]decimal.Decimal{"AAA": decimal.NewFromInt(10)},
			USDCHF:           decimal.NewFromInt(1), BenchmarkOpenUSD: decimal.NewFromInt(10),
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
	days = testDays()
	days[0].RankingsAsOfDate = days[0].Date
	if _, err := Run(days, testConfig()); err == nil {
		t.Fatal("backtest accepted rankings calculated after the decision date")
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

func TestDividendWithholdingAndSplitAccounting(t *testing.T) {
	days := testDays()
	days[1].CorporateActions = []CorporateAction{
		{Symbol: "AAA", Type: "dividend", Amount: decimal.NewFromInt(1)},
	}
	thirdDay := MarketDay{
		Date:              time.Date(2024, time.January, 5, 0, 0, 0, 0, time.UTC),
		OpenPricesUSD:     map[string]decimal.Decimal{"AAA": decimal.NewFromInt(5)},
		ClosePricesUSD:    map[string]decimal.Decimal{"AAA": decimal.NewFromInt(5)},
		USDCHF:            decimal.NewFromInt(1),
		BenchmarkCloseUSD: decimal.NewFromInt(11),
		CorporateActions: []CorporateAction{
			{Symbol: "AAA", Type: "split", Ratio: decimal.NewFromInt(2)},
		},
	}
	days = append(days, thirdDay)
	config := testConfig()
	config.Costs.WithholdingRate = decimal.RequireFromString("0.15")

	result, err := Run(days, config)
	if err != nil {
		t.Fatal(err)
	}
	if !result.WithholdingTaxCHF.Equal(decimal.NewFromInt(15)) {
		t.Fatalf("withholding tax = %s, want CHF 15", result.WithholdingTaxCHF)
	}
	if !result.FinalCashCHF.Equal(decimal.NewFromInt(85)) {
		t.Fatalf("final cash = %s, want CHF 85", result.FinalCashCHF)
	}
	if got := result.Positions["AAA"].Quantity; !got.Equal(decimal.NewFromInt(200)) {
		t.Fatalf("post-split quantity = %s, want 200", got)
	}
	if !result.Equity[2].PortfolioCHF.Equal(decimal.NewFromInt(1085)) {
		t.Fatalf("final portfolio = %s, want CHF 1085", result.Equity[2].PortfolioCHF)
	}
	if len(result.CorporateActions) != 2 || result.CorporateActions[0].WithholdingCHF.String() != "15" {
		t.Fatalf("corporate action records = %+v", result.CorporateActions)
	}
	for i, entry := range result.Ledger {
		if i > 0 && entry.BalanceCHF.Sub(result.Ledger[i-1].BalanceCHF).Cmp(entry.AmountCHF) != 0 {
			t.Fatalf("ledger balance mismatch at entry %d: %+v", i, entry)
		}
	}
}

func TestBankruptcyDelistingUsesConfiguredRecovery(t *testing.T) {
	days := testDays()
	days[1].CorporateActions = []CorporateAction{
		{Symbol: "AAA", Type: "delisting", Reason: "bankruptcy"},
	}
	config := testConfig()
	config.BankruptcyRecoveryRate = decimal.Zero

	result, err := Run(days, config)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Positions) != 0 || !result.FinalCashCHF.IsZero() {
		t.Fatalf("bankruptcy position was not written off: positions=%v cash=%s", result.Positions, result.FinalCashCHF)
	}
	if result.Equity[1].PortfolioCHF.Sign() != 0 {
		t.Fatalf("bankruptcy equity = %s, want zero", result.Equity[1].PortfolioCHF)
	}
}

func TestDefaultConfigIncludesDocumentedDelistingPolicy(t *testing.T) {
	config := DefaultConfig(strategy.DefaultConfig())
	if !config.UnknownDelistingRecoveryRate.Equal(decimal.RequireFromString("0.70")) ||
		!config.BankruptcyRecoveryRate.IsZero() {
		t.Fatalf("unexpected delisting defaults: bankruptcy=%s unknown=%s",
			config.BankruptcyRecoveryRate, config.UnknownDelistingRecoveryRate)
	}
}

func TestRunRejectsMissingBenchmarkObservations(t *testing.T) {
	days := testDays()
	days[1].BenchmarkCloseUSD = decimal.Zero
	if _, err := Run(days, testConfig()); err == nil {
		t.Fatal("backtest accepted a missing benchmark close price")
	}
	days = testDays()
	days[0].BenchmarkOpenUSD = decimal.Zero
	if _, err := Run(days, testConfig()); err == nil {
		t.Fatal("backtest accepted a missing initial benchmark price")
	}
}

func TestAppendingFutureMarketDataDoesNotChangePastDecisions(t *testing.T) {
	days := testDays()
	original, err := Run(days, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	futureDay := MarketDay{
		Date:           days[len(days)-1].Date.AddDate(0, 0, 1),
		OpenPricesUSD:  map[string]decimal.Decimal{"AAA": decimal.NewFromInt(1000)},
		ClosePricesUSD: map[string]decimal.Decimal{"AAA": decimal.NewFromInt(1000)},
		USDCHF:         decimal.NewFromInt(1), BenchmarkCloseUSD: decimal.NewFromInt(20),
	}
	withFutureData, err := Run(append(days, futureDay), testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(original.Orders) != len(withFutureData.Orders) ||
		len(original.Equity) != 2 || !original.Equity[0].PortfolioCHF.Equal(withFutureData.Equity[0].PortfolioCHF) ||
		!original.Equity[1].PortfolioCHF.Equal(withFutureData.Equity[1].PortfolioCHF) {
		t.Fatalf("future observations changed prior decisions or equity: before=%+v after=%+v",
			original, withFutureData)
	}
}
