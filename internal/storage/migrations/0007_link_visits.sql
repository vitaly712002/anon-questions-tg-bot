-- +goose Up
CREATE TABLE IF NOT EXISTS link_visits (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    link_id            BIGINT      NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    recipient_id       BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    guest_tg_id        BIGINT      NOT NULL,
    guest_username     TEXT,
    guest_display_name TEXT        NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS link_visits_recipient_idx ON link_visits (recipient_id, created_at DESC);
CREATE INDEX IF NOT EXISTS link_visits_created_at_idx ON link_visits (created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS link_visits;
