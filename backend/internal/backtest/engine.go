package backtest

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/angelaschereraiza/trend_radar/backend/internal/domain"
	"github.com/angelaschereraiza/trend_radar/backend/internal/strategy"
)

type Config struct {
	Strategy                     strategy.Config
	Costs                        CostConfig
	BankruptcyRecoveryRate       decimal.Decimal
	UnknownDelistingRecoveryRate decimal.Decimal
}

func DefaultConfig(strategyConfig strategy.Config) Config {
	return Config{
		Strategy:                     strategyConfig,
		Costs:                        DefaultCosts(),
		BankruptcyRecoveryRate:       decimal.Zero,
		UnknownDelistingRecoveryRate: decimal.RequireFromString("0.70"),
	}
}

type MarketDay struct {
	Date              time.Time
	DecisionDate      time.Time
	RankingsAsOfDate  time.Time
	Rebalance         bool
	Rankings          []strategy.Candidate
	OpenPricesUSD     map[string]decimal.Decimal
	ClosePricesUSD    map[string]decimal.Decimal
	USDCHF            decimal.Decimal
	BenchmarkOpenUSD  decimal.Decimal
	BenchmarkCloseUSD decimal.Decimal
	CorporateActions  []CorporateAction
}

type CorporateAction struct {
	Symbol string
	Type   string
	Reason string
	Amount decimal.Decimal
	Ratio  decimal.Decimal
}

type CorporateActionRecord struct {
	Symbol         string
	Date           time.Time
	Type           string
	QuantityBefore decimal.Decimal
	QuantityAfter  decimal.Decimal
	GrossCHF       decimal.Decimal
	WithholdingCHF decimal.Decimal
	CostBasisCHF   decimal.Decimal
}

type Order struct {
	Symbol       string
	Date         time.Time
	Side         string
	Quantity     decimal.Decimal
	Reason       string
	Status       string
	RejectReason string
}

type Fill struct {
	Symbol   string
	Date     time.Time
	Side     string
	Quantity decimal.Decimal
	PriceUSD decimal.Decimal
	FXRate   decimal.Decimal
	GrossCHF decimal.Decimal
	Costs    CostBreakdown
}

type LedgerEntry struct {
	Date       time.Time
	Type       string
	AmountCHF  decimal.Decimal
	BalanceCHF decimal.Decimal
}

type EquityPoint struct {
	Date         time.Time
	PortfolioCHF decimal.Decimal
	CashCHF      decimal.Decimal
	InvestedCHF  decimal.Decimal
	BenchmarkCHF decimal.Decimal
}

type Position struct {
	Symbol     string
	Quantity   decimal.Decimal
	EntryDate  time.Time
	EntryValue decimal.Decimal
}

type Result struct {
	Orders            []Order
	Fills             []Fill
	Ledger            []LedgerEntry
	Equity            []EquityPoint
	CorporateActions  []CorporateActionRecord
	FinalCashCHF      decimal.Decimal
	WithholdingTaxCHF decimal.Decimal
	Positions         map[string]Position
}

