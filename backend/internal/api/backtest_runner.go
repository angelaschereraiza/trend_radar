package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/angelaschereraiza/trend_radar/backend/internal/backtest"
	"github.com/angelaschereraiza/trend_radar/backend/internal/calendar"
	"github.com/angelaschereraiza/trend_radar/backend/internal/metrics"
	"github.com/angelaschereraiza/trend_radar/backend/internal/repository"
	"github.com/angelaschereraiza/trend_radar/backend/internal/validation"
)

type persistedBacktestConfig struct {
	Strategy        StrategyConfig `json:"strategy"`
	StrategyName    string         `json:"strategy_name"`
	StrategyVersion string         `json:"strategy_version"`
	StartDate       string         `json:"start_date"`
	EndDate         string         `json:"end_date"`
}

func RecoverInterruptedBacktests(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		UPDATE backtest_runs
		SET status = 'failed', finished_at = now(),
		    error_message = 'backtest was interrupted by an API restart; resubmit the run'
		WHERE status IN ('queued', 'running')`)
	if err != nil {
		return fmt.Errorf("recover interrupted backtests: %w", err)
	}
	return nil
}

func (h *HealthHandler) executeBacktest(ctx context.Context, runID uuid.UUID, request BacktestRequest) {
	if err := h.setBacktestStatus(ctx, runID, "running", ""); err != nil {
		slog.ErrorContext(ctx, "mark backtest running failed", "run_id", runID, "error", err)
		return
	}
	runErr := h.runBacktest(ctx, runID, request)
	if runErr != nil {
		slog.ErrorContext(ctx, "backtest execution failed", "run_id", runID, "error", runErr)
		if err := h.setBacktestStatus(context.WithoutCancel(ctx), runID, "failed", "backtest failed; consult server logs for details"); err != nil {
			slog.ErrorContext(ctx, "persist backtest failure failed", "run_id", runID, "error", err)
		}
	}
}

func (h *HealthHandler) runBacktest(ctx context.Context, runID uuid.UUID, request BacktestRequest) error {
	var configJSON []byte
	var snapshotID, universeID uuid.UUID
	err := h.db.QueryRowContext(ctx, `
		SELECT configuration_json, data_snapshot_id, universe_id
		FROM backtest_runs WHERE id = $1`, runID).
		Scan(&configJSON, &snapshotID, &universeID)
	if err != nil {
		return fmt.Errorf("load backtest configuration: %w", err)
	}
	var stored persistedBacktestConfig
	if err := json.Unmarshal(configJSON, &stored); err != nil {
		return fmt.Errorf("decode backtest configuration: %w", err)
	}
	strategyConfig, err := strategyConfig(stored.Strategy)
	if err != nil {
		return fmt.Errorf("validate stored strategy configuration: %w", err)
	}
	costs, err := backtestCosts(stored.Strategy)
	if err != nil {
		return fmt.Errorf("validate stored backtest costs: %w", err)
	}
	backtestConfig := backtest.DefaultConfig(strategyConfig)
	backtestConfig.Costs = costs
	days, err := h.data.LoadBacktestDays(ctx, request.StartDate.Time, request.EndDate.Time)
	if err != nil {
		return err
	}
	report, err := validation.RunResearch(validation.ResearchRequest{
		Context:        ctx,
		Days:           days,
		BacktestConfig: backtestConfig,
		MetricsConfig:  metrics.Config{RiskFreeAnnualRate: 0},
		ReportConfig: validation.ReportConfig{
			Role:              validation.Role(request.DatasetRole),
			DevelopmentPeriod: request.DatasetRole == Development,
			Bootstrap:         validation.DefaultBootstrapConfig(),
		},
		HoldoutConfirmed: request.ConfirmHoldout != nil && *request.ConfirmHoldout,
		Registry:         repository.NewValidation(h.db),
		StrategyName:     stored.StrategyName,
		StrategyVersion:  stored.StrategyVersion,
		SnapshotID:       snapshotID,
		UniverseID:       universeID,
		RunID:            runID,
	})
	if err != nil {
		return fmt.Errorf("run research workflow: %w", err)
	}
	return h.persistBacktest(ctx, runID, report)
}

func (h *HealthHandler) persistBacktest(ctx context.Context, runID uuid.UUID, result validation.ResearchResult) error {
	metricsReport, err := metrics.Calculate(result.Simulation, metrics.Config{})
	if err != nil {
		return fmt.Errorf("calculate backtest metrics: %w", err)
	}
	summary, err := json.Marshal(map[string]any{
		"metrics":            metricsReport,
		"statistical_report": result.Report,
		"configuration":      map[string]string{"code_version": "working-tree"},
	})
	if err != nil {
		return fmt.Errorf("encode backtest summary: %w", err)
	}
	warnings, err := json.Marshal(result.Report.Warnings)
	if err != nil {
		return fmt.Errorf("encode backtest warnings: %w", err)
	}
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin backtest result persistence: %w", err)
	}
	defer tx.Rollback()
	stockIDs := make(map[string]uuid.UUID)
	for _, order := range result.Simulation.Orders {
		stockID, ok := stockIDs[order.Symbol]
		if !ok {
			if err := tx.QueryRowContext(ctx, "SELECT id FROM stocks WHERE symbol = $1", order.Symbol).Scan(&stockID); err != nil {
				return fmt.Errorf("find backtest stock %s: %w", order.Symbol, err)
			}
			stockIDs[order.Symbol] = stockID
		}
		decisionDate, err := calendar.PreviousTradingDay(calendar.FromTime(order.Date))
		if err != nil {
			return fmt.Errorf("calculate decision date for order on %s: %w", order.Date.Format("2006-01-02"), err)
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO backtest_orders
				(id, run_id, stock_id, decision_date, intended_execution_date, side,
				 requested_quantity, reason, status, reject_reason)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			uuid.New(), runID, stockID, decisionDate.Time(), order.Date,
			order.Side, order.Quantity.String(), order.Reason, order.Status, order.RejectReason)
		if err != nil {
			return fmt.Errorf("persist backtest order for %s: %w", order.Symbol, err)
		}
	}
	for _, fill := range result.Simulation.Fills {
		stockID, ok := stockIDs[fill.Symbol]
		if !ok {
			if err := tx.QueryRowContext(ctx, "SELECT id FROM stocks WHERE symbol = $1", fill.Symbol).Scan(&stockID); err != nil {
				return fmt.Errorf("find fill stock %s: %w", fill.Symbol, err)
			}
			stockIDs[fill.Symbol] = stockID
		}
		var orderID uuid.UUID
		err := tx.QueryRowContext(ctx, `
			SELECT id FROM backtest_orders
			WHERE run_id = $1 AND stock_id = $2 AND intended_execution_date = $3
			  AND side = $4 AND status = 'filled'
			ORDER BY id LIMIT 1`, runID, stockID, fill.Date, fill.Side).Scan(&orderID)
		if err != nil {
			return fmt.Errorf("find persisted fill order for %s: %w", fill.Symbol, err)
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO backtest_fills
				(id, order_id, execution_date, quantity, price_usd, fx_rate_usd_chf,
				 gross_value_chf, commission_chf, fx_cost_chf, stamp_duty_chf, slippage_chf)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			uuid.New(), orderID, fill.Date, fill.Quantity.String(), fill.PriceUSD.String(),
			fill.FXRate.String(), fill.GrossCHF.StringFixed(4), fill.Costs.Commission.StringFixed(4),
			fill.Costs.FXCost.StringFixed(4), fill.Costs.StampDuty.StringFixed(4),
			fill.Costs.Slippage.StringFixed(4))
		if err != nil {
			return fmt.Errorf("persist backtest fill for %s: %w", fill.Symbol, err)
		}
	}
	for _, entry := range result.Simulation.Ledger {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO backtest_cash_ledger (id, run_id, booking_date, entry_type, amount_chf, balance_after_chf)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			uuid.New(), runID, entry.Date, entry.Type, entry.AmountCHF.StringFixed(4), entry.BalanceCHF.StringFixed(4))
		if err != nil {
			return fmt.Errorf("persist backtest cash ledger: %w", err)
		}
	}
	for _, point := range result.Simulation.Equity {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO backtest_equity
				(run_id, trading_date, portfolio_value_chf, cash_balance_chf, invested_value_chf, benchmark_value_chf)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			runID, point.Date, point.PortfolioCHF.StringFixed(4), point.CashCHF.StringFixed(4),
			point.InvestedCHF.StringFixed(4), point.BenchmarkCHF.StringFixed(4))
		if err != nil {
			return fmt.Errorf("persist backtest equity: %w", err)
		}
	}
	update, err := tx.ExecContext(ctx, `
		UPDATE backtest_runs
		SET status = 'completed', finished_at = now(), summary_json = $2::jsonb, warnings_json = $3::jsonb
		WHERE id = $1 AND status = 'running'`, runID, summary, warnings)
	if err != nil {
		return fmt.Errorf("persist backtest summary: %w", err)
	}
	affected, err := update.RowsAffected()
	if err != nil {
		return fmt.Errorf("read backtest completion result: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("backtest %s was no longer running while results were persisted", runID)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit backtest results: %w", err)
	}
	return nil
}

func (h *HealthHandler) setBacktestStatus(ctx context.Context, id uuid.UUID, status, message string) error {
	result, err := h.db.ExecContext(ctx, `
		UPDATE backtest_runs
		SET status = $2, error_message = $3,
		    finished_at = CASE WHEN $2 IN ('failed', 'completed') THEN now() ELSE NULL END
		WHERE id = $1 AND status IN ('queued', 'running')`, id, status, message)
	if err != nil {
		return fmt.Errorf("update backtest status: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read backtest status update result: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("backtest %s status changed before update", id)
	}
	return nil
}
