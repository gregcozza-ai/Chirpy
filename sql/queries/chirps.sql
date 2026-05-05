-- name: CreateChirp :one
INSERT INTO chirps (body, user_id)
VALUES ($1, $2)
RETURNING id, created_at, updated_at, body, user_id;