package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/angelaschereraiza/trend_radar/backend/internal/domain"
	"github.com/angelaschereraiza/trend_radar/backend/internal/repository"
)

type HealthHandler struct {
	db     *sql.DB
	stocks *repository.Stocks
	data   *repository.MarketData
}

func NewHandler(db *sql.DB) http.Handler {
	handler := &HealthHandler{db: db, stocks: repository.NewStocks(db), data: repository.NewMarketData(db)}
	return http.TimeoutHandler(HandlerWithOptions(handler, StdHTTPServerOptions{
		BaseURL: "/api/v1",
	}), 30*time.Second, `{"error":"request timed out"}`)
}

func (h *HealthHandler) GetHealthLive(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{Status: Live})
}

func (h *HealthHandler) GetHealthReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if h.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, HealthResponse{Status: NotReady})
		return
	}
	if err := h.db.PingContext(ctx); err != nil {
		slog.ErrorContext(ctx, "database readiness check failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, HealthResponse{Status: NotReady})
		return
	}
	writeJSON(w, http.StatusOK, HealthResponse{Status: Ready})
}

func (h *HealthHandler) GetStocks(w http.ResponseWriter, r *http.Request, params GetStocksParams) {
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
	search, sector := "", ""
	if params.Search != nil {
		search = *params.Search
	}
	if params.Sector != nil {
		sector = *params.Sector
	}
	result, err := h.stocks.List(r.Context(), search, sector, limit, offset)
	if err != nil {
		slog.ErrorContext(r.Context(), "list stocks failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to list stocks")
		return
	}
	writeJSON(w, http.StatusOK, stockPage(result))
}

func (h *HealthHandler) GetStock(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	stock, err := h.stocks.Get(r.Context(), uuid.UUID(id))
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "stock not found")
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "get stock failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to retrieve stock")
		return
	}
	writeJSON(w, http.StatusOK, apiStock(stock))
}

func (h *HealthHandler) GetStockPrices(w http.ResponseWriter, r *http.Request, id openapi_types.UUID, params GetStockPricesParams) {
	limit, offset := 500, 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}
	if limit < 1 || limit > 2000 || offset < 0 {
		writeError(w, http.StatusBadRequest, "invalid_pagination", "limit must be 1-2000 and offset must be non-negative")
		return
	}
	var from, to *time.Time
	if params.From != nil {
		value := params.From.Time
		from = &value
	}
	if params.To != nil {
		value := params.To.Time
		to = &value
	}
	if from != nil && to != nil && from.After(*to) {
		writeError(w, http.StatusBadRequest, "invalid_date_range", "from must not be after to")
		return
	}
	stockID := uuid.UUID(id)
	exists, err := h.stocks.Exists(r.Context(), stockID)
	if err != nil {
		slog.ErrorContext(r.Context(), "check stock failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to retrieve stock prices")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "not_found", "stock not found")
		return
	}
	result, err := h.stocks.Prices(r.Context(), stockID, from, to, limit, offset)
	if err != nil {
		slog.ErrorContext(r.Context(), "list stock prices failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to retrieve stock prices")
		return
	}
	writeJSON(w, http.StatusOK, pricePage(result))
}

func (h *HealthHandler) GetRankings(w http.ResponseWriter, r *http.Request, params GetRankingsParams) {
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
	strategyName, sector := "momentum_v1", ""
	if params.Strategy != nil {
		strategyName = *params.Strategy
	}
	if params.Sector != nil {
		sector = *params.Sector
	}
	if params.MinimumScore != nil && (*params.MinimumScore < 0 || *params.MinimumScore > 100) {
		writeError(w, http.StatusBadRequest, "invalid_score", "minimum_score must be between 0 and 100")
		return
	}
	var rankingDate *time.Time
	if params.Date != nil {
		value := params.Date.Time
		rankingDate = &value
	} else {
		var err error
		rankingDate, err = h.data.LatestRankingDate(r.Context(), strategyName)
		if err != nil {
			slog.ErrorContext(r.Context(), "find latest rankings failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "unable to retrieve rankings")
			return
		}
	}
	minimumScore := (*float64)(nil)
	if params.MinimumScore != nil {
		value := float64(*params.MinimumScore)
		minimumScore = &value
	}
	affordableOnly := params.AffordableOnly != nil && *params.AffordableOnly
	entries, total, err := h.data.Rankings(r.Context(), rankingDate, strategyName, minimumScore, sector, affordableOnly, limit, offset)
	if err != nil {
		slog.ErrorContext(r.Context(), "query stock rankings failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "unable to retrieve rankings")
		return
	}
	items := make([]RankingEntry, len(entries))
	for i, entry := range entries {
		items[i] = RankingEntry{
			StockId: openapi_types.UUID(entry.StockID), Symbol: entry.Symbol, Sector: entry.Sector,
			Score: float32(entry.Score), Rank: offset + i + 1, Affordable: entry.Affordable,
		}
		items[i].Components.Momentum = float32(entry.Components.Momentum)
		items[i].Components.RelativeStrength = float32(entry.Components.RelativeStrength)
		items[i].Components.TrendQuality = float32(entry.Components.TrendQuality)
		items[i].Components.RelativeVolume = float32(entry.Components.RelativeVolume)
		items[i].Components.VolatilityAdjustedMomentum = float32(entry.Components.VolatilityAdjustedMomentum)
	}
	writeJSON(w, http.StatusOK, RankingPage{Items: items, Total: total, Limit: limit, Offset: offset})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func apiStock(stock domain.Stock) Stock {
	return Stock{
		Id: openapi_types.UUID(stock.ID), Symbol: stock.Symbol, Exchange: stock.Exchange,
		Name: stock.Name, Currency: stock.Currency, Sector: stock.Sector,
		Industry: stock.Industry, Active: stock.Active,
	}
}

func stockPage(page domain.StockPage) StockPage {
	items := make([]Stock, len(page.Items))
	for i, stock := range page.Items {
		items[i] = apiStock(stock)
	}
	return StockPage{Items: items, Total: page.Total, Limit: page.Limit, Offset: page.Offset}
}

func pricePage(page domain.PricePage) PricePage {
	items := make([]PriceBar, len(page.Items))
	for i, item := range page.Items {
		items[i] = PriceBar{
			Date: openapi_types.Date{Time: item.Date}, Open: item.Open, High: item.High, Low: item.Low,
			Close: item.Close, AdjustedClose: item.AdjustedClose,
			Volume: item.Volume, Currency: item.Currency,
		}
	}
	return PricePage{Items: items, Total: page.Total, Limit: page.Limit, Offset: page.Offset}
}
