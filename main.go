package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sync/atomic"
	"time"
	"sort"

	"github.com/google/uuid"
	"github.com/gregcozza-ai/Chirpy/internal/auth"
	"github.com/gregcozza-ai/Chirpy/internal/database"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
	db             *sql.DB
	platform       string
	jwtSecret      string
	polkaKey       string
}

type User struct {
	ID          uuid.UUID `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Email       string    `json:"email"`
	IsChirpyRed bool      `json:"is_chirpy_red"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

type loginRequest struct {
	Password         string `json:"password"`
	Email            string `json:"email"`
	ExpiresInSeconds *int   `json:"expires_in_seconds,omitempty"`
}

func replaceProfanity(s string) string {
	profaneRegex := regexp.MustCompile(`(?i)\b(kerfuffle|sharbert|fornax)\b`)
	return profaneRegex.ReplaceAllString(s, "****")
}

func (cfg *apiConfig) respondWithError(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (cfg *apiConfig) respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.WriteHeader(code)
	w.Header().Set("Content-Type", "application/json")
	// Marshal the payload into JSON bytes first to catch any encoding errors before writing the status code
	data, err := json.Marshal(payload)
	if err != nil {
		// Log the error and return a generic error message
		fmt.Printf("Error marshaling JSON response: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to encode response"})
		return
	}
	// fmt.Print("Encoded JSON response: ", string(data), "\n") // Debug log to verify the JSON output
	w.Write(data)
}

/*
if err := json.NewEncoder(w).Encode(payload); err != nil {
        // Log the error for debugging purposes, but we cannot change the status code here
        // as it has already been written by w.WriteHeader(code).
        // In a real application, you might want to log this error to a dedicated service.
        fmt.Printf("Error encoding JSON response: %v\n", err)
    }

	json.NewEncoder(w).Encode(payload)
}*/

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		cfg.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := fmt.Sprintf(`
        <html>
          <body>
            <h1>Welcome, Chirpy Admin</h1>
            <p>Chirpy has been visited %d times!</p>
          </body>
        </html>`, cfg.fileserverHits.Load())
	w.Write([]byte(html))
}

func (cfg *apiConfig) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		cfg.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Only allow in dev environment
	if cfg.platform != "dev" {
		cfg.respondWithError(w, http.StatusForbidden, "Only allowed in dev environment")
		return
	}

	// Delete all users
	if err := cfg.dbQueries.DeleteAllUsers(context.Background()); err != nil {
		cfg.respondWithError(w, http.StatusInternalServerError, "Failed to delete users")
		return
	}

	// Reset metrics counter
	cfg.fileserverHits.Store(0)
}

