package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/angelaschereraiza/trend_radar/backend/internal/calendar"
	"github.com/angelaschereraiza/trend_radar/backend/internal/config"
	"github.com/angelaschereraiza/trend_radar/backend/internal/repository"
	"github.com/angelaschereraiza/trend_radar/backend/internal/worker"
)

type Scheduler struct {
	db             *sql.DB
	market         *repository.MarketData
	owner          string
	capital        int
	alertThreshold float64
	logger         *slog.Logger
	location       *time.Location
	marketTZ       *time.Location
}

func New(db *sql.DB, cfg config.Config, logger *slog.Logger) (*Scheduler, error) {
	owner, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("create scheduler owner: %w", err)
	}
	location, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		return nil, fmt.Errorf("load Europe/Zurich timezone: %w", err)
	}
	marketTZ, err := time.LoadLocation("America/New_York")
	if err != nil {
		return nil, fmt.Errorf("load America/New_York timezone: %w", err)
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		db: db, market: repository.NewMarketData(db), owner: owner.String(),
		capital: cfg.MaxCapitalCHF, alertThreshold: cfg.AlertScoreThreshold,
		logger: logger, location: location, marketTZ: marketTZ,
	}, nil
}

func (s *Scheduler) Run(ctx context.Context) error {
	s.enqueueDaily(ctx, time.Now())
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.RunOne(ctx); err != nil && !errors.Is(err, context.Canceled) {
			s.logger.ErrorContext(ctx, "scheduled job failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-ticker.C:
			s.enqueueDaily(ctx, now)
		}
	}
}

func (s *Scheduler) enqueueDaily(ctx context.Context, now time.Time) {
	scheduledFor, due, err := DailyScheduleTime(now, s.location, s.marketTZ)
	if err != nil {
		s.logger.ErrorContext(ctx, "calculate daily job schedule", "error", err)
		return
	}
	if !due {
		return
	}
	for _, name := range []string{"market_data", "fx_rates", "rankings", "alerts", "data_quality"} {
		if _, err := s.market.QueueJob(ctx, name, scheduledFor); err != nil {
			s.logger.ErrorContext(ctx, "queue scheduled job", "job", name, "error", err)
		}
	}
}

func DailyScheduleTime(now time.Time, zurich, market *time.Location) (time.Time, bool, error) {
	if zurich == nil || market == nil {
		return time.Time{}, false, fmt.Errorf("Zurich and New York timezones are required")
	}
	zurichNow, marketNow := now.In(zurich), now.In(market)
	if zurichNow.Hour() < 18 || marketNow.Hour() < 16 ||
		(marketNow.Hour() == 16 && marketNow.Minute() < 30) {
		return time.Time{}, false, nil
	}
	date := calendar.FromTime(marketNow)
	tradingDay, err := calendar.IsTradingDay(date)
	if err != nil {
		return time.Time{}, false, err
	}
	if !tradingDay {
		return time.Time{}, false, nil
	}
	scheduledFor := time.Date(date.Year, date.Month, date.Day, 16, 30, 0, 0, market).UTC()
	return scheduledFor, true, nil
}

func (s *Scheduler) RunOne(ctx context.Context) error {
	run, found, err := s.market.ClaimJob(ctx, s.owner, 2*time.Minute)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	runErr := s.execute(ctx, run.Name)
	if err := s.market.FinishJob(context.WithoutCancel(ctx), run.ID, s.owner, runErr); err != nil {
		return errors.Join(runErr, err)
	}
	if runErr != nil {
		return fmt.Errorf("%s job %s failed: %w", run.Name, run.ID, runErr)
	}
	return nil
}

