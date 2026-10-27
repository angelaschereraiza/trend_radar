package marketdata

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/angelaschereraiza/trend_radar/backend/internal/calendar"
)

type FixtureProvider struct {
	batch ImportBatch
}

func NewFixtureProvider() *FixtureProvider {
	provider := &FixtureProvider{}
	provider.batch = fixtureBatch()
	return provider
}

func (p *FixtureProvider) Batch() ImportBatch {
	return ImportBatch{
		Symbols:          append([]Symbol(nil), p.batch.Symbols...),
		Prices:           append([]PriceBar(nil), p.batch.Prices...),
		CorporateActions: append([]CorporateAction(nil), p.batch.CorporateActions...),
		FXRates:          append([]FXRate(nil), p.batch.FXRates...),
	}
}

func (p *FixtureProvider) GetSymbols(ctx context.Context) ([]Symbol, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]Symbol(nil), p.batch.Symbols...), nil
}

func (p *FixtureProvider) GetHistoricalPrices(ctx context.Context, symbol string, from, to time.Time) ([]PriceBar, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]PriceBar, 0)
	for _, bar := range p.batch.Prices {
		if bar.Symbol == symbol && !bar.TradingDate.Before(from) && !bar.TradingDate.After(to) {
			result = append(result, bar)
		}
	}
	return result, nil
}

func (p *FixtureProvider) GetCorporateActions(ctx context.Context, symbol string, from, to time.Time) ([]CorporateAction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]CorporateAction, 0)
	for _, action := range p.batch.CorporateActions {
		if action.Symbol == symbol && !action.EffectiveDate.Before(from) && !action.EffectiveDate.After(to) {
			result = append(result, action)
		}
	}
	return result, nil
}

func (p *FixtureProvider) GetDailyRates(ctx context.Context, base, quote string, from, to time.Time) ([]FXRate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]FXRate, 0)
	for _, rate := range p.batch.FXRates {
		if rate.BaseCurrency == base && rate.QuoteCurrency == quote &&
			!rate.RateDate.Before(from) && !rate.RateDate.After(to) {
			result = append(result, rate)
		}
	}
	return result, nil
}

func fixtureBatch() ImportBatch {
	symbols := []Symbol{
		{ID: "00000000-0000-4000-8000-000000000001", Symbol: "FIXA", Exchange: "NYSE", Name: "Fixture Alpha", Currency: "USD", Sector: "Technology"},
		{ID: "00000000-0000-4000-8000-000000000002", Symbol: "FIXB", Exchange: "NASDAQ", Name: "Fixture Beta", Currency: "USD", Sector: "Industrials"},
		{ID: "00000000-0000-4000-8000-000000000003", Symbol: "FIXC", Exchange: "NYSE", Name: "Fixture Gamma", Currency: "USD", Sector: "Healthcare"},
		{ID: "00000000-0000-4000-8000-000000000004", Symbol: "SPY", Exchange: "NYSE", Name: "Fixture Benchmark", Currency: "USD", Sector: "Benchmark"},
	}
	batch := ImportBatch{Symbols: symbols}
	start := time.Date(2022, time.January, 3, 0, 0, 0, 0, time.UTC)
	dataStart := time.Date(2023, time.January, 3, 0, 0, 0, 0, time.UTC)
	sessions := 0
	preHistorySessions := 0
	for calendarDays := 0; sessions < 300; calendarDays++ {
		date := start.AddDate(0, 0, calendarDays)
		open, err := calendar.IsTradingDay(calendar.FromTime(date))
		if err != nil {
			panic(fmt.Sprintf("fixture date is outside calendar support: %v", err))
		}
		if !open {
			continue
		}
		for i, symbol := range symbols {
			baseClose := decimal.NewFromInt(int64(100 + i*30))
			if symbol.Symbol == "SPY" {
				baseClose = decimal.NewFromInt(400)
			}
			closeValue := baseClose
			if date.Before(dataStart) {
				progress := decimal.NewFromInt(int64(252 + preHistorySessions)).Div(decimal.NewFromInt(504))
				closeValue = baseClose.Mul(progress)
			} else if symbol.Symbol == "SPY" {
				closeValue = baseClose.Add(decimal.NewFromInt(int64(sessions)).Div(decimal.NewFromInt(2)))
			} else {
				closeValue = baseClose.Add(decimal.NewFromInt(int64(sessions * (i + 1))))
			}
			bar := PriceBar{
				Symbol: symbol.Symbol, TradingDate: date,
				Open:  closeValue.Sub(decimal.NewFromInt(1)),
				High:  closeValue.Add(decimal.NewFromInt(1)),
				Low:   closeValue.Sub(decimal.NewFromInt(2)),
				Close: closeValue, AdjustedClose: closeValue,
				Volume: 1_000_000 + int64(i*1000), Currency: "USD",
				Provider: "fixture", DataVersion: "fixture-v4",
			}
			batch.Prices = append(batch.Prices, bar)
		}
		batch.FXRates = append(batch.FXRates, FXRate{
			BaseCurrency: "USD", QuoteCurrency: "CHF", RateDate: date,
			Rate: decimal.RequireFromString("0.88"), Provider: "fixture", DataVersion: "fixture-v4",
		})
		if date.Before(dataStart) {
			preHistorySessions++
		} else {
			sessions++
		}
	}
	return batch
}

func (p *FixtureProvider) String() string {
	return fmt.Sprintf("fixture provider (%d symbols)", len(p.batch.Symbols))
}
