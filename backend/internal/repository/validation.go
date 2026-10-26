package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

type Validation struct {
	db *sql.DB
}

type Trial struct {
	StrategyName    string
	StrategyVersion string
	DatasetRole     string
	Parameters      any
	SnapshotID      uuid.UUID
	RunID           uuid.UUID
	PrimaryMetric   float64
}

func NewValidation(db *sql.DB) *Validation {
	return &Validation{db: db}
}

func HashTrialParameters(parameters any) (string, []byte, error) {
	encoded, err := json.Marshal(parameters)
	if err != nil {
		return "", nil, fmt.Errorf("encode trial parameters: %w", err)
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), encoded, nil
}

func (r *Validation) RecordTrial(ctx context.Context, trial Trial) error {
	if trial.StrategyName == "" || trial.StrategyVersion == "" ||
		!validDatasetRole(trial.DatasetRole) || trial.Parameters == nil ||
		trial.SnapshotID == uuid.Nil || trial.RunID == uuid.Nil {
		return errors.New("trial requires a strategy, version, valid dataset role, snapshot, and run")
	}
	if math.IsNaN(trial.PrimaryMetric) || math.IsInf(trial.PrimaryMetric, 0) {
		return errors.New("trial primary metric must be finite")
	}
	hash, parameters, err := HashTrialParameters(trial.Parameters)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO experiment_trials
			(id, strategy_name, strategy_version, parameters_hash, parameters_json,
			 dataset_role, data_snapshot_id, run_id, primary_metric_value)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9)`,
		uuid.New(), trial.StrategyName, trial.StrategyVersion, hash, parameters,
		trial.DatasetRole, trial.SnapshotID, trial.RunID, trial.PrimaryMetric,
	)
	if err != nil {
		return fmt.Errorf("record experiment trial: %w", err)
	}
	return nil
}

func (r *Validation) RecordTrialForRun(
	ctx context.Context,
	strategyName, strategyVersion, datasetRole string,
	parameters any,
	snapshotID, runID uuid.UUID,
	primaryMetric float64,
) error {
	return r.RecordTrial(ctx, Trial{
		StrategyName: strategyName, StrategyVersion: strategyVersion,
		DatasetRole: datasetRole, Parameters: parameters,
		SnapshotID: snapshotID, RunID: runID, PrimaryMetric: primaryMetric,
	})
}

func (r *Validation) TrialCount(ctx context.Context, strategyName, strategyVersion string) (int, error) {
	if strategyName == "" || strategyVersion == "" {
		return 0, errors.New("strategy name and version are required")
	}
	var count int
	if err := r.db.QueryRowContext(ctx, `
		SELECT count(*) FROM experiment_trials
		WHERE strategy_name = $1 AND strategy_version = $2`,
		strategyName, strategyVersion).Scan(&count); err != nil {
		return 0, fmt.Errorf("count experiment trials: %w", err)
	}
	return count, nil
}

func (r *Validation) SnapshotSurvivorshipBiased(ctx context.Context, snapshotID uuid.UUID) (bool, error) {
	if snapshotID == uuid.Nil {
		return false, errors.New("data snapshot id is required")
	}
	var biased bool
	if err := r.db.QueryRowContext(ctx, `
		SELECT survivorship_biased FROM data_snapshots WHERE id = $1`,
		snapshotID).Scan(&biased); err != nil {
		return false, fmt.Errorf("load data snapshot survivorship flag: %w", err)
	}
	return biased, nil
}

func (r *Validation) ValidateResearchRun(ctx context.Context, runID, snapshotID, universeID uuid.UUID, datasetRole string) error {
	if runID == uuid.Nil || snapshotID == uuid.Nil || universeID == uuid.Nil || !validDatasetRole(datasetRole) {
		return errors.New("research run requires valid run, snapshot, universe, and dataset identifiers")
	}
	var storedSnapshot uuid.UUID
	var storedUniverse uuid.UUID
	var storedRole string
	err := r.db.QueryRowContext(ctx, `
		SELECT data_snapshot_id, universe_id, dataset_role
		FROM backtest_runs WHERE id = $1`, runID).Scan(&storedSnapshot, &storedUniverse, &storedRole)
	if err != nil {
		return fmt.Errorf("load persisted backtest run: %w", err)
	}
	if storedSnapshot != snapshotID {
		return errors.New("research snapshot does not match the persisted backtest run")
	}
	if storedUniverse != universeID {
		return errors.New("research universe does not match the persisted backtest run")
	}
	if storedRole != datasetRole {
		return errors.New("research dataset role does not match the persisted backtest run")
	}
	return nil
}

func (r *Validation) HistoricalUniverseMembership(
	ctx context.Context,
	universeID uuid.UUID,
	dates []time.Time,
) (map[string][]string, error) {
	if universeID == uuid.Nil || len(dates) == 0 {
		return nil, errors.New("universe id and dates are required")
	}
	for i, date := range dates {
		if date.IsZero() || (i > 0 && !date.After(dates[i-1])) {
			return nil, fmt.Errorf("universe membership dates must be strictly ascending at index %d", i)
		}
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.symbol, m.valid_from, m.valid_to
		FROM market_universe_memberships m
		JOIN stocks s ON s.id = m.stock_id
		WHERE m.universe_id = $1
		  AND m.valid_from <= $3
		  AND (m.valid_to IS NULL OR m.valid_to >= $2)
		ORDER BY s.symbol, m.valid_from`, universeID, dates[0], dates[len(dates)-1])
	if err != nil {
		return nil, fmt.Errorf("load historical universe membership range: %w", err)
	}
	defer rows.Close()
	type membership struct {
		symbol string
		from   time.Time
		to     sql.NullTime
	}
	memberships := make([]membership, 0)
	for rows.Next() {
		var item membership
		if err := rows.Scan(&item.symbol, &item.from, &item.to); err != nil {
			return nil, fmt.Errorf("scan historical universe membership range: %w", err)
		}
		memberships = append(memberships, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read historical universe membership range: %w", err)
	}
	result := make(map[string][]string, len(dates))
	for _, date := range dates {
		for _, member := range memberships {
			if !date.Before(member.from) && (!member.to.Valid || !date.After(member.to.Time)) {
				key := date.Format("2006-01-02")
				result[key] = append(result[key], member.symbol)
			}
		}
	}
	return result, nil
}

