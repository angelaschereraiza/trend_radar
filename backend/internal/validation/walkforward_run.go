package validation

import (
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/angelaschereraiza/trend_radar/backend/internal/backtest"
	"github.com/angelaschereraiza/trend_radar/backend/internal/strategy"
)

type WalkForwardFold struct {
	Window Window
	Result backtest.Result
}

type WalkForwardResult struct {
	Folds  []WalkForwardFold
	Equity []backtest.EquityPoint
}

type WalkForwardFit[T any] func(trainingDays []backtest.MarketDay) (T, error)
type WalkForwardEvaluate[T any] func(testDays []backtest.MarketDay, parameters T) (backtest.Result, error)

func EvaluateWalkForward[T any](
	days []backtest.MarketDay,
	windows []Window,
	fit WalkForwardFit[T],
	evaluate WalkForwardEvaluate[T],
) (WalkForwardResult, error) {
	if len(days) == 0 || len(windows) == 0 || fit == nil || evaluate == nil {
		return WalkForwardResult{}, errors.New("market days, walk-forward windows, and callbacks are required")
	}
	for i, day := range days {
		if day.Date.IsZero() || (i > 0 && !day.Date.After(days[i-1].Date)) {
			return WalkForwardResult{}, fmt.Errorf("market days must be strictly ascending at index %d", i)
		}
	}
	if err := validateWindows(windows); err != nil {
		return WalkForwardResult{}, err
	}

	result := WalkForwardResult{Folds: make([]WalkForwardFold, 0, len(windows))}
	for i, window := range windows {
		trainingDays := selectMarketDays(days, window.TrainStart, window.TrainEnd)
		testDays := selectMarketDays(days, window.TestStart, window.TestEnd)
		if len(trainingDays) == 0 || len(testDays) < 2 {
			return WalkForwardResult{}, fmt.Errorf("walk-forward window %d has insufficient training or test observations", i)
		}
		parameters, err := fit(trainingDays)
		if err != nil {
			return WalkForwardResult{}, fmt.Errorf("fit walk-forward window %d: %w", i, err)
		}
		foldResult, err := evaluate(testDays, parameters)
		if err != nil {
			return WalkForwardResult{}, fmt.Errorf("evaluate walk-forward window %d: %w", i, err)
		}
		if err := validateFoldEquity(foldResult.Equity, window); err != nil {
			return WalkForwardResult{}, fmt.Errorf("validate walk-forward window %d: %w", i, err)
		}
		result.Folds = append(result.Folds, WalkForwardFold{Window: window, Result: foldResult})
	}
	equity, err := concatenateWalkForwardEquity(result.Folds)
	if err != nil {
		return WalkForwardResult{}, err
	}
	result.Equity = equity
	return result, nil
}

func validateWindows(windows []Window) error {
	for i, window := range windows {
		if window.TrainStart.IsZero() || window.TrainEnd.IsZero() ||
			window.TestStart.IsZero() || window.TestEnd.IsZero() ||
			window.TrainEnd.Before(window.TrainStart) || window.TestEnd.Before(window.TestStart) {
			return fmt.Errorf("walk-forward window %d has invalid boundaries", i)
		}
		if !window.TestStart.After(window.TrainEnd) {
			return fmt.Errorf("walk-forward window %d overlaps training and test periods", i)
		}
		if i > 0 {
			previous := windows[i-1]
			if window.TrainStart.Before(previous.TrainStart) ||
				!window.TrainEnd.After(previous.TrainEnd) ||
				!window.TestStart.After(previous.TestStart) ||
				!window.TestStart.After(previous.TestEnd) {
				return fmt.Errorf("walk-forward window %d is unordered or overlaps the previous test period", i)
			}
		}
	}
	return nil
}

func selectMarketDays(days []backtest.MarketDay, start, end time.Time) []backtest.MarketDay {
	selected := make([]backtest.MarketDay, 0)
	for _, day := range days {
		if day.Date.Before(start) || day.Date.After(end) {
			continue
		}
		selected = append(selected, cloneMarketDay(day))
	}
	return selected
}

