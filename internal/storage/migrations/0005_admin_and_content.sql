-- +goose Up
ALTER TABLE questions ADD COLUMN IF NOT EXISTS text_enc BYTEA;
ALTER TABLE questions ADD COLUMN IF NOT EXISTS sender_username_enc BYTEA;
ALTER TABLE questions ADD COLUMN IF NOT EXISTS file_id TEXT;

-- Таблица динамических настроек бота для управления из веб-панели
CREATE TABLE IF NOT EXISTS bot_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO bot_settings (key, value) VALUES
    ('rate_burst_per_hour', '5'),
    ('rate_daily_limit', '20'),
    ('maintenance_mode', 'false'),
    ('welcome_message', '👋 Привет! Отправь мне ссылку или алиас пользователя, чтобы задать ему анонимный вопрос.')
ON CONFLICT (key) DO NOTHING;

-- Учетные записи администраторов для входа в веб-панель
CREATE TABLE IF NOT EXISTS admin_accounts (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'admin',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
ALTER TABLE questions DROP COLUMN IF EXISTS text_enc;
ALTER TABLE questions DROP COLUMN IF EXISTS sender_username_enc;
ALTER TABLE questions DROP COLUMN IF EXISTS file_id;
DROP TABLE IF EXISTS bot_settings;
DROP TABLE IF EXISTS admin_accounts;
