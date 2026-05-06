-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email)
VALUES (gen_random_uuid(), NOW(), NOW(), $1)
RETURNING id, email, created_at, updated_at;

-- name: DeleteAllUsers :exec 
DELETE FROM users;

-- name: GetUserByEmail :one
SELECT id, created_at, updated_at, email, hashed_password
FROM users
WHERE email = $1;
