package marketdata

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const yahooProviderName = "yahoo_finance"
const maxYahooResponseBytes = 32 << 20

type YahooProvider struct {
	client  *http.Client
	baseURL string
}

type yahooInstrument struct {
	symbol   Symbol
	ticker   string
	currency string
}

var yahooInstruments = []yahooInstrument{
	{
		symbol: Symbol{
			ID:       uuid.NewSHA1(uuid.NameSpaceURL, []byte("https://finance.yahoo.com/quote/NESN.SW")).String(),
			Symbol:   "NESN",
			Exchange: "SIX",
			Name:     "Nestlé S.A.",
			Currency: "CHF",
			Sector:   "Consumer Staples",
			Industry: "Food Products",
		},
		ticker: "NESN.SW", currency: "CHF",
	},
	{
		symbol: Symbol{
			ID:       uuid.NewSHA1(uuid.NameSpaceURL, []byte("https://finance.yahoo.com/quote/NOVN.SW")).String(),
			Symbol:   "NOVN",
			Exchange: "SIX",
			Name:     "Novartis AG",
			Currency: "CHF",
			Sector:   "Health Care",
			Industry: "Pharmaceuticals",
		},
		ticker: "NOVN.SW", currency: "CHF",
	},
	{
		symbol: Symbol{
			ID:       uuid.NewSHA1(uuid.NameSpaceURL, []byte("https://finance.yahoo.com/quote/ROG.SW")).String(),
			Symbol:   "ROG",
			Exchange: "SIX",
			Name:     "Roche Holding AG",
			Currency: "CHF",
			Sector:   "Health Care",
			Industry: "Pharmaceuticals",
		},
		ticker: "ROG.SW", currency: "CHF",
	},
	{
		symbol: Symbol{
			ID:       uuid.NewSHA1(uuid.NameSpaceURL, []byte("https://finance.yahoo.com/quote/SPY")).String(),
			Symbol:   "SPY",
			Exchange: "NYSE Arca",
			Name:     "SPDR S&P 500 ETF Trust",
			Currency: "USD",
			Sector:   "Benchmark",
			Industry: "Exchange Traded Fund",
		},
		ticker: "SPY", currency: "USD",
	},
}

