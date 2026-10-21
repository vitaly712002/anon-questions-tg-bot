-- name: AddBlock :exec
INSERT INTO blocks (recipient_id, sender_hash)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: IsBlocked :one
SELECT EXISTS (
    SELECT 1 FROM blocks WHERE recipient_id = $1 AND sender_hash = $2
);

-- name: CountBlocksByRecipient :one
SELECT count(*) FROM blocks WHERE recipient_id = $1;
