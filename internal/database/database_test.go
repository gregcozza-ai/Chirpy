package database

import (
	"context"
	"testing"

	"database/sql"

	"github.com/google/uuid"
	"github.com/gregcozza-ai/Chirpy/internal/auth"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
)

func setupTestDB(t *testing.T) (*sql.DB, func()) {
	db, err := sql.Open("postgres", "postgres://hanibal@localhost:5432/chirpy_test?sslmode=disable")
	assert.NoError(t, err)

	// ✅ FIX: Updated users table schema to include is_chirpy_red
	_, err = db.Exec(`
		CREATE EXTENSION IF NOT EXISTS pgcrypto;
		DROP TABLE IF EXISTS refresh_tokens;
		DROP TABLE IF EXISTS chirps;
		DROP TABLE IF EXISTS users;        
		CREATE TABLE users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email TEXT NOT NULL,
			hashed_password TEXT NOT NULL,
			is_chirpy_red BOOLEAN NOT NULL DEFAULT false,  -- ADDED COLUMN
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		);
		CREATE TABLE chirps (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			body TEXT NOT NULL,
			user_id UUID NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		);
		CREATE TABLE refresh_tokens (
			token TEXT PRIMARY KEY,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			expires_at TIMESTAMPTZ NOT NULL,
			revoked_at TIMESTAMPTZ
		);
	`)
	assert.NoError(t, err)

	cleanup := func() {
		db.Exec("DELETE FROM refresh_tokens")
		db.Exec("DELETE FROM chirps")
		db.Exec("DELETE FROM users")
	}

	return db, cleanup
}

func TestCreateChirp(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	queries := New(db)

	userID := uuid.New()
	body := "Test chirp"

	// ✅ FIX 2: Use CreateChirp function (handles id generation)
	chirp, err := queries.CreateChirp(context.Background(), CreateChirpParams{
		Body:   body,
		UserID: userID,
	})
	assert.NoError(t, err)
	assert.Equal(t, body, chirp.Body)
	assert.Equal(t, userID, chirp.UserID)
}

func TestGetUserByEmail(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	queries := New(db)

	email := "test@example.com"
	password := "testpass"
	hashedPassword, _ := auth.HashPassword(password)

	// ✅ FIX 3: Insert using database defaults (no id needed)
	_, err := db.Exec("INSERT INTO users (email, hashed_password) VALUES ($1, $2)", email, hashedPassword)
	assert.NoError(t, err)

	user, err := queries.GetUserByEmail(context.Background(), email)
	assert.NoError(t, err)
	assert.Equal(t, email, user.Email)
}