func Run(days []MarketDay, config Config) (Result, error) {
	if err := config.Strategy.Validate(); err != nil {
		return Result{}, err
	}
	if err := config.Costs.Validate(); err != nil {
		return Result{}, err
	}
	if !validRate(config.BankruptcyRecoveryRate) || !validRate(config.UnknownDelistingRecoveryRate) {
		return Result{}, errors.New("delisting recovery rates must be between zero and one")
	}
	if len(days) == 0 {
		return Result{}, errors.New("backtest requires at least one market day")
	}
	result := Result{Positions: make(map[string]Position)}
	cash := config.Strategy.InitialCapitalCHF
	ledger := makeLedger(&result, days[0].Date, "initial_capital", cash, cash)
	_ = ledger
	var benchmarkQuantity, benchmarkCash decimal.Decimal
	benchmarkInitialized := false
	var err error

	for i, day := range days {
		if day.Date.IsZero() || (i > 0 && !day.Date.After(days[i-1].Date)) {
			return Result{}, fmt.Errorf("market days must have unique ascending dates at index %d", i)
		}
		if day.USDCHF.IsNegative() {
			return Result{}, fmt.Errorf("negative FX rate on %s", day.Date.Format("2006-01-02"))
		}
		if day.Rebalance && (day.DecisionDate.IsZero() || !day.DecisionDate.Before(day.Date)) {
			return Result{}, fmt.Errorf("rebalance on %s must use a strictly earlier decision date", day.Date.Format("2006-01-02"))
		}
		if day.Rebalance && (day.RankingsAsOfDate.IsZero() || !day.RankingsAsOfDate.Equal(day.DecisionDate)) {
			return Result{}, fmt.Errorf("rebalance rankings must be calculated as of the decision date on %s",
				day.Date.Format("2006-01-02"))
		}
		cash, err = applyCorporateActions(day, config, &result, cash)
		if err != nil {
			return Result{}, err
		}
		if day.Rebalance {
			cash, err = rebalance(day, config, &result, cash)
			if err != nil {
				return Result{}, err
			}
		}
		if !benchmarkInitialized {
			if !day.BenchmarkOpenUSD.IsPositive() || !day.USDCHF.IsPositive() {
				return Result{}, fmt.Errorf("benchmark open price and FX rate are required on initial date %s",
					day.Date.Format("2006-01-02"))
			}
			unitValue := day.BenchmarkOpenUSD.Mul(day.USDCHF)
			quantity, costs, err := affordableQuantity(config.Strategy.InitialCapitalCHF, unitValue, config.Costs, true)
			if err != nil {
				return Result{}, err
			}
			gross := quantity.Mul(unitValue)
			benchmarkQuantity = quantity
			benchmarkCash = config.Strategy.InitialCapitalCHF.Sub(gross).Sub(costs.Total)
			benchmarkInitialized = true
		}
		point, err := markToMarket(day, cash, result.Positions, benchmarkQuantity, benchmarkCash)
		if err != nil {
			return Result{}, err
		}
		result.Equity = append(result.Equity, point)
	}
	result.FinalCashCHF = cash
	if err := validateLedger(result.Ledger, config.Strategy.InitialCapitalCHF, cash, result.Equity); err != nil {
		return Result{}, err
	}
	return result, nil
}

