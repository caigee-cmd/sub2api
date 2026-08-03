-- Migration: 186_channel_monitor_first_token_latency
-- Channel monitor probes now use streaming requests and record the time to
-- the first content token (TTFT) in addition to the total request latency.
-- Old rows keep NULL (pre-streaming history).

ALTER TABLE channel_monitor_histories
    ADD COLUMN IF NOT EXISTS first_token_latency_ms INT;
