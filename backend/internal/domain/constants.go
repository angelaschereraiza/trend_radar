package domain

import (
	"time"

	"github.com/google/uuid"
)

const MaxCapitalCHF = 5000

type Stock struct {
	ID       uuid.UUID
	Symbol   string
	Exchange string
	Name     string
	Currency string
	Sector   string
	Industry string
	Active   bool
}

type StockPage struct {
	Items  []Stock
	Total  int
	Limit  int
	Offset int
}

type PriceBar struct {
	Date          time.Time
	Open          string
	High          string
	Low           string
	Close         string
	AdjustedClose string
	Volume        int64
	Currency      string
}

type PricePage struct {
	Items  []PriceBar
	Total  int
	Limit  int
	Offset int
}
