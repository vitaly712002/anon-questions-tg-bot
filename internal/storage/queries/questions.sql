-- name: InsertQuestion :one
INSERT INTO questions (link_id, recipient_id, reply_to, sender_enc, sender_hash,
                       content_type, src_chat_id, src_message_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: UpdateQuestionSenderEnc :exec
UPDATE questions SET sender_enc = $2 WHERE id = $1;

-- name: GetQuestion :one
SELECT * FROM questions WHERE id = $1;

-- name: MarkQuestionDelivered :exec
-- После доставки src_* затираются: src_chat_id отправителя — это его Telegram ID,
-- хранить его открыто нельзя (см. docs/04).
UPDATE questions
SET delivered_msg_id = $2, src_chat_id = 0, src_message_id = 0
WHERE id = $1;

-- name: WipeQuestionSrc :exec
UPDATE questions SET src_chat_id = 0, src_message_id = 0 WHERE id = $1;

-- name: MarkQuestionAnswered :exec
UPDATE questions SET answered = TRUE WHERE id = $1;

-- name: CountQuestionsBySenderSince :one
SELECT count(*) FROM questions
WHERE sender_hash = $1 AND created_at > sqlc.arg(since)::timestamptz;

-- name: CountQuestionsFromSenderToRecipient :one
SELECT count(*) FROM questions WHERE sender_hash = $1 AND recipient_id = $2;

-- name: CountQuestionsFromSenderToRecipientSince :one
SELECT count(*) FROM questions
WHERE sender_hash = $1 AND recipient_id = $2 AND created_at > sqlc.arg(since)::timestamptz;

-- name: CountQuestionsToRecipient :one
SELECT count(*) FROM questions WHERE recipient_id = $1;

-- name: CountQuestionsAnsweredByRecipient :one
SELECT count(*) FROM questions WHERE recipient_id = $1 AND answered;

-- name: CountQuestionsSince :one
SELECT count(*) FROM questions WHERE created_at > sqlc.arg(since)::timestamptz;

-- name: CountQuestions :one
SELECT count(*) FROM questions;
