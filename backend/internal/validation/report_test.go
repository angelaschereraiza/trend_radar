package validation

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/angelaschereraiza/trend_radar/backend/internal/backtest"
	"github.com/angelaschereraiza/trend_radar/backend/internal/metrics"
	"github.com/angelaschereraiza/trend_radar/backend/internal/stats"
)

func TestBuildPerformanceReportIncludesStatisticalAndBiasWarnings(t *testing.T) {
	start := time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC)
	equity := make([]backtest.EquityPoint, 8)
	for i := range equity {
		factor := decimal.NewFromInt(int64(100 + i))
		equity[i] = backtest.EquityPoint{
			Date: start.AddDate(0, 0, i), PortfolioCHF: factor,
			BenchmarkCHF: decimal.NewFromInt(int64(100 + i/2)),
		}
	}
	report, err := BuildPerformanceReport(backtest.Result{Equity: equity}, metrics.Config{}, ReportConfig{
		Role: Development, TrialCount: 4, SurvivorshipBiased: true,
		HoldoutAlreadyUsed: true, DevelopmentPeriod: true,
		Bootstrap: stats.BootstrapConfig{
			Replicates: 100, BlockLength: 2, Seed: 42, ConfidenceLevel: 0.95,
		},
		Baselines: map[string]backtest.Result{"buy_and_hold": {Equity: equity}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.TrialCount != 4 || report.DatasetRole != Development || !report.SurvivorshipBiased ||
		!report.HoldoutAlreadyUsed || !report.DevelopmentLacksBearMarket {
		t.Fatalf("report metadata = %+v", report)
	}
	if report.ConfidenceIntervals.Replicates != 100 || report.ConfidenceIntervals.Seed != 42 {
		t.Fatalf("bootstrap reproducibility metadata missing: %+v", report.ConfidenceIntervals)
	}
	if report.ConfidenceIntervals.ConfidenceLevel != 0.95 || report.Promising {
		t.Fatalf("development report was incorrectly labelled: %+v", report)
	}
	if _, ok := report.Baselines["buy_and_hold"]; !ok {
		t.Fatalf("baseline comparison missing from report: %+v", report.Baselines)
	}
	for _, warning := range []string{
		"survivorship-biased", "holdout has already been accessed", "development-period benchmark drawdown",
	} {
		found := false
		for _, actual := range report.Warnings {
			found = found || strings.Contains(actual, warning)
		}
		if !found {
			t.Errorf("warning %q missing from %v", warning, report.Warnings)
		}
	}
}

func TestBuildPerformanceReportRejectsUnalignedOrInsufficientEquity(t *testing.T) {
	_, err := BuildPerformanceReport(backtest.Result{}, metrics.Config{}, ReportConfig{
		Role: Development, TrialCount: 1,
	})
	if err == nil {
		t.Fatal("report accepted an empty equity curve")
	}
}

func TestDatasetSplitAndHoldoutConfirmation(t *testing.T) {
	date := func(year int) time.Time {
		return time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	}
	split := DatasetSplit{
		DevelopmentStart: date(2004), DevelopmentEnd: date(2017),
		HoldoutStart: date(2018), HoldoutEnd: date(2025), ForwardStart: date(2026),
	}
	if err := split.Validate(); err != nil {
		t.Fatal(err)
	}
	if role, err := split.RoleForDate(date(2020)); err != nil || role != Holdout {
		t.Fatalf("holdout role = %q, %v", role, err)
	}
	if err := RequireHoldoutConfirmation(Holdout, false); err == nil {
		t.Fatal("holdout run did not require explicit confirmation")
	}
	if err := RequireHoldoutConfirmation(Holdout, true); err != nil {
		t.Fatalf("confirmed holdout run rejected: %v", err)
	}
	split.HoldoutStart = split.DevelopmentEnd
	if err := split.Validate(); err == nil {
		t.Fatal("overlapping dataset split accepted")
	}
}
