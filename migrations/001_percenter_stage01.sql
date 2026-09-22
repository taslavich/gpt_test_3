BEGIN;

ALTER TABLE campaigns
    ADD COLUMN IF NOT EXISTS type_model INTEGER NOT NULL DEFAULT 1;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint c
        JOIN pg_class t ON t.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = t.relnamespace
        WHERE c.conname = 'campaigns_type_model_check'
          AND t.relname = 'campaigns'
          AND n.nspname = current_schema()
    ) THEN
        ALTER TABLE campaigns
            ADD CONSTRAINT campaigns_type_model_check
            CHECK (type_model IN (1, 2, 3));
    END IF;
END $$;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS promo_spend_remaining NUMERIC NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS adv_promo_spend_events (
    event_id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    campaign_id TEXT NOT NULL,
    spend_delta NUMERIC NOT NULL CHECK (spend_delta > 0),
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_adv_promo_spend_events_applied_at
    ON adv_promo_spend_events(applied_at);

COMMIT;
