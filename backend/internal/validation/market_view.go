package validation

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

type Price struct {
	Date  time.Time
	Open  decimal.Decimal
	Close decimal.Decimal
}

type MarketView struct {
	asOf   time.Time
	prices map[string][]Price
}

func NewMarketView(asOf time.Time, prices map[string][]Price) (MarketView, error) {
	if asOf.IsZero() {
		return MarketView{}, errors.New("market view requires an as-of date")
	}
	copyPrices := make(map[string][]Price, len(prices))
	for symbol, series := range prices {
		copied := append([]Price(nil), series...)
		sort.Slice(copied, func(i, j int) bool { return copied[i].Date.Before(copied[j].Date) })
		for i, price := range copied {
			if price.Date.After(asOf) {
				return MarketView{}, fmt.Errorf("future price %s for %s exceeds as-of date", price.Date.Format("2006-01-02"), symbol)
			}
			if i > 0 && !price.Date.After(copied[i-1].Date) {
				return MarketView{}, fmt.Errorf("duplicate price date for %s", symbol)
			}
		}
		copyPrices[symbol] = copied
	}
	return MarketView{asOf: asOf, prices: copyPrices}, nil
}

func (v MarketView) AsOf() time.Time {
	return v.asOf
}

func (v MarketView) PriceOnOrBefore(symbol string, date time.Time) (Price, bool) {
	if date.After(v.asOf) {
		return Price{}, false
	}
	series := v.prices[symbol]
	index := sort.Search(len(series), func(i int) bool { return series[i].Date.After(date) })
	if index == 0 {
		return Price{}, false
	}
	return series[index-1], true
}
