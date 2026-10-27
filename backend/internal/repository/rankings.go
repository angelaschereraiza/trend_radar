package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/angelaschereraiza/trend_radar/backend/internal/scoring"
)

type PriceHistory struct {
	StockID       uuid.UUID
	Symbol        string
	Exchange      string
	Name          string
	Currency      string
	Sector        string
	Date          time.Time
	Close         decimal.Decimal
	AdjustedClose decimal.Decimal
	Volume        int64
}

type RankingEntry struct {
	StockID    uuid.UUID
	Symbol     string
	Sector     string
	Score      float64
	Rank       int
	Components scoring.ComponentScores
	Affordable bool
}

func (r *MarketData) LoadLatestPriceHistory(ctx context.Context) ([]PriceHistory, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT stock_id, symbol, exchange, name, currency, sector, trading_date,
			close, adjusted_close, volume
		FROM (
			SELECT DISTINCT ON (p.stock_id, p.trading_date)
				p.stock_id, s.symbol, s.exchange, s.name, s.currency, s.sector,
				p.trading_date, p.close, p.adjusted_close, p.volume, p.imported_at, p.data_version
			FROM stock_prices p
			JOIN stocks s ON s.id = p.stock_id
			WHERE s.active AND s.currency = 'USD' AND p.currency = 'USD'
			ORDER BY p.stock_id, p.trading_date, p.imported_at DESC, p.data_version DESC
		) latest
		ORDER BY symbol ASC, trading_date ASC`)
	if err != nil {
		return nil, fmt.Errorf("load latest market history: %w", err)
	}
	defer rows.Close()
	history := make([]PriceHistory, 0)
	for rows.Next() {
		var item PriceHistory
		if err := rows.Scan(&item.StockID, &item.Symbol, &item.Exchange, &item.Name, &item.Currency,
			&item.Sector, &item.Date, &item.Close, &item.AdjustedClose, &item.Volume); err != nil {
			return nil, fmt.Errorf("scan latest market history: %w", err)
		}
		history = append(history, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read latest market history: %w", err)
	}
	return history, nil
}

func (r *MarketData) SaveRanking(ctx context.Context, name, version string, date time.Time, entries []RankingEntry, correlations []scoring.Correlation) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin ranking persistence: %w", err)
	}
	defer tx.Rollback()
	parameters, err := json.Marshal(map[string]any{
		"initial_capital_chf": "5000",
		"max_positions":       10,
		"rank_buffer":         15,
		"max_holding_months":  3,
		"min_trade_value_chf": "150",
		"fractional_shares":   false,
		"scoring_weights":     scoring.DefaultWeights(),
	})
	if err != nil {
		return fmt.Errorf("encode ranking configuration: %w", err)
	}
	var strategyID uuid.UUID
	err = tx.QueryRowContext(ctx, `
		INSERT INTO strategy_definitions (id, name, version, parameters_json)
		VALUES ($1, $2, $3, $4::jsonb)
		ON CONFLICT (name, version) DO UPDATE SET parameters_json = EXCLUDED.parameters_json
		RETURNING id`, uuid.New(), name, version, parameters).Scan(&strategyID)
	if err != nil {
		return fmt.Errorf("persist ranking strategy: %w", err)
	}
	for _, entry := range entries {
		components, err := json.Marshal(struct {
			scoring.ComponentScores
			Affordable bool `json:"affordable"`
		}{ComponentScores: entry.Components, Affordable: entry.Affordable})
		if err != nil {
			return fmt.Errorf("encode score components for %s: %w", entry.Symbol, err)
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO stock_scores (stock_id, strategy_id, calculation_date, score, component_scores_json, calculation_version)
			VALUES ($1, $2, $3, $4, $5::jsonb, $6)
			ON CONFLICT (stock_id, strategy_id, calculation_date, calculation_version)
			DO UPDATE SET score = EXCLUDED.score, component_scores_json = EXCLUDED.component_scores_json`,
			entry.StockID, strategyID, date, entry.Score, components, "scoring-v1",
		)
		if err != nil {
			return fmt.Errorf("persist stock score for %s: %w", entry.Symbol, err)
		}
	}
	for _, correlation := range correlations {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO factor_correlations
				(strategy_id, calculation_date, factor_a, factor_b, correlation, sample_size)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (strategy_id, calculation_date, factor_a, factor_b)
			DO UPDATE SET correlation = EXCLUDED.correlation, sample_size = EXCLUDED.sample_size`,
			strategyID, date, correlation.FactorA, correlation.FactorB, correlation.Value, correlation.Samples,
		)
		if err != nil {
			return fmt.Errorf("persist factor correlation %s/%s: %w", correlation.FactorA, correlation.FactorB, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit ranking persistence: %w", err)
	}
	return nil
}

func (r *MarketData) Rankings(ctx context.Context, date *time.Time, strategyName string, minimumScore *float64, sector, search, sortBy string, affordableOnly bool, limit, offset int) ([]RankingEntry, int, error) {
	orderBy := "ss.score DESC, (ss.component_scores_json->>'momentum')::double precision DESC, s.symbol ASC, s.id ASC"
	switch sortBy {
	case "momentum":
		orderBy = "(ss.component_scores_json->>'momentum')::double precision DESC, ss.score DESC, s.symbol ASC, s.id ASC"
	case "volatility":
		orderBy = "(ss.component_scores_json->>'volatility_adjusted_momentum')::double precision DESC, ss.score DESC, s.symbol ASC, s.id ASC"
	}
	query := fmt.Sprintf(`
		SELECT s.id, s.symbol, s.sector, ss.score, ss.component_scores_json
		FROM stock_scores ss
		JOIN stocks s ON s.id = ss.stock_id
		JOIN strategy_definitions sd ON sd.id = ss.strategy_id
		WHERE ($1::date IS NULL OR ss.calculation_date = $1)
		  AND ($2 = '' OR sd.name = $2)
		  AND ($3::double precision IS NULL OR ss.score >= $3)
		  AND ($4 = '' OR s.sector = $4)
		  AND ($5 = '' OR s.symbol ILIKE '%%' || $5 || '%%' OR s.name ILIKE '%%' || $5 || '%%')
		  AND (NOT $6 OR (ss.component_scores_json->>'affordable')::boolean)
		ORDER BY %s
		LIMIT $7 OFFSET $8`, orderBy)
	search = strings.TrimSpace(search)
	rows, err := r.db.QueryContext(ctx, query, date, strategyName, minimumScore, sector, search, affordableOnly, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query stock rankings: %w", err)
	}

	defer rows.Close()
	entries := make([]RankingEntry, 0)
	for rows.Next() {
		var entry RankingEntry
		var raw []byte
		if err := rows.Scan(&entry.StockID, &entry.Symbol, &entry.Sector, &entry.Score, &raw); err != nil {
			return nil, 0, fmt.Errorf("scan stock ranking: %w", err)
		}
		var components struct {
			scoring.ComponentScores
			Affordable bool `json:"affordable"`
		}
		if err := json.Unmarshal(raw, &components); err != nil {
			return nil, 0, fmt.Errorf("decode score components for %s: %w", entry.Symbol, err)
		}
		entry.Components = components.ComponentScores
		entry.Affordable = components.Affordable
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read stock rankings: %w", err)
	}

	var total int
	err = r.db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM stock_scores ss
		JOIN stocks s ON s.id = ss.stock_id
		JOIN strategy_definitions sd ON sd.id = ss.strategy_id
		WHERE ($1::date IS NULL OR ss.calculation_date = $1)
		  AND ($2 = '' OR sd.name = $2)
		  AND ($3::double precision IS NULL OR ss.score >= $3)
		  AND ($4 = '' OR s.sector = $4)
		  AND ($5 = '' OR s.symbol ILIKE '%' || $5 || '%' OR s.name ILIKE '%' || $5 || '%')
		  AND (NOT $6 OR (ss.component_scores_json->>'affordable')::boolean)`,
		date, strategyName, minimumScore, sector, search, affordableOnly).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count stock rankings: %w", err)
	}
	return entries, total, nil
}

