-- +goose Up

-- Блокировки: "этот получатель заблокировал этого отправителя", отправитель не раскрывается
CREATE TABLE blocks (
    recipient_id BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sender_hash  BYTEA       NOT NULL,               -- HMAC, а не telegram id!
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (recipient_id, sender_hash)
);

-- +goose Down
DROP TABLE blocks;