func cloneMarketDay(day backtest.MarketDay) backtest.MarketDay {
	cloned := day
	cloned.Rankings = append([]strategy.Candidate(nil), day.Rankings...)
	cloned.CorporateActions = append([]backtest.CorporateAction(nil), day.CorporateActions...)
	cloned.OpenPricesUSD = clonePrices(day.OpenPricesUSD)
	cloned.ClosePricesUSD = clonePrices(day.ClosePricesUSD)
	return cloned
}

func clonePrices(prices map[string]decimal.Decimal) map[string]decimal.Decimal {
	cloned := make(map[string]decimal.Decimal, len(prices))
	for symbol, price := range prices {
		cloned[symbol] = price
	}
	return cloned
}

func validateFoldEquity(equity []backtest.EquityPoint, window Window) error {
	if len(equity) < 2 {
		return errors.New("test result must contain at least two equity observations")
	}
	for i, point := range equity {
		if point.Date.Before(window.TestStart) || point.Date.After(window.TestEnd) {
			return fmt.Errorf("equity date %s is outside the test period", point.Date.Format("2006-01-02"))
		}
		if i > 0 && !point.Date.After(equity[i-1].Date) {
			return errors.New("test equity dates must be strictly ascending")
		}
		if !point.PortfolioCHF.IsPositive() || !point.BenchmarkCHF.IsPositive() {
			return errors.New("test portfolio and benchmark equity must remain positive")
		}
	}
	return nil
}

func concatenateWalkForwardEquity(folds []WalkForwardFold) ([]backtest.EquityPoint, error) {
	equity := make([]backtest.EquityPoint, 0)
	for foldIndex, fold := range folds {
		segment := fold.Result.Equity
		if len(equity) == 0 {
			equity = append(equity, segment...)
			continue
		}
		if !segment[0].Date.After(equity[len(equity)-1].Date) {
			return nil, errors.New("walk-forward test equity segments must be strictly chronological")
		}
		currentCurve := equity[len(equity)-1]
		firstSegmentPoint := segment[0]
		if !firstSegmentPoint.PortfolioCHF.IsPositive() || !firstSegmentPoint.BenchmarkCHF.IsPositive() {
			return nil, fmt.Errorf("walk-forward fold %d has non-positive starting equity", foldIndex)
		}
		cashShare := firstSegmentPoint.CashCHF.Div(firstSegmentPoint.PortfolioCHF)
		investedShare := firstSegmentPoint.InvestedCHF.Div(firstSegmentPoint.PortfolioCHF)
		equity = append(equity, backtest.EquityPoint{
			Date: firstSegmentPoint.Date, PortfolioCHF: currentCurve.PortfolioCHF,
			CashCHF:      currentCurve.PortfolioCHF.Mul(cashShare),
			InvestedCHF:  currentCurve.PortfolioCHF.Mul(investedShare),
			BenchmarkCHF: currentCurve.BenchmarkCHF,
		})
		previous := segment[0]
		for _, point := range segment[1:] {
			if !point.Date.After(equity[len(equity)-1].Date) {
				return nil, fmt.Errorf("walk-forward fold %d overlaps the concatenated out-of-sample curve", foldIndex)
			}
			portfolioFactor := point.PortfolioCHF.Div(previous.PortfolioCHF)
			benchmarkFactor := point.BenchmarkCHF.Div(previous.BenchmarkCHF)
			linkedPortfolio := equity[len(equity)-1].PortfolioCHF.Mul(portfolioFactor)
			linkedBenchmark := equity[len(equity)-1].BenchmarkCHF.Mul(benchmarkFactor)
			cashShare := point.CashCHF.Div(point.PortfolioCHF)
			investedShare := point.InvestedCHF.Div(point.PortfolioCHF)
			equity = append(equity, backtest.EquityPoint{
				Date: point.Date, PortfolioCHF: linkedPortfolio,
				CashCHF:      linkedPortfolio.Mul(cashShare),
				InvestedCHF:  linkedPortfolio.Mul(investedShare),
				BenchmarkCHF: linkedBenchmark,
			})
			previous = point
		}
	}
	return equity, nil
}
