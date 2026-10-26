package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/shopspring/decimal"

	"github.com/angelaschereraiza/trend_radar/backend/internal/backtest"
	"github.com/angelaschereraiza/trend_radar/backend/internal/repository"
	"github.com/angelaschereraiza/trend_radar/backend/internal/strategy"
)

func (h *HealthHandler) ListStrategies(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), `
		SELECT id, name, version, parameters_json
		FROM strategy_definitions
		ORDER BY name, version, id`)
	if err != nil {
		slog.ErrorContext(r.Context(), "list strategies failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to list strategies")
		return
	}
	defer rows.Close()
	items := make([]Strategy, 0)
	for rows.Next() {
		var item Strategy
		var raw []byte
		if err := rows.Scan(&item.Id, &item.Name, &item.Version, &raw); err != nil {
			slog.ErrorContext(r.Context(), "scan strategy failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to list strategies")
			return
		}
		item.Parameters, err = decodeStoredStrategyConfig(raw)
		if err != nil {
			slog.ErrorContext(r.Context(), "decode strategy configuration failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "stored strategy configuration is invalid")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		slog.ErrorContext(r.Context(), "read strategies failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to list strategies")
		return
	}
	writeJSON(w, http.StatusOK, StrategyPage{Items: items, Total: len(items)})
}

func (h *HealthHandler) CreateStrategy(w http.ResponseWriter, r *http.Request) {
	var request StrategyRequest
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if strings.TrimSpace(request.Name) == "" || strings.TrimSpace(request.Version) == "" {
		writeError(w, http.StatusBadRequest, "invalid_strategy", "name and version are required")
		return
	}
	config, err := strategyConfig(request.Parameters)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_strategy", err.Error())
		return
	}
	if err := config.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_strategy", err.Error())
		return
	}
	parameters, err := json.Marshal(request.Parameters)
	if err != nil {
		slog.ErrorContext(r.Context(), "encode strategy configuration failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to create strategy")
		return
	}
	item := Strategy{Id: openapi_types.UUID(uuid.New()), Name: request.Name, Version: request.Version, Parameters: request.Parameters}
	err = h.db.QueryRowContext(r.Context(), `
		INSERT INTO strategy_definitions (id, name, version, parameters_json)
		VALUES ($1, $2, $3, $4::jsonb)
		RETURNING id`, uuid.UUID(item.Id), item.Name, item.Version, parameters).Scan(&item.Id)
	if err != nil {
		var state interface{ SQLState() string }
		if errors.As(err, &state) && state.SQLState() == "23505" {
			writeError(w, http.StatusConflict, "strategy_exists", "a strategy with this name and version already exists")
			return
		}
		slog.ErrorContext(r.Context(), "create strategy failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to create strategy")
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/v1/strategies/%s", uuid.UUID(item.Id)))
	writeJSON(w, http.StatusCreated, item)
}

func (h *HealthHandler) GetStrategy(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	var item Strategy
	var raw []byte
	err := h.db.QueryRowContext(r.Context(), `
		SELECT id, name, version, parameters_json
		FROM strategy_definitions WHERE id = $1`, uuid.UUID(id)).
		Scan(&item.Id, &item.Name, &item.Version, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "strategy not found")
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "get strategy failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to retrieve strategy")
		return
	}
	item.Parameters, err = decodeStoredStrategyConfig(raw)
	if err != nil {
		slog.ErrorContext(r.Context(), "decode strategy configuration failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "stored strategy configuration is invalid")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func decodeStoredStrategyConfig(raw []byte) (StrategyConfig, error) {
	var config StrategyConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return StrategyConfig{}, err
	}
	if config.InitialCapitalChf != "" {
		return config, nil
	}
	var legacyWeights map[string]json.RawMessage
	if err := json.Unmarshal(raw, &legacyWeights); err != nil {
		return StrategyConfig{}, err
	}
	if _, ok := legacyWeights["Momentum"]; !ok {
		return StrategyConfig{}, errors.New("stored strategy configuration is missing initial_capital_chf")
	}
	fractional := false
	return StrategyConfig{
		InitialCapitalChf: "5000", MaxPositions: 10, RankBuffer: 15,
		MaxHoldingMonths: 3, MinTradeValueChf: "150", FractionalShares: &fractional,
	}, nil
}

