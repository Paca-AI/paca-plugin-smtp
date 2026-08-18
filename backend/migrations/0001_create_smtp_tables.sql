-- 0001_create_smtp_tables.sql
-- Creates the SMTP plugin's tables. Run with
-- search_path = plugin_data_com_paca_smtp, public.

-- Singleton row (same trick as the core app's workspace_settings table) —
-- one SMTP server configuration per instance.
CREATE TABLE IF NOT EXISTS smtp_config (
    id             BOOLEAN     PRIMARY KEY DEFAULT TRUE CHECK (id),
    host           TEXT        NOT NULL DEFAULT '',
    port           INT         NOT NULL DEFAULT 587,
    username       TEXT        NOT NULL DEFAULT '',
    password_enc   TEXT        NOT NULL DEFAULT '',
    from_address   TEXT        NOT NULL DEFAULT '',
    from_name      TEXT        NOT NULL DEFAULT '',
    use_tls        BOOLEAN     NOT NULL DEFAULT TRUE,
    -- JSON array of optional (non-mandatory) event topics the admin has
    -- enabled. user.created is always sent once host/from_address are set —
    -- it has no "off" state, so it isn't tracked here. Defaults to all
    -- optional events enabled; the admin can disable individual ones.
    enabled_events JSONB       NOT NULL DEFAULT '["notification.assigned","notification.mentioned","notification.doc_mentioned","notification.task_description_mentioned"]',
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO smtp_config (id) VALUES (TRUE) ON CONFLICT (id) DO NOTHING;

-- Per-user opt-out list for the optional events above.
CREATE TABLE IF NOT EXISTS user_email_preferences (
    user_id         UUID        PRIMARY KEY,
    disabled_events JSONB       NOT NULL DEFAULT '[]',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
