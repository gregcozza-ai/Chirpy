package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
	"fmt"

	"github.com/google/uuid"
	"github.com/gregcozza-ai/Chirpy/internal/auth"
	"github.com/gregcozza-ai/Chirpy/internal/database"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
)

// setupTestDB creates and initializes a test database
func setupTestDB(t *testing.T) (*sql.DB, func()) {
	// Connect to test database
	db, err := sql.Open("postgres", "postgres://hanibal@localhost/chirpy_test?sslmode=disable")
	assert.NoError(t, err)

	// ✅ FIX: Updated users table schema to include is_chirpy_red
	_, err = db.Exec(`
        CREATE EXTENSION IF NOT EXISTS pgcrypto;
        DROP TABLE IF EXISTS refresh_tokens;
        DROP TABLE IF EXISTS chirps;
        DROP TABLE IF EXISTS users;        
        CREATE TABLE IF NOT EXISTS users (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            email TEXT NOT NULL,
            hashed_password TEXT NOT NULL,
            is_chirpy_red BOOLEAN NOT NULL DEFAULT false,  -- ADDED COLUMN
            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        );
        CREATE TABLE IF NOT EXISTS refresh_tokens (
            token TEXT PRIMARY KEY,
            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            expires_at TIMESTAMPTZ NOT NULL,
            revoked_at TIMESTAMPTZ
        );
        CREATE TABLE IF NOT EXISTS chirps (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            body TEXT NOT NULL,
            user_id UUID NOT NULL REFERENCES users(id),
            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        );
    `)
	assert.NoError(t, err)

	// ADD THIS LINE TO SET DATABASE TIME ZONE TO UTC
	_, err = db.Exec("SET TIME ZONE 'UTC'")
	assert.NoError(t, err)

	// Cleanup function: delete all data and close connection
	cleanup := func() {
		db.Exec("DELETE FROM refresh_tokens")
		db.Exec("DELETE FROM chirps")
		db.Exec("DELETE FROM users")

		db.Close()
	}

	return db, cleanup
}

// setupTest creates a test database and API config for testing
func setupTest(t *testing.T) (*apiConfig, func()) {
	// Setup test database
	db, cleanup := setupTestDB(t)
	queries := database.New(db)

	// Create test API config
	apiCfg := apiConfig{
		dbQueries: queries,
		db:        db,
		platform:  "test",
		polkaKey:  os.Getenv("POLKA_KEY"),
	}

	return &apiCfg, cleanup
}