func strategyConfig(parameters StrategyConfig) (strategy.Config, error) {
	capital, err := decimal.NewFromString(parameters.InitialCapitalChf)
	if err != nil {
		return strategy.Config{}, fmt.Errorf("initial_capital_chf must be a decimal amount")
	}
	minimumTrade, err := decimal.NewFromString(parameters.MinTradeValueChf)
	if err != nil {
		return strategy.Config{}, fmt.Errorf("min_trade_value_chf must be a decimal amount")
	}
	fractional := parameters.FractionalShares != nil && *parameters.FractionalShares
	return strategy.Config{
		InitialCapitalCHF: capital, MaxPositions: parameters.MaxPositions,
		RankBuffer: parameters.RankBuffer, MaxHoldingMonths: parameters.MaxHoldingMonths,
		MinTradeValueCHF: minimumTrade, FractionalShares: fractional,
	}, nil
}

func backtestCosts(parameters StrategyConfig) (backtest.CostConfig, error) {
	if parameters.Costs == nil {
		return backtest.DefaultCosts(), nil
	}
	values := []struct {
		name  string
		value string
		dest  *decimal.Decimal
	}{
		{"commission_bps", parameters.Costs.CommissionBps, new(decimal.Decimal)},
		{"min_commission_chf", parameters.Costs.MinCommissionChf, new(decimal.Decimal)},
		{"fx_spread_bps", parameters.Costs.FxSpreadBps, new(decimal.Decimal)},
		{"stamp_duty_bps", parameters.Costs.StampDutyBps, new(decimal.Decimal)},
		{"slippage_bps", parameters.Costs.SlippageBps, new(decimal.Decimal)},
		{"withholding_rate", parameters.Costs.WithholdingRate, new(decimal.Decimal)},
	}
	for i := range values {
		value, err := decimal.NewFromString(values[i].value)
		if err != nil {
			return backtest.CostConfig{}, fmt.Errorf("%s must be a decimal amount", values[i].name)
		}
		*values[i].dest = value
	}
	config := backtest.CostConfig{
		CommissionBPS: *values[0].dest, CommissionMin: *values[1].dest,
		FXSpreadBPS: *values[2].dest, StampDutyBPS: *values[3].dest,
		SlippageBPS: *values[4].dest, WithholdingRate: *values[5].dest,
	}
	if err := config.Validate(); err != nil {
		return backtest.CostConfig{}, err
	}
	return config, nil
}

func (h *HealthHandler) ListAlerts(w http.ResponseWriter, r *http.Request, params ListAlertsParams) {
	limit, offset := 50, 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}
	if limit < 1 || limit > 200 || offset < 0 {
		writeError(w, http.StatusBadRequest, "invalid_pagination", "limit must be 1-200 and offset must be non-negative")
		return
	}
	var total int
	if err := h.db.QueryRowContext(r.Context(), `
		SELECT count(*) FROM alerts
		WHERE $1::boolean IS NULL OR (acknowledged_at IS NOT NULL) = $1`,
		params.Acknowledged).Scan(&total); err != nil {
		slog.ErrorContext(r.Context(), "count alerts failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to list alerts")
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `
		SELECT a.id, a.stock_id, s.symbol, a.strategy_id, a.alert_type, a.message,
		       a.created_at, (a.acknowledged_at IS NOT NULL)
		FROM alerts a JOIN stocks s ON s.id = a.stock_id
		WHERE $1::boolean IS NULL OR (a.acknowledged_at IS NOT NULL) = $1
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT $2 OFFSET $3`, params.Acknowledged, limit, offset)
	if err != nil {
		slog.ErrorContext(r.Context(), "list alerts failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to list alerts")
		return
	}
	defer rows.Close()
	items := make([]Alert, 0)
	for rows.Next() {
		var item Alert
		if err := rows.Scan(&item.Id, &item.StockId, &item.Symbol, &item.StrategyId, &item.AlertType,
			&item.Message, &item.CreatedAt, &item.Acknowledged); err != nil {
			slog.ErrorContext(r.Context(), "scan alert failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to list alerts")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		slog.ErrorContext(r.Context(), "read alerts failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to list alerts")
		return
	}
	writeJSON(w, http.StatusOK, AlertPage{Items: items, Total: total, Limit: limit, Offset: offset})
}

