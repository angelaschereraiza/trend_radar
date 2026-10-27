package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	eodhdProviderName    = "eodhd"
	maxEODHDResponseSize = 64 << 20
)

var validEODHDCode = regexp.MustCompile(`^[A-Z0-9._-]{1,24}$`)

type EODHDProvider struct {
	client  *http.Client
	baseURL string
	apiKey  string
}

type EODHDExchange struct {
	Code     string `json:"Code"`
	Name     string `json:"Name"`
	Country  string `json:"Country"`
	Currency string `json:"Currency"`
}

type eodhdCatalogItem struct {
	Code     string `json:"Code"`
	Name     string `json:"Name"`
	Currency string `json:"Currency"`
	Exchange string `json:"Exchange"`
	Type     string `json:"Type"`
}

type eodhdPriceRow struct {
	Date          string      `json:"date"`
	Open          json.Number `json:"open"`
	High          json.Number `json:"high"`
	Low           json.Number `json:"low"`
	Close         json.Number `json:"close"`
	AdjustedClose json.Number `json:"adjusted_close"`
	Volume        json.Number `json:"volume"`
}

type eodhdDividend struct {
	Date            string      `json:"date"`
	UnadjustedValue json.Number `json:"unadjustedValue"`
	Currency        string      `json:"currency"`
}

type eodhdSplit struct {
	Date  string `json:"date"`
	Split string `json:"split"`
}

func NewEODHDProvider(apiKey string) *EODHDProvider {
	return &EODHDProvider{
		client:  &http.Client{Timeout: 30 * time.Second},
		baseURL: "https://eodhd.com/api",
		apiKey:  strings.TrimSpace(apiKey),
	}
}

func (p *EODHDProvider) FetchExchanges(ctx context.Context) ([]EODHDExchange, error) {
	var exchanges []EODHDExchange
	if err := p.getJSON(ctx, "/exchanges-list/", nil, &exchanges); err != nil {
		return nil, fmt.Errorf("fetch EODHD exchange list: %w", err)
	}
	if len(exchanges) == 0 {
		return nil, errors.New("EODHD returned an empty exchange list")
	}
	return exchanges, nil
}

