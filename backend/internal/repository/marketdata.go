package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/angelaschereraiza/trend_radar/backend/internal/marketdata"
)

type MarketData struct {
	db *sql.DB
}

type ImportReport struct {
	SnapshotID string
	Inserted   int
	Unchanged  int
}

func NewMarketData(db *sql.DB) *MarketData {
	return &MarketData{db: db}
}

func (r *MarketData) Import(ctx context.Context, batch marketdata.ImportBatch, description string, survivorshipBiased bool) (ImportReport, error) {
	if err := marketdata.ValidateBatch(batch); err != nil {
		return ImportReport{}, err
	}
	snapshot, err := marketdata.BuildSnapshot(batch, survivorshipBiased)
	if err != nil {
		return ImportReport{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ImportReport{}, fmt.Errorf("begin market-data import: %w", err)
	}
	defer tx.Rollback()

	for _, symbol := range batch.Symbols {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO stocks (id, symbol, exchange, name, currency, sector, industry)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (id) DO UPDATE SET
				symbol = EXCLUDED.symbol, exchange = EXCLUDED.exchange,
				name = EXCLUDED.name, currency = EXCLUDED.currency,
				sector = EXCLUDED.sector, industry = EXCLUDED.industry,
				updated_at = now()`,
			symbol.ID, symbol.Symbol, symbol.Exchange, symbol.Name, symbol.Currency, symbol.Sector, symbol.Industry,
		); err != nil {
			return ImportReport{}, fmt.Errorf("upsert stock %s: %w", symbol.Symbol, err)
		}
	}

	symbolIDs := make(map[string]string, len(batch.Symbols))
	for _, symbol := range batch.Symbols {
		symbolIDs[symbol.Symbol] = symbol.ID
		firstDate := time.Time{}
		for _, bar := range batch.Prices {
			if bar.Symbol == symbol.Symbol && (firstDate.IsZero() || bar.TradingDate.Before(firstDate)) {
				firstDate = bar.TradingDate
			}
		}
		if firstDate.IsZero() {
			firstDate = time.Now().UTC().Truncate(24 * time.Hour)
		}
		var recordedID string
		err = tx.QueryRowContext(ctx, `
			INSERT INTO symbol_history (stock_id, symbol, valid_from)
			VALUES ($1, $2, $3)
			ON CONFLICT (symbol) WHERE valid_to IS NULL
			DO UPDATE SET valid_from = LEAST(symbol_history.valid_from, EXCLUDED.valid_from)
			WHERE symbol_history.stock_id = EXCLUDED.stock_id
			RETURNING stock_id`,
			symbol.ID, symbol.Symbol, firstDate,
		).Scan(&recordedID)
		if err == sql.ErrNoRows {
			return ImportReport{}, fmt.Errorf("symbol %s is already assigned to another security; close the prior symbol history first", symbol.Symbol)
		}
		if err != nil {
			return ImportReport{}, fmt.Errorf("record symbol history for %s: %w", symbol.Symbol, err)
		}
	}

	report := ImportReport{}
	for _, bar := range batch.Prices {
		inserted, err := insertPrice(ctx, tx, symbolIDs[bar.Symbol], bar)
		if err != nil {
			return ImportReport{}, err
		}
		if inserted {
			report.Inserted++
		} else {
			report.Unchanged++
		}
	}
	for _, rate := range batch.FXRates {
		inserted, err := insertFXRate(ctx, tx, rate)
		if err != nil {
			return ImportReport{}, err
		}
		if inserted {
			report.Inserted++
		} else {
			report.Unchanged++
		}
	}
	for _, action := range batch.CorporateActions {
		stockID := symbolIDs[action.Symbol]
		if stockID == "" {
			return ImportReport{}, fmt.Errorf("corporate action references unknown symbol %q", action.Symbol)
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO corporate_actions
				(id, stock_id, action_type, effective_date, amount, ratio, delisting_reason, source)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (stock_id, action_type, effective_date, amount, ratio, source) DO NOTHING`,
			uuid.New(), stockID, action.Type, action.EffectiveDate,
			action.Amount.StringFixed(8), action.Ratio.StringFixed(8), action.Reason, action.Provider,
		)
		if err != nil {
			return ImportReport{}, fmt.Errorf("insert corporate action for %s: %w", action.Symbol, err)
		}
	}

	rowCounts, err := json.Marshal(map[string]int{
		"stocks": len(batch.Symbols), "prices": snapshot.PriceRows,
		"corporate_actions": snapshot.CorporateRows, "fx_rates": snapshot.FXRows,
	})
	if err != nil {
		return ImportReport{}, fmt.Errorf("encode snapshot counts: %w", err)
	}
	var startDate, endDate any
	if !snapshot.RangeStart.IsZero() {
		startDate = snapshot.RangeStart
		endDate = snapshot.RangeEnd
	}
	snapshotID := uuid.New()
	providerSummary, _ := json.Marshal(map[string]any{"providers": providers(batch)})
	_, err = tx.ExecContext(ctx, `
		INSERT INTO data_snapshots
			(id, description, content_hash, row_counts_json, date_range_start, date_range_end,
			 survivorship_biased, provider_summary_json)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8::jsonb)`,
		snapshotID, description, snapshot.ContentHash, rowCounts,
		startDate, endDate, survivorshipBiased, providerSummary,
	)
	if err != nil {
		return ImportReport{}, fmt.Errorf("persist data snapshot: %w", err)
	}
	if survivorshipBiased && !snapshot.RangeStart.IsZero() {
		universeVersion := snapshot.ContentHash[:16]
		universeID := uuid.New()
		definition, err := json.Marshal(map[string]string{
			"source_snapshot_id": snapshotID.String(),
			"membership_quality": "fixture_survivorship_biased",
		})
		if err != nil {
			return ImportReport{}, fmt.Errorf("encode fixture universe definition: %w", err)
		}
		err = tx.QueryRowContext(ctx, `
			INSERT INTO universes (id, name, version, definition_json)
			VALUES ($1, 'fixture_universe', $2, $3::jsonb)
			ON CONFLICT (name, version) DO UPDATE SET definition_json = EXCLUDED.definition_json
			RETURNING id`, universeID, universeVersion, definition).Scan(&universeID)
		if err != nil {
			return ImportReport{}, fmt.Errorf("persist fixture universe: %w", err)
		}
		for _, symbol := range batch.Symbols {
			_, err := tx.ExecContext(ctx, `
				INSERT INTO market_universe_memberships
					(universe_id, stock_id, valid_from, valid_to, source)
				VALUES ($1, $2, $3, $4, 'fixture_snapshot')
				ON CONFLICT (universe_id, stock_id, valid_from) DO NOTHING`,
				universeID, symbol.ID, snapshot.RangeStart, snapshot.RangeEnd)
			if err != nil {
				return ImportReport{}, fmt.Errorf("persist fixture membership for %s: %w", symbol.Symbol, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return ImportReport{}, fmt.Errorf("commit market-data import: %w", err)
	}
	report.SnapshotID = snapshotID.String()
	return report, nil
}

func insertPrice(ctx context.Context, tx *sql.Tx, stockID string, bar marketdata.PriceBar) (bool, error) {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO stock_prices
			(stock_id, trading_date, open, high, low, close, adjusted_close, volume, currency, provider, data_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (stock_id, trading_date, provider, data_version) DO NOTHING`,
		stockID, bar.TradingDate, bar.Open.StringFixed(8), bar.High.StringFixed(8),
		bar.Low.StringFixed(8), bar.Close.StringFixed(8), bar.AdjustedClose.StringFixed(8),
		bar.Volume, bar.Currency, bar.Provider, bar.DataVersion,
	)
	if err != nil {
		return false, fmt.Errorf("insert price bar %s %s: %w", bar.Symbol, bar.TradingDate.Format("2006-01-02"), err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read price insert result: %w", err)
	}
	if count == 1 {
		return true, nil
	}
	var open, high, low, closePrice, adjusted decimal.Decimal
	var volume int64
	var currency string
	err = tx.QueryRowContext(ctx, `
		SELECT open, high, low, close, adjusted_close, volume, currency
		FROM stock_prices
		WHERE stock_id = $1 AND trading_date = $2 AND provider = $3 AND data_version = $4`,
		stockID, bar.TradingDate, bar.Provider, bar.DataVersion,
	).Scan(&open, &high, &low, &closePrice, &adjusted, &volume, &currency)
	if err != nil {
		return false, fmt.Errorf("check existing price bar: %w", err)
	}
	if open.Cmp(bar.Open) != 0 || high.Cmp(bar.High) != 0 || low.Cmp(bar.Low) != 0 ||
		closePrice.Cmp(bar.Close) != 0 || adjusted.Cmp(bar.AdjustedClose) != 0 ||
		volume != bar.Volume || currency != bar.Currency {
		return false, fmt.Errorf("conflicting historical value for %s on %s from %s version %s",
			bar.Symbol, bar.TradingDate.Format("2006-01-02"), bar.Provider, bar.DataVersion)
	}
	return false, nil
}

func insertFXRate(ctx context.Context, tx *sql.Tx, rate marketdata.FXRate) (bool, error) {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO fx_rates (base_currency, quote_currency, rate_date, rate, provider, data_version)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (base_currency, quote_currency, rate_date, provider, data_version) DO NOTHING`,
		rate.BaseCurrency, rate.QuoteCurrency, rate.RateDate, rate.Rate.StringFixed(8), rate.Provider, rate.DataVersion,
	)
	if err != nil {
		return false, fmt.Errorf("insert FX rate %s/%s on %s: %w",
			rate.BaseCurrency, rate.QuoteCurrency, rate.RateDate.Format("2006-01-02"), err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read FX insert result: %w", err)
	}
	if count == 1 {
		return true, nil
	}
	var existing decimal.Decimal
	err = tx.QueryRowContext(ctx, `
		SELECT rate FROM fx_rates
		WHERE base_currency = $1 AND quote_currency = $2 AND rate_date = $3
			AND provider = $4 AND data_version = $5`,
		rate.BaseCurrency, rate.QuoteCurrency, rate.RateDate, rate.Provider, rate.DataVersion,
	).Scan(&existing)
	if err != nil {
		return false, fmt.Errorf("check existing FX rate: %w", err)
	}
	if existing.Cmp(rate.Rate) != 0 {
		return false, fmt.Errorf("conflicting FX rate for %s/%s on %s from %s version %s",
			rate.BaseCurrency, rate.QuoteCurrency, rate.RateDate.Format("2006-01-02"), rate.Provider, rate.DataVersion)
	}
	return false, nil
}

func providers(batch marketdata.ImportBatch) []string {
	set := make(map[string]struct{})
	for _, bar := range batch.Prices {
		set[bar.Provider] = struct{}{}
	}
	for _, rate := range batch.FXRates {
		set[rate.Provider] = struct{}{}
	}
	for _, action := range batch.CorporateActions {
		set[action.Provider] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for provider := range set {
		result = append(result, provider)
	}
	return result
}
