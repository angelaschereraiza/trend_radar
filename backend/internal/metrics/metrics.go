package metrics

import (
	"errors"
	"math"

	"github.com/shopspring/decimal"

	"github.com/angelaschereraiza/trend_radar/backend/internal/backtest"
)

type Config struct {
	RiskFreeAnnualRate float64
}

type Report struct {
	TotalReturn          float64         `json:"total_return"`
	CAGR                 float64         `json:"cagr"`
	AnnualizedVolatility float64         `json:"annualized_volatility"`
	SharpeRatio          float64         `json:"sharpe_ratio"`
	SortinoRatio         float64         `json:"sortino_ratio"`
	MaximumDrawdown      float64         `json:"maximum_drawdown"`
	CalmarRatio          float64         `json:"calmar_ratio"`
	WinRate              float64         `json:"win_rate"`
	AverageTradeReturn   float64         `json:"average_trade_return"`
	Turnover             float64         `json:"turnover"`
	TotalFeesCHF         decimal.Decimal `json:"total_fees_chf"`
	WithholdingTaxCHF    decimal.Decimal `json:"withholding_tax_chf"`
	BenchmarkReturn      float64         `json:"benchmark_return"`
	ExcessReturn         float64         `json:"excess_return"`
	AverageExposure      float64         `json:"average_exposure"`
	FeeDragAnnualized    float64         `json:"fee_drag_annualized"`
	BreakEvenGrossReturn float64         `json:"break_even_gross_return"`
}

func Calculate(result backtest.Result, config Config) (Report, error) {
	if len(result.Equity) == 0 {
		return Report{}, errors.New("equity series is empty")
	}
	if math.IsNaN(config.RiskFreeAnnualRate) || math.IsInf(config.RiskFreeAnnualRate, 0) {
		return Report{}, errors.New("risk-free rate must be finite")
	}
	first, last := result.Equity[0], result.Equity[len(result.Equity)-1]
	report := Report{
		TotalFeesCHF:      decimal.Zero,
		WithholdingTaxCHF: decimal.Zero,
	}
	if first.PortfolioCHF.IsPositive() {
		report.TotalReturn = last.PortfolioCHF.Div(first.PortfolioCHF).Sub(decimal.NewFromInt(1)).InexactFloat64()
	}
	if first.BenchmarkCHF.IsPositive() {
		report.BenchmarkReturn = last.BenchmarkCHF.Div(first.BenchmarkCHF).Sub(decimal.NewFromInt(1)).InexactFloat64()
	}
	report.ExcessReturn = report.TotalReturn - report.BenchmarkReturn
	returns := make([]float64, 0, len(result.Equity)-1)
	for i := 1; i < len(result.Equity); i++ {
		previous, current := result.Equity[i-1].PortfolioCHF, result.Equity[i].PortfolioCHF
		if !previous.IsPositive() {
			continue
		}
		returns = append(returns, current.Div(previous).Sub(decimal.NewFromInt(1)).InexactFloat64())
	}
	if len(returns) > 0 {
		report.CAGR = cagr(first.PortfolioCHF, last.PortfolioCHF, len(returns))
		report.AnnualizedVolatility = sampleStdDev(returns) * math.Sqrt(252)
		excessReturns := make([]float64, len(returns))
		downsideSquares := 0.0
		for i, dailyReturn := range returns {
			excessReturns[i] = dailyReturn - config.RiskFreeAnnualRate/252
			if excessReturns[i] < 0 {
				downsideSquares += excessReturns[i] * excessReturns[i]
			}
		}
		excessStdDev := sampleStdDev(excessReturns)
		if excessStdDev > 0 {
			report.SharpeRatio = mean(excessReturns) / excessStdDev * math.Sqrt(252)
		}
		downsideDeviation := math.Sqrt(downsideSquares / float64(len(returns)))
		if downsideDeviation > 0 {
			report.SortinoRatio = mean(excessReturns) / downsideDeviation * math.Sqrt(252)
		}
	}
	peak := first.PortfolioCHF
	for _, point := range result.Equity {
		if point.PortfolioCHF.GreaterThan(peak) {
			peak = point.PortfolioCHF
		}
		if peak.IsPositive() {
			drawdown := peak.Sub(point.PortfolioCHF).Div(peak).InexactFloat64()
			report.MaximumDrawdown = math.Max(report.MaximumDrawdown, drawdown)
		}
		if point.PortfolioCHF.IsPositive() {
			report.AverageExposure += point.InvestedCHF.Div(point.PortfolioCHF).InexactFloat64()
		}
	}
	report.AverageExposure /= float64(len(result.Equity))
	if report.MaximumDrawdown > 0 {
		report.CalmarRatio = report.CAGR / report.MaximumDrawdown
	}
	closedReturns := tradeReturns(result.Fills)
	if len(closedReturns) > 0 {
		wins := 0
		for _, value := range closedReturns {
			report.AverageTradeReturn += value
			if value > 0 {
				wins++
			}
		}
		report.WinRate = float64(wins) / float64(len(closedReturns))
		report.AverageTradeReturn /= float64(len(closedReturns))
	}
	var totalTurnover decimal.Decimal
	for _, fill := range result.Fills {
		totalTurnover = totalTurnover.Add(fill.GrossCHF)
		report.TotalFeesCHF = report.TotalFeesCHF.Add(fill.Costs.Total)
	}
	averageValue := decimal.Zero
	for _, point := range result.Equity {
		averageValue = averageValue.Add(point.PortfolioCHF)
	}
	averageValue = averageValue.Div(decimal.NewFromInt(int64(len(result.Equity))))
	if averageValue.IsPositive() {
		report.Turnover = totalTurnover.Div(averageValue).InexactFloat64()
		years := float64(len(returns)) / 252
		if years > 0 {
			report.FeeDragAnnualized = report.TotalFeesCHF.Div(averageValue).InexactFloat64() / years
		}
	}
	if averageValue.IsPositive() {
		report.BreakEvenGrossReturn = report.TotalFeesCHF.Div(averageValue).InexactFloat64()
	}
	return report, nil
}

func cagr(first, last decimal.Decimal, sessions int) float64 {
	if sessions <= 0 || !first.IsPositive() || !last.IsPositive() {
		return 0
	}
	ratio := last.Div(first).InexactFloat64()
	return math.Pow(ratio, 252/float64(sessions)) - 1
}

func sampleStdDev(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	average := mean(values)
	var sum float64
	for _, value := range values {
		delta := value - average
		sum += delta * delta
	}
	return math.Sqrt(sum / float64(len(values)-1))
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var total float64
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func tradeReturns(fills []backtest.Fill) []float64 {
	openTrades := make(map[string]backtest.Fill)
	returns := make([]float64, 0)
	for _, fill := range fills {
		switch fill.Side {
		case "buy":
			openTrades[fill.Symbol] = fill
		case "sell":
			entry, ok := openTrades[fill.Symbol]
			if !ok || !entry.GrossCHF.IsPositive() {
				continue
			}
			netProceeds := fill.GrossCHF.Sub(fill.Costs.Total)
			costBasis := entry.GrossCHF.Add(entry.Costs.Total)
			returns = append(returns, netProceeds.Div(costBasis).Sub(decimal.NewFromInt(1)).InexactFloat64())
			delete(openTrades, fill.Symbol)
		}
	}
	return returns
}
