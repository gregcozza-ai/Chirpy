package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sync/atomic"

	_ "github.com/google/uuid"
	"github.com/gregcozza-ai/Chirpy/internal/database"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
    dbQueries *database.Queries
}

type ChirpRequest struct {
	Body string `json:"body"`
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
		w.WriteHeader(http.StatusMethodNotAllowed)
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
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	cfg.fileserverHits.Store(0)
}

func (cfg *apiConfig) handleValidateChirp(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		cfg.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	var req ChirpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cfg.respondWithError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if len(req.Body) > 140 {
		cfg.respondWithError(w, http.StatusBadRequest, "Chirp is too long")
		return
	}
	cleanedBody := replaceProfanity(req.Body)
	cfg.respondWithJSON(w, http.StatusOK, map[string]string{"cleaned_body": cleanedBody})
}

func main() {
    godotenv.Load()
    dbURL := os.Getenv("DB_URL")
    db, err := sql.Open("postgres", dbURL)
    if err != nil {
        panic(err)
    }
    dbQueries := database.New(db)

	apiCfg := apiConfig{dbQueries: dbQueries}

	mux := http.NewServeMux()

	// Wrap fileserver with metrics middleware
	fileServer := http.StripPrefix("/app/", http.FileServer(http.Dir(".")))
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(fileServer))

	// Health endpoint - only GET
	mux.HandleFunc("/api/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Metrics endpoint - only GET (now /admin/metrics)
	mux.HandleFunc("/admin/metrics", apiCfg.handleMetrics)

	// Reset endpoint - only POST (now /admin/reset)
	mux.HandleFunc("/admin/reset", apiCfg.handleReset)

	// New validation endpoint
	mux.HandleFunc("/api/validate_chirp", apiCfg.handleValidateChirp)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	server.ListenAndServe()
}
