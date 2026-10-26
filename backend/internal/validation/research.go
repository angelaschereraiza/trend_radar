package validation

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/angelaschereraiza/trend_radar/backend/internal/backtest"
	"github.com/angelaschereraiza/trend_radar/backend/internal/metrics"
	"github.com/angelaschereraiza/trend_radar/backend/internal/stats"
)

type ResearchRegistry interface {
	TrialCount(ctx context.Context, strategyName, strategyVersion string) (int, error)
	SnapshotSurvivorshipBiased(ctx context.Context, snapshotID uuid.UUID) (bool, error)
	ValidateResearchRun(ctx context.Context, runID, snapshotID, universeID uuid.UUID, datasetRole string) error
	HistoricalUniverseMembership(ctx context.Context, universeID uuid.UUID, dates []time.Time) (map[string][]string, error)
	LogHoldoutAccess(ctx context.Context, strategyName, strategyVersion string, runID uuid.UUID, note string, confirmed bool) (bool, error)
	RecordTrialForRun(ctx context.Context, strategyName, strategyVersion, datasetRole string, parameters any, snapshotID, runID uuid.UUID, primaryMetric float64) error
}

type ResearchRequest struct {
	Context             context.Context
	Days                []backtest.MarketDay
	BacktestConfig      backtest.Config
	MetricsConfig       metrics.Config
	ReportConfig        ReportConfig
	HoldoutConfirmed    bool
	PointInTimeUniverse map[string][]string
	Registry            ResearchRegistry
	StrategyName        string
	StrategyVersion     string
	SnapshotID          uuid.UUID
	UniverseID          uuid.UUID
	RunID               uuid.UUID
}

type persistedTrialParameters struct {
	Backtest   backtest.Config       `json:"backtest"`
	Metrics    metrics.Config        `json:"metrics"`
	Bootstrap  stats.BootstrapConfig `json:"bootstrap"`
	UniverseID uuid.UUID             `json:"universe_id"`
}

type ResearchResult struct {
	Simulation                 backtest.Result
	MomentumBaseline           backtest.Result
	BuyAndHold                 backtest.Result
	UniverseExcludedCandidates UniverseExclusions
	Report                     PerformanceReport
}

