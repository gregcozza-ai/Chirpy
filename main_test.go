package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gregcozza-ai/Chirpy/internal/auth"
	"github.com/gregcozza-ai/Chirpy/internal/database"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
)

// setupTestDB creates and initializes a test database
func setupTestDB(t *testing.T) (*sql.DB, func()) {
	// Create in-memory database for testing
	db, err := sql.Open("postgres", "postgres://hanibal@localhost/chirpy_test?sslmode=disable")
	assert.NoError(t, err)

	// ADD THIS LINE TO SET DATABASE TIME ZONE TO UTC
	_, err = db.Exec("SET TIME ZONE 'UTC'")
	assert.NoError(t, err)

	// Cleanup function: delete all data and close connection
	cleanup := func() {
		db.Exec("DELETE FROM users")
		db.Exec("DELETE FROM chirps")
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
	assert.Equal(t, http.StatusCreated, rr.Code)

	// Parse response body
	var user User
	err := json.Unmarshal(rr.Body.Bytes(), &user)
	assert.NoError(t, err)

	// Verify user data (using UTC for comparison)
	now := time.Now().UTC()
	assert.WithinDuration(t, now, user.CreatedAt, 5*time.Second)
	assert.WithinDuration(t, now, user.UpdatedAt, 5*time.Second)
	assert.NotEmpty(t, user.ID)
	assert.Equal(t, "test@example.com", user.Email)
}

// TestLogin tests login endpoint
func TestLogin(t *testing.T) {
	apiCfg, cleanup := setupTest(t)
	defer cleanup()

	// Create test user
	password := "testpass"
	hashedPassword, _ := auth.HashPassword(password)
	_, err := apiCfg.db.Exec("INSERT INTO users (email, hashed_password) VALUES ($1, $2)", "test@example.com", hashedPassword)
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
}

// TestCreateChirp tests chirp creation endpoint
func TestCreateChirp(t *testing.T) {
	apiCfg, cleanup := setupTest(t)
	defer cleanup()

	// Create test user
	userID := uuid.New()
	_, err := apiCfg.db.Exec("INSERT INTO users (id, email, hashed_password) VALUES ($1, $2, $3)",
		userID, "test@example.com", "dummy_hash")
	assert.NoError(t, err)

	// Prepare chirp data
	chirpData := map[string]interface{}{
		"body":    "Test chirp",
		"user_id": userID.String(),
	}

	// Convert to JSON
	jsonData, _ := json.Marshal(chirpData)
	req, _ := http.NewRequest("POST", "/api/chirps", bytes.NewBuffer(jsonData))

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
	assert.Equal(t, userID, chirp.UserID)
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
	assert.Equal(t, http.StatusOK, rr.Code)

	// Parse response body
	var chirps []Chirp
	err = json.Unmarshal(rr.Body.Bytes(), &chirps)
	assert.NoError(t, err)

	// Verify chirp data
	assert.Len(t, chirps, 1)
	assert.Equal(t, "Test chirp", chirps[0].Body)
	assert.Equal(t, userID, chirps[0].UserID)
}
