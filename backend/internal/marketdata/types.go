package marketdata

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

type Symbol struct {
	ID       string
	Symbol   string
	Exchange string
	Name     string
	Currency string
	Sector   string
	Industry string
}

type PriceBar struct {
	Symbol        string
	TradingDate   time.Time
	Open          decimal.Decimal
	High          decimal.Decimal
	Low           decimal.Decimal
	Close         decimal.Decimal
	AdjustedClose decimal.Decimal
	Volume        int64
	Currency      string
	Provider      string
	DataVersion   string
}

type CorporateAction struct {
	Symbol        string
	Type          string
	EffectiveDate time.Time
	Amount        decimal.Decimal
	Ratio         decimal.Decimal
	Reason        string
	Provider      string
}

type FXRate struct {
	BaseCurrency  string
	QuoteCurrency string
	RateDate      time.Time
	Rate          decimal.Decimal
	Provider      string
	DataVersion   string
}

type MarketDataProvider interface {
	GetSymbols(ctx context.Context) ([]Symbol, error)
	GetHistoricalPrices(ctx context.Context, symbol string, from, to time.Time) ([]PriceBar, error)
	GetCorporateActions(ctx context.Context, symbol string, from, to time.Time) ([]CorporateAction, error)
}

type FXProvider interface {
	GetDailyRates(ctx context.Context, base, quote string, from, to time.Time) ([]FXRate, error)
}

type ImportBatch struct {
	Symbols          []Symbol
	Prices           []PriceBar
	CorporateActions []CorporateAction
	FXRates          []FXRate
}

type Snapshot struct {
	ContentHash        string    `json:"content_hash"`
	PriceRows          int       `json:"price_rows"`
	CorporateRows      int       `json:"corporate_action_rows"`
	FXRows             int       `json:"fx_rows"`
	RangeStart         time.Time `json:"range_start"`
	RangeEnd           time.Time `json:"range_end"`
	SurvivorshipBiased bool      `json:"survivorship_biased"`
}

func ValidateBatch(batch ImportBatch) error {
	symbols := make(map[string]struct{}, len(batch.Symbols))
	for i, symbol := range batch.Symbols {
		if symbol.ID == "" || symbol.Symbol == "" || symbol.Exchange == "" || symbol.Currency == "" {
			return fmt.Errorf("symbol %d is missing a required field", i)
		}
		if _, ok := symbols[symbol.Symbol]; ok {
			return fmt.Errorf("duplicate symbol %q", symbol.Symbol)
		}
		symbols[symbol.Symbol] = struct{}{}
	}
	seenPrices := make(map[string]struct{}, len(batch.Prices))
	for i, bar := range batch.Prices {
		if bar.Symbol == "" || bar.Provider == "" || bar.DataVersion == "" || bar.Currency == "" {
			return fmt.Errorf("price bar %d is missing identity metadata", i)
		}
		if _, ok := symbols[bar.Symbol]; len(batch.Symbols) > 0 && !ok {
			return fmt.Errorf("price bar %d references unknown symbol %q", i, bar.Symbol)
		}
		if bar.TradingDate.IsZero() || bar.TradingDate.Hour() != 0 || bar.TradingDate.Minute() != 0 || bar.TradingDate.Second() != 0 {
			return fmt.Errorf("price bar %d must use a date-only trading date", i)
		}
		if bar.Open.Sign() <= 0 || bar.High.Sign() <= 0 || bar.Low.Sign() <= 0 || bar.Close.Sign() <= 0 || bar.AdjustedClose.Sign() <= 0 {
			return fmt.Errorf("price bar %d has a non-positive price", i)
		}
		if bar.High.LessThan(bar.Low) || bar.High.LessThan(bar.Open) || bar.High.LessThan(bar.Close) ||
			bar.Low.GreaterThan(bar.Open) || bar.Low.GreaterThan(bar.Close) {
			return fmt.Errorf("price bar %d has inconsistent OHLC values", i)
		}
		if bar.Volume < 0 {
			return fmt.Errorf("price bar %d has negative volume", i)
		}
		key := fmt.Sprintf("%s|%s|%s|%s", bar.Symbol, bar.TradingDate.Format("2006-01-02"), bar.Provider, bar.DataVersion)
		if _, exists := seenPrices[key]; exists {
			return fmt.Errorf("duplicate price bar %s", key)
		}
		seenPrices[key] = struct{}{}
	}
	for i, rate := range batch.FXRates {
		if rate.BaseCurrency == "" || rate.QuoteCurrency == "" || rate.Provider == "" || rate.DataVersion == "" || rate.RateDate.IsZero() {
			return fmt.Errorf("FX rate %d is missing required fields", i)
		}
		if rate.Rate.Sign() <= 0 {
			return fmt.Errorf("FX rate %d must be positive", i)
		}
	}
	for i, action := range batch.CorporateActions {
		if action.Symbol == "" || action.Type == "" || action.Provider == "" || action.EffectiveDate.IsZero() {
			return fmt.Errorf("corporate action %d is missing required fields", i)
		}
		switch action.Type {
		case "split", "dividend", "delisting", "symbol_change", "merger":
		default:
			return fmt.Errorf("corporate action %d has unsupported type %q", i, action.Type)
		}
		if action.Type == "split" && action.Ratio.Sign() <= 0 {
			return fmt.Errorf("corporate action %d split ratio must be positive", i)
		}
	}
	return nil
}

