-- 175_subscription_upgrade.sql
-- 订阅升级功能：proration 抵扣差价
-- - user_subscriptions.plan_id：记录订阅由哪个套餐产生，升级时回溯原套餐算 proration
-- - payment_orders.upgrade_from_subscription_id：标记升级订单的源订阅
-- - payment_orders.proration_credit：源订阅剩余价值按天折算的抵扣额

ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS plan_id BIGINT;

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS upgrade_from_subscription_id BIGINT;

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS proration_credit DECIMAL(20,2) DEFAULT 0;
