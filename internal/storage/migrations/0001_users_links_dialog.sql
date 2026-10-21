-- +goose Up

-- Роли как enum: значения известны на этапе компиляции, БД не даст записать мусор
CREATE TYPE user_role AS ENUM ('user', 'admin', 'sysadmin');

CREATE TABLE users (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tg_id         BIGINT       NOT NULL UNIQUE,       -- Telegram ID (нужен для доставки)
    tg_username   TEXT,                               -- @username на момент последнего апдейта
    display_name  TEXT         NOT NULL DEFAULT '',
    role          user_role    NOT NULL DEFAULT 'user',
    accepting     BOOLEAN      NOT NULL DEFAULT TRUE, -- приём вопросов вкл/выкл (пауза/отписка)
    banned        BOOLEAN      NOT NULL DEFAULT FALSE,-- бан на уровне всего бота (модерация)
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE links (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token      TEXT        NOT NULL UNIQUE,  -- случайный токен: q_<22 символа base64url>
    alias      TEXT,                          -- кастомный алиас, задаёт пользователь
    is_active  BOOLEAN     NOT NULL DEFAULT TRUE,
    clicks     BIGINT      NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Уникальность алиаса без учёта регистра: "Vasya" и "vasya" — одна ссылка
CREATE UNIQUE INDEX links_alias_lower_uq ON links (lower(alias)) WHERE alias IS NOT NULL;
CREATE INDEX links_user_id_idx ON links (user_id);

-- Состояние диалога с ботом (переживает рестарты)
CREATE TABLE dialog_states (
    tg_id      BIGINT      PRIMARY KEY,   -- чей диалог (может быть гость, не users.id)
    state      TEXT        NOT NULL,      -- asking | replying | idle ...
    payload    JSONB       NOT NULL DEFAULT '{}', -- link_id, question_id и т.п.
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE dialog_states;
DROP TABLE links;
DROP TABLE users;
DROP TYPE user_role;
