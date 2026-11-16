package service

import (
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/shopspring/decimal"
)

// ProrationResult 升级 proration 计算结果
type ProrationResult struct {
	// RemainingDays 当前订阅剩余天数（向下取整，最小 0）
	RemainingDays int `json:"remaining_days"`
	// TotalDays 当前订阅套餐总有效期天数（按 validity_unit 折算）
	TotalDays int `json:"total_days"`
	// Credit 剩余价值抵扣额（oldPlan.price × remaining/total × 未用额度比例，2 位小数）
	Credit float64 `json:"credit"`
	// UnusedRatio 当前月窗口未用额度比例（0–1）。无月限额时为 1。
	UnusedRatio float64 `json:"unused_ratio"`
	// Payable 实付金额（max(0, newPlan.price - credit)，2 位小数）
	Payable float64 `json:"payable"`
	// NewExpiresAt 新订阅过期时间（now + newPlan 有效期天数）
	NewExpiresAt time.Time `json:"new_expires_at"`
}

// ComputeUpgradeProration 计算升级 proration。
//
// 业务语义：
//   - 只允许升级：newPlan.price 必须 > oldPlan.price，否则返回 ErrSubscriptionUpgradeNotHigher
//   - proration 基数：oldPlan.price（当前套餐现价，非用户实付价）
//   - credit = oldPlan.price × remainingDays / totalDays × unusedRatio
//     unusedRatio 只看月额度：(月限额 - 本月已用) / 月限额。
//     日、周限额不参与。用得越多抵扣越少；月额度用满则不抵扣。
//     没有月限额时按 1（只看天数）。
//   - payable = max(0, newPlan.price - credit)
//   - 新有效期从 today 重算：newExpiresAt = now + newPlan 折算天数
//     剩余价值只用于抵扣差价，不折算进新订阅有效期
//
// 参数 now 由调用方传入（便于测试），生产环境传 time.Now()。
// sub.Group 需要带上月限额；缺失时 unusedRatio=1，退回纯按天折算。
func ComputeUpgradeProration(oldPlan, newPlan *dbent.SubscriptionPlan, sub *UserSubscription, now time.Time) (*ProrationResult, error) {
	if oldPlan == nil || newPlan == nil || sub == nil {
		return nil, ErrSubscriptionNilInput
	}

	// 只允许升级：新套餐价格必须高于旧套餐
	if newPlan.Price <= oldPlan.Price {
		return nil, ErrSubscriptionUpgradeNotHigher
	}

	totalDays := psComputeValidityDays(oldPlan.ValidityDays, oldPlan.ValidityUnit)
	if totalDays <= 0 {
		// 防御：套餐配了 0 天有效期，无法算 proration
		return nil, ErrSubscriptionUpgradeInvalidPlan
	}

	// 剩余天数（向下取整，最小 0）
	remainingDays := sub.DaysRemaining()
	if remainingDays < 0 {
		remainingDays = 0
	}
	if remainingDays > totalDays {
		// 防御：剩余天数不应超过总天数（可能因续期累加），按总天数封顶
		remainingDays = totalDays
	}

	// unusedRatio：只看月额度还没用掉的比例。
	unusedRatio := upgradeUnusedMonthlyRatio(sub)

	// credit = oldPlan.price × remaining / total × unusedRatio，2 位小数
	oldPrice := decimal.NewFromFloat(oldPlan.Price)
	credit := oldPrice.
		Mul(decimal.NewFromInt(int64(remainingDays))).
		Div(decimal.NewFromInt(int64(totalDays))).
		Mul(decimal.NewFromFloat(unusedRatio)).
		Round(2)

	// payable = max(0, newPlan.price - credit)，2 位小数
	newPrice := decimal.NewFromFloat(newPlan.Price)
	payable := newPrice.Sub(credit)
	if payable.IsNegative() {
		payable = decimal.Zero
	}
	payable = payable.Round(2)

	// 新有效期从今天重算
	newValidityDays := psComputeValidityDays(newPlan.ValidityDays, newPlan.ValidityUnit)
	newExpiresAt := now.AddDate(0, 0, newValidityDays)
	if newExpiresAt.After(MaxExpiresAt) {
		newExpiresAt = MaxExpiresAt
	}

	return &ProrationResult{
		RemainingDays: remainingDays,
		TotalDays:     totalDays,
		Credit:        credit.InexactFloat64(),
		UnusedRatio:   unusedRatio,
		Payable:       payable.InexactFloat64(),
		NewExpiresAt:  newExpiresAt,
	}, nil
}

// upgradeUnusedMonthlyRatio 返回当前月窗口还没花掉的额度比例，范围 [0, 1]。
//
// 只看月限额：(limit - used) / limit。日、周限额不参与。
// 用满或超额则为 0，抵扣归零。
// 没配月限额、或月窗口还没开始（用量尚未产生），按 1，退回纯按天折算。
func upgradeUnusedMonthlyRatio(sub *UserSubscription) float64 {
	if sub == nil || sub.Group == nil || sub.MonthlyWindowStart == nil {
		return 1
	}
	limit := sub.Group.MonthlyLimitUSD
	if limit == nil || *limit <= 0 {
		return 1
	}
	unused := 1 - sub.MonthlyUsageUSD / *limit
	if unused < 0 {
		return 0
	}
	if unused > 1 {
		return 1
	}
	return unused
}