func (h *HealthHandler) AcknowledgeAlert(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	var item Alert
	err := h.db.QueryRowContext(r.Context(), `
		UPDATE alerts a
		SET acknowledged_at = COALESCE(a.acknowledged_at, now())
		FROM stocks s
		WHERE a.id = $1 AND s.id = a.stock_id
		RETURNING a.id, a.stock_id, s.symbol, a.strategy_id, a.alert_type, a.message,
		          a.created_at, true`, uuid.UUID(id)).
		Scan(&item.Id, &item.StockId, &item.Symbol, &item.StrategyId, &item.AlertType,
			&item.Message, &item.CreatedAt, &item.Acknowledged)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "alert not found")
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "acknowledge alert failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to acknowledge alert")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *HealthHandler) ListJobs(w http.ResponseWriter, r *http.Request, params ListJobsParams) {
	limit, offset := 50, 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}
	if limit < 1 || limit > 200 || offset < 0 {
		writeError(w, http.StatusBadRequest, "invalid_pagination", "limit must be 1-200 and offset must be non-negative")
		return
	}
	runs, total, err := h.data.ListJobs(r.Context(), limit, offset)
	if err != nil {
		slog.ErrorContext(r.Context(), "list jobs failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to list jobs")
		return
	}
	items := make([]JobRun, len(runs))
	for i, run := range runs {
		items[i] = apiJobRun(run)
	}
	writeJSON(w, http.StatusOK, JobPage{Items: items, Total: total, Limit: limit, Offset: offset})
}

func (h *HealthHandler) RunJob(w http.ResponseWriter, r *http.Request, name string) {
	run, err := h.data.QueueJob(r.Context(), name, time.Now().UTC())
	if err != nil {
		if strings.Contains(err.Error(), "unsupported job") {
			writeError(w, http.StatusBadRequest, "unsupported_job", err.Error())
			return
		}
		slog.ErrorContext(r.Context(), "queue manual job failed", "job", name, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to queue job")
		return
	}
	writeJSON(w, http.StatusAccepted, apiJobRun(run))
}

func apiJobRun(run repository.JobRun) JobRun {
	item := JobRun{
		Id: openapi_types.UUID(run.ID), JobName: run.Name,
		ScheduledFor: run.ScheduledFor, Status: run.Status,
	}
	if run.StartedAt.Valid {
		value := run.StartedAt.Time
		item.StartedAt = &value
	}
	if run.FinishedAt.Valid {
		value := run.FinishedAt.Time
		item.FinishedAt = &value
	}
	if run.Error != "" {
		item.ErrorMessage = &run.Error
	}
	return item
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("request body must be valid JSON: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain exactly one JSON value")
	}
	return nil
}

func (h *HealthHandler) CreateBacktest(w http.ResponseWriter, r *http.Request) {
	var request BacktestRequest
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if request.StartDate.Time.IsZero() || request.EndDate.Time.IsZero() ||
		request.EndDate.Time.Before(request.StartDate.Time) {
		writeError(w, http.StatusBadRequest, "invalid_date_range", "start_date and end_date must define an ascending date range")
		return
	}
	if uuid.UUID(request.StrategyId) == uuid.Nil {
		writeError(w, http.StatusBadRequest, "invalid_strategy", "strategy_id is required")
		return
	}
	if request.EndDate.Time.Sub(request.StartDate.Time) > 5*365*24*time.Hour {
		writeError(w, http.StatusBadRequest, "date_range_too_large", "backtests are limited to five years per run")
		return
	}
	switch request.DatasetRole {
	case Development, WalkForward, Holdout, Forward:
	default:
		writeError(w, http.StatusBadRequest, "invalid_dataset_role", "dataset_role must be development, walk_forward, holdout, or forward")
		return
	}
	if request.DatasetRole == Holdout && (request.ConfirmHoldout == nil || !*request.ConfirmHoldout) {
		writeError(w, http.StatusBadRequest, "holdout_confirmation_required", "holdout runs require explicit confirmation")
		return
	}
	select {
	case h.backtestSlots <- struct{}{}:
	default:
		writeError(w, http.StatusServiceUnavailable, "backtest_capacity", "all backtest execution slots are busy")
		return
	}
	runID, err := h.queueBacktest(r.Context(), request)
	if err != nil {
		<-h.backtestSlots
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusConflict, "research_data_unavailable", "a strategy, data snapshot, and point-in-time universe are required")
			return
		}
		slog.ErrorContext(r.Context(), "queue backtest failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to queue backtest")
		return
	}
	h.backtestWG.Add(1)
	go func() {
		defer h.backtestWG.Done()
		defer func() { <-h.backtestSlots }()
		ctx, cancel := context.WithTimeout(h.backtestCtx, 30*time.Minute)
		defer cancel()
		h.executeBacktest(ctx, runID, request)
	}()
	item, err := h.loadBacktestRun(r.Context(), runID)
	if err != nil {
		slog.ErrorContext(r.Context(), "load queued backtest failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "backtest was queued but its status could not be loaded")
		return
	}
	writeJSON(w, http.StatusAccepted, item)
}

