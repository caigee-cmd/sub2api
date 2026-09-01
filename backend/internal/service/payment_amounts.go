package service

import (
	"math"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/shopspring/decimal"
)

const defaultBalanceRechargeMultiplier = 1.0
const maxRechargeBonusPercent = 1000.0

func normalizeBalanceRechargeMultiplier(multiplier float64) float64 {
	if math.IsNaN(multiplier) || math.IsInf(multiplier, 0) || multiplier <= 0 {
		return defaultBalanceRechargeMultiplier
	}
	return multiplier
}

// normalizeSubscriptionUSDToCNYRate 将非法值归一为 0（换算关闭）。
// 与余额倍率不同，0 是合法状态：表示订阅保持 price 直付的存量行为。
func normalizeSubscriptionUSDToCNYRate(rate float64) float64 {
	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 {
		return 0
	}
	return rate
}

func calculateCreditedBalance(paymentAmount, multiplier float64) float64 {
	return decimal.NewFromFloat(paymentAmount).
		Mul(decimal.NewFromFloat(normalizeBalanceRechargeMultiplier(multiplier))).
		Round(2).
		InexactFloat64()
}

func normalizeRechargeBonusPercent(percent float64) float64 {
	if math.IsNaN(percent) || math.IsInf(percent, 0) || percent < 0 {
		return 0
	}
	if percent > maxRechargeBonusPercent {
		return maxRechargeBonusPercent
	}
	return percent
}

func effectiveRechargeBonusPercent(enabled bool, percent float64) float64 {
	if !enabled {
		return 0
	}
	return normalizeRechargeBonusPercent(percent)
}

func normalizeRechargeBonusMaxAmount(maxAmount float64) float64 {
	if math.IsNaN(maxAmount) || math.IsInf(maxAmount, 0) || maxAmount < 0 {
		return 0
	}
	return maxAmount
}

func applyRechargeBonus(baseAmount, percent, maxAmount float64) float64 {
	pct := normalizeRechargeBonusPercent(percent)
	base := decimal.NewFromFloat(baseAmount).Round(2)
	if baseAmount <= 0 || pct <= 0 {
		return base.InexactFloat64()
	}
	bonus := base.Mul(decimal.NewFromFloat(pct).Div(decimal.NewFromInt(100))).Round(2)
	cap := normalizeRechargeBonusMaxAmount(maxAmount)
	if cap > 0 {
		maxBonus := decimal.NewFromFloat(cap).Round(2)
		if bonus.GreaterThan(maxBonus) {
			bonus = maxBonus
		}
	}
	return base.Add(bonus).Round(2).InexactFloat64()
}

func calculateCreditedBalanceWithBonus(paymentAmount, multiplier float64, bonusEnabled bool, bonusPercent, maxAmount float64) float64 {
	base := calculateCreditedBalance(paymentAmount, multiplier)
	return applyRechargeBonus(base, effectiveRechargeBonusPercent(bonusEnabled, bonusPercent), maxAmount)
}

func RechargeBonusApplies(enabled, firstOnly, isFirstRecharge bool) bool {
	if !enabled {
		return false
	}
	if firstOnly {
		return isFirstRecharge
	}
	return true
}

func calculateGatewayRefundAmount(orderAmount, payAmount, refundAmount float64, currency string) float64 {
	if orderAmount <= 0 || payAmount <= 0 || refundAmount <= 0 {
		return 0
	}
	fractionDigits := int32(payment.CurrencyMaxFractionDigits(currency))
	if math.Abs(refundAmount-orderAmount) <= paymentAmountToleranceForCurrency(currency) {
		return decimal.NewFromFloat(payAmount).Round(fractionDigits).InexactFloat64()
	}
	return decimal.NewFromFloat(payAmount).
		Mul(decimal.NewFromFloat(refundAmount)).
		Div(decimal.NewFromFloat(orderAmount)).
		Round(fractionDigits).
		InexactFloat64()
}