func (r *MarketData) LatestRankingDate(ctx context.Context, strategyName string) (*time.Time, error) {
	var date sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT max(ss.calculation_date)
		FROM stock_scores ss
		JOIN strategy_definitions sd ON sd.id = ss.strategy_id
		WHERE ($1 = '' OR sd.name = $1)`, strategyName).Scan(&date)
	if err != nil {
		return nil, fmt.Errorf("find latest ranking date: %w", err)
	}
	if !date.Valid {
		return nil, nil
	}
	return &date.Time, nil
}

func (r *MarketData) LoadLatestFXRates(ctx context.Context, base, quote string) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT ON (rate_date) rate_date, rate::text
		FROM fx_rates
		WHERE base_currency = $1 AND quote_currency = $2
		ORDER BY rate_date, imported_at DESC, data_version DESC`, base, quote)
	if err != nil {
		return nil, fmt.Errorf("load latest FX rates for %s/%s: %w", base, quote, err)
	}
	defer rows.Close()
	rates := make(map[string]string)
	for rows.Next() {
		var date time.Time
		var rate string
		if err := rows.Scan(&date, &rate); err != nil {
			return nil, fmt.Errorf("scan latest FX rate: %w", err)
		}
		rates[date.Format("2006-01-02")] = rate
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read latest FX rates: %w", err)
	}
	return rates, nil
}