func (p *EODHDProvider) FetchCatalog(ctx context.Context, exchange string) ([]Symbol, error) {
	exchange, err := validateEODHDCode(exchange)
	if err != nil {
		return nil, fmt.Errorf("invalid EODHD exchange code: %w", err)
	}
	query := url.Values{
		"fmt":  {"json"},
		"type": {"common_stock"},
	}
	var items []eodhdCatalogItem
	if err := p.getJSON(ctx, "/exchange-symbol-list/"+url.PathEscape(exchange), query, &items); err != nil {
		return nil, fmt.Errorf("fetch EODHD %s stock catalog: %w", exchange, err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("EODHD returned no common stocks for exchange %s", exchange)
	}

	symbols := make([]Symbol, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		code := strings.ToUpper(strings.TrimSpace(item.Code))
		if _, err := validateEODHDCode(code); err != nil {
			return nil, fmt.Errorf("EODHD returned invalid ticker code %q for %s: %w", item.Code, exchange, err)
		}
		if item.Currency == "" || len(item.Currency) != 3 {
			return nil, fmt.Errorf("EODHD returned invalid currency %q for %s.%s", item.Currency, code, exchange)
		}
		if _, ok := seen[code]; ok {
			return nil, fmt.Errorf("EODHD returned duplicate ticker %s for exchange %s", code, exchange)
		}
		seen[code] = struct{}{}
		market := canonicalEODHDExchange(exchange)
		if item.Exchange != "" {
			market = canonicalEODHDListingExchange(item.Exchange)
		}
		symbols = append(symbols, Symbol{
			ID:       uuid.NewSHA1(uuid.NameSpaceURL, []byte("https://trend-radar.local/stock/"+market+"/"+code)).String(),
			Symbol:   code,
			Exchange: market,
			Name:     strings.TrimSpace(item.Name),
			Currency: strings.ToUpper(item.Currency),
			Industry: strings.TrimSpace(item.Type),
		})
	}
	return symbols, nil
}

func (p *EODHDProvider) FetchHistory(ctx context.Context, exchange, ticker string, from, to time.Time) (ImportBatch, error) {
	exchange, err := validateEODHDCode(exchange)
	if err != nil {
		return ImportBatch{}, fmt.Errorf("invalid EODHD exchange code: %w", err)
	}
	ticker, err = validateEODHDCode(ticker)
	if err != nil {
		return ImportBatch{}, fmt.Errorf("invalid EODHD ticker: %w", err)
	}
	if from.IsZero() || to.IsZero() || dateOnly(from).After(dateOnly(to)) {
		return ImportBatch{}, errors.New("EODHD import requires an ascending date range")
	}

	catalog, err := p.FetchCatalog(ctx, exchange)
	if err != nil {
		return ImportBatch{}, err
	}
	var instrument *Symbol
	for i := range catalog {
		if catalog[i].Symbol == ticker {
			instrument = &catalog[i]
			break
		}
	}
	if instrument == nil {
		return ImportBatch{}, fmt.Errorf("ticker %s is not listed as an active common stock on EODHD exchange %s", ticker, exchange)
	}

	query := url.Values{
		"api_token": {p.apiKey},
		"from":      {dateOnly(from).Format("2006-01-02")},
		"to":        {dateOnly(to).Format("2006-01-02")},
		"period":    {"d"},
		"fmt":       {"json"},
	}
	path := "/eod/" + url.PathEscape(ticker+"."+exchange)
	var rows []eodhdPriceRow
	if err := p.getJSON(ctx, path, query, &rows); err != nil {
		return ImportBatch{}, fmt.Errorf("fetch EODHD history for %s.%s: %w", ticker, exchange, err)
	}
	if len(rows) == 0 {
		return ImportBatch{}, fmt.Errorf("EODHD returned no daily prices for %s.%s in the requested date range", ticker, exchange)
	}

	batch := ImportBatch{Symbols: []Symbol{*instrument}}
	for i, row := range rows {
		date, err := time.Parse("2006-01-02", row.Date)
		if err != nil {
			return ImportBatch{}, fmt.Errorf("parse EODHD date %q for %s.%s: %w", row.Date, ticker, exchange, err)
		}
		if date.Before(dateOnly(from)) || date.After(dateOnly(to)) {
			return ImportBatch{}, fmt.Errorf("EODHD returned out-of-range price date %s for %s.%s",
				row.Date, ticker, exchange)
		}
		open, err := eodhdDecimal(row.Open)
		if err != nil {
			return ImportBatch{}, fmt.Errorf("parse EODHD open for %s.%s on %s: %w", ticker, exchange, row.Date, err)
		}
		high, err := eodhdDecimal(row.High)
		if err != nil {
			return ImportBatch{}, fmt.Errorf("parse EODHD high for %s.%s on %s: %w", ticker, exchange, row.Date, err)
		}
		low, err := eodhdDecimal(row.Low)
		if err != nil {
			return ImportBatch{}, fmt.Errorf("parse EODHD low for %s.%s on %s: %w", ticker, exchange, row.Date, err)
		}
		closePrice, err := eodhdDecimal(row.Close)
		if err != nil {
			return ImportBatch{}, fmt.Errorf("parse EODHD close for %s.%s on %s: %w", ticker, exchange, row.Date, err)
		}
		adjustedClose, err := eodhdDecimal(row.AdjustedClose)
		if err != nil {
			return ImportBatch{}, fmt.Errorf("parse EODHD adjusted close for %s.%s on %s: %w", ticker, exchange, row.Date, err)
		}
		volume, err := strconv.ParseInt(row.Volume.String(), 10, 64)
		if err != nil || volume < 0 {
			return ImportBatch{}, fmt.Errorf("parse EODHD volume %q for %s.%s on %s", row.Volume, ticker, exchange, row.Date)
		}
		version, err := marketDataVersion(date, open, high, low, closePrice, adjustedClose, volume)
		if err != nil {
			return ImportBatch{}, fmt.Errorf("version EODHD bar for %s.%s: %w", ticker, exchange, err)
		}
		batch.Prices = append(batch.Prices, PriceBar{
			Symbol: ticker, TradingDate: date.UTC(),
			Open: open, High: high, Low: low, Close: closePrice, AdjustedClose: adjustedClose,
			Volume: volume, Currency: instrument.Currency,
			Provider: eodhdProviderName, DataVersion: version,
		})
		if i > 0 && !batch.Prices[i-1].TradingDate.Before(date) {
			return ImportBatch{}, fmt.Errorf("EODHD returned duplicate or out-of-order dates for %s.%s", ticker, exchange)
		}
	}
	actionQuery := url.Values{
		"from": {dateOnly(from).Format("2006-01-02")},
		"to":   {dateOnly(to).Format("2006-01-02")},
		"fmt":  {"json"},
	}
	var dividends []eodhdDividend
	if err := p.getJSON(ctx, "/div/"+url.PathEscape(ticker+"."+exchange), actionQuery, &dividends); err != nil {
		return ImportBatch{}, fmt.Errorf("fetch EODHD dividends for %s.%s: %w", ticker, exchange, err)
	}
	for _, dividend := range dividends {
		date, err := parseEODHDActionDate(dividend.Date, from, to)
		if err != nil {
			return ImportBatch{}, fmt.Errorf("parse EODHD dividend date for %s.%s: %w", ticker, exchange, err)
		}
		if !strings.EqualFold(dividend.Currency, instrument.Currency) {
			return ImportBatch{}, fmt.Errorf("EODHD dividend currency %q for %s.%s does not match listing currency %s",
				dividend.Currency, ticker, exchange, instrument.Currency)
		}
		amount, err := eodhdDecimal(dividend.UnadjustedValue)
		if err != nil {
			return ImportBatch{}, fmt.Errorf("parse EODHD dividend for %s.%s on %s: %w", ticker, exchange, dividend.Date, err)
		}
		batch.CorporateActions = append(batch.CorporateActions, CorporateAction{
			Symbol: ticker, Type: "dividend", EffectiveDate: date,
			Amount: amount, Provider: eodhdProviderName,
		})
	}
	var splits []eodhdSplit
	if err := p.getJSON(ctx, "/splits/"+url.PathEscape(ticker+"."+exchange), actionQuery, &splits); err != nil {
		return ImportBatch{}, fmt.Errorf("fetch EODHD splits for %s.%s: %w", ticker, exchange, err)
	}
	for _, split := range splits {
		date, err := parseEODHDActionDate(split.Date, from, to)
		if err != nil {
			return ImportBatch{}, fmt.Errorf("parse EODHD split date for %s.%s: %w", ticker, exchange, err)
		}
		parts := strings.Split(split.Split, "/")
		if len(parts) != 2 {
			return ImportBatch{}, fmt.Errorf("EODHD returned invalid split ratio %q for %s.%s", split.Split, ticker, exchange)
		}
		numerator, err := decimal.NewFromString(parts[0])
		if err != nil {
			return ImportBatch{}, fmt.Errorf("parse EODHD split numerator %q: %w", parts[0], err)
		}
		denominator, err := decimal.NewFromString(parts[1])
		if err != nil || !numerator.IsPositive() || !denominator.IsPositive() {
			return ImportBatch{}, fmt.Errorf("EODHD returned invalid split ratio %q for %s.%s", split.Split, ticker, exchange)
		}
		batch.CorporateActions = append(batch.CorporateActions, CorporateAction{
			Symbol: ticker, Type: "split", EffectiveDate: date,
			Ratio: numerator.Div(denominator), Provider: eodhdProviderName,
		})
	}
	if err := ValidateBatch(batch); err != nil {
		return ImportBatch{}, fmt.Errorf("validate EODHD history for %s.%s: %w", ticker, exchange, err)
	}
	return batch, nil
}

func (p *EODHDProvider) getJSON(ctx context.Context, path string, query url.Values, destination any) error {
	if p.client == nil || p.baseURL == "" {
		return errors.New("EODHD client is not configured")
	}
	if p.apiKey == "" {
		return errors.New("EODHD_API_TOKEN is required")
	}
	endpoint, err := url.Parse(strings.TrimRight(p.baseURL, "/") + path)
	if err != nil {
		return fmt.Errorf("build EODHD URL: %w", err)
	}
	values := endpoint.Query()
	for key, entries := range query {
		for _, value := range entries {
			values.Add(key, value)
		}
	}
	values.Set("api_token", p.apiKey)
	endpoint.RawQuery = values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("create EODHD request for %s: %w", path, err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return fmt.Errorf("request EODHD %s: %s", path, strings.ReplaceAll(err.Error(), p.apiKey, "[redacted]"))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("EODHD request for %s returned HTTP %d", path, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxEODHDResponseSize+1))
	if err != nil {
		return fmt.Errorf("read EODHD response for %s: %w", path, err)
	}
	if len(body) > maxEODHDResponseSize {
		return fmt.Errorf("EODHD response for %s exceeds %d bytes", path, maxEODHDResponseSize)
	}
	if err := json.Unmarshal(body, destination); err != nil {
		return fmt.Errorf("decode EODHD response for %s: %w", path, err)
	}
	return nil
}

func validateEODHDCode(code string) (string, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !validEODHDCode.MatchString(code) {
		return "", fmt.Errorf("unsupported code %q", code)
	}
	return code, nil
}

func canonicalEODHDExchange(exchange string) string {
	switch exchange {
	case "SW", "XSWX":
		return "SIX"
	default:
		return exchange
	}
}

func canonicalEODHDListingExchange(exchange string) string {
	switch strings.ToUpper(strings.TrimSpace(exchange)) {
	case "SW", "SIX":
		return "SIX"
	case "NYSE ARCA", "NYSEARCA":
		return "NYSE Arca"
	case "NASDAQ":
		return "NASDAQ"
	case "NYSE":
		return "NYSE"
	default:
		return strings.TrimSpace(exchange)
	}
}

func eodhdDecimal(value json.Number) (decimal.Decimal, error) {
	parsed, err := decimal.NewFromString(value.String())
	if err != nil {
		return decimal.Zero, fmt.Errorf("invalid decimal %q: %w", value, err)
	}
	if !parsed.IsPositive() || math.IsNaN(parsed.InexactFloat64()) || math.IsInf(parsed.InexactFloat64(), 0) {
		return decimal.Zero, fmt.Errorf("non-positive or non-finite numeric value %q", value)
	}
	return parsed, nil
}

func parseEODHDActionDate(value string, from, to time.Time) (time.Time, error) {
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q: %w", value, err)
	}
	if date.Before(dateOnly(from)) || date.After(dateOnly(to)) {
		return time.Time{}, fmt.Errorf("date %s is outside requested range", value)
	}
	return date.UTC(), nil
}