func (h *HealthHandler) queueBacktest(ctx context.Context, request BacktestRequest) (uuid.UUID, error) {
	var strategyParameters json.RawMessage
	var strategyName, strategyVersion string
	if err := h.db.QueryRowContext(ctx, `
		SELECT name, version, parameters_json
		FROM strategy_definitions WHERE id = $1`, uuid.UUID(request.StrategyId)).
		Scan(&strategyName, &strategyVersion, &strategyParameters); err != nil {
		return uuid.Nil, err
	}
	var snapshotID, universeID uuid.UUID
	var survivorshipBiased bool
	err := h.db.QueryRowContext(ctx, `
		SELECT ds.id, ds.survivorship_biased, u.id
		FROM data_snapshots ds
		CROSS JOIN LATERAL (
			SELECT id FROM universes
			WHERE definition_json->>'source_snapshot_id' IS NULL
			   OR definition_json->>'source_snapshot_id' = ds.id::text
			ORDER BY created_at DESC, id LIMIT 1
		) u
		WHERE (ds.date_range_start IS NULL OR ds.date_range_start <= $1)
		  AND (ds.date_range_end IS NULL OR ds.date_range_end >= $2)
		ORDER BY ds.created_at DESC, ds.id
		LIMIT 1`, request.StartDate.Time, request.EndDate.Time).
		Scan(&snapshotID, &survivorshipBiased, &universeID)
	if err != nil {
		return uuid.Nil, err
	}
	configuration, err := json.Marshal(map[string]any{
		"strategy":         json.RawMessage(strategyParameters),
		"strategy_name":    strategyName,
		"strategy_version": strategyVersion,
		"start_date":       request.StartDate.Time.Format("2006-01-02"),
		"end_date":         request.EndDate.Time.Format("2006-01-02"),
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("encode backtest configuration: %w", err)
	}
	runID := uuid.New()
	_, err = h.db.ExecContext(ctx, `
		INSERT INTO backtest_runs
			(id, strategy_id, universe_id, start_date, end_date, configuration_json,
			 data_snapshot_id, code_version, dataset_role, survivorship_biased, status)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, 'queued')`,
		runID, uuid.UUID(request.StrategyId), universeID, request.StartDate.Time, request.EndDate.Time,
		configuration, snapshotID, "working-tree", string(request.DatasetRole), survivorshipBiased)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert backtest run: %w", err)
	}
	return runID, nil
}

func (h *HealthHandler) GetBacktest(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	item, err := h.loadBacktestRun(r.Context(), uuid.UUID(id))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "backtest not found")
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "get backtest failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to retrieve backtest")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *HealthHandler) loadBacktestRun(ctx context.Context, id uuid.UUID) (BacktestRun, error) {
	var item BacktestRun
	var startDate, endDate time.Time
	var snapshotID sql.NullString
	var summaryJSON, warningsJSON []byte
	var message string
	err := h.db.QueryRowContext(ctx, `
		SELECT id, strategy_id, start_date, end_date, status, dataset_role,
		       data_snapshot_id::text, survivorship_biased, summary_json, warnings_json, error_message
		FROM backtest_runs WHERE id = $1`, id).
		Scan(&item.Id, &item.StrategyId, &startDate, &endDate, &item.Status, &item.DatasetRole,
			&snapshotID, &item.SurvivorshipBiased, &summaryJSON, &warningsJSON, &message)
	if err != nil {
		return BacktestRun{}, err
	}
	item.StartDate = openapi_types.Date{Time: startDate}
	item.EndDate = openapi_types.Date{Time: endDate}
	if snapshotID.Valid {
		parsed, err := uuid.Parse(snapshotID.String)
		if err != nil {
			return BacktestRun{}, fmt.Errorf("decode backtest snapshot id: %w", err)
		}
		value := openapi_types.UUID(parsed)
		item.SnapshotId = &value
	}
	if message != "" {
		item.Error = &message
	}
	if len(summaryJSON) > 0 {
		var summary map[string]interface{}
		if err := json.Unmarshal(summaryJSON, &summary); err != nil {
			return BacktestRun{}, fmt.Errorf("decode backtest summary: %w", err)
		}
		item.Summary = &summary
	}
	if err := json.Unmarshal(warningsJSON, &item.Warnings); err != nil {
		return BacktestRun{}, fmt.Errorf("decode backtest warnings: %w", err)
	}
	if item.Warnings == nil {
		item.Warnings = []string{}
	}
	return item, nil
}

