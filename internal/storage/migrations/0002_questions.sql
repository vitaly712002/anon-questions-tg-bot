-- +goose Up

CREATE TABLE questions (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    link_id        BIGINT      NOT NULL REFERENCES links(id)  ON DELETE CASCADE,
    recipient_id   BIGINT      NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
    reply_to       BIGINT      REFERENCES questions(id),      -- цепочка анонимного диалога
    sender_enc     BYTEA       NOT NULL,  -- AES-256-GCM(sender_tg_id), nonce внутри
    sender_hash    BYTEA       NOT NULL,  -- HMAC-SHA256(sender_tg_id) — для блокировок/лимитов
    content_type   TEXT        NOT NULL,  -- text|photo|video|voice|video_note|audio|sticker|document|animation|other
    src_chat_id    BIGINT      NOT NULL,  -- откуда копировать (copyMessage); затирается после доставки
    src_message_id BIGINT      NOT NULL,
    delivered_msg_id BIGINT,              -- id сообщения у получателя (для связи reply)
    answered       BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX questions_recipient_idx   ON questions (recipient_id, created_at DESC);
CREATE INDEX questions_sender_hash_idx ON questions (sender_hash);

-- +goose Down
DROP TABLE questions;