func (cfg *apiConfig) handleChirps(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		// Authentication required for POST
		token, err := auth.GetBearerToken(r.Header)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Parse chirp body
		var chirpData struct {
			Body string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&chirpData); err != nil {
			cfg.respondWithError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		// Apply profanity filter before saving
		body := replaceProfanity(chirpData.Body)

		// Create chirp with authenticated user ID
		chirp, err := cfg.dbQueries.CreateChirp(r.Context(), database.CreateChirpParams{
			Body:   body,
			UserID: userID,
		})
		if err != nil {
			cfg.respondWithError(w, http.StatusInternalServerError, "Failed to create chirp")
			return
		}

		// Convert to response format
		response := Chirp{
			ID:        chirp.ID,
			Body:      chirp.Body,
			UserID:    chirp.UserID,
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
		}

		cfg.respondWithJSON(w, http.StatusCreated, response)
		return
	}

	switch r.Method {
	case "GET":
		// Get author_id from query parameters
		authorID := r.URL.Query().Get("author_id")

		var dbChirps []database.Chirp
		var err error

		// Declare separate variables for each database method
		var authorRows []database.GetChirpsByAuthorRow
		var allRows []database.GetChirpsRow

		// Filter by author_id if provided
		if authorID != "" {
			// Parse string to UUID
			uuidAuthor, err := uuid.Parse(authorID)
			if err != nil {
				cfg.respondWithError(w, http.StatusBadRequest, "Invalid author_id format")
				return
			}

			// Get chirps by author (returns []GetChirpsByAuthorRow)
			authorRows, err = cfg.dbQueries.GetChirpsByAuthor(context.Background(), uuidAuthor)
			if err != nil {
				cfg.respondWithError(w, http.StatusInternalServerError, "Failed to fetch chirps")
				return
			}

			// Convert rows to []database.Chirp
			dbChirps = make([]database.Chirp, len(authorRows))
			for i, row := range authorRows {
				dbChirps[i] = database.Chirp{
					ID:        row.ID,
					CreatedAt: row.CreatedAt,
					UpdatedAt: row.UpdatedAt,
					Body:      row.Body,
					UserID:    row.UserID,
				}
			}
		} else {
			// Get all chirps (returns []GetChirpsRow)
			allRows, err = cfg.dbQueries.GetChirps(context.Background())
			if err != nil {
				cfg.respondWithError(w, http.StatusInternalServerError, "Failed to fetch chirps")
				return
			}

			// Convert rows to []database.Chirp
			dbChirps = make([]database.Chirp, len(allRows))
			for i, row := range allRows {
				dbChirps[i] = database.Chirp{
					ID:        row.ID,
					CreatedAt: row.CreatedAt,
					UpdatedAt: row.UpdatedAt,
					Body:      row.Body,
					UserID:    row.UserID,
				}
			}
		}

		// Sort chirps based on 'sort' query parameter
		sortParam := r.URL.Query().Get("sort")
		if sortParam == "desc" {
			sort.Slice(dbChirps, func(i, j int) bool {
				return dbChirps[i].CreatedAt.After(dbChirps[j].CreatedAt)
			})
		} else {
			// Default to ascending order
			sort.Slice(dbChirps, func(i, j int) bool {
				return dbChirps[i].CreatedAt.Before(dbChirps[j].CreatedAt)
			})
		}

		// Convert to Chirp array
		chirps := make([]Chirp, len(dbChirps))
		for i, dbChirp := range dbChirps {
			chirps[i] = Chirp{
				ID:        dbChirp.ID,
				CreatedAt: dbChirp.CreatedAt,
				UpdatedAt: dbChirp.UpdatedAt,
				Body:      dbChirp.Body,
				UserID:    dbChirp.UserID,
			}
		}

		// Return 200 OK with chirps array
		cfg.respondWithJSON(w, http.StatusOK, chirps)

	default:
		cfg.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (cfg *apiConfig) handleChirpByID(w http.ResponseWriter, r *http.Request) {
	chirpID := r.PathValue("chirpID")
	id, err := uuid.Parse(chirpID)
	if err != nil {
		cfg.respondWithError(w, http.StatusBadRequest, "Invalid chirp ID format")
		return
	}

	switch r.Method {
	case "GET":
		// Existing GET logic
		dbChirp, err := cfg.dbQueries.GetChirpByID(context.Background(), id)
		if err != nil {
			cfg.respondWithError(w, http.StatusNotFound, "Chirp not found")
			return
		}

		chirp := Chirp{
			ID:        dbChirp.ID,
			CreatedAt: dbChirp.CreatedAt,
			UpdatedAt: dbChirp.UpdatedAt,
			Body:      dbChirp.Body,
			UserID:    dbChirp.UserID,
		}

		cfg.respondWithJSON(w, http.StatusOK, chirp)

	case "DELETE":
		// DELETE logic
		dbChirp, err := cfg.dbQueries.GetChirpByID(context.Background(), id)
		if err != nil {
			cfg.respondWithError(w, http.StatusNotFound, "Chirp not found")
			return
		}

		// Authenticate user
		token, err := auth.GetBearerToken(r.Header)
		if err != nil {
			cfg.respondWithError(w, http.StatusUnauthorized, "Missing token")
			return
		}
		userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
		if err != nil {
			cfg.respondWithError(w, http.StatusUnauthorized, "Invalid token")
			return
		}

		// Check if user is the author
		if dbChirp.UserID != userID {
			cfg.respondWithError(w, http.StatusForbidden, "Not authorized to delete this chirp")
			return
		}

		// Delete the chirp
		err = cfg.dbQueries.DeleteChirp(context.Background(), id)
		if err != nil {
			cfg.respondWithError(w, http.StatusInternalServerError, "Failed to delete chirp")
			return
		}

		// Return 204 No Content
		w.WriteHeader(http.StatusNoContent)

	default:
		cfg.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (cfg *apiConfig) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		cfg.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cfg.respondWithError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Hash password
	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		cfg.respondWithError(w, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	// Generate UUID before inserting
	id := uuid.New()

	// Create user in database
	user := User{}
	if err := cfg.db.QueryRowContext(context.Background(),
		`INSERT INTO users (id, email, hashed_password) VALUES ($1, $2, $3)
		 RETURNING id, created_at, updated_at, email`,
		id,
		req.Email,
		hashedPassword,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt, &user.Email); err != nil {
		fmt.Printf("Database error: %v\n", err)
		cfg.respondWithError(w, http.StatusInternalServerError, "Failed to create user")
		return
	}

	// Return 201 Created
	cfg.respondWithJSON(w, http.StatusCreated, user)
}

func (cfg *apiConfig) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		cfg.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cfg.respondWithError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Set default expiration to 1 hour (3600 seconds)
	expiresIn := 3600 * time.Second
	if req.ExpiresInSeconds != nil {
		// Clamp to max 1 hour
		if *req.ExpiresInSeconds > 3600 {
			expiresIn = 3600 * time.Second
		} else {
			expiresIn = time.Duration(*req.ExpiresInSeconds) * time.Second
		}
	}

	// Get user by email
	dbUser, err := cfg.dbQueries.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		cfg.respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
		return
	}

	// Verify password
	match, err := auth.CheckPasswordHash(req.Password, dbUser.HashedPassword)
	if err != nil || !match {
		cfg.respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
		return
	}

	// Create JWT token
	token, err := auth.MakeJWT(dbUser.ID, cfg.jwtSecret, expiresIn)
	if err != nil {
		cfg.respondWithError(w, http.StatusInternalServerError, "Failed to create token")
		return
	}

	// Generate and store refresh token
	refreshToken := auth.MakeRefreshToken()
	expiresAt := time.Now().Add(60 * 24 * time.Hour) // 60 days

	_, err = cfg.dbQueries.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		Token:     refreshToken,
		UserID:    dbUser.ID,
		ExpiresAt: expiresAt,
		RevokedAt: sql.NullTime{},
	})
	if err != nil {
		cfg.respondWithError(w, http.StatusInternalServerError, "Failed to create refresh token")
		return
	}

	// Add refresh_token to response (now includes is_chirpy_red)
	response := struct {
		ID           uuid.UUID `json:"id"`
		CreatedAt    time.Time `json:"created_at"`
		UpdatedAt    time.Time `json:"updated_at"`
		Email        string    `json:"email"`
		IsChirpyRed  bool      `json:"is_chirpy_red"`
		Token        string    `json:"token"`
		RefreshToken string    `json:"refresh_token"`
	}{
		ID:           dbUser.ID,
		CreatedAt:    dbUser.CreatedAt,
		UpdatedAt:    dbUser.UpdatedAt,
		Email:        dbUser.Email,
		IsChirpyRed:  dbUser.IsChirpyRed, // Added this field
		Token:        token,
		RefreshToken: refreshToken,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (cfg *apiConfig) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		cfg.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	token, err := auth.GetBearerToken(r.Header)

	// fmt.Printf("Received refresh request with token: %s\n", token)

	if err != nil {
		cfg.respondWithError(w, http.StatusUnauthorized, "Missing refresh token")
		return
	}

	refreshToken, err := cfg.dbQueries.GetRefreshTokenByToken(r.Context(), token)

	if err != nil || refreshToken.RevokedAt.Valid || refreshToken.ExpiresAt.Before(time.Now()) {
		cfg.respondWithError(w, http.StatusUnauthorized, "Invalid refresh token")
		return
	}

	newToken, err := generateToken(cfg, &refreshToken)
	// fmt.Printf("Generated new token: %s\n", newToken)

	if err != nil {
		cfg.respondWithError(w, http.StatusInternalServerError, "Failed to create new token")
		return
	}

	cfg.respondWithJSON(w, http.StatusOK, struct {
		Token string `json:"token"`
	}{Token: newToken})
}

func (cfg *apiConfig) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		cfg.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		cfg.respondWithError(w, http.StatusUnauthorized, "Missing refresh token")
		return
	}

	err = cfg.dbQueries.RevokeRefreshToken(r.Context(), token)
	if err != nil {
		cfg.respondWithError(w, http.StatusInternalServerError, "Failed to revoke token")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func generateToken(cfg *apiConfig, refreshToken *database.RefreshToken) (string, error) {
	// FIX: Pass refreshToken.UserID (value) instead of &refreshToken (pointer)
	token, err := auth.MakeJWT(refreshToken.UserID, cfg.jwtSecret, 1*time.Hour)
	if err != nil {
		return "", err
	}
	return token, nil
}

func (cfg *apiConfig) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != "PUT" {
		cfg.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Authenticate user via token
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		cfg.respondWithError(w, http.StatusUnauthorized, "Missing token")
		return
	}
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		cfg.respondWithError(w, http.StatusUnauthorized, "Invalid token")
		return
	}

	// Parse request body
	var req struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cfg.respondWithError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Hash new password
	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		cfg.respondWithError(w, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	// Update user in database
	err = cfg.dbQueries.UpdateUser(r.Context(), database.UpdateUserParams{
		Email:          req.Email,
		HashedPassword: hashedPassword,
		ID:             userID,
	})
	if err != nil {
		cfg.respondWithError(w, http.StatusInternalServerError, "Failed to update user")
		return
	}

	// Fetch updated user
	updatedUser, err := cfg.dbQueries.GetUserByID(r.Context(), userID)
	if err != nil {
		cfg.respondWithError(w, http.StatusInternalServerError, "Failed to fetch updated user")
		return
	}

	// Respond with updated user (without password)
	user := User{
		ID:        updatedUser.ID,
		CreatedAt: updatedUser.CreatedAt,
		UpdatedAt: updatedUser.UpdatedAt,
		Email:     updatedUser.Email,
	}
	cfg.respondWithJSON(w, http.StatusOK, user)
}

