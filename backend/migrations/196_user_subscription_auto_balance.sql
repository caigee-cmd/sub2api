-- Add auto_balance_enabled flag to user_subscriptions.
-- When true, requests fall back to balance billing when subscription quota is
-- exhausted or the subscription expires. Default TRUE backfills existing rows.
ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS auto_balance_enabled BOOLEAN NOT NULL DEFAULT TRUE;
