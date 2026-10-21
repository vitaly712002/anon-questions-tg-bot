-- name: SetDialogState :exec
INSERT INTO dialog_states (tg_id, state, payload)
VALUES ($1, $2, $3)
ON CONFLICT (tg_id) DO UPDATE
    SET state = EXCLUDED.state, payload = EXCLUDED.payload, updated_at = now();

-- name: GetDialogState :one
SELECT * FROM dialog_states WHERE tg_id = $1;

-- name: ClearDialogState :exec
DELETE FROM dialog_states WHERE tg_id = $1;