func applyCorporateActions(day MarketDay, config Config, result *Result, cash decimal.Decimal) (decimal.Decimal, error) {
	for _, action := range day.CorporateActions {
		if action.Symbol == "" {
			return cash, errors.New("corporate action requires a symbol")
		}
		position, held := result.Positions[action.Symbol]
		switch action.Type {
		case "split":
			if !action.Ratio.IsPositive() {
				return cash, fmt.Errorf("split ratio must be positive for %s", action.Symbol)
			}
			if !held {
				continue
			}
			quantityBefore := position.Quantity
			position.Quantity = position.Quantity.Mul(action.Ratio)
			result.Positions[action.Symbol] = position
			result.CorporateActions = append(result.CorporateActions, CorporateActionRecord{
				Symbol: action.Symbol, Date: day.Date, Type: action.Type,
				QuantityBefore: quantityBefore, QuantityAfter: position.Quantity,
			})
		case "dividend":
			if action.Amount.IsNegative() {
				return cash, fmt.Errorf("dividend amount must be non-negative for %s", action.Symbol)
			}
			if !held || action.Amount.IsZero() {
				continue
			}
			if !day.USDCHF.IsPositive() {
				return cash, fmt.Errorf("missing valid FX rate for dividend on %s", action.Symbol)
			}
			gross := position.Quantity.Mul(action.Amount).Mul(day.USDCHF)
			withholding := gross.Mul(config.Costs.WithholdingRate)
			cash = cash.Add(gross).Sub(withholding)
			appendCashLedger(result, day.Date, "dividend", gross)
			if withholding.IsPositive() {
				appendCashLedger(result, day.Date, "withholding_tax", withholding.Neg())
			}
			result.WithholdingTaxCHF = result.WithholdingTaxCHF.Add(withholding)
			result.CorporateActions = append(result.CorporateActions, CorporateActionRecord{
				Symbol: action.Symbol, Date: day.Date, Type: action.Type,
				QuantityBefore: position.Quantity, QuantityAfter: position.Quantity,
				GrossCHF: gross, WithholdingCHF: withholding,
			})
		case "delisting":
			if action.Amount.IsNegative() {
				return cash, fmt.Errorf("delisting amount must be non-negative for %s", action.Symbol)
			}
			if !held {
				continue
			}
			if !day.USDCHF.IsPositive() {
				return cash, fmt.Errorf("missing valid FX rate for delisting on %s", action.Symbol)
			}
			price := action.Amount
			if !price.IsPositive() {
				price = day.ClosePricesUSD[action.Symbol]
				recovery := config.UnknownDelistingRecoveryRate
				if action.Reason == "bankruptcy" {
					recovery = config.BankruptcyRecoveryRate
				}
				if price.IsPositive() {
					price = price.Mul(recovery)
				} else {
					price = decimal.Zero
				}
			}
			gross := position.Quantity.Mul(price).Mul(day.USDCHF)
			recordedGross := gross
			if gross.IsPositive() {
				costs, err := CalculateCosts(gross, config.Costs)
				if err != nil {
					return cash, err
				}
				if gross.LessThan(costs.Total) {
					recordedGross = decimal.Zero
					result.Orders = append(result.Orders, Order{
						Symbol: action.Symbol, Date: day.Date, Side: "sell", Quantity: position.Quantity,
						Reason: "delisting", Status: "rejected", RejectReason: "estimated costs exceed delisting proceeds",
					})
				} else {
					cash = cash.Add(gross).Sub(costs.Total)
					result.Fills = append(result.Fills, Fill{
						Symbol: action.Symbol, Date: day.Date, Side: "sell", Quantity: position.Quantity,
						PriceUSD: price, FXRate: day.USDCHF, GrossCHF: gross, Costs: costs,
					})
					result.Orders = append(result.Orders, Order{
						Symbol: action.Symbol, Date: day.Date, Side: "sell", Quantity: position.Quantity,
						Reason: "delisting", Status: "filled",
					})
					appendTradeLedger(result, day.Date, "sell", gross, costs, cash)
				}
			}
			delete(result.Positions, action.Symbol)
			result.CorporateActions = append(result.CorporateActions, CorporateActionRecord{
				Symbol: action.Symbol, Date: day.Date, Type: action.Type,
				QuantityBefore: position.Quantity, QuantityAfter: decimal.Zero,
				GrossCHF: recordedGross, CostBasisCHF: position.EntryValue,
			})
		case "merger":
			return cash, fmt.Errorf("merger action for %s requires an acquirer symbol and cannot be represented by this market-day model", action.Symbol)
		default:
			return cash, fmt.Errorf("unsupported corporate action type %q for %s", action.Type, action.Symbol)
		}
	}
	return cash, nil
}

func appendCashLedger(result *Result, date time.Time, kind string, amount decimal.Decimal) {
	balance := decimal.Zero
	if len(result.Ledger) > 0 {
		balance = result.Ledger[len(result.Ledger)-1].BalanceCHF
	}
	result.Ledger = append(result.Ledger, LedgerEntry{
		Date: date, Type: kind, AmountCHF: amount, BalanceCHF: balance.Add(amount),
	})
}

func validRate(rate decimal.Decimal) bool {
	return !rate.IsNegative() && !rate.GreaterThan(decimal.NewFromInt(1))
}