func (r *Validation) LogHoldoutAccess(ctx context.Context, strategyName, strategyVersion string, runID uuid.UUID, note string, confirmed bool) (bool, error) {
	if strategyName == "" || strategyVersion == "" || runID == uuid.Nil {
		return false, errors.New("holdout access requires a strategy, version, and run id")
	}
	if !confirmed {
		return false, errors.New("holdout access requires explicit confirmation")
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return false, fmt.Errorf("begin holdout access transaction: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		strategyName+"\x00"+strategyVersion); err != nil {
		return false, fmt.Errorf("serialize holdout access: %w", err)
	}

	var alreadyUsed bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM holdout_access_log
			WHERE strategy_name = $1 AND strategy_version = $2
		)`, strategyName, strategyVersion).Scan(&alreadyUsed); err != nil {
		return false, fmt.Errorf("check previous holdout access: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO holdout_access_log (id, strategy_name, strategy_version, run_id, note)
		VALUES ($1, $2, $3, $4, $5)`,
		uuid.New(), strategyName, strategyVersion, runID, note,
	); err != nil {
		return false, fmt.Errorf("log holdout access: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit holdout access: %w", err)
	}
	return alreadyUsed, nil
}

func (r *Validation) HistoricalUniverseMembers(ctx context.Context, universeID uuid.UUID, date time.Time) ([]uuid.UUID, error) {
	if universeID == uuid.Nil || date.IsZero() {
		return nil, errors.New("universe id and effective date are required")
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT stock_id FROM market_universe_memberships
		WHERE universe_id = $1 AND valid_from <= $2 AND (valid_to IS NULL OR valid_to >= $2)
		ORDER BY stock_id`, universeID, date)
	if err != nil {
		return nil, fmt.Errorf("load historical universe membership: %w", err)
	}
	defer rows.Close()
	members := make([]uuid.UUID, 0)
	for rows.Next() {
		var member uuid.UUID
		if err := rows.Scan(&member); err != nil {
			return nil, fmt.Errorf("scan historical universe member: %w", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read historical universe membership: %w", err)
	}
	return members, nil
}

func (r *Validation) HistoricalUniverseSymbols(ctx context.Context, universeID uuid.UUID, date time.Time) ([]string, error) {
	if universeID == uuid.Nil || date.IsZero() {
		return nil, errors.New("universe id and effective date are required")
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.symbol
		FROM market_universe_memberships m
		JOIN stocks s ON s.id = m.stock_id
		WHERE m.universe_id = $1 AND m.valid_from <= $2 AND (m.valid_to IS NULL OR m.valid_to >= $2)
		ORDER BY s.symbol`, universeID, date)
	if err != nil {
		return nil, fmt.Errorf("load historical universe symbols: %w", err)
	}
	defer rows.Close()
	symbols := make([]string, 0)
	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err != nil {
			return nil, fmt.Errorf("scan historical universe symbol: %w", err)
		}
		symbols = append(symbols, symbol)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read historical universe symbols: %w", err)
	}
	return symbols, nil
}

func validDatasetRole(role string) bool {
	switch role {
	case "development", "walk_forward", "holdout", "forward":
		return true
	default:
		return false
	}
}
