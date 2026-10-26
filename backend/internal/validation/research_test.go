package validation

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/angelaschereraiza/trend_radar/backend/internal/backtest"
	"github.com/angelaschereraiza/trend_radar/backend/internal/metrics"
	"github.com/angelaschereraiza/trend_radar/backend/internal/strategy"
)

func TestRunResearchBuildsSimulationAndBaselineReport(t *testing.T) {
	request := testResearchRequest()
	result, err := RunResearch(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"momentum", "buy_and_hold", "spy", "cash_chf"} {
		if _, ok := result.Report.Baselines[name]; !ok {
			t.Errorf("%s baseline missing from integrated report", name)
		}
	}
}

func testResearchRequest() ResearchRequest {
	day1 := time.Date(2024, time.January, 3, 0, 0, 0, 0, time.UTC)
	day2 := day1.AddDate(0, 0, 1)
	day3 := day2.AddDate(0, 0, 1)
	days := []backtest.MarketDay{
		{
			Date: day1, DecisionDate: day1.AddDate(0, 0, -1), RankingsAsOfDate: day1.AddDate(0, 0, -1), Rebalance: true,
			Rankings: []strategy.Candidate{{Symbol: "AAA", Rank: 1, Momentum: 0.1, Eligible: true}},
			OpenPricesUSD: map[string]decimal.Decimal{
				"AAA": decimal.NewFromInt(10), "BBB": decimal.NewFromInt(10),
			},
			ClosePricesUSD: map[string]decimal.Decimal{
				"AAA": decimal.NewFromInt(10), "BBB": decimal.NewFromInt(10),
			},
			USDCHF: decimal.NewFromInt(1), BenchmarkOpenUSD: decimal.NewFromInt(10),
			BenchmarkCloseUSD: decimal.NewFromInt(10),
		},
		{
			Date: day2, DecisionDate: day1, RankingsAsOfDate: day1, Rebalance: true,
			Rankings: []strategy.Candidate{
				{Symbol: "AAA", Rank: 1, Momentum: 0.1, Eligible: true},
				{Symbol: "BBB", Rank: 2, Momentum: 0.2, Eligible: true},
			},
			OpenPricesUSD: map[string]decimal.Decimal{
				"AAA": decimal.NewFromInt(10), "BBB": decimal.NewFromInt(10),
			},
			ClosePricesUSD: map[string]decimal.Decimal{
				"AAA": decimal.NewFromInt(10), "BBB": decimal.NewFromInt(10),
			},
			USDCHF: decimal.NewFromInt(1), BenchmarkCloseUSD: decimal.NewFromInt(10),
		},
		{
			Date: day3,
			OpenPricesUSD: map[string]decimal.Decimal{
				"AAA": decimal.NewFromInt(11), "BBB": decimal.NewFromInt(10),
			},
			ClosePricesUSD: map[string]decimal.Decimal{
				"AAA": decimal.NewFromInt(11), "BBB": decimal.NewFromInt(10),
			},
			USDCHF: decimal.NewFromInt(1), BenchmarkCloseUSD: decimal.NewFromInt(11),
		},
	}
	strategyConfig := strategy.DefaultConfig()
	strategyConfig.InitialCapitalCHF = decimal.NewFromInt(1000)
	strategyConfig.MaxPositions = 1
	strategyConfig.RankBuffer = 1
	strategyConfig.MaxHoldingMonths = 12
	strategyConfig.MinTradeValueCHF = decimal.Zero
	strategyConfig.FractionalShares = true
	backtestConfig := backtest.Config{
		Strategy: strategyConfig,
		Costs: backtest.CostConfig{
			CommissionMin: decimal.Zero, WithholdingRate: decimal.Zero,
		},
	}

	return ResearchRequest{
		Days: days, BacktestConfig: backtestConfig, MetricsConfig: metrics.Config{},
		PointInTimeUniverse: map[string][]string{
			MembershipDateKey(day1): {"AAA", "BBB"},
			MembershipDateKey(day2): {"AAA", "BBB"},
			MembershipDateKey(day3): {"AAA", "BBB"},
		},
		ReportConfig: ReportConfig{
			Role: Development, TrialCount: 1, Bootstrap: DefaultBootstrapConfig(),
		},
	}
}

