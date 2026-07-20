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
		name    string
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