func (cfg *apiConfig) handleUser(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		cfg.handleCreateUser(w, r)
	case "PUT":
		cfg.handleUpdateUser(w, r)
	default:
		cfg.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		panic(err)
	}

	dbQueries := database.New(db)

	apiCfg := apiConfig{
		dbQueries: dbQueries,
		db:        db,
		platform:  platform,
		jwtSecret: os.Getenv("JWT_SECRET"),
		polkaKey:  os.Getenv("POLKA_KEY"),
	}

	if apiCfg.jwtSecret == "" {
		panic("JWT_SECRET environment variable is not set")
	}

	mux := http.NewServeMux()

	// Wrap fileserver with metrics middleware
	fileServer := http.StripPrefix("/app/", http.FileServer(http.Dir("./app")))
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(fileServer))

	// Health endpoint
	mux.HandleFunc("/api/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Metrics endpoint
	mux.HandleFunc("/admin/metrics", apiCfg.handleMetrics)

	// Reset endpoint
	mux.HandleFunc("/admin/reset", apiCfg.handleReset)

	// User endpoints
	mux.HandleFunc("/api/users", apiCfg.handleUser) // Single handler for both methods
	mux.HandleFunc("/api/login", apiCfg.handleLogin)

	// Chirp endpoints
	mux.HandleFunc("/api/chirps", apiCfg.handleChirps)
	mux.HandleFunc("/api/chirps/{chirpID}", apiCfg.handleChirpByID) // Handles both GET and DELETE

	// Refresh endpoint
	mux.HandleFunc("/api/refresh", apiCfg.handleRefresh)
	mux.HandleFunc("/api/revoke", apiCfg.handleRevoke)

	// Polka webhook endpoint
	mux.HandleFunc("/api/polka/webhooks", apiCfg.handlePolkaWebhook)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	server.ListenAndServe()
}

func (cfg *apiConfig) handlePolkaWebhook(w http.ResponseWriter, r *http.Request) {
	// Validate API key
	apiKey, err := auth.GetAPIKey(r.Header)
	if err != nil || apiKey != cfg.polkaKey {
		cfg.respondWithError(w, http.StatusUnauthorized, "Invalid API key")
		return
	}

	var payload struct {
		Event string `json:"event"`
		Data  struct {
			UserID string `json:"user_id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		cfg.respondWithError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if payload.Event != "user.upgraded" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	userID, err := uuid.Parse(payload.Data.UserID)
	if err != nil {
		cfg.respondWithError(w, http.StatusBadRequest, "Invalid user ID format")
		return
	}

	rowsAffected, err := cfg.dbQueries.UpgradeUserToChirpyRed(r.Context(), userID)
	if err != nil {
		cfg.respondWithError(w, http.StatusInternalServerError, "Failed to upgrade user")
		return
	}

	if rowsAffected == 0 {
		cfg.respondWithError(w, http.StatusNotFound, "User not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