func (s *Scheduler) execute(ctx context.Context, name string) error {
	switch name {
	case "market_data":
		return errors.New("scheduled market-data collection is not configured; run cmd/collector --yahoo to import the supported Yahoo Finance instruments")
	case "fx_rates":
		return errors.New("scheduled FX collection is not configured; USD/CHF rates are imported with cmd/collector --yahoo")
	case "rankings":
		history, err := s.market.LoadLatestPriceHistory(ctx)
		if err != nil {
			return err
		}
		rates, err := s.market.LoadLatestFXRates(ctx, "USD", "CHF")
		if err != nil {
			return err
		}
		run, err := worker.CalculateLatestRanking(history, rates, s.capital)
		if err != nil {
			return err
		}
		return s.market.SaveRanking(ctx, "momentum_v1", "1", run.Date, run.Entries, run.Correlations)
	case "alerts":
		return s.generateAlerts(ctx)
	case "data_quality":
		return s.checkDataQuality(ctx)
	default:
		return fmt.Errorf("unsupported job %q", name)
	}
}

func (s *Scheduler) checkDataQuality(ctx context.Context) error {
	var stockCount, priceCount, fxCount int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM stocks WHERE active").Scan(&stockCount); err != nil {
		return fmt.Errorf("count active stocks: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM stock_prices").Scan(&priceCount); err != nil {
		return fmt.Errorf("count price bars: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM fx_rates WHERE base_currency = 'USD' AND quote_currency = 'CHF'").Scan(&fxCount); err != nil {
		return fmt.Errorf("count USD/CHF rates: %w", err)
	}
	if stockCount == 0 || priceCount == 0 || fxCount == 0 {
		return fmt.Errorf("market data incomplete: active_stocks=%d price_bars=%d usd_chf_rates=%d", stockCount, priceCount, fxCount)
	}
	var invalidBars int
	if err := s.db.QueryRowContext(ctx, `
		SELECT count(*) FROM stock_prices
		WHERE high < low OR high < open OR high < close OR low > open OR low > close
		   OR open <= 0 OR high <= 0 OR low <= 0 OR close <= 0 OR adjusted_close <= 0 OR volume < 0`).Scan(&invalidBars); err != nil {
		return fmt.Errorf("check price bar integrity: %w", err)
	}
	if invalidBars > 0 {
		return fmt.Errorf("data quality check found %d invalid price bars", invalidBars)
	}
	var missingLatestFX bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT NOT EXISTS (
			SELECT 1 FROM fx_rates f
			WHERE f.base_currency = 'USD' AND f.quote_currency = 'CHF'
			  AND f.rate_date = (SELECT max(p.trading_date) FROM stock_prices p
			                     JOIN stocks st ON st.id = p.stock_id WHERE st.symbol = 'SPY')
		)`).Scan(&missingLatestFX); err != nil {
		return fmt.Errorf("check latest USD/CHF coverage: %w", err)
	}
	if missingLatestFX {
		return errors.New("latest SPY trading date has no matching USD/CHF rate")
	}
	return nil
}

func (s *Scheduler) generateAlerts(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO alerts (id, stock_id, strategy_id, alert_type, message, generated_date)
		SELECT gen_random_uuid(), ss.stock_id, ss.strategy_id, 'score_threshold',
		       $2,
		       ss.calculation_date
		FROM stock_scores ss
		WHERE ss.score >= $1
		  AND COALESCE((
		      SELECT previous.score
		      FROM stock_scores previous
		      WHERE previous.stock_id = ss.stock_id
		        AND previous.strategy_id = ss.strategy_id
		        AND previous.calculation_date < ss.calculation_date
		      ORDER BY previous.calculation_date DESC
		      LIMIT 1
		  ), 0) < $1
		  AND ss.calculation_date = (
		      SELECT max(latest.calculation_date)
		      FROM stock_scores latest
		      WHERE latest.strategy_id = ss.strategy_id
		  )
		  ON CONFLICT (stock_id, strategy_id, alert_type, generated_date) DO NOTHING`,
		s.alertThreshold, fmt.Sprintf("Score crossed the configured research threshold of %g.", s.alertThreshold))
	if err != nil {
		return fmt.Errorf("generate score alerts: %w", err)
	}
	return nil
}
