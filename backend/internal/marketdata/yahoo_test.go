package marketdata

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestYahooProviderFetchBatch(t *testing.T) {
	firstDate := time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC)
	secondDate := firstDate.AddDate(0, 0, 1)
	charts := map[string]string{
		"NESN.SW":  yahooTestChart("CHF", "Europe/Zurich", firstDate, secondDate, true),
		"SPY":      yahooTestChart("USD", "America/New_York", firstDate, secondDate, false),
		"USDCHF=X": yahooTestChart("CHF", "Europe/London", firstDate, secondDate, false),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("interval") != "1d" || r.URL.Query().Get("events") != "div,splits" {
			t.Errorf("unexpected Yahoo chart query: %s", r.URL.RawQuery)
		}
		ticker := strings.TrimPrefix(r.URL.Path, "/v8/finance/chart/")
		payload, ok := charts[ticker]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	provider := &YahooProvider{client: server.Client(), baseURL: server.URL}
	batch, err := provider.FetchBatch(context.Background(), firstDate, secondDate)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateBatch(batch); err != nil {
		t.Fatalf("provider returned invalid batch: %v", err)
	}
	if len(batch.Symbols) != 2 || len(batch.Prices) != 4 || len(batch.FXRates) != 2 {
		t.Fatalf("unexpected batch counts: symbols=%d prices=%d fx=%d",
			len(batch.Symbols), len(batch.Prices), len(batch.FXRates))
	}
	if batch.Prices[0].Symbol != "NESN" || batch.Prices[0].Currency != "CHF" ||
		batch.Prices[0].Provider != yahooProviderName || batch.Prices[0].DataVersion == "" {
		t.Fatalf("unexpected Nestlé bar metadata: %+v", batch.Prices[0])
	}
	if len(batch.CorporateActions) != 1 || batch.CorporateActions[0].Type != "dividend" ||
		batch.CorporateActions[0].Symbol != "NESN" {
		t.Fatalf("unexpected corporate actions: %+v", batch.CorporateActions)
	}
	if batch.FXRates[0].BaseCurrency != "USD" || batch.FXRates[0].QuoteCurrency != "CHF" {
		t.Fatalf("unexpected FX pair: %+v", batch.FXRates[0])
	}
}

func TestYahooProviderRejectsUnexpectedCurrency(t *testing.T) {
	date := time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(yahooTestChart("USD", "UTC", date, date, false)))
	}))
	defer server.Close()

	provider := &YahooProvider{client: server.Client(), baseURL: server.URL}
	if _, err := provider.FetchBatch(context.Background(), date, date); err == nil ||
		!strings.Contains(err.Error(), `expected "CHF"`) {
		t.Fatalf("unexpected currency was not rejected explicitly: %v", err)
	}
}

func yahooTestChart(currency, timezone string, first, second time.Time, dividend bool) string {
	firstTimestamp := time.Date(first.Year(), first.Month(), first.Day(), 18, 0, 0, 0, time.UTC)
	secondTimestamp := time.Date(second.Year(), second.Month(), second.Day(), 18, 0, 0, 0, time.UTC)
	timestamps := fmt.Sprintf("%d,%d", firstTimestamp.Unix(), secondTimestamp.Unix())
	events := `{}`
	if dividend {
		events = fmt.Sprintf(`{"dividends":{"%d":{"amount":0.3}}}`, secondTimestamp.Unix())
	}
	return fmt.Sprintf(`{
		"chart":{"result":[{
			"meta":{"currency":%q,"exchangeTimezoneName":%q},
			"timestamp":[%s],
			"indicators":{
				"quote":[{"open":[100,101],"high":[102,103],"low":[99,100],"close":[101,102],"volume":[1000,1200]}],
				"adjclose":[{"adjclose":[100.5,101.5]}]
			},
			"events":%s
		}],"error":null}
	}`, currency, timezone, timestamps, events)
}
