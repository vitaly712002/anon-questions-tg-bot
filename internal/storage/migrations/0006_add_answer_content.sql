-- +goose Up
ALTER TABLE questions ADD COLUMN IF NOT EXISTS answer_text_enc BYTEA;
ALTER TABLE questions ADD COLUMN IF NOT EXISTS answer_file_id TEXT;
ALTER TABLE questions ADD COLUMN IF NOT EXISTS answered_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE questions DROP COLUMN IF EXISTS answer_text_enc;
ALTER TABLE questions DROP COLUMN IF EXISTS answer_file_id;
ALTER TABLE questions DROP COLUMN IF EXISTS answered_at;
