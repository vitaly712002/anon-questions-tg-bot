-- name: UpsertUser :one
INSERT INTO users (tg_id, tg_username, display_name)
VALUES ($1, $2, $3)
ON CONFLICT (tg_id) DO UPDATE
    SET tg_username  = EXCLUDED.tg_username,
        display_name = EXCLUDED.display_name,
        updated_at   = now()
RETURNING *;

-- name: GetUserByTgID :one
SELECT * FROM users WHERE tg_id = $1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE lower(tg_username) = lower(sqlc.arg(username)::text);

-- name: SetUserRole :exec
UPDATE users SET role = $2, updated_at = now() WHERE id = $1;

-- name: EnsureOwnerSysadmin :exec
UPDATE users SET role = 'sysadmin', updated_at = now()
WHERE tg_id = $1 AND role <> 'sysadmin';

-- name: SetUserAccepting :exec
UPDATE users SET accepting = $2, updated_at = now() WHERE id = $1;

-- name: SetUserBanned :exec
UPDATE users SET banned = $2, updated_at = now() WHERE id = $1;

-- name: ListAdmins :many
SELECT * FROM users WHERE role IN ('admin', 'sysadmin') ORDER BY id;

-- name: ListBannedUsers :many
SELECT * FROM users WHERE banned ORDER BY updated_at DESC LIMIT 50;

-- name: CountUsers :one
SELECT count(*) FROM users;