func rebalance(day MarketDay, config Config, result *Result, cash decimal.Decimal) (decimal.Decimal, error) {
	rankings := make(map[string]strategy.Candidate, len(day.Rankings))
	for _, candidate := range day.Rankings {
		if candidate.Symbol == "" || candidate.Rank < 1 {
			return cash, errors.New("rebalance candidates require symbol and positive rank")
		}
		if _, duplicate := rankings[candidate.Symbol]; duplicate {
			return cash, fmt.Errorf("duplicate rebalance candidate %q", candidate.Symbol)
		}
		rankings[candidate.Symbol] = candidate
	}
	held := make(map[string]time.Time, len(result.Positions))
	symbols := make([]string, 0, len(result.Positions))
	for symbol, position := range result.Positions {
		held[symbol] = position.EntryDate
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	for _, symbol := range symbols {
		position := result.Positions[symbol]
		candidate, ok := rankings[symbol]
		reason := ""
		switch {
		case monthsBetween(position.EntryDate, day.Date) >= config.Strategy.MaxHoldingMonths:
			reason = "max_holding"
		case !ok || !candidate.Eligible:
			reason = "rebalance_exit"
		case candidate.Rank > config.Strategy.RankBuffer:
			reason = "rebalance_exit"
		}
		if reason == "" {
			continue
		}
		price, exists := day.OpenPricesUSD[symbol]
		if !exists || !price.IsPositive() || !day.USDCHF.IsPositive() {
			result.Orders = append(result.Orders, Order{
				Symbol: symbol, Date: day.Date, Side: "sell", Quantity: position.Quantity,
				Reason: reason, Status: "rejected", RejectReason: "missing valid execution price or FX rate",
			})
			continue
		}
		gross := position.Quantity.Mul(price).Mul(day.USDCHF)
		if gross.LessThan(config.Strategy.MinTradeValueCHF) {
			result.Orders = append(result.Orders, Order{
				Symbol: symbol, Date: day.Date, Side: "sell", Quantity: position.Quantity,
				Reason: reason, Status: "rejected", RejectReason: "below minimum trade value",
			})
			continue
		}
		costs, err := CalculateCosts(gross, config.Costs)
		if err != nil {
			return cash, err
		}
		cash = cash.Add(gross).Sub(costs.Total)
		result.Fills = append(result.Fills, Fill{
			Symbol: symbol, Date: day.Date, Side: "sell", Quantity: position.Quantity,
			PriceUSD: price, FXRate: day.USDCHF, GrossCHF: gross, Costs: costs,
		})
		result.Orders = append(result.Orders, Order{Symbol: symbol, Date: day.Date, Side: "sell",
			Quantity: position.Quantity, Reason: reason, Status: "filled"})
		appendTradeLedger(result, day.Date, "sell", gross, costs, cash)
		delete(result.Positions, symbol)
		delete(held, symbol)
	}

	candidateList := make([]strategy.Candidate, 0, len(rankings))
	for _, candidate := range rankings {
		candidate.PriceUSD = day.OpenPricesUSD[candidate.Symbol]
		candidate.FXRate = day.USDCHF
		candidateList = append(candidateList, candidate)
	}
	selectionConfig := config.Strategy
	positionValue, err := portfolioValue(day, cash, result.Positions)
	if err != nil {
		return cash, err
	}
	if positionValue.IsPositive() && positionValue.LessThan(decimal.NewFromInt(domain.MaxCapitalCHF)) {
		selectionConfig.InitialCapitalCHF = positionValue
	}
	targets, _, err := strategy.Select(candidateList, held, day.Date, selectionConfig)
	if err != nil {
		return cash, err
	}
	slotValue := positionValue.Div(decimal.NewFromInt(int64(config.Strategy.MaxPositions)))
	for _, target := range targets {
		price, exists := day.OpenPricesUSD[target.Symbol]
		if !exists || !price.IsPositive() || !day.USDCHF.IsPositive() {
			result.Orders = append(result.Orders, Order{
				Symbol: target.Symbol, Date: day.Date, Side: "buy", Reason: "rebalance_entry",
				Status: "rejected", RejectReason: "missing valid execution price or FX rate",
			})
			continue
		}
		unitValue := price.Mul(day.USDCHF)
		budget := decimal.Min(slotValue, cash)
		quantity, costs, err := affordableQuantity(budget, unitValue, config.Costs, config.Strategy.FractionalShares)
		if err != nil {
			return cash, err
		}
		if !quantity.IsPositive() {
			result.Orders = append(result.Orders, Order{
				Symbol: target.Symbol, Date: day.Date, Side: "buy", Reason: "rebalance_entry",
				Status: "rejected", RejectReason: "one whole share exceeds position budget",
			})
			continue
		}
		gross := quantity.Mul(unitValue)
		if gross.LessThan(config.Strategy.MinTradeValueCHF) {
			result.Orders = append(result.Orders, Order{
				Symbol: target.Symbol, Date: day.Date, Side: "buy", Reason: "rebalance_entry",
				Status: "rejected", RejectReason: "insufficient cash or below minimum trade value",
			})
			continue
		}
		cash = cash.Sub(gross).Sub(costs.Total)
		result.Positions[target.Symbol] = Position{
			Symbol: target.Symbol, Quantity: quantity, EntryDate: day.Date, EntryValue: gross.Add(costs.Total),
		}
		result.Fills = append(result.Fills, Fill{
			Symbol: target.Symbol, Date: day.Date, Side: "buy", Quantity: quantity,
			PriceUSD: price, FXRate: day.USDCHF, GrossCHF: gross, Costs: costs,
		})
		result.Orders = append(result.Orders, Order{Symbol: target.Symbol, Date: day.Date,
			Side: "buy", Quantity: quantity, Reason: "rebalance_entry", Status: "filled"})
		appendTradeLedger(result, day.Date, "buy", gross, costs, cash)
	}
	return cash, nil
}

func appendTradeLedger(result *Result, date time.Time, side string, gross decimal.Decimal, costs CostBreakdown, balance decimal.Decimal) {
	grossAmount := gross
	if side == "buy" {
		grossAmount = gross.Neg()
	}
	result.Ledger = append(result.Ledger, LedgerEntry{Date: date, Type: side, AmountCHF: grossAmount})
	result.Ledger = append(result.Ledger,
		LedgerEntry{Date: date, Type: "commission", AmountCHF: costs.Commission.Neg()},
		LedgerEntry{Date: date, Type: "fx_cost", AmountCHF: costs.FXCost.Neg()},
		LedgerEntry{Date: date, Type: "stamp_duty", AmountCHF: costs.StampDuty.Neg()},
		LedgerEntry{Date: date, Type: "slippage", AmountCHF: costs.Slippage.Neg()},
	)
	start := len(result.Ledger) - 5
	balance = balance.Sub(result.Ledger[start].AmountCHF).
		Sub(result.Ledger[start+1].AmountCHF).
		Sub(result.Ledger[start+2].AmountCHF).
		Sub(result.Ledger[start+3].AmountCHF).
		Sub(result.Ledger[start+4].AmountCHF)
	for i := start; i < len(result.Ledger); i++ {
		balance = balance.Add(result.Ledger[i].AmountCHF)
		result.Ledger[i].BalanceCHF = balance
	}
}

func makeLedger(result *Result, date time.Time, kind string, amount, balance decimal.Decimal) LedgerEntry {
	entry := LedgerEntry{Date: date, Type: kind, AmountCHF: amount, BalanceCHF: balance}
	result.Ledger = append(result.Ledger, entry)
	return entry
}

func markToMarket(day MarketDay, cash decimal.Decimal, positions map[string]Position, benchmarkQuantity, benchmarkCash decimal.Decimal) (EquityPoint, error) {
	invested := decimal.Zero
	for symbol, position := range positions {
		price, ok := day.ClosePricesUSD[symbol]
		if !ok || !price.IsPositive() {
			return EquityPoint{}, fmt.Errorf("missing closing price for held position %s on %s", symbol, day.Date.Format("2006-01-02"))
		}
		if !day.USDCHF.IsPositive() {
			return EquityPoint{}, fmt.Errorf("missing valid FX rate on %s while positions are open", day.Date.Format("2006-01-02"))
		}
		invested = invested.Add(position.Quantity.Mul(price).Mul(day.USDCHF))
	}
	benchmark := benchmarkCash
	if benchmarkQuantity.IsPositive() {
		if !day.BenchmarkCloseUSD.IsPositive() || !day.USDCHF.IsPositive() {
			return EquityPoint{}, fmt.Errorf("missing benchmark close price or FX rate on %s",
				day.Date.Format("2006-01-02"))
		}
		benchmark = benchmark.Add(benchmarkQuantity.Mul(day.BenchmarkCloseUSD).Mul(day.USDCHF))
	}
	return EquityPoint{
		Date: day.Date, PortfolioCHF: cash.Add(invested), CashCHF: cash,
		InvestedCHF: invested, BenchmarkCHF: benchmark,
	}, nil
}

func portfolioValue(day MarketDay, cash decimal.Decimal, positions map[string]Position) (decimal.Decimal, error) {
	value := cash
	for symbol, position := range positions {
		price, ok := day.OpenPricesUSD[symbol]
		if !ok || !price.IsPositive() {
			price, ok = day.ClosePricesUSD[symbol]
		}
		if !ok || !price.IsPositive() || !day.USDCHF.IsPositive() {
			return decimal.Zero, fmt.Errorf("missing price or FX for portfolio valuation of %s", symbol)
		}
		value = value.Add(position.Quantity.Mul(price).Mul(day.USDCHF))
	}
	return value, nil
}

func validateLedger(ledger []LedgerEntry, initial, finalCash decimal.Decimal, equity []EquityPoint) error {
	balance := decimal.Zero
	for i := range ledger {
		if i > 0 && ledger[i].Date.Before(ledger[i-1].Date) {
			return fmt.Errorf("ledger dates are not chronological at entry %d", i)
		}
		balance = balance.Add(ledger[i].AmountCHF)
		if !balance.Equal(ledger[i].BalanceCHF) {
			return fmt.Errorf("ledger balance mismatch at entry %d: got %s, recorded %s", i, balance, ledger[i].BalanceCHF)
		}
	}
	if len(ledger) > 0 && !ledger[0].AmountCHF.Equal(initial) {
		return errors.New("ledger does not start with initial capital")
	}
	if !balance.Equal(finalCash) {
		return fmt.Errorf("ledger balance %s does not equal final cash %s", balance, finalCash)
	}
	ledgerIndex := 0
	balance = decimal.Zero
	for _, point := range equity {
		if point.CashCHF.IsNegative() {
			return fmt.Errorf("cash balance is negative on %s", point.Date.Format("2006-01-02"))
		}
		for ledgerIndex < len(ledger) && !ledger[ledgerIndex].Date.After(point.Date) {
			balance = balance.Add(ledger[ledgerIndex].AmountCHF)
			ledgerIndex++
		}
		if !balance.Equal(point.CashCHF) {
			return fmt.Errorf("cash ledger balance %s does not match equity cash %s on %s",
				balance, point.CashCHF, point.Date.Format("2006-01-02"))
		}
	}
	if ledgerIndex != len(ledger) {
		return errors.New("cash ledger contains entries after the final equity date")
	}
	return nil
}

func affordableQuantity(budget, unitValue decimal.Decimal, costsConfig CostConfig, fractional bool) (decimal.Decimal, CostBreakdown, error) {
	if !budget.IsPositive() || !unitValue.IsPositive() {
		return decimal.Zero, CostBreakdown{}, nil
	}
	maximum := budget.Div(unitValue)
	if !fractional {
		maximum = decimal.NewFromInt(maximum.IntPart())
		low, high := int64(0), maximum.IntPart()
		for low <= high {
			mid := low + (high-low)/2
			quantity := decimal.NewFromInt(mid)
			gross := quantity.Mul(unitValue)
			if mid == 0 {
				low = 1
				continue
			}
			costs, err := CalculateCosts(gross, costsConfig)
			if err != nil {
				return decimal.Zero, CostBreakdown{}, err
			}
			if gross.Add(costs.Total).LessThanOrEqual(budget) {
				low = mid + 1
			} else {
				high = mid - 1
			}
		}
		quantity := decimal.NewFromInt(high)
		if !quantity.IsPositive() {
			return decimal.Zero, CostBreakdown{}, nil
		}
		costs, err := CalculateCosts(quantity.Mul(unitValue), costsConfig)
		if err != nil {
			return decimal.Zero, CostBreakdown{}, err
		}
		return quantity, costs, nil
	}
	low, high := decimal.Zero, maximum
	for i := 0; i < 64; i++ {
		mid := low.Add(high).Div(decimal.NewFromInt(2))
		gross := mid.Mul(unitValue)
		costs, err := CalculateCosts(gross, costsConfig)
		if err != nil {
			return decimal.Zero, CostBreakdown{}, err
		}
		if gross.Add(costs.Total).LessThanOrEqual(budget) {
			low = mid
		} else {
			high = mid
		}
	}
	if !low.IsPositive() {
		return decimal.Zero, CostBreakdown{}, nil
	}
	gross := low.Mul(unitValue)
	costs, err := CalculateCosts(gross, costsConfig)
	if err != nil {
		return decimal.Zero, CostBreakdown{}, err
	}
	return low, costs, nil
}

func monthsBetween(start, end time.Time) int {
	return (end.Year()-start.Year())*12 + int(end.Month()-start.Month())
}
