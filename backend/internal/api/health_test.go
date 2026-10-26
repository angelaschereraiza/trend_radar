package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthLive(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewHandler(nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Body.String(); got != "{\"status\":\"live\"}\n" {
		t.Fatalf("body = %q, want live status JSON", got)
	}
}

func TestHealthReadyWithoutDatabase(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewHandler(nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestStockListRejectsInvalidPagination(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewHandler(nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/stocks?limit=201", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
}

func TestStockPricesRejectsInvalidDateRange(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/stocks/00000000-0000-4000-8000-000000000001/prices?from=2024-02-01&to=2024-01-01", nil)
	NewHandler(nil).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestRankingsRejectInvalidMinimumScoreWithoutDatabase(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewHandler(nil).ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "/api/v1/rankings?minimum_score=101", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestUnknownRouteReturnsNotFound(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewHandler(nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestCreateStrategyRejectsCapitalAboveLimit(t *testing.T) {
	body := `{"name":"test","version":"1","parameters":{"initial_capital_chf":"5000.01","max_positions":10,"rank_buffer":15,"max_holding_months":3,"min_trade_value_chf":"150"}}`
	recorder := httptest.NewRecorder()
	NewHandler(nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/strategies", strings.NewReader(body)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestCreateStrategyRejectsUnknownFields(t *testing.T) {
	body := `{"name":"test","version":"1","unexpected":true,"parameters":{"initial_capital_chf":"5000","max_positions":10,"rank_buffer":15,"max_holding_months":3,"min_trade_value_chf":"150"}}`
	recorder := httptest.NewRecorder()
	NewHandler(nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/strategies", strings.NewReader(body)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestCreateBacktestRequiresExplicitHoldoutConfirmation(t *testing.T) {
	body := `{"strategy_id":"00000000-0000-4000-8000-000000000001","start_date":"2024-01-01","end_date":"2024-02-01","dataset_role":"holdout"}`
	recorder := httptest.NewRecorder()
	NewHandler(nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/backtests", strings.NewReader(body)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}
