-- name: AddWatch :exec
INSERT INTO admin_watches (admin_id, target_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RemoveWatch :exec
DELETE FROM admin_watches WHERE admin_id = $1 AND target_id = $2;

-- name: ListWatchesByAdmin :many
SELECT u.* FROM admin_watches w
JOIN users u ON u.id = w.target_id
WHERE w.admin_id = $1
ORDER BY w.created_at;

-- name: ListWatcherTgIDs :many
-- Telegram ID админов, подписанных на пользователя (для дублирования вопросов).
SELECT u.tg_id, u.id FROM admin_watches w
JOIN users u ON u.id = w.admin_id
WHERE w.target_id = $1;

-- name: CountWatchesByAdmin :one
SELECT count(*) FROM admin_watches WHERE admin_id = $1;

-- name: InsertAudit :exec
INSERT INTO audit_log (actor_id, action, target_id, details)
VALUES ($1, $2, $3, $4);

-- name: ListAuditRecent :many
SELECT a.*, actor.display_name AS actor_name, target.display_name AS target_name
FROM audit_log a
JOIN users actor ON actor.id = a.actor_id
LEFT JOIN users target ON target.id = a.target_id
ORDER BY a.created_at DESC
LIMIT $1;