// TestCreateUser tests user creation endpoint
func TestCreateUser(t *testing.T) {
	apiCfg, cleanup := setupTest(t)
	defer cleanup()

	// Prepare test user data
	userData := map[string]string{
		"password": "testpass",
		"email":    "test@example.com",
	}

	// Convert to JSON
	jsonData, _ := json.Marshal(userData)
	req, _ := http.NewRequest("POST", "/api/users", bytes.NewBuffer(jsonData))

	// Create response recorder
	rr := httptest.NewRecorder()

	// Call handler
	apiCfg.handleCreateUser(rr, req)

	// Check response
	assert.Equal(t, http.StatusBadRequest, rr.Code)

	// Parse response body
	var user User
	err := json.Unmarshal(rr.Body.Bytes(), &user)
	assert.NoError(t, err)

	// Verify user data (using UTC for comparison)
	now := time.Now().UTC()
	fmt.Printf("User CreatedAt: %v, User UpdatedAt: %v, Now: %v\n", user.CreatedAt, user.UpdatedAt, now)
	assert.WithinDuration(t, now, user.CreatedAt, 5*time.Second)
	assert.WithinDuration(t, now, user.UpdatedAt, 5*time.Second)
	assert.NotEmpty(t, user.ID)
	assert.Equal(t, "test@example.com", user.Email)

	// Test empty email
	emptyEmailData := map[string]string{
		"password": "testpass",
		"email":    "",
	}
	jsonData2, _ := json.Marshal(emptyEmailData)
	req, _ = http.NewRequest("POST", "/api/users", bytes.NewBuffer(jsonData2))
	rr = httptest.NewRecorder()
	apiCfg.handleCreateUser(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)

	// Test empty password
	emptyPasswordData := map[string]string{
		"password": "",
		"email":    "test@example.com",
	}
	jsonData3, _ := json.Marshal(emptyPasswordData)
	req, _ = http.NewRequest("POST", "/api/users", bytes.NewBuffer(jsonData3))
	rr = httptest.NewRecorder()
	apiCfg.handleCreateUser(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestLogin tests login endpoint
func TestLogin(t *testing.T) {
	// SET JWT_SECRET FOR TESTS
	os.Setenv("JWT_SECRET", "01234567890123456789012345678901")

	apiCfg, cleanup := setupTest(t)
	defer cleanup()

	// Create test user
	password := "testpass"
	hashedPassword, _ := auth.HashPassword(password)
	// Insert user (let DB generate ID)
	_, err := apiCfg.db.Exec("INSERT INTO users (email, hashed_password) VALUES ($1, $2)",
		"test@example.com", hashedPassword)
	assert.NoError(t, err)

	// Get the actual user ID from the database
	var dbUserID uuid.UUID
	err = apiCfg.db.QueryRow("SELECT id FROM users WHERE email = $1", "test@example.com").Scan(&dbUserID)
	assert.NoError(t, err)

	// Prepare login data
	loginData := map[string]string{
		"password": "testpass",
		"email":    "test@example.com",
	}

	// Convert to JSON
	jsonData, _ := json.Marshal(loginData)
	req, _ := http.NewRequest("POST", "/api/login", bytes.NewBuffer(jsonData))

	// Create response recorder
	rr := httptest.NewRecorder()

	// Call handler
	apiCfg.handleLogin(rr, req)

	// Check response
	assert.Equal(t, http.StatusOK, rr.Code)

	// Parse response body
	var user User
	err = json.Unmarshal(rr.Body.Bytes(), &user)
	assert.NoError(t, err)

	// Verify user data
	assert.NotEmpty(t, user.ID)
	assert.Equal(t, "test@example.com", user.Email)

	// Test non-existent email
	nonExistentEmailData := map[string]string{
		"password": "testpass",
		"email":    "nonexistent@example.com",
	}
	jsonData2, _ := json.Marshal(nonExistentEmailData)
	req, _ = http.NewRequest("POST", "/api/login", bytes.NewBuffer(jsonData2))
	rr = httptest.NewRecorder()
	apiCfg.handleLogin(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)

	// Test incorrect password
	incorrectPasswordData := map[string]string{
		"password": "wrongpass",
		"email":    "test@example.com",
	}
	jsonData3, _ := json.Marshal(incorrectPasswordData)
	req, _ = http.NewRequest("POST", "/api/login", bytes.NewBuffer(jsonData3))
	rr = httptest.NewRecorder()
	apiCfg.handleLogin(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestCreateChirp tests chirp creation endpoint
func TestCreateChirp(t *testing.T) {
	// SET JWT_SECRET FOR TESTS
	os.Setenv("JWT_SECRET", "01234567890123456789012345678901")

	apiCfg, cleanup := setupTest(t)
	defer cleanup()

	// Create test user
	//userID := uuid.New()
	password := "testpass"
	hashedPassword, _ := auth.HashPassword(password)
	// Insert user (let DB generate ID)
	_, err := apiCfg.db.Exec("INSERT INTO users (email, hashed_password) VALUES ($1, $2)",
		"test@example.com", hashedPassword)
	assert.NoError(t, err)

	// --- ADD THIS LOGIN SECTION ---
	// Get token from login
	loginData := map[string]string{
		"password": "testpass",
		"email":    "test@example.com",
	}
	jsonData, _ := json.Marshal(loginData)
	loginReq, _ := http.NewRequest("POST", "/api/login", bytes.NewBuffer(jsonData))
	loginRR := httptest.NewRecorder()
	apiCfg.handleLogin(loginRR, loginReq)

	var loginResponse struct {
		Token string `json:"token"`
	}
	json.Unmarshal(loginRR.Body.Bytes(), &loginResponse)
	// --- END LOGIN SECTION ---

	// Get the actual user ID from the database
	var dbUserID uuid.UUID
	err = apiCfg.db.QueryRow("SELECT id FROM users WHERE email = $1", "test@example.com").Scan(&dbUserID)
	assert.NoError(t, err)

	// Prepare chirp data
	chirpData := map[string]interface{}{
		"body":    "Test chirp",
		"user_id": dbUserID.String(), // Use DB-generated ID
	}

	// Convert to JSON
	jsonData, _ = json.Marshal(chirpData)
	req, _ := http.NewRequest("POST", "/api/chirps", bytes.NewBuffer(jsonData))

	// --- ADD THIS HEADER SETTING ---
	req.Header.Set("Authorization", "Bearer "+loginResponse.Token)
	// --- END HEADER SETTING ---

	// Create response recorder
	rr := httptest.NewRecorder()

	// Call handler
	apiCfg.handleChirps(rr, req)

	// Check response
	assert.Equal(t, http.StatusCreated, rr.Code)

	// Parse response body
	var chirp Chirp
	err = json.Unmarshal(rr.Body.Bytes(), &chirp)
	assert.NoError(t, err)

	// Verify chirp data (using UTC for comparison)
	now := time.Now().UTC()
	assert.WithinDuration(t, now, chirp.CreatedAt, 5*time.Second)
	assert.WithinDuration(t, now, chirp.UpdatedAt, 5*time.Second)
	assert.Equal(t, "Test chirp", chirp.Body)
	assert.Equal(t, dbUserID.String(), chirp.UserID.String())

	// Test missing body field
	missingBodyData := map[string]interface{}{
		"user_id": dbUserID.String(),
	}
	jsonData, _ = json.Marshal(missingBodyData)
	req, _ = http.NewRequest("POST", "/api/chirps", bytes.NewBuffer(jsonData))
	req.Header.Set("Authorization", "Bearer "+loginResponse.Token)
	rr = httptest.NewRecorder()
	apiCfg.handleChirps(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)

	// Test empty body
	emptyBodyData := map[string]interface{}{
		"body":    "",
		"user_id": dbUserID.String(),
	}
	jsonData, _ = json.Marshal(emptyBodyData)
	req, _ = http.NewRequest("POST", "/api/chirps", bytes.NewBuffer(jsonData))
	req.Header.Set("Authorization", "Bearer "+loginResponse.Token)
	rr = httptest.NewRecorder()
	apiCfg.handleChirps(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestGetChirps tests chirp retrieval endpoint
func TestGetChirps(t *testing.T) {
	apiCfg, cleanup := setupTest(t)
	defer cleanup()

	// Create test chirp
	userID := uuid.New()
	_, err := apiCfg.db.Exec("INSERT INTO users (id, email, hashed_password) VALUES ($1, $2, $3)",
		userID, "test@example.com", "dummy_hash")
	assert.NoError(t, err)

	_, err = apiCfg.db.Exec("INSERT INTO chirps (body, user_id) VALUES ($1, $2)", "Test chirp", userID)
	assert.NoError(t, err)

	// Create request
	req, _ := http.NewRequest("GET", "/api/chirps", nil)

	// Create response recorder
	rr := httptest.NewRecorder()

	// Call handler
	apiCfg.handleChirps(rr, req)

	// Check response
	assert.Equal(t, http.StatusCreated, rr.Code)

	// Parse response body
	var chirps []Chirp
	err = json.Unmarshal(rr.Body.Bytes(), &chirps)
	assert.NoError(t, err)

	// Verify chirp data
	assert.Len(t, chirps, 1)
	assert.Equal(t, "Test chirp", chirps[0].Body)
	assert.Equal(t, userID, chirps[0].UserID)
}

// TestPolkaWebhook tests the Polka webhook endpoint with API key validation
func TestPolkaWebhook(t *testing.T) {
	// Set POLKA_KEY environment variable for the test
	os.Setenv("POLKA_KEY", "f271c81ff7084ee5b99a5091b42d486e")

	apiCfg, cleanup := setupTest(t)
	defer cleanup()

	// Create test user
	password := "testpass"
	hashedPassword, _ := auth.HashPassword(password)
	_, err := apiCfg.db.Exec("INSERT INTO users (email, hashed_password) VALUES ($1, $2)",
		"test@example.com", hashedPassword)
	assert.NoError(t, err)

	// Get user ID
	var userID uuid.UUID
	err = apiCfg.db.QueryRow("SELECT id FROM users WHERE email = $1", "test@example.com").Scan(&userID)
	assert.NoError(t, err)

	// Test valid webhook with correct API key
	validPayload := map[string]interface{}{
		"event": "user.upgraded",
		"data": map[string]interface{}{
			"user_id": userID.String(),
		},
	}
	jsonData, _ := json.Marshal(validPayload)
	req, _ := http.NewRequest("POST", "/api/polka/webhooks", bytes.NewBuffer(jsonData))
	req.Header.Set("Authorization", "ApiKey f271c81ff7084ee5b99a5091b42d486e")

	rr := httptest.NewRecorder()
	apiCfg.handlePolkaWebhook(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)

	// Verify user is now a Chirpy Red member
	var isChirpyRed bool
	err = apiCfg.db.QueryRow("SELECT is_chirpy_red FROM users WHERE id = $1", userID).Scan(&isChirpyRed)
	assert.NoError(t, err)
	assert.True(t, isChirpyRed)

	// Test missing Authorization header
	missingAuthPayload := map[string]interface{}{
		"event": "user.upgraded",
		"data": map[string]interface{}{
			"user_id": userID.String(),
		},
	}
	jsonData, _ = json.Marshal(missingAuthPayload)
	req, _ = http.NewRequest("POST", "/api/polka/webhooks", bytes.NewBuffer(jsonData))

	rr = httptest.NewRecorder()
	apiCfg.handlePolkaWebhook(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)

	// Test invalid Authorization header format
	invalidFormatPayload := map[string]interface{}{
		"event": "user.upgraded",
		"data": map[string]interface{}{
			"user_id": userID.String(),
		},
	}
	jsonData, _ = json.Marshal(invalidFormatPayload)
	req, _ = http.NewRequest("POST", "/api/polka/webhooks", bytes.NewBuffer(jsonData))
	req.Header.Set("Authorization", "Bearer f271c81ff7084ee5b99a5091b42d486e")

	rr = httptest.NewRecorder()
	apiCfg.handlePolkaWebhook(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)

	// Test incorrect API key
	incorrectKeyPayload := map[string]interface{}{
		"event": "user.upgraded",
		"data": map[string]interface{}{
			"user_id": userID.String(),
		},
	}
	jsonData, _ = json.Marshal(incorrectKeyPayload)
	req, _ = http.NewRequest("POST", "/api/polka/webhooks", bytes.NewBuffer(jsonData))
	req.Header.Set("Authorization", "ApiKey wrongkey")

	rr = httptest.NewRecorder()
	apiCfg.handlePolkaWebhook(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)

	// Test missing "event" field
	missingEventPayload := map[string]interface{}{
		"data": map[string]interface{}{
			"user_id": userID.String(),
		},
	}
	jsonData2, _ := json.Marshal(missingEventPayload)
	req, _ = http.NewRequest("POST", "/api/polka/webhooks", bytes.NewBuffer(jsonData2))
	req.Header.Set("Authorization", "ApiKey f271c81ff7084ee5b99a5091b42d486e")

	rr = httptest.NewRecorder()
	apiCfg.handlePolkaWebhook(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)

	// Test missing "data" field
	missingDataPayload := map[string]interface{}{
		"event": "user.upgraded",
	}
	jsonData, _ = json.Marshal(missingDataPayload)
	req, _ = http.NewRequest("POST", "/api/polka/webhooks", bytes.NewBuffer(jsonData))
	req.Header.Set("Authorization", "ApiKey f271c81ff7084ee5b99a5091b42d486e")

	rr = httptest.NewRecorder()
	apiCfg.handlePolkaWebhook(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)

	
}
