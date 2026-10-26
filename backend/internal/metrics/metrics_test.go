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

func TestCalculateReportsFeeBreakdownAndWithholding(t *testing.T) {
	result := backtest.Result{
		Equity: []backtest.EquityPoint{
			{PortfolioCHF: decimal.NewFromInt(100), BenchmarkCHF: decimal.NewFromInt(100)},
			{PortfolioCHF: decimal.NewFromInt(101), BenchmarkCHF: decimal.NewFromInt(100)},
		},
		Fills: []backtest.Fill{{
			GrossCHF: decimal.NewFromInt(10),
			Costs: backtest.CostBreakdown{
				Commission: decimal.NewFromInt(1), FXCost: decimal.NewFromInt(2),
				StampDuty: decimal.NewFromInt(3), Slippage: decimal.NewFromInt(4),
				Total: decimal.NewFromInt(10),
			},
		}},
		WithholdingTaxCHF: decimal.NewFromInt(5),
	}
	report, err := Calculate(result, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if !report.CommissionCHF.Equal(decimal.NewFromInt(1)) ||
		!report.FXCostCHF.Equal(decimal.NewFromInt(2)) ||
		!report.StampDutyCHF.Equal(decimal.NewFromInt(3)) ||
		!report.SlippageCHF.Equal(decimal.NewFromInt(4)) ||
		!report.WithholdingTaxCHF.Equal(decimal.NewFromInt(5)) {
		t.Fatalf("fee and tax breakdown = %+v", report)
	}
}

func TestCalculateCountsTotalLossOnDelisting(t *testing.T) {
	result := backtest.Result{
		Equity: []backtest.EquityPoint{
			{PortfolioCHF: decimal.NewFromInt(100), BenchmarkCHF: decimal.NewFromInt(100)},
			{PortfolioCHF: decimal.Zero, BenchmarkCHF: decimal.NewFromInt(100)},
		},
		CorporateActions: []backtest.CorporateActionRecord{{
			Type: "delisting", CostBasisCHF: decimal.NewFromInt(100),
		}},
	}
	report, err := Calculate(result, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if report.AverageTradeReturn != -1 || report.WinRate != 0 {
		t.Fatalf("delisting loss not counted as a closed trade: %+v", report)
	}
}
