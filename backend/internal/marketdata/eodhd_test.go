package marketdata

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestEODHDProviderFetchHistory(t *testing.T) {
	firstDate := time.Date(2025, time.January, 2, 0, 0, 0, 0, time.UTC)
	secondDate := firstDate.AddDate(0, 0, 1)
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("api_token") != "test-token" {
			t.Errorf("EODHD API token was not sent")
		}
		switch r.URL.Path {
		case "/api/exchange-symbol-list/SW":
			if r.URL.Query().Get("type") != "common_stock" || r.URL.Query().Get("fmt") != "json" {
				t.Errorf("unexpected EODHD catalog query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`[{"Code":"NOVN","Name":"Novartis AG","Currency":"CHF","Exchange":"SW","Type":"Common Stock"}]`))
		case "/api/eod/NOVN.SW":
			if r.URL.Query().Get("from") != firstDate.Format("2006-01-02") ||
				r.URL.Query().Get("to") != secondDate.Format("2006-01-02") {
				t.Errorf("unexpected EODHD history range: %s", r.URL.RawQuery)
			}
			_, _ = fmt.Fprintf(w, `[
				{"date":"%s","open":90.1,"high":91.2,"low":89.9,"close":90.8,"adjusted_close":90.7,"volume":1000},
				{"date":"%s","open":90.8,"high":92.0,"low":90.2,"close":91.5,"adjusted_close":91.4,"volume":1200}
			]`, firstDate.Format("2006-01-02"), secondDate.Format("2006-01-02"))
		case "/api/div/NOVN.SW":
			_, _ = fmt.Fprintf(w, `[{"date":"%s","unadjustedValue":3.0,"currency":"CHF"}]`,
				secondDate.Format("2006-01-02"))
		case "/api/splits/NOVN.SW":
			_, _ = fmt.Fprintf(w, `[{"date":"%s","split":"3.000000/2.000000"}]`,
				firstDate.Format("2006-01-02"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewEODHDProvider("test-token")
	provider.client = server.Client()
	provider.baseURL = server.URL + "/api"
	batch, err := provider.FetchHistory(context.Background(), "sw", "novn", firstDate, secondDate)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateBatch(batch); err != nil {
		t.Fatalf("provider returned invalid batch: %v", err)
	}
	if requests != 4 || len(batch.Symbols) != 1 || len(batch.Prices) != 2 || len(batch.CorporateActions) != 2 {
		t.Fatalf("unexpected result: requests=%d symbols=%d bars=%d actions=%d",
			requests, len(batch.Symbols), len(batch.Prices), len(batch.CorporateActions))
	}
	if batch.Symbols[0].Exchange != "SIX" || batch.Symbols[0].Currency != "CHF" ||
		batch.Prices[0].Symbol != "NOVN" || batch.Prices[0].Provider != eodhdProviderName {
		t.Fatalf("unexpected Swiss stock mapping: %+v %+v", batch.Symbols[0], batch.Prices[0])
	}
	if batch.CorporateActions[0].Type != "dividend" ||
		!batch.CorporateActions[0].Amount.Equal(decimal.NewFromInt(3)) ||
		batch.CorporateActions[1].Type != "split" ||
		!batch.CorporateActions[1].Ratio.Equal(decimal.NewFromFloat(1.5)) {
		t.Fatalf("unexpected EODHD corporate actions: %+v", batch.CorporateActions)
	}
}

func TestEODHDProviderRequiresTokenAndListedTicker(t *testing.T) {
	ctx := context.Background()
	date := time.Date(2025, time.January, 2, 0, 0, 0, 0, time.UTC)
	if _, err := NewEODHDProvider("").FetchCatalog(ctx, "SW"); err == nil ||
		!strings.Contains(err.Error(), "EODHD_API_TOKEN is required") {
		t.Fatalf("missing token was not rejected: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"Code":"NESN","Name":"Nestle SA","Currency":"CHF","Exchange":"SW","Type":"Common Stock"}]`))
	}))
	defer server.Close()
	provider := NewEODHDProvider("test-token")
	provider.client = server.Client()
	provider.baseURL = server.URL
	if _, err := provider.FetchHistory(ctx, "SW", "NOVN", date, date); err == nil ||
		!strings.Contains(err.Error(), "not listed as an active common stock") {
		t.Fatalf("unlisted ticker was not rejected: %v", err)
	}
}

func TestEODHDRequestErrorDoesNotExposeToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close()

	provider := NewEODHDProvider("secret-test-token")
	provider.client = server.Client()
	provider.baseURL = server.URL
	_, err := provider.FetchCatalog(context.Background(), "SW")
	if err == nil {
		t.Fatal("request to closed test server unexpectedly succeeded")
	}
	if strings.Contains(err.Error(), "secret-test-token") {
		t.Fatalf("EODHD error exposed API token: %v", err)
	}
}
