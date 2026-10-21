-- +goose Up

-- Подписки админов: админ X видит карточки отправителей вопросов пользователю Y
CREATE TABLE admin_watches (
    admin_id   BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_id  BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (admin_id, target_id)
);

-- Журнал административных действий (аудит)
CREATE TABLE audit_log (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id   BIGINT      NOT NULL REFERENCES users(id),
    action     TEXT        NOT NULL,      -- role_granted | role_revoked | watch_added | watch_removed | sender_revealed | user_banned | user_unbanned
    target_id  BIGINT      REFERENCES users(id),
    details    JSONB       NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX audit_log_created_idx ON audit_log (created_at DESC);

-- +goose Down
DROP TABLE audit_log;
DROP TABLE admin_watches;
