package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/angelaschereraiza/trend_radar/backend/internal/domain"
)

type Stocks struct {
	db *sql.DB
}

func NewStocks(db *sql.DB) *Stocks {
	return &Stocks{db: db}
}

func (s *Stocks) List(ctx context.Context, search, sector string, limit, offset int) (domain.StockPage, error) {
	var page domain.StockPage
	page.Limit, page.Offset = limit, offset
	if err := s.db.QueryRowContext(ctx, `
		SELECT count(*) FROM stocks
		WHERE ($1 = '' OR symbol ILIKE '%' || $1 || '%' OR name ILIKE '%' || $1 || '%')
		  AND ($2 = '' OR sector = $2)`, search, sector).Scan(&page.Total); err != nil {
		return page, fmt.Errorf("count stocks: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, symbol, exchange, name, currency, sector, industry, active
		FROM stocks
		WHERE ($1 = '' OR symbol ILIKE '%' || $1 || '%' OR name ILIKE '%' || $1 || '%')
		  AND ($2 = '' OR sector = $2)
		ORDER BY symbol ASC, id ASC
		LIMIT $3 OFFSET $4`, search, sector, limit, offset)
	if err != nil {
		return page, fmt.Errorf("list stocks: %w", err)
	}
	defer rows.Close()
	page.Items = make([]domain.Stock, 0)
	for rows.Next() {
		var stock domain.Stock
		if err := rows.Scan(&stock.ID, &stock.Symbol, &stock.Exchange, &stock.Name, &stock.Currency,
			&stock.Sector, &stock.Industry, &stock.Active); err != nil {
			return page, fmt.Errorf("scan stock: %w", err)
		}
		page.Items = append(page.Items, stock)
	}
	if err := rows.Err(); err != nil {
		return page, fmt.Errorf("read stocks: %w", err)
	}
	return page, nil
}

func (s *Stocks) Get(ctx context.Context, id uuid.UUID) (domain.Stock, error) {
	var stock domain.Stock
	err := s.db.QueryRowContext(ctx, `
		SELECT id, symbol, exchange, name, currency, sector, industry, active
		FROM stocks WHERE id = $1`, id).
		Scan(&stock.ID, &stock.Symbol, &stock.Exchange, &stock.Name, &stock.Currency,
			&stock.Sector, &stock.Industry, &stock.Active)
	if err != nil {
		return stock, err
	}
	return stock, nil
}

func (s *Stocks) Prices(ctx context.Context, id uuid.UUID, from, to *time.Time, limit, offset int) (domain.PricePage, error) {
	var page domain.PricePage
	page.Limit, page.Offset = limit, offset
	var countErr error
	if from == nil && to == nil {
		countErr = s.db.QueryRowContext(ctx, `
			SELECT count(*) FROM (
				SELECT DISTINCT trading_date FROM stock_prices WHERE stock_id = $1
			) dates`, id).Scan(&page.Total)
	} else {
		countErr = s.db.QueryRowContext(ctx, `
			SELECT count(*) FROM (
				SELECT DISTINCT trading_date FROM stock_prices
				WHERE stock_id = $1
				  AND ($2::date IS NULL OR trading_date >= $2)
				  AND ($3::date IS NULL OR trading_date <= $3)
			) dates`, id, from, to).Scan(&page.Total)
	}
	if countErr != nil {
		return page, fmt.Errorf("count stock prices: %w", countErr)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT ON (trading_date)
			trading_date, open::text, high::text, low::text, close::text,
			adjusted_close::text, volume, currency
		FROM stock_prices
		WHERE stock_id = $1
		  AND ($2::date IS NULL OR trading_date >= $2)
		  AND ($3::date IS NULL OR trading_date <= $3)
		ORDER BY trading_date DESC, imported_at DESC, data_version DESC
		LIMIT $4 OFFSET $5`, id, from, to, limit, offset)
	if err != nil {
		return page, fmt.Errorf("list stock prices: %w", err)
	}
	defer rows.Close()
	page.Items = make([]domain.PriceBar, 0)
	for rows.Next() {
		var item domain.PriceBar
		var date time.Time
		if err := rows.Scan(&date, &item.Open, &item.High, &item.Low, &item.Close,
			&item.AdjustedClose, &item.Volume, &item.Currency); err != nil {
			return page, fmt.Errorf("scan stock price: %w", err)
		}
		item.Date = date
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		return page, fmt.Errorf("read stock prices: %w", err)
	}
	return page, nil
}

func (s *Stocks) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM stocks WHERE id = $1)", id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check stock existence: %w", err)
	}
	return exists, nil
}
