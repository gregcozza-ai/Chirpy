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
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
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
	json.NewEncoder(w).Encode(payload)
}

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
	switch r.Method {
	case "POST":
		var req struct {
			Body   string `json:"body"`
			UserID string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			cfg.respondWithError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		// Validate body length
		if len(req.Body) > 140 {
			cfg.respondWithError(w, http.StatusBadRequest, "Chirp is too long")
			return
		}

		// Replace profanity
		cleanedBody := replaceProfanity(req.Body)

		// Generate UUID before inserting
		//id := uuid.New()

		// Convert user_id string to UUID
		userID, err := uuid.Parse(req.UserID)
		if err != nil {
			cfg.respondWithError(w, http.StatusBadRequest, "Invalid user_id format")
			return
		}

		// Create chirp in database
		dbChirp, err := cfg.dbQueries.CreateChirp(context.Background(), database.CreateChirpParams{
			Body:   cleanedBody,
			UserID: userID,
		})
		if err != nil {
			cfg.respondWithError(w, http.StatusInternalServerError, "Failed to create chirp")
			return
		}

		// Return 201 Created with full chirp
		chirp := Chirp{
			ID:        dbChirp.ID,
			CreatedAt: dbChirp.CreatedAt,
			UpdatedAt: dbChirp.UpdatedAt,
			Body:      dbChirp.Body,
			UserID:    dbChirp.UserID,
		}
		cfg.respondWithJSON(w, http.StatusCreated, chirp)

	case "GET":
		dbChirps, err := cfg.dbQueries.GetChirps(context.Background())
		if err != nil {
			cfg.respondWithError(w, http.StatusInternalServerError, "Failed to fetch chirps")
			return
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

	var req struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cfg.respondWithError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Get user by email
	dbUser, err := cfg.dbQueries.GetUserByEmail(context.Background(), req.Email)
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

	// Return user without password
	user := User{
		ID:        dbUser.ID,
		CreatedAt: dbUser.CreatedAt,
		UpdatedAt: dbUser.UpdatedAt,
		Email:     dbUser.Email,
	}

	cfg.respondWithJSON(w, http.StatusOK, user)
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
	mux.HandleFunc("/api/users", apiCfg.handleCreateUser)
	mux.HandleFunc("/api/login", apiCfg.handleLogin)

	// Chirp endpoints
	mux.HandleFunc("/api/chirps", apiCfg.handleChirps)
	mux.HandleFunc("/api/chirps/{chirpID}", apiCfg.handleChirpByID)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	server.ListenAndServe()
}
