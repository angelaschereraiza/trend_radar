package metrics

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/angelaschereraiza/trend_radar/backend/internal/backtest"
)

func TestCalculateReturnsAndDrawdown(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	result := backtest.Result{Equity: []backtest.EquityPoint{
		{Date: base, PortfolioCHF: decimal.NewFromInt(100), BenchmarkCHF: decimal.NewFromInt(100)},
		{Date: base.AddDate(0, 0, 1), PortfolioCHF: decimal.NewFromInt(120), BenchmarkCHF: decimal.NewFromInt(110)},
		{Date: base.AddDate(0, 0, 2), PortfolioCHF: decimal.NewFromInt(90), BenchmarkCHF: decimal.NewFromInt(105)},
	}}
	report, err := Calculate(result, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalReturn != -0.1 || report.BenchmarkReturn != 0.05 || report.MaximumDrawdown != 0.25 {
		t.Fatalf("unexpected performance report: %+v", report)
	}
}

func TestCalculateRejectsEmptyEquity(t *testing.T) {
	if _, err := Calculate(backtest.Result{}, Config{}); err == nil {
		t.Fatal("Calculate accepted empty equity series")
	}
}
