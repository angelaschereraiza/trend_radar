package validation

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/angelaschereraiza/trend_radar/backend/internal/backtest"
)

func TestEvaluateWalkForwardFitsAndEvaluatesDisjointPeriods(t *testing.T) {
	start := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	days := make([]backtest.MarketDay, 8)
	for i := range days {
		days[i].Date = start.AddDate(0, 0, i)
	}
	windows := []Window{
		{
			TrainStart: days[0].Date, TrainEnd: days[1].Date,
			TestStart: days[2].Date, TestEnd: days[3].Date,
		},
		{
			TrainStart: days[0].Date, TrainEnd: days[3].Date,
			TestStart: days[4].Date, TestEnd: days[5].Date,
		},
	}
	fitCalls, evaluateCalls := 0, 0
	result, err := EvaluateWalkForward(days, windows,
		func(training []backtest.MarketDay) (time.Time, error) {
			fitCalls++
			for _, day := range training {
				if day.Date.After(windows[fitCalls-1].TrainEnd) {
					t.Fatalf("training callback received future observation %s", day.Date)
				}
			}
			return training[len(training)-1].Date, nil
		},
		func(testDays []backtest.MarketDay, fittedThrough time.Time) (backtest.Result, error) {
			evaluateCalls++
			for _, day := range testDays {
				if !day.Date.After(fittedThrough) || day.Date.Before(windows[evaluateCalls-1].TestStart) ||
					day.Date.After(windows[evaluateCalls-1].TestEnd) {
					t.Fatalf("test callback received observation outside test period: %s", day.Date)
				}
			}
			base := decimal.NewFromInt(int64(100 * evaluateCalls))
			return backtest.Result{Equity: []backtest.EquityPoint{
				{Date: testDays[0].Date, PortfolioCHF: base, CashCHF: base, BenchmarkCHF: base},
				{Date: testDays[1].Date, PortfolioCHF: base.Mul(decimal.RequireFromString("1.1")),
					CashCHF: base.Mul(decimal.RequireFromString("1.1")), BenchmarkCHF: base.Mul(decimal.RequireFromString("1.05"))},
			}}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if fitCalls != 2 || evaluateCalls != 2 || len(result.Folds) != 2 || len(result.Equity) != 4 {
		t.Fatalf("unexpected walk-forward output: fits=%d evaluations=%d folds=%d equity=%d",
			fitCalls, evaluateCalls, len(result.Folds), len(result.Equity))
	}
	if !result.Equity[3].PortfolioCHF.Equal(decimal.NewFromInt(121)) ||
		!result.Equity[3].BenchmarkCHF.Equal(decimal.RequireFromString("110.25")) {
		t.Fatalf("out-of-sample curve did not chain returns across folds: %+v", result.Equity)
	}
}

func TestEvaluateWalkForwardRejectsTestLeakage(t *testing.T) {
	start := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	days := []backtest.MarketDay{
		{Date: start}, {Date: start.AddDate(0, 0, 1)}, {Date: start.AddDate(0, 0, 2)},
	}
	window := Window{
		TrainStart: days[0].Date, TrainEnd: days[0].Date,
		TestStart: days[1].Date, TestEnd: days[2].Date,
	}
	_, err := EvaluateWalkForward(days, []Window{window},
		func([]backtest.MarketDay) (struct{}, error) { return struct{}{}, nil },
		func(testDays []backtest.MarketDay, _ struct{}) (backtest.Result, error) {
			return backtest.Result{Equity: []backtest.EquityPoint{
				{Date: testDays[0].Date.AddDate(0, 0, -1), PortfolioCHF: decimal.NewFromInt(100), BenchmarkCHF: decimal.NewFromInt(100)},
				{Date: testDays[1].Date, PortfolioCHF: decimal.NewFromInt(101), BenchmarkCHF: decimal.NewFromInt(101)},
			}}, nil
		})
	if err == nil {
		t.Fatal("walk-forward accepted equity from before its test window")
	}
}
