package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSetAutoBalanceEnabled_UpdatesAndReturns 验证用户可切换自己订阅的自动余额开关。
func TestSetAutoBalanceEnabled_UpdatesAndReturns(t *testing.T) {
	sub := UserSubscription{ID: 7, UserID: 11, GroupID: 13, AutoBalanceEnabled: true}
	repo := &autoBalanceRepoStub{byID: map[int64]*UserSubscription{7: &sub}}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil, nil)

	updated, err := svc.SetAutoBalanceEnabled(context.Background(), 7, 11, false)
	require.NoError(t, err)
	require.False(t, updated.AutoBalanceEnabled)
	require.False(t, repo.byID[7].AutoBalanceEnabled, "repo value must be persisted")

	// 再切回 true
	updated, err = svc.SetAutoBalanceEnabled(context.Background(), 7, 11, true)
	require.NoError(t, err)
	require.True(t, updated.AutoBalanceEnabled)
}

// TestSetAutoBalanceEnabled_RejectsOtherUser 验证不能修改他人订阅。
func TestSetAutoBalanceEnabled_RejectsOtherUser(t *testing.T) {
	sub := UserSubscription{ID: 7, UserID: 11, GroupID: 13, AutoBalanceEnabled: true}
	repo := &autoBalanceRepoStub{byID: map[int64]*UserSubscription{7: &sub}}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil, nil)

	_, err := svc.SetAutoBalanceEnabled(context.Background(), 7, 999, false)
	require.Error(t, err)
	require.True(t, repo.byID[7].AutoBalanceEnabled, "value must not change on forbidden request")
}

// TestSetAutoBalanceEnabled_NotFound 验证订阅不存在时返回 NotFound。
func TestSetAutoBalanceEnabled_NotFound(t *testing.T) {
	repo := &autoBalanceRepoStub{byID: map[int64]*UserSubscription{}}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil, nil)

	_, err := svc.SetAutoBalanceEnabled(context.Background(), 99, 11, false)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)
}

// TestCheckAutoBalanceFallback 验证降级检查读取最近订阅的开关。
func TestCheckAutoBalanceFallback(t *testing.T) {
	enabledSub := UserSubscription{ID: 7, UserID: 11, GroupID: 13, AutoBalanceEnabled: true}
	disabledSub := UserSubscription{ID: 8, UserID: 11, GroupID: 15, AutoBalanceEnabled: false}
	repo := &autoBalanceRepoStub{
		byID:        map[int64]*UserSubscription{7: &enabledSub, 8: &disabledSub},
		byUserGroup: map[[2]int64]*UserSubscription{{11, 13}: &enabledSub, {11, 15}: &disabledSub},
	}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil, nil)

	require.True(t, svc.CheckAutoBalanceFallback(context.Background(), 11, 13))
	require.False(t, svc.CheckAutoBalanceFallback(context.Background(), 11, 15))
	require.False(t, svc.CheckAutoBalanceFallback(context.Background(), 11, 99), "no subscription → false")
}

// autoBalanceRepoStub 是 SetAutoBalanceEnabled / CheckAutoBalanceFallback 测试用的内存 stub。
type autoBalanceRepoStub struct {
	userSubRepoNoop
	byID        map[int64]*UserSubscription
	byUserGroup map[[2]int64]*UserSubscription
}

func (r *autoBalanceRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	if sub, ok := r.byID[id]; ok {
		cp := *sub
		return &cp, nil
	}
	return nil, ErrSubscriptionNotFound
}

func (r *autoBalanceRepoStub) GetByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	if sub, ok := r.byUserGroup[[2]int64{userID, groupID}]; ok {
		cp := *sub
		return &cp, nil
	}
	return nil, ErrSubscriptionNotFound
}

func (r *autoBalanceRepoStub) UpdateAutoBalance(_ context.Context, id int64, enabled bool) error {
	sub, ok := r.byID[id]
	if !ok {
		return ErrSubscriptionNotFound
	}
	sub.AutoBalanceEnabled = enabled
	return nil
}