func BuildSnapshot(batch ImportBatch, survivorshipBiased bool) (Snapshot, error) {
	if err := ValidateBatch(batch); err != nil {
		return Snapshot{}, err
	}
	canonical := ImportBatch{
		Symbols:          append([]Symbol(nil), batch.Symbols...),
		Prices:           append([]PriceBar(nil), batch.Prices...),
		CorporateActions: append([]CorporateAction(nil), batch.CorporateActions...),
		FXRates:          append([]FXRate(nil), batch.FXRates...),
	}
	sort.Slice(canonical.Symbols, func(i, j int) bool {
		if canonical.Symbols[i].Symbol != canonical.Symbols[j].Symbol {
			return canonical.Symbols[i].Symbol < canonical.Symbols[j].Symbol
		}
		return canonical.Symbols[i].ID < canonical.Symbols[j].ID
	})
	sort.Slice(canonical.Prices, func(i, j int) bool {
		a, b := canonical.Prices[i], canonical.Prices[j]
		if a.Symbol != b.Symbol {
			return a.Symbol < b.Symbol
		}
		if !a.TradingDate.Equal(b.TradingDate) {
			return a.TradingDate.Before(b.TradingDate)
		}
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
		return a.DataVersion < b.DataVersion
	})
	sort.Slice(canonical.CorporateActions, func(i, j int) bool {
		a, b := canonical.CorporateActions[i], canonical.CorporateActions[j]
		if a.Symbol != b.Symbol {
			return a.Symbol < b.Symbol
		}
		if !a.EffectiveDate.Equal(b.EffectiveDate) {
			return a.EffectiveDate.Before(b.EffectiveDate)
		}
		return a.Type < b.Type
	})
	sort.Slice(canonical.FXRates, func(i, j int) bool {
		a, b := canonical.FXRates[i], canonical.FXRates[j]
		if a.BaseCurrency != b.BaseCurrency {
			return a.BaseCurrency < b.BaseCurrency
		}
		if a.QuoteCurrency != b.QuoteCurrency {
			return a.QuoteCurrency < b.QuoteCurrency
		}
		return a.RateDate.Before(b.RateDate)
	})
	hash, err := canonicalHash(canonical)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{
		ContentHash: hash, PriceRows: len(batch.Prices),
		CorporateRows: len(batch.CorporateActions), FXRows: len(batch.FXRates),
		SurvivorshipBiased: survivorshipBiased,
	}
	for _, bar := range batch.Prices {
		if snapshot.RangeStart.IsZero() || bar.TradingDate.Before(snapshot.RangeStart) {
			snapshot.RangeStart = bar.TradingDate
		}
		if snapshot.RangeEnd.IsZero() || bar.TradingDate.After(snapshot.RangeEnd) {
			snapshot.RangeEnd = bar.TradingDate
		}
	}
	for _, rate := range batch.FXRates {
		if snapshot.RangeStart.IsZero() || rate.RateDate.Before(snapshot.RangeStart) {
			snapshot.RangeStart = rate.RateDate
		}
		if snapshot.RangeEnd.IsZero() || rate.RateDate.After(snapshot.RangeEnd) {
			snapshot.RangeEnd = rate.RateDate
		}
	}
	return snapshot, nil
}

func canonicalHash(batch ImportBatch) (string, error) {
	content, err := json.Marshal(batch)
	if err != nil {
		return "", fmt.Errorf("%w: %v", errors.New("unable to serialize snapshot input"), err)
	}
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:]), nil
}
