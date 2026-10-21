-- name: CreateLink :one
INSERT INTO links (user_id, token) VALUES ($1, $2) RETURNING *;

-- name: GetActiveLinkByUserID :one
SELECT * FROM links WHERE user_id = $1 AND is_active ORDER BY id LIMIT 1;

-- name: GetLinkByToken :one
SELECT * FROM links WHERE token = $1;

-- name: GetLinkByAlias :one
SELECT * FROM links WHERE lower(alias) = lower(sqlc.arg(alias)::text);

-- name: GetLinkByID :one
SELECT * FROM links WHERE id = $1;

-- name: SetLinkAlias :exec
UPDATE links SET alias = $2 WHERE id = $1;

-- name: SetLinksActiveByUser :exec
UPDATE links SET is_active = $2 WHERE user_id = $1;

-- name: IncrementLinkClicks :exec
UPDATE links SET clicks = clicks + 1 WHERE id = $1;
