package service

import (
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

// newSubWithRemainingDays 构造一个剩余约 N 天的订阅。
// DaysRemaining() 用真实 time.Now() 向下取整，所以这里用 now.AddDate(0,0,N+1)
// 留 1 天缓冲避免测试执行耗时导致向下取整少 1 天。
func newSubWithRemainingDays(days int) *UserSubscription {
	return &UserSubscription{
		ExpiresAt: time.Now().AddDate(0, 0, days+1),
	}
}

// assertRoundedTo2 检查浮点数恰好为 2 位小数（如 3.33、10.00、0.00）
func assertRoundedTo2(t *testing.T, name string, v float64) {
	t.Helper()
	s := strconv.FormatFloat(v, 'f', -1, 64)
	// 允许整数形式（如 "10"）或 2 位小数形式（如 "3.33"）
	dotIdx := -1
	for i, c := range s {
		if c == '.' {
			dotIdx = i
			break
		}
	}
	if dotIdx == -1 {
		return // 整数形式，OK
	}
	if len(s)-dotIdx-1 > 2 {
		t.Errorf("%s = %s, want at most 2 decimal places", name, s)
	}
}

func TestComputeUpgradeProration_HalfwayRemaining(t *testing.T) {
	// 30 天套餐，剩约 15 天，价格 30 元
	// credit = 30 × 15/30 = 15（容忍 DaysRemaining 向下取整 ±1）
	oldPlan := &dbent.SubscriptionPlan{Price: 30, ValidityDays: 30, ValidityUnit: "day"}
	newPlan := &dbent.SubscriptionPlan{Price: 60, ValidityDays: 30, ValidityUnit: "day"}
	now := time.Now()
	sub := newSubWithRemainingDays(15)

	result, err := ComputeUpgradeProration(oldPlan, newPlan, sub, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalDays != 30 {
		t.Errorf("TotalDays = %d, want 30", result.TotalDays)
	}
	// DaysRemaining 向下取整，可能 14 或 15
	if result.RemainingDays < 14 || result.RemainingDays > 16 {
		t.Errorf("RemainingDays = %d, want 14-16", result.RemainingDays)
	}
	// credit = 30 × remaining / 30 = remaining（整数天恰好等于 credit）
	wantCredit := float64(result.RemainingDays)
	if result.Credit != wantCredit {
		t.Errorf("Credit = %.2f, want %.2f", result.Credit, wantCredit)
	}
	wantPayable := 60.0 - wantCredit
	if result.Payable != wantPayable {
		t.Errorf("Payable = %.2f, want %.2f", result.Payable, wantPayable)
	}
	wantExpires := now.AddDate(0, 0, 30)
	if !result.NewExpiresAt.Equal(wantExpires) {
		t.Errorf("NewExpiresAt = %v, want %v", result.NewExpiresAt, wantExpires)
	}
}

func TestComputeUpgradeProration_WeekUnit(t *testing.T) {
	// validity_days=1, unit=week -> totalDays=7
	// 剩约 6 天，价格 70 元 -> credit = 70 × 6/7 = 60
	// 新套餐 140 元 -> payable = 140 - 60 = 80
	oldPlan := &dbent.SubscriptionPlan{Price: 70, ValidityDays: 1, ValidityUnit: "week"}
	newPlan := &dbent.SubscriptionPlan{Price: 140, ValidityDays: 1, ValidityUnit: "week"}
	now := time.Now()
	sub := newSubWithRemainingDays(6)

	result, err := ComputeUpgradeProration(oldPlan, newPlan, sub, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalDays != 7 {
		t.Errorf("TotalDays = %d, want 7", result.TotalDays)
	}
	if result.RemainingDays < 5 || result.RemainingDays > 7 {
		t.Errorf("RemainingDays = %d, want 5-7", result.RemainingDays)
	}
	// credit = 70 × remaining / 7，验证精度（2 位小数）
	wantCredit := float64(result.RemainingDays) * 70 / 7
	if result.Credit != wantCredit {
		t.Errorf("Credit = %.2f, want %.2f", result.Credit, wantCredit)
	}
	wantPayable := 140.0 - wantCredit
	if result.Payable != wantPayable {
		t.Errorf("Payable = %.2f, want %.2f", result.Payable, wantPayable)
	}
}

func TestComputeUpgradeProration_ZeroRemaining(t *testing.T) {
	// 已过期订阅，剩 0 天 -> credit = 0，payable = newPlan.price
	oldPlan := &dbent.SubscriptionPlan{Price: 30, ValidityDays: 30, ValidityUnit: "day"}
	newPlan := &dbent.SubscriptionPlan{Price: 60, ValidityDays: 30, ValidityUnit: "day"}
	now := time.Now()
	sub := &UserSubscription{
		ExpiresAt: now.AddDate(0, 0, -1), // 已过期 1 天
	}

	result, err := ComputeUpgradeProration(oldPlan, newPlan, sub, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RemainingDays != 0 {
		t.Errorf("RemainingDays = %d, want 0", result.RemainingDays)
	}
	if result.Credit != 0 {
		t.Errorf("Credit = %.2f, want 0", result.Credit)
	}
	if result.Payable != 60 {
		t.Errorf("Payable = %.2f, want 60.00", result.Payable)
	}
}

func TestComputeUpgradeProration_NotHigherRejected(t *testing.T) {
	// newPlan.price <= oldPlan.price 应该被拒绝
	cases := []struct {
		name     string
		oldPrice float64
		newPrice float64
	}{
		{"equal", 30, 30},
		{"lower", 60, 30},
	}
	now := time.Now()
	sub := newSubWithRemainingDays(15)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oldPlan := &dbent.SubscriptionPlan{Price: tc.oldPrice, ValidityDays: 30, ValidityUnit: "day"}
			newPlan := &dbent.SubscriptionPlan{Price: tc.newPrice, ValidityDays: 30, ValidityUnit: "day"}
			_, err := ComputeUpgradeProration(oldPlan, newPlan, sub, now)
			if err != ErrSubscriptionUpgradeNotHigher {
				t.Errorf("err = %v, want ErrSubscriptionUpgradeNotHigher", err)
			}
		})
	}
}

func TestComputeUpgradeProration_RemainingDaysCappedToTotal(t *testing.T) {
	// 防御：剩余天数 > 总天数（可能因续期累加），按总天数封顶
	// totalDays=30, 剩 45 天 -> remaining=30, credit = oldPlan.price × 30/30 = oldPlan.price
	oldPlan := &dbent.SubscriptionPlan{Price: 30, ValidityDays: 30, ValidityUnit: "day"}
	newPlan := &dbent.SubscriptionPlan{Price: 60, ValidityDays: 30, ValidityUnit: "day"}
	now := time.Now()
	sub := &UserSubscription{
		ExpiresAt: now.AddDate(0, 0, 45), // 剩 45 天 > 总 30 天
	}

	result, err := ComputeUpgradeProration(oldPlan, newPlan, sub, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RemainingDays != 30 {
		t.Errorf("RemainingDays = %d, want 30 (capped to total)", result.RemainingDays)
	}
	if result.Credit != 30 {
		t.Errorf("Credit = %.2f, want 30.00 (full old price)", result.Credit)
	}
	if result.Payable != 30 {
		t.Errorf("Payable = %.2f, want 30.00", result.Payable)
	}
	if result.UnusedRatio != 1 {
		t.Errorf("UnusedRatio = %.4f, want 1 (no quota limits)", result.UnusedRatio)
	}
}

func TestComputeUpgradeProration_UnusedQuotaReducesCredit(t *testing.T) {
	// 剩 15/30 天，月额度用了一半：credit = 30 × 15/30 × 0.5 = 7.5，而不是按天的 15。
	oldPlan := &dbent.SubscriptionPlan{Price: 30, ValidityDays: 30, ValidityUnit: "day"}
	newPlan := &dbent.SubscriptionPlan{Price: 60, ValidityDays: 30, ValidityUnit: "day"}
	now := time.Now()
	limit := 100.0
	window := now.AddDate(0, 0, -10)
	sub := newSubWithRemainingDays(15)
	sub.Group = &Group{MonthlyLimitUSD: &limit}
	sub.MonthlyWindowStart = &window
	sub.MonthlyUsageUSD = 50

	result, err := ComputeUpgradeProration(oldPlan, newPlan, sub, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.UnusedRatio != 0.5 {
		t.Errorf("UnusedRatio = %.4f, want 0.5", result.UnusedRatio)
	}
	wantCredit := float64(result.RemainingDays) * 0.5
	if result.Credit != wantCredit {
		t.Errorf("Credit = %.2f, want %.2f", result.Credit, wantCredit)
	}
	if result.Payable != 60-wantCredit {
		t.Errorf("Payable = %.2f, want %.2f", result.Payable, 60-wantCredit)
	}
}

func TestComputeUpgradeProration_IgnoresDailyAndWeeklyQuota(t *testing.T) {
	// 周额度只剩 20%，月额度还剩 80%：只按月额度抵扣，周额度不参与。
	oldPlan := &dbent.SubscriptionPlan{Price: 30, ValidityDays: 30, ValidityUnit: "day"}
	newPlan := &dbent.SubscriptionPlan{Price: 60, ValidityDays: 30, ValidityUnit: "day"}
	now := time.Now()
	monthly, weekly := 100.0, 50.0
	window := now.AddDate(0, 0, -3)
	sub := newSubWithRemainingDays(15)
	sub.Group = &Group{MonthlyLimitUSD: &monthly, WeeklyLimitUSD: &weekly}
	sub.MonthlyWindowStart = &window
	sub.WeeklyWindowStart = &window
	sub.MonthlyUsageUSD = 20
	sub.WeeklyUsageUSD = 40

	result, err := ComputeUpgradeProration(oldPlan, newPlan, sub, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.UnusedRatio != 0.8 {
		t.Errorf("UnusedRatio = %.4f, want 0.8 (monthly only)", result.UnusedRatio)
	}
	wantCredit := float64(result.RemainingDays) * 0.8
	if result.Credit != wantCredit {
		t.Errorf("Credit = %.2f, want %.2f", result.Credit, wantCredit)
	}
}

func TestComputeUpgradeProration_QuotaExhaustedNoCredit(t *testing.T) {
	// 天数还剩一半，但月额度已用满：不再抵扣，实付等于新套餐价。
	oldPlan := &dbent.SubscriptionPlan{Price: 30, ValidityDays: 30, ValidityUnit: "day"}
	newPlan := &dbent.SubscriptionPlan{Price: 60, ValidityDays: 30, ValidityUnit: "day"}
	now := time.Now()
	limit := 100.0
	window := now.AddDate(0, 0, -20)
	sub := newSubWithRemainingDays(15)
	sub.Group = &Group{MonthlyLimitUSD: &limit}
	sub.MonthlyWindowStart = &window
	sub.MonthlyUsageUSD = 100

	result, err := ComputeUpgradeProration(oldPlan, newPlan, sub, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RemainingDays == 0 {
		t.Fatal("RemainingDays = 0, want days still left")
	}
	if result.UnusedRatio != 0 {
		t.Errorf("UnusedRatio = %.4f, want 0", result.UnusedRatio)
	}
	if result.Credit != 0 {
		t.Errorf("Credit = %.2f, want 0", result.Credit)
	}
	if result.Payable != 60 {
		t.Errorf("Payable = %.2f, want 60", result.Payable)
	}
}

func TestComputeUpgradeProration_UnstartedWindowIgnored(t *testing.T) {
	// 限额配了但窗口还没开始：用量尚未产生，不把抵扣打成 0。
	oldPlan := &dbent.SubscriptionPlan{Price: 30, ValidityDays: 30, ValidityUnit: "day"}
	newPlan := &dbent.SubscriptionPlan{Price: 60, ValidityDays: 30, ValidityUnit: "day"}
	now := time.Now()
	limit := 100.0
	sub := newSubWithRemainingDays(15)
	sub.Group = &Group{MonthlyLimitUSD: &limit}

	result, err := ComputeUpgradeProration(oldPlan, newPlan, sub, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.UnusedRatio != 1 {
		t.Errorf("UnusedRatio = %.4f, want 1", result.UnusedRatio)
	}
	if result.Credit != float64(result.RemainingDays) {
		t.Errorf("Credit = %.2f, want %.2f", result.Credit, float64(result.RemainingDays))
	}
}

func TestComputeUpgradeProration_RoundingPrecision(t *testing.T) {
	// 验证 credit/payable 恰好 2 位小数（即使除不尽）
	// oldPlan.price = 10.00, totalDays=3, remaining=1 -> credit = 10 × 1/3 = 3.333... -> 3.33
	// newPlan.price = 20.00 -> payable = 20 - 3.33 = 16.67
	oldPlan := &dbent.SubscriptionPlan{Price: 10, ValidityDays: 3, ValidityUnit: "day"}
	newPlan := &dbent.SubscriptionPlan{Price: 20, ValidityDays: 30, ValidityUnit: "day"}
	now := time.Now()
	sub := newSubWithRemainingDays(1)

	result, err := ComputeUpgradeProration(oldPlan, newPlan, sub, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalDays != 3 {
		t.Errorf("TotalDays = %d, want 3", result.TotalDays)
	}
	assertRoundedTo2(t, "Credit", result.Credit)
	assertRoundedTo2(t, "Payable", result.Payable)
}

func TestComputeUpgradeProration_NilInput(t *testing.T) {
	now := time.Now()
	_, err := ComputeUpgradeProration(nil, nil, nil, now)
	if err != ErrSubscriptionNilInput {
		t.Errorf("err = %v, want ErrSubscriptionNilInput", err)
	}
}
