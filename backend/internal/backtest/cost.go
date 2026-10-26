package backtest

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

type CostConfig struct {
	CommissionBPS   decimal.Decimal
	CommissionMin   decimal.Decimal
	FXSpreadBPS     decimal.Decimal
	StampDutyBPS    decimal.Decimal
	SlippageBPS     decimal.Decimal
	WithholdingRate decimal.Decimal
}

type CostBreakdown struct {
	Commission decimal.Decimal `json:"commission_chf"`
	FXCost     decimal.Decimal `json:"fx_cost_chf"`
	StampDuty  decimal.Decimal `json:"stamp_duty_chf"`
	Slippage   decimal.Decimal `json:"slippage_chf"`
	Total      decimal.Decimal `json:"total_cost_chf"`
}

func DefaultCosts() CostConfig {
	return CostConfig{
		CommissionBPS: decimal.NewFromInt(5), CommissionMin: decimal.NewFromInt(1),
		FXSpreadBPS: decimal.NewFromInt(25), StampDutyBPS: decimal.NewFromInt(30),
		SlippageBPS: decimal.NewFromInt(10), WithholdingRate: decimal.RequireFromString("0.15"),
	}
}

func (c CostConfig) Validate() error {
	values := []decimal.Decimal{c.CommissionBPS, c.CommissionMin, c.FXSpreadBPS, c.StampDutyBPS, c.SlippageBPS}
	for _, value := range values {
		if value.IsNegative() {
			return errors.New("cost values must be non-negative")
		}
	}
	if c.WithholdingRate.IsNegative() || c.WithholdingRate.GreaterThan(decimal.NewFromInt(1)) {
		return errors.New("withholding rate must be between zero and one")
	}
	return nil
}

func CalculateCosts(grossValueCHF decimal.Decimal, config CostConfig) (CostBreakdown, error) {
	if err := config.Validate(); err != nil {
		return CostBreakdown{}, err
	}
	if !grossValueCHF.IsPositive() {
		return CostBreakdown{}, errors.New("gross trade value must be positive")
	}
	bps := decimal.NewFromInt(10000)
	commission := grossValueCHF.Mul(config.CommissionBPS).Div(bps)
	if commission.LessThan(config.CommissionMin) {
		commission = config.CommissionMin
	}
	breakdown := CostBreakdown{
		Commission: commission,
		FXCost:     grossValueCHF.Mul(config.FXSpreadBPS).Div(bps),
		StampDuty:  grossValueCHF.Mul(config.StampDutyBPS).Div(bps),
		Slippage:   grossValueCHF.Mul(config.SlippageBPS).Div(bps),
	}
	breakdown.Total = breakdown.Commission.Add(breakdown.FXCost).Add(breakdown.StampDuty).Add(breakdown.Slippage)
	return breakdown, nil
}

func (c CostConfig) BreakEvenGrossReturn(positionValueCHF decimal.Decimal) (decimal.Decimal, error) {
	cost, err := CalculateCosts(positionValueCHF, c)
	if err != nil {
		return decimal.Zero, fmt.Errorf("calculate break-even costs: %w", err)
	}
	roundTrip := cost.Total.Mul(decimal.NewFromInt(2))
	return roundTrip.Div(positionValueCHF), nil
}