func (h *HealthHandler) GetBacktestEquity(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	var exists bool
	if err := h.db.QueryRowContext(r.Context(), "SELECT EXISTS (SELECT 1 FROM backtest_runs WHERE id = $1)", uuid.UUID(id)).Scan(&exists); err != nil {
		slog.ErrorContext(r.Context(), "check backtest existence failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to retrieve backtest equity")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "not_found", "backtest not found")
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `
		SELECT trading_date, portfolio_value_chf::text, cash_balance_chf::text,
		       invested_value_chf::text, benchmark_value_chf::text
		FROM backtest_equity WHERE run_id = $1 ORDER BY trading_date`, uuid.UUID(id))
	if err != nil {
		slog.ErrorContext(r.Context(), "query backtest equity failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to retrieve backtest equity")
		return
	}
	defer rows.Close()
	items := make([]EquityPoint, 0)
	for rows.Next() {
		var item EquityPoint
		var date time.Time
		if err := rows.Scan(&date, &item.PortfolioChf, &item.CashChf, &item.InvestedChf, &item.BenchmarkChf); err != nil {
			slog.ErrorContext(r.Context(), "scan backtest equity failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to retrieve backtest equity")
			return
		}
		item.Date = openapi_types.Date{Time: date}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		slog.ErrorContext(r.Context(), "read backtest equity failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to retrieve backtest equity")
		return
	}
	writeJSON(w, http.StatusOK, EquityPage{Items: items})
}

func (h *HealthHandler) GetBacktestTrades(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	rows, err := h.db.QueryContext(r.Context(), `
		SELECT o.intended_execution_date, s.symbol, o.side, o.requested_quantity::text, o.reason,
		       o.status, o.reject_reason, f.price_usd::text, f.gross_value_chf::text,
		       f.commission_chf::text, f.fx_cost_chf::text, f.stamp_duty_chf::text, f.slippage_chf::text
		FROM backtest_runs br
		LEFT JOIN backtest_orders o ON o.run_id = br.id
		LEFT JOIN stocks s ON s.id = o.stock_id
		LEFT JOIN backtest_fills f ON f.order_id = o.id
		WHERE br.id = $1
		ORDER BY o.intended_execution_date, o.id`, uuid.UUID(id))
	if err != nil {
		slog.ErrorContext(r.Context(), "query backtest trades failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to retrieve backtest trades")
		return
	}
	defer rows.Close()
	items := make([]Trade, 0)
	found := false
	for rows.Next() {
		found = true
		var item Trade
		var date sql.NullTime
		var symbol, side, quantity, reason, status sql.NullString
		var reject, price, gross, commission, fxCost, stampDuty, slippage sql.NullString
		if err := rows.Scan(&date, &symbol, &side, &quantity, &reason, &status, &reject,
			&price, &gross, &commission, &fxCost, &stampDuty, &slippage); err != nil {
			slog.ErrorContext(r.Context(), "scan backtest trade failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to retrieve backtest trades")
			return
		}
		if !date.Valid {
			continue
		}
		item.Date = openapi_types.Date{Time: date.Time}
		item.Symbol, item.Side, item.Quantity, item.Reason, item.Status = symbol.String, side.String, quantity.String, reason.String, status.String
		item.RejectReason, item.PriceUsd, item.GrossChf = optionalString(reject), optionalString(price), optionalString(gross)
		item.CommissionChf, item.FxCostChf = optionalString(commission), optionalString(fxCost)
		item.StampDutyChf, item.SlippageChf = optionalString(stampDuty), optionalString(slippage)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		slog.ErrorContext(r.Context(), "read backtest trades failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to retrieve backtest trades")
		return
	}
	if !found {
		var exists bool
		if err := h.db.QueryRowContext(r.Context(), "SELECT EXISTS (SELECT 1 FROM backtest_runs WHERE id = $1)", uuid.UUID(id)).Scan(&exists); err != nil {
			slog.ErrorContext(r.Context(), "check backtest existence failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to retrieve backtest trades")
			return
		}
		if !exists {
			writeError(w, http.StatusNotFound, "not_found", "backtest not found")
			return
		}
	}
	writeJSON(w, http.StatusOK, TradePage{Items: items})
}

func (h *HealthHandler) GetBacktestMetrics(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	run, err := h.loadBacktestRun(r.Context(), uuid.UUID(id))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "backtest not found")
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "get backtest metrics failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to retrieve backtest metrics")
		return
	}
	metrics := map[string]interface{}{}
	report := map[string]interface{}{}
	if run.Summary != nil {
		metrics, _ = (*run.Summary)["metrics"].(map[string]interface{})
		report, _ = (*run.Summary)["statistical_report"].(map[string]interface{})
	}
	writeJSON(w, http.StatusOK, BacktestMetrics{Metrics: metrics, StatisticalReport: report, Warnings: run.Warnings})
}

func optionalString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