type yahooChartResponse struct {
	Chart struct {
		Result []yahooChartResult `json:"result"`
		Error  *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

type yahooChartResult struct {
	Meta struct {
		Currency             string `json:"currency"`
		ExchangeTimezoneName string `json:"exchangeTimezoneName"`
	} `json:"meta"`
	Timestamp  []int64 `json:"timestamp"`
	Indicators struct {
		Quote    []yahooQuote    `json:"quote"`
		AdjClose []yahooAdjClose `json:"adjclose"`
	} `json:"indicators"`
	Events struct {
		Dividends map[string]struct {
			Amount float64 `json:"amount"`
		} `json:"dividends"`
		Splits map[string]struct {
			Numerator   float64 `json:"numerator"`
			Denominator float64 `json:"denominator"`
		} `json:"splits"`
	} `json:"events"`
}

type yahooQuote struct {
	Open   []*float64 `json:"open"`
	High   []*float64 `json:"high"`
	Low    []*float64 `json:"low"`
	Close  []*float64 `json:"close"`
	Volume []*int64   `json:"volume"`
}

type yahooAdjClose struct {
	AdjClose []*float64 `json:"adjclose"`
}

func NewYahooProvider() *YahooProvider {
	return &YahooProvider{
		client:  &http.Client{Timeout: 30 * time.Second},
		baseURL: "https://query1.finance.yahoo.com",
	}
}

func (p *YahooProvider) FetchBatch(ctx context.Context, from, to time.Time) (ImportBatch, error) {
	if p.client == nil || p.baseURL == "" {
		return ImportBatch{}, errors.New("Yahoo Finance client is not configured")
	}
	if from.IsZero() || to.IsZero() || from.After(to) {
		return ImportBatch{}, errors.New("Yahoo Finance import requires an ascending date range")
	}

	batch := ImportBatch{}
	observedDates := make(map[string]struct{})
	for _, instrument := range yahooInstruments {
		result, err := p.fetchChart(ctx, instrument.ticker, from, to)
		if err != nil {
			return ImportBatch{}, err
		}
		if result.Meta.Currency != instrument.currency {
			return ImportBatch{}, fmt.Errorf("Yahoo Finance returned currency %q for %s, expected %q",
				result.Meta.Currency, instrument.ticker, instrument.currency)
		}
		batch.Symbols = append(batch.Symbols, instrument.symbol)
		bars, actions, err := result.priceBars(instrument, from, to)
		if err != nil {
			return ImportBatch{}, err
		}
		batch.Prices = append(batch.Prices, bars...)
		batch.CorporateActions = append(batch.CorporateActions, actions...)
		if instrument.ticker == "SPY" {
			for _, bar := range bars {
				observedDates[bar.TradingDate.Format("2006-01-02")] = struct{}{}
			}
		}
	}

	fxResult, err := p.fetchChart(ctx, "USDCHF=X", from, to)
	if err != nil {
		return ImportBatch{}, err
	}
	if fxResult.Meta.Currency != "CHF" {
		return ImportBatch{}, fmt.Errorf("Yahoo Finance returned currency %q for USDCHF=X, expected CHF", fxResult.Meta.Currency)
	}
	fxRates, err := fxResult.fxRates(from, to, observedDates)
	if err != nil {
		return ImportBatch{}, err
	}
	batch.FXRates = fxRates
	if err := ValidateBatch(batch); err != nil {
		return ImportBatch{}, fmt.Errorf("validate Yahoo Finance data: %w", err)
	}
	return batch, nil
}

func (p *YahooProvider) fetchChart(ctx context.Context, ticker string, from, to time.Time) (yahooChartResult, error) {
	endpoint, err := url.Parse(strings.TrimRight(p.baseURL, "/") + "/v8/finance/chart/" + url.PathEscape(ticker))
	if err != nil {
		return yahooChartResult{}, fmt.Errorf("build Yahoo Finance URL for %s: %w", ticker, err)
	}
	query := endpoint.Query()
	query.Set("period1", strconv.FormatInt(dateOnly(from).Unix(), 10))
	query.Set("period2", strconv.FormatInt(dateOnly(to).AddDate(0, 0, 1).Unix(), 10))
	query.Set("interval", "1d")
	query.Set("events", "div,splits")
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return yahooChartResult{}, fmt.Errorf("create Yahoo Finance request for %s: %w", ticker, err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "TrendRadar local research application")
	response, err := p.client.Do(request)
	if err != nil {
		return yahooChartResult{}, fmt.Errorf("request Yahoo Finance chart for %s: %w", ticker, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return yahooChartResult{}, fmt.Errorf("Yahoo Finance chart request for %s returned HTTP %d", ticker, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxYahooResponseBytes+1))
	if err != nil {
		return yahooChartResult{}, fmt.Errorf("read Yahoo Finance chart for %s: %w", ticker, err)
	}
	if len(body) > maxYahooResponseBytes {
		return yahooChartResult{}, fmt.Errorf("Yahoo Finance chart response for %s exceeds %d bytes",
			ticker, maxYahooResponseBytes)
	}
	var payload yahooChartResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return yahooChartResult{}, fmt.Errorf("decode Yahoo Finance chart for %s: %w", ticker, err)
	}
	if payload.Chart.Error != nil {
		return yahooChartResult{}, fmt.Errorf("Yahoo Finance chart for %s: %s (%s)",
			ticker, payload.Chart.Error.Description, payload.Chart.Error.Code)
	}
	if len(payload.Chart.Result) != 1 {
		return yahooChartResult{}, fmt.Errorf("Yahoo Finance returned %d chart results for %s, expected one",
			len(payload.Chart.Result), ticker)
	}
	if len(payload.Chart.Result[0].Timestamp) == 0 {
		return yahooChartResult{}, fmt.Errorf("Yahoo Finance returned no daily observations for %s", ticker)
	}
	return payload.Chart.Result[0], nil
}

func (r yahooChartResult) priceBars(instrument yahooInstrument, from, to time.Time) ([]PriceBar, []CorporateAction, error) {
	location, err := time.LoadLocation(r.Meta.ExchangeTimezoneName)
	if err != nil {
		return nil, nil, fmt.Errorf("load Yahoo Finance timezone %q for %s: %w",
			r.Meta.ExchangeTimezoneName, instrument.ticker, err)
	}
	if len(r.Indicators.Quote) != 1 || len(r.Indicators.AdjClose) != 1 {
		return nil, nil, fmt.Errorf("Yahoo Finance chart for %s is missing quote or adjusted-close data", instrument.ticker)
	}
	quote, adjusted := r.Indicators.Quote[0], r.Indicators.AdjClose[0].AdjClose
	count := len(r.Timestamp)
	if len(quote.Open) != count || len(quote.High) != count || len(quote.Low) != count ||
		len(quote.Close) != count || len(quote.Volume) != count || len(adjusted) != count {
		return nil, nil, fmt.Errorf("Yahoo Finance chart arrays for %s do not align with timestamps", instrument.ticker)
	}

	bars := make([]PriceBar, 0, count)
	for i, timestamp := range r.Timestamp {
		date := time.Unix(timestamp, 0).In(location)
		date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
		if date.Before(dateOnly(from)) || date.After(dateOnly(to)) {
			continue
		}
		if quote.Open[i] == nil || quote.High[i] == nil || quote.Low[i] == nil ||
			quote.Close[i] == nil || quote.Volume[i] == nil || adjusted[i] == nil {
			return nil, nil, fmt.Errorf("Yahoo Finance chart for %s has missing values on %s",
				instrument.ticker, date.Format("2006-01-02"))
		}
		open, err := yahooDecimal(*quote.Open[i])
		if err != nil {
			return nil, nil, err
		}
		high, err := yahooDecimal(*quote.High[i])
		if err != nil {
			return nil, nil, err
		}
		low, err := yahooDecimal(*quote.Low[i])
		if err != nil {
			return nil, nil, err
		}
		closePrice, err := yahooDecimal(*quote.Close[i])
		if err != nil {
			return nil, nil, err
		}
		adjustedClose, err := yahooDecimal(*adjusted[i])
		if err != nil {
			return nil, nil, err
		}
		version, err := marketDataVersion(date, open, high, low, closePrice, adjustedClose, *quote.Volume[i])
		if err != nil {
			return nil, nil, fmt.Errorf("version Yahoo Finance bar for %s: %w", instrument.ticker, err)
		}
		bars = append(bars, PriceBar{
			Symbol: instrument.symbol.Symbol, TradingDate: date,
			Open: open, High: high, Low: low, Close: closePrice, AdjustedClose: adjustedClose,
			Volume: *quote.Volume[i], Currency: instrument.currency,
			Provider: yahooProviderName, DataVersion: version,
		})
	}
	actions, err := r.corporateActions(instrument, location, from, to)
	if err != nil {
		return nil, nil, err
	}
	return bars, actions, nil
}

func (r yahooChartResult) corporateActions(instrument yahooInstrument, location *time.Location, from, to time.Time) ([]CorporateAction, error) {
	actions := make([]CorporateAction, 0, len(r.Events.Dividends)+len(r.Events.Splits))
	for timestamp, dividend := range r.Events.Dividends {
		date, err := yahooEventDate(timestamp, location)
		if err != nil {
			return nil, fmt.Errorf("parse Yahoo Finance dividend date for %s: %w", instrument.ticker, err)
		}
		if date.Before(dateOnly(from)) || date.After(dateOnly(to)) {
			continue
		}
		amount, err := yahooDecimal(dividend.Amount)
		if err != nil {
			return nil, fmt.Errorf("parse Yahoo Finance dividend for %s: %w", instrument.ticker, err)
		}
		actions = append(actions, CorporateAction{
			Symbol: instrument.symbol.Symbol, Type: "dividend", EffectiveDate: date,
			Amount: amount, Provider: yahooProviderName,
		})
	}
	for timestamp, split := range r.Events.Splits {
		date, err := yahooEventDate(timestamp, location)
		if err != nil {
			return nil, fmt.Errorf("parse Yahoo Finance split date for %s: %w", instrument.ticker, err)
		}
		if date.Before(dateOnly(from)) || date.After(dateOnly(to)) {
			continue
		}
		if split.Numerator <= 0 || split.Denominator <= 0 ||
			math.IsNaN(split.Numerator) || math.IsInf(split.Numerator, 0) ||
			math.IsNaN(split.Denominator) || math.IsInf(split.Denominator, 0) {
			return nil, fmt.Errorf("Yahoo Finance returned invalid split ratio for %s on %s",
				instrument.ticker, date.Format("2006-01-02"))
		}
		ratio, err := yahooDecimal(split.Numerator / split.Denominator)
		if err != nil {
			return nil, err
		}
		actions = append(actions, CorporateAction{
			Symbol: instrument.symbol.Symbol, Type: "split", EffectiveDate: date,
			Ratio: ratio, Provider: yahooProviderName,
		})
	}
	return actions, nil
}

func (r yahooChartResult) fxRates(from, to time.Time, marketDates map[string]struct{}) ([]FXRate, error) {
	location, err := time.LoadLocation(r.Meta.ExchangeTimezoneName)
	if err != nil {
		return nil, fmt.Errorf("load Yahoo Finance FX timezone %q: %w", r.Meta.ExchangeTimezoneName, err)
	}
	if len(r.Indicators.Quote) != 1 || len(r.Indicators.Quote[0].Close) != len(r.Timestamp) {
		return nil, errors.New("Yahoo Finance USD/CHF chart is missing aligned daily closes")
	}
	rates := make([]FXRate, 0, len(marketDates))
	observed := make(map[string]struct{}, len(r.Timestamp))
	for i, timestamp := range r.Timestamp {
		date := time.Unix(timestamp, 0).In(location)
		date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
		if date.Before(dateOnly(from)) || date.After(dateOnly(to)) {
			continue
		}
		if r.Indicators.Quote[0].Close[i] == nil {
			return nil, fmt.Errorf("Yahoo Finance USD/CHF close is missing on %s", date.Format("2006-01-02"))
		}
		rate, err := yahooDecimal(*r.Indicators.Quote[0].Close[i])
		if err != nil || !rate.IsPositive() {
			return nil, fmt.Errorf("Yahoo Finance returned invalid USD/CHF rate on %s", date.Format("2006-01-02"))
		}
		key := date.Format("2006-01-02")
		observed[key] = struct{}{}
		version, err := marketDataVersion(date, rate)
		if err != nil {
			return nil, fmt.Errorf("version Yahoo Finance USD/CHF rate: %w", err)
		}
		rates = append(rates, FXRate{
			BaseCurrency: "USD", QuoteCurrency: "CHF", RateDate: date, Rate: rate,
			Provider: yahooProviderName, DataVersion: version,
		})
	}
	for date := range marketDates {
		if _, ok := observed[date]; !ok {
			return nil, fmt.Errorf("Yahoo Finance USD/CHF rate missing for SPY session %s", date)
		}
	}
	return rates, nil
}

func yahooEventDate(timestamp string, location *time.Location) (time.Time, error) {
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid Unix timestamp %q: %w", timestamp, err)
	}
	event := time.Unix(seconds, 0).In(location)
	return time.Date(event.Year(), event.Month(), event.Day(), 0, 0, 0, 0, time.UTC), nil
}

func yahooDecimal(value float64) (decimal.Decimal, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return decimal.Zero, fmt.Errorf("Yahoo Finance returned non-finite numeric data")
	}
	result, err := decimal.NewFromString(strconv.FormatFloat(value, 'f', -1, 64))
	if err != nil {
		return decimal.Zero, fmt.Errorf("parse Yahoo Finance numeric data: %w", err)
	}
	if !result.IsPositive() {
		return decimal.Zero, fmt.Errorf("Yahoo Finance returned a non-positive price or rate")
	}
	return result, nil
}

func marketDataVersion(date time.Time, values ...any) (string, error) {
	payload, err := json.Marshal(struct {
		Date   string `json:"date"`
		Values []any  `json:"values"`
	}{Date: date.Format("2006-01-02"), Values: values})
	if err != nil {
		return "", fmt.Errorf("encode Yahoo Finance bar version: %w", err)
	}
	hash := sha256.Sum256(payload)
	return "yahoo-v1-" + hex.EncodeToString(hash[:8]), nil
}

func dateOnly(date time.Time) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
}