func RunResearch(request ResearchRequest) (ResearchResult, error) {
	if err := RequireHoldoutConfirmation(request.ReportConfig.Role, request.HoldoutConfirmed); err != nil {
		return ResearchResult{}, err
	}
	ctx := request.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ResearchResult{}, fmt.Errorf("research request cancelled: %w", err)
	}
	var err error
	if request.ReportConfig.Role == Holdout && request.Registry == nil {
		return ResearchResult{}, errors.New("holdout research requires persistent access logging")
	}
	if len(request.Days) == 0 {
		return ResearchResult{}, errors.New("research run requires market days")
	}
	days := request.Days
	alreadyUsedHoldout := request.ReportConfig.HoldoutAlreadyUsed
	if request.Registry != nil {
		if request.StrategyName == "" || request.StrategyVersion == "" ||
			request.SnapshotID == uuid.Nil || request.UniverseID == uuid.Nil || request.RunID == uuid.Nil {
			return ResearchResult{}, errors.New("persisted research requires strategy identity, snapshot, universe, and run ids")
		}
		if err := request.Registry.ValidateResearchRun(
			ctx, request.RunID, request.SnapshotID, request.UniverseID, string(request.ReportConfig.Role),
		); err != nil {
			return ResearchResult{}, fmt.Errorf("validate persisted research run: %w", err)
		}
		dates := make([]time.Time, len(days))
		for i, day := range days {
			dates[i] = day.Date
		}
		request.PointInTimeUniverse, err = request.Registry.HistoricalUniverseMembership(
			ctx, request.UniverseID, dates,
		)
		if err != nil {
			return ResearchResult{}, fmt.Errorf("load point-in-time universe membership: %w", err)
		}
	}
	days, universeExclusions, err := ApplyPointInTimeUniverse(days, request.PointInTimeUniverse)
	if err != nil {
		return ResearchResult{}, fmt.Errorf("apply point-in-time universe: %w", err)
	}
	if request.Registry != nil {
		survivorshipBiased, err := request.Registry.SnapshotSurvivorshipBiased(ctx, request.SnapshotID)
		if err != nil {
			return ResearchResult{}, fmt.Errorf("load data snapshot bias flag: %w", err)
		}
		request.ReportConfig.SurvivorshipBiased = survivorshipBiased
		if request.ReportConfig.Role == Holdout {
			alreadyUsedHoldout, err = request.Registry.LogHoldoutAccess(
				ctx, request.StrategyName, request.StrategyVersion,
				request.RunID, "holdout research run", request.HoldoutConfirmed,
			)
			if err != nil {
				return ResearchResult{}, fmt.Errorf("persist holdout access: %w", err)
			}
		}
	}
	simulation, err := backtest.Run(days, request.BacktestConfig)
	if err != nil {
		return ResearchResult{}, err
	}
	momentum, err := backtest.RunMomentumBaseline(days, request.BacktestConfig)
	if err != nil {
		return ResearchResult{}, err
	}
	buyAndHold, err := backtest.RunBuyAndHoldBaseline(days, request.BacktestConfig)
	if err != nil {
		return ResearchResult{}, err
	}

	performance, err := metrics.Calculate(simulation, request.MetricsConfig)
	if err != nil {
		return ResearchResult{}, fmt.Errorf("calculate primary trial metric: %w", err)
	}
	reportConfig := request.ReportConfig
	reportConfig.Baselines = make(map[string]backtest.Result, len(request.ReportConfig.Baselines)+4)
	for name, baseline := range request.ReportConfig.Baselines {
		reportConfig.Baselines[name] = baseline
	}
	reportConfig.Baselines["momentum"] = momentum
	reportConfig.Baselines["buy_and_hold"] = buyAndHold
	spy, err := benchmarkBaseline(simulation.Equity)
	if err != nil {
		return ResearchResult{}, err
	}
	reportConfig.Baselines["spy"] = spy
	cash, err := cashBaseline(simulation.Equity, request.MetricsConfig.RiskFreeAnnualRate)
	if err != nil {
		return ResearchResult{}, err
	}
	reportConfig.Baselines["cash_chf"] = cash
	reportConfig.HoldoutAlreadyUsed = alreadyUsedHoldout
	if request.Registry != nil {
		parameters := persistedTrialParameters{
			Backtest: request.BacktestConfig, Metrics: request.MetricsConfig,
			Bootstrap: request.ReportConfig.Bootstrap, UniverseID: request.UniverseID,
		}
		if err := request.Registry.RecordTrialForRun(
			ctx, request.StrategyName, request.StrategyVersion,
			string(request.ReportConfig.Role), parameters,
			request.SnapshotID, request.RunID, performance.SharpeRatio,
		); err != nil {
			return ResearchResult{}, fmt.Errorf("persist experiment trial: %w", err)
		}
		trialCount, err := request.Registry.TrialCount(ctx, request.StrategyName, request.StrategyVersion)
		if err != nil {
			return ResearchResult{}, fmt.Errorf("load experiment trial count: %w", err)
		}
		reportConfig.TrialCount = trialCount
	}
	report, err := BuildPerformanceReport(simulation, request.MetricsConfig, reportConfig)
	if err != nil {
		return ResearchResult{}, err
	}
	return ResearchResult{
		Simulation: simulation, MomentumBaseline: momentum,
		BuyAndHold: buyAndHold, UniverseExcludedCandidates: universeExclusions,
		Report: report,
	}, nil
}

func benchmarkBaseline(equity []backtest.EquityPoint) (backtest.Result, error) {
	if len(equity) == 0 {
		return backtest.Result{}, errors.New("SPY baseline requires equity observations")
	}
	points := make([]backtest.EquityPoint, len(equity))
	for i, point := range equity {
		if !point.BenchmarkCHF.IsPositive() {
			return backtest.Result{}, fmt.Errorf("missing valid CHF benchmark value on %s", point.Date.Format("2006-01-02"))
		}
		points[i] = backtest.EquityPoint{
			Date: point.Date, PortfolioCHF: point.BenchmarkCHF,
			InvestedCHF: point.BenchmarkCHF, BenchmarkCHF: point.BenchmarkCHF,
		}
	}
	return backtest.Result{Equity: points}, nil
}

func cashBaseline(equity []backtest.EquityPoint, riskFreeAnnualRate float64) (backtest.Result, error) {
	points := make([]backtest.EquityPoint, len(equity))
	if len(equity) == 0 {
		return backtest.Result{Equity: points}, errors.New("cash baseline requires equity observations")
	}
	if math.IsNaN(riskFreeAnnualRate) || math.IsInf(riskFreeAnnualRate, 0) || riskFreeAnnualRate <= -1 {
		return backtest.Result{}, errors.New("annual risk-free rate must be finite and greater than -100%")
	}
	value := equity[0].PortfolioCHF
	points[0] = backtest.EquityPoint{
		Date: equity[0].Date, PortfolioCHF: value, CashCHF: value, BenchmarkCHF: value,
	}
	dailyRate := decimal.NewFromFloat(riskFreeAnnualRate / 252)
	for i := 1; i < len(equity); i++ {
		value = value.Mul(decimal.NewFromInt(1).Add(dailyRate))
		points[i] = backtest.EquityPoint{
			Date: equity[i].Date, PortfolioCHF: value, CashCHF: value, BenchmarkCHF: value,
		}
	}
	return backtest.Result{Equity: points}, nil
}
