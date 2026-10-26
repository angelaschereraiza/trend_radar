package validation

import (
	"errors"
	"fmt"
	"sort"

	"github.com/angelaschereraiza/trend_radar/backend/internal/backtest"
	"github.com/angelaschereraiza/trend_radar/backend/internal/metrics"
	"github.com/angelaschereraiza/trend_radar/backend/internal/stats"
)

type ReportConfig struct {
	Role               Role
	TrialCount         int
	SurvivorshipBiased bool
	HoldoutAlreadyUsed bool
	DevelopmentPeriod  bool
	Bootstrap          stats.BootstrapConfig
	Baselines          map[string]backtest.Result
}

type PerformanceReport struct {
	Metrics                    metrics.Report            `json:"metrics"`
	Baselines                  map[string]metrics.Report `json:"baselines"`
	ConfidenceIntervals        stats.ConfidenceIntervals `json:"confidence_intervals"`
	DeflatedSharpeRatio        float64                   `json:"deflated_sharpe_ratio"`
	TrialCount                 int                       `json:"trial_count"`
	DatasetRole                Role                      `json:"dataset_role"`
	SurvivorshipBiased         bool                      `json:"survivorship_biased"`
	HoldoutAlreadyUsed         bool                      `json:"holdout_already_used"`
	DevelopmentLacksBearMarket bool                      `json:"development_lacks_bear_market"`
	Promising                  bool                      `json:"promising"`
	Warnings                   []string                  `json:"warnings"`
}

func DefaultBootstrapConfig() stats.BootstrapConfig {
	return stats.BootstrapConfig{
		Replicates: 2000, BlockLength: 20, Seed: 42, ConfidenceLevel: 0.95,
	}
}

func BuildPerformanceReport(result backtest.Result, metricConfig metrics.Config, config ReportConfig) (PerformanceReport, error) {
	if config.Role != Development && config.Role != WalkForward && config.Role != Holdout && config.Role != Forward {
		return PerformanceReport{}, fmt.Errorf("unsupported dataset role %q", config.Role)
	}
	if config.TrialCount < 1 {
		return PerformanceReport{}, errors.New("trial count must be at least one")
	}
	if len(result.Equity) < 3 {
		return PerformanceReport{}, errors.New("at least three equity observations are required for statistical reporting")
	}

	strategyReturns, benchmarkReturns, err := alignedReturns(result.Equity)
	if err != nil {
		return PerformanceReport{}, err
	}
	if len(strategyReturns) < 2 {
		return PerformanceReport{}, errors.New("at least two aligned daily returns are required for statistical reporting")
	}
	intervalConfig := config.Bootstrap
	intervalConfig.RiskFreeAnnual = metricConfig.RiskFreeAnnualRate
	intervals, err := stats.StationaryBootstrap(strategyReturns, benchmarkReturns, intervalConfig)
	if err != nil {
		return PerformanceReport{}, fmt.Errorf("calculate confidence intervals: %w", err)
	}
	deflated, err := stats.DeflatedSharpeRatio(strategyReturns, config.TrialCount)
	if err != nil {
		return PerformanceReport{}, fmt.Errorf("calculate deflated Sharpe ratio: %w", err)
	}
	performance, err := metrics.Calculate(result, metricConfig)
	if err != nil {
		return PerformanceReport{}, fmt.Errorf("calculate performance metrics: %w", err)
	}
	baselines := make(map[string]metrics.Report, len(config.Baselines))
	baselineNames := make([]string, 0, len(config.Baselines))
	for name := range config.Baselines {
		baselineNames = append(baselineNames, name)
	}
	sort.Strings(baselineNames)
	for _, name := range baselineNames {
		if name == "" {
			return PerformanceReport{}, errors.New("baseline names must not be empty")
		}
		baseline, err := metrics.Calculate(config.Baselines[name], metricConfig)
		if err != nil {
			return PerformanceReport{}, fmt.Errorf("calculate %s baseline metrics: %w", name, err)
		}
		baselines[name] = baseline
	}

	report := PerformanceReport{
		Metrics: performance, Baselines: baselines, ConfidenceIntervals: intervals,
		DeflatedSharpeRatio: deflated, TrialCount: config.TrialCount,
		DatasetRole: config.Role, SurvivorshipBiased: config.SurvivorshipBiased,
		HoldoutAlreadyUsed: config.HoldoutAlreadyUsed,
		Promising:          config.Role != Development && intervals.ExcessReturn.Lower > 0,
		Warnings:           make([]string, 0),
	}
	if config.SurvivorshipBiased {
		report.Warnings = append(report.Warnings, "survivorship-biased market data; historical delisted securities may be missing")
	}
	if config.HoldoutAlreadyUsed {
		report.Warnings = append(report.Warnings, "the holdout has already been accessed for this strategy version")
	}
	if config.Role == Development {
		report.Warnings = append(report.Warnings, "development results are exploratory and are not labelled promising")
	} else if !report.Promising {
		if intervals.ExcessReturn.Upper >= 0 {
			report.Warnings = append(report.Warnings, "bootstrap confidence interval for excess return includes zero; results are not labelled promising")
		} else {
			report.Warnings = append(report.Warnings, "bootstrap confidence interval for excess return is not strictly positive; results are not labelled promising")
		}
	}
	if config.DevelopmentPeriod && !hasBearMarket(result.Equity) {
		report.DevelopmentLacksBearMarket = true
		report.Warnings = append(report.Warnings, "development-period benchmark drawdown did not reach 20%; results may reflect a single bull-market regime")
	}
	return report, nil
}

func alignedReturns(equity []backtest.EquityPoint) ([]float64, []float64, error) {
	strategyReturns := make([]float64, 0, len(equity)-1)
	benchmarkReturns := make([]float64, 0, len(equity)-1)
	for i := 1; i < len(equity); i++ {
		previous, current := equity[i-1], equity[i]
		if !current.Date.After(previous.Date) {
			return nil, nil, fmt.Errorf("equity dates must be strictly ascending at index %d", i)
		}
		if !previous.PortfolioCHF.IsPositive() || !previous.BenchmarkCHF.IsPositive() {
			return nil, nil, fmt.Errorf("portfolio and benchmark values must be positive before observation %d", i)
		}
		strategyReturns = append(strategyReturns, current.PortfolioCHF.Div(previous.PortfolioCHF).InexactFloat64()-1)
		benchmarkReturns = append(benchmarkReturns, current.BenchmarkCHF.Div(previous.BenchmarkCHF).InexactFloat64()-1)
	}
	return strategyReturns, benchmarkReturns, nil
}

func hasBearMarket(equity []backtest.EquityPoint) bool {
	if len(equity) == 0 || !equity[0].BenchmarkCHF.IsPositive() {
		return false
	}
	peak := equity[0].BenchmarkCHF
	for _, point := range equity[1:] {
		if point.BenchmarkCHF.GreaterThan(peak) {
			peak = point.BenchmarkCHF
		}
		if peak.IsPositive() && peak.Sub(point.BenchmarkCHF).Div(peak).InexactFloat64() >= 0.20 {
			return true
		}
	}
	return false
}
