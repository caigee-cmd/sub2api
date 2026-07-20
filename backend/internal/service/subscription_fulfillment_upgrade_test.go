package service

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

// TestFulfillUpgradeSubscription_SoftDeleteOldAndCreateNew 验证升级履约的核心行为：
// 软删旧订阅 + 建新订阅（带 plan_id）+ 返回 oldGroupID
func TestFulfillUpgradeSubscription_SoftDeleteOldAndCreateNew(t *testing.T) {
	ctx := context.Background()
	userID := int64(1001)
	oldGroupID := int64(7)
	newGroupID := int64(8)

	// 旧订阅：active，剩 15 天，属于 group 7
	subRepo := newSubscriptionUserSubRepoStub()
	oldSubID := int64(50)
	oldPlanID := int64(100)
	subRepo.seed(&UserSubscription{
		ID:        oldSubID,
		UserID:    userID,
		GroupID:   oldGroupID,
		PlanID:    &oldPlanID,
		StartsAt:  time.Now().AddDate(0, 0, -15),
		ExpiresAt: time.Now().AddDate(0, 0, 15),
		Status:    SubscriptionStatusActive,
	})

	// groupRepo stub：两个 group 都 active + subscription 类型
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: newGroupID, Status: payment.EntityStatusActive, SubscriptionType: SubscriptionTypeSubscription},
	}

	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil, nil)

	// 目标 plan：group 8，30 天有效期
	newPlan := &dbent.SubscriptionPlan{
		ID:           200,
		GroupID:      newGroupID,
		Price:        60,
		ValidityDays: 30,
		ValidityUnit: "day",
	}

	newSub, returnedOldGroupID, err := svc.FulfillUpgradeSubscription(ctx, userID, oldSubID, newPlan, "upgrade test note")
	require.NoError(t, err)
	require.NotNil(t, newSub)

	// 旧订阅应被软删（GetByID 返回 NotFound）
	_, err = subRepo.GetByID(ctx, oldSubID)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)

	// 新订阅应在 newGroupID 上，带 plan_id
	require.Equal(t, newGroupID, newSub.GroupID)
	require.NotNil(t, newSub.PlanID)
	require.Equal(t, int64(200), *newSub.PlanID)
	require.Equal(t, SubscriptionStatusActive, newSub.Status)

	// 返回的 oldGroupID 用于缓存失效
	require.Equal(t, oldGroupID, returnedOldGroupID)

	// 新订阅有效期应从今天起 30 天（不是旧订阅的剩余天数累加）
	expectedExpiresStart := time.Now().AddDate(0, 0, 30)
	require.WithinDuration(t, expectedExpiresStart, newSub.ExpiresAt, time.Minute)
}

// TestFulfillUpgradeSubscription_RejectsSameGroup 升级目标 group 与当前相同应拒绝
func TestFulfillUpgradeSubscription_RejectsSameGroup(t *testing.T) {
	ctx := context.Background()
	userID := int64(1001)
	groupID := int64(7)

	subRepo := newSubscriptionUserSubRepoStub()
	oldSubID := int64(50)
	oldPlanID := int64(100)
	subRepo.seed(&UserSubscription{
		ID:        oldSubID,
		UserID:    userID,
		GroupID:   groupID,
		PlanID:    &oldPlanID,
		ExpiresAt: time.Now().AddDate(0, 0, 15),
		Status:    SubscriptionStatusActive,
	})

	svc := NewSubscriptionService(&subscriptionGroupRepoStub{}, subRepo, nil, nil, nil, nil)

	// 目标 plan 与旧订阅同 group
	newPlan := &dbent.SubscriptionPlan{
		ID:      200,
		GroupID: groupID,
		Price:   60,
	}
	_, _, err := svc.FulfillUpgradeSubscription(ctx, userID, oldSubID, newPlan, "")
	require.ErrorIs(t, err, ErrSubscriptionUpgradeSameGroup)
}

// TestFulfillUpgradeSubscription_RejectsExpiredSubscription 旧订阅已过期应拒绝
func TestFulfillUpgradeSubscription_RejectsExpiredSubscription(t *testing.T) {
	ctx := context.Background()
	userID := int64(1001)

	subRepo := newSubscriptionUserSubRepoStub()
	oldSubID := int64(50)
	oldPlanID := int64(100)
	subRepo.seed(&UserSubscription{
		ID:        oldSubID,
		UserID:    userID,
		GroupID:   7,
		PlanID:    &oldPlanID,
		ExpiresAt: time.Now().AddDate(0, 0, -1), // 已过期
		Status:    SubscriptionStatusActive,
	})

	svc := NewSubscriptionService(&subscriptionGroupRepoStub{}, subRepo, nil, nil, nil, nil)
	newPlan := &dbent.SubscriptionPlan{ID: 200, GroupID: 8, Price: 60}

	_, _, err := svc.FulfillUpgradeSubscription(ctx, userID, oldSubID, newPlan, "")
	require.ErrorIs(t, err, ErrSubscriptionUpgradeNotActive)
}

// TestFulfillUpgradeSubscription_RejectsWrongUser 旧订阅不属于当前用户应拒绝
func TestFulfillUpgradeSubscription_RejectsWrongUser(t *testing.T) {
	ctx := context.Background()
	subRepo := newSubscriptionUserSubRepoStub()
	oldSubID := int64(50)
	oldPlanID := int64(100)
	subRepo.seed(&UserSubscription{
		ID:        oldSubID,
		UserID:    999, // 属于其他用户
		GroupID:   7,
		PlanID:    &oldPlanID,
		ExpiresAt: time.Now().AddDate(0, 0, 15),
		Status:    SubscriptionStatusActive,
	})

	svc := NewSubscriptionService(&subscriptionGroupRepoStub{}, subRepo, nil, nil, nil, nil)
	newPlan := &dbent.SubscriptionPlan{ID: 200, GroupID: 8, Price: 60}

	_, _, err := svc.FulfillUpgradeSubscription(ctx, 1001, oldSubID, newPlan, "")
	require.Error(t, err) // Forbidden
}
