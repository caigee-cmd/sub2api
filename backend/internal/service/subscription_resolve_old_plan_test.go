package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestResolveOldPlanForSubscription_PlanIDPriority plan_id 非空时优先用 plan_id
func TestResolveOldPlanForSubscription_PlanIDPriority(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	configSvc := &PaymentConfigService{entClient: client}

	// 建一个 plan（id 自增）
	plan, err := client.SubscriptionPlan.Create().
		SetGroupID(10).
		SetName("PRO Monthly").
		SetPrice(69).
		SetValidityDays(30).
		SetValidityUnit("day").
		SetForSale(true).
		Save(ctx)
	require.NoError(t, err)

	svc := &SubscriptionService{configService: configSvc}
	sub := &UserSubscription{
		GroupID:   10,
		PlanID:    &plan.ID, // 指向刚建的 plan
		StartsAt:  time.Now().AddDate(0, 0, -15),
		ExpiresAt: time.Now().AddDate(0, 0, 15),
	}

	got, err := svc.resolveOldPlanForSubscription(ctx, sub)
	require.NoError(t, err)
	require.Equal(t, plan.ID, got.ID)
	require.Equal(t, 69.0, got.Price)
}

// TestResolveOldPlanForSubscription_FallbackByDays plan_id 为空时按天数匹配
func TestResolveOldPlanForSubscription_FallbackByDays(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	configSvc := &PaymentConfigService{entClient: client}

	// group 10 下建一个 30 天的 plan
	plan, err := client.SubscriptionPlan.Create().
		SetGroupID(10).
		SetName("PRO Monthly").
		SetPrice(69).
		SetValidityDays(30).
		SetValidityUnit("day").
		SetForSale(true).
		Save(ctx)
	require.NoError(t, err)

	svc := &SubscriptionService{configService: configSvc}
	// 历史订阅：plan_id 为空，但 starts_at/expires_at 算出来是 30 天
	sub := &UserSubscription{
		GroupID:   10,
		PlanID:    nil, // 历史订阅没 plan_id
		StartsAt:  time.Now().AddDate(0, 0, -15),
		ExpiresAt: time.Now().AddDate(0, 0, 15), // 约 30 天
	}

	got, err := svc.resolveOldPlanForSubscription(ctx, sub)
	require.NoError(t, err)
	require.Equal(t, plan.ID, got.ID)
	require.Equal(t, 69.0, got.Price)
}

// TestResolveOldPlanForSubscription_NoEligiblePlan group 下无 for_sale plan 时报错
func TestResolveOldPlanForSubscription_NoEligiblePlan(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	configSvc := &PaymentConfigService{entClient: client}

	// group 99 下没有任何 plan
	svc := &SubscriptionService{configService: configSvc}
	sub := &UserSubscription{
		GroupID:   99,
		PlanID:    nil,
		StartsAt:  time.Now().AddDate(0, 0, -15),
		ExpiresAt: time.Now().AddDate(0, 0, 15),
	}

	_, err := svc.resolveOldPlanForSubscription(ctx, sub)
	require.ErrorIs(t, err, ErrSubscriptionUpgradeNoOldPlan)
}

// TestResolveOldPlanForSubscription_FallbackAnyDelta
// admin 直接分配的订阅天数可能不匹配任何 plan（1天/7天/365天等），
// 只要 group 下有 for_sale plan 就取最接近的那个，不限制天数误差。
func TestResolveOldPlanForSubscription_FallbackAnyDelta(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	configSvc := &PaymentConfigService{entClient: client}

	// group 10 下建一个 30 天的 plan
	plan, err := client.SubscriptionPlan.Create().
		SetGroupID(10).
		SetName("PRO Monthly").
		SetPrice(69).
		SetValidityDays(30).
		SetValidityUnit("day").
		SetForSale(true).
		Save(ctx)
	require.NoError(t, err)

	svc := &SubscriptionService{configService: configSvc}

	// 场景 1：订阅实际 32 天（plan 30 天，差 2 天）-> 应匹配
	t.Run("32 days matches 30-day plan", func(t *testing.T) {
		sub := &UserSubscription{
			GroupID:   10,
			PlanID:    nil,
			StartsAt:  time.Now().AddDate(0, 0, -2),
			ExpiresAt: time.Now().AddDate(0, 0, 30), // 共 32 天
		}
		got, err := svc.resolveOldPlanForSubscription(ctx, sub)
		require.NoError(t, err)
		require.Equal(t, plan.ID, got.ID)
	})

	// 场景 2：admin 分配 1 天的测试订阅 -> 应匹配 30 天 plan（差距大也接受）
	t.Run("1 day matches 30-day plan", func(t *testing.T) {
		sub := &UserSubscription{
			GroupID:   10,
			PlanID:    nil,
			StartsAt:  time.Now(),
			ExpiresAt: time.Now().AddDate(0, 0, 1), // 共 1 天
		}
		got, err := svc.resolveOldPlanForSubscription(ctx, sub)
		require.NoError(t, err)
		require.Equal(t, plan.ID, got.ID)
	})

	// 场景 3：admin 分配 365 天 -> 应匹配 30 天 plan（取最接近的）
	t.Run("365 days matches 30-day plan", func(t *testing.T) {
		sub := &UserSubscription{
			GroupID:   10,
			PlanID:    nil,
			StartsAt:  time.Now(),
			ExpiresAt: time.Now().AddDate(0, 0, 365),
		}
		got, err := svc.resolveOldPlanForSubscription(ctx, sub)
		require.NoError(t, err)
		require.Equal(t, plan.ID, got.ID)
	})
}