type testResearchRegistry struct {
	trials       int
	holdoutUsed  bool
	logged       int
	recorded     int
	confirmation bool
	biased       bool
	snapshotRead int
}

func (r *testResearchRegistry) TrialCount(context.Context, string, string) (int, error) {
	return r.trials, nil
}

func (r *testResearchRegistry) SnapshotSurvivorshipBiased(context.Context, uuid.UUID) (bool, error) {
	r.snapshotRead++
	return r.biased, nil
}

func (*testResearchRegistry) ValidateResearchRun(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string) error {
	return nil
}

func (*testResearchRegistry) HistoricalUniverseMembership(_ context.Context, _ uuid.UUID, dates []time.Time) (map[string][]string, error) {
	membership := make(map[string][]string, len(dates))
	for _, date := range dates {
		membership[MembershipDateKey(date)] = []string{"AAA", "BBB"}
	}
	return membership, nil
}

func (r *testResearchRegistry) LogHoldoutAccess(_ context.Context, _, _ string, _ uuid.UUID, _ string, confirmed bool) (bool, error) {
	r.logged++
	r.confirmation = confirmed
	alreadyUsed := r.holdoutUsed
	r.holdoutUsed = true
	return alreadyUsed, nil
}

func (r *testResearchRegistry) RecordTrialForRun(_ context.Context, _, _, _ string, _ any, _, _ uuid.UUID, _ float64) error {
	r.trials++
	r.recorded++
	return nil
}

func TestRunResearchPersistsHoldoutAndTrialMetadata(t *testing.T) {
	request := testResearchRequest()
	request.ReportConfig.Role = Holdout
	request.HoldoutConfirmed = true
	request.Registry = &testResearchRegistry{trials: 2, holdoutUsed: true, biased: true}
	request.StrategyName = "momentum"
	request.StrategyVersion = "1"
	request.SnapshotID = uuid.New()
	request.UniverseID = uuid.New()
	request.RunID = uuid.New()
	result, err := RunResearch(request)
	if err != nil {
		t.Fatal(err)
	}
	registry := request.Registry.(*testResearchRegistry)
	if registry.logged != 1 || registry.recorded != 1 || registry.snapshotRead != 1 || !registry.confirmation {
		t.Fatalf("persisted workflow calls: %+v", registry)
	}
	if result.Report.TrialCount != 3 || !result.Report.HoldoutAlreadyUsed || !result.Report.SurvivorshipBiased {
		t.Fatalf("report did not use persisted trial and holdout history: %+v", result.Report)
	}
}

func TestRunResearchRequiresHoldoutConfirmationBeforeSimulation(t *testing.T) {
	_, err := RunResearch(ResearchRequest{
		ReportConfig: ReportConfig{Role: Holdout},
	})
	if err == nil {
		t.Fatal("research run entered holdout without explicit confirmation")
	}
}

func TestRunResearchRequiresPointInTimeMembership(t *testing.T) {
	request := testResearchRequest()
	request.PointInTimeUniverse = nil
	if _, err := RunResearch(request); err == nil {
		t.Fatal("research run accepted missing point-in-time universe data")
	}
}

func TestRunResearchConfirmedHoldoutRequiresPersistentRegistry(t *testing.T) {
	request := testResearchRequest()
	request.ReportConfig.Role = Holdout
	request.HoldoutConfirmed = true
	if _, err := RunResearch(request); err == nil {
		t.Fatal("confirmed holdout run without persistent access logging was accepted")
	}
}

func TestCashBaselineUsesConfiguredCHFReferenceRate(t *testing.T) {
	start := time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC)
	result, err := cashBaseline([]backtest.EquityPoint{
		{Date: start, PortfolioCHF: decimal.NewFromInt(100)},
		{Date: start.AddDate(0, 0, 1), PortfolioCHF: decimal.NewFromInt(100)},
	}, 0.252)
	if err != nil {
		t.Fatal(err)
	}
	want := decimal.NewFromInt(100).Mul(decimal.NewFromFloat(1.001))
	if !result.Equity[1].PortfolioCHF.Equal(want) {
		t.Fatalf("cash risk-free baseline value = %s, want %s", result.Equity[1].PortfolioCHF, want)
	}
}
