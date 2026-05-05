package main

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
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
	fmt.Fprintf(w, "Hits: %d", cfg.fileserverHits.Load())
}

func (cfg *apiConfig) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	cfg.fileserverHits.Store(0)
}

func main() {
	apiCfg := apiConfig{}

	mux := http.NewServeMux()

	// Wrap fileserver with metrics middleware
	fileServer := http.StripPrefix("/app/", http.FileServer(http.Dir(".")))
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(fileServer))

	// Health endpoint - only GET
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Metrics endpoint - only GET
	mux.HandleFunc("/metrics", apiCfg.handleMetrics)

	// Reset endpoint - only POST
	mux.HandleFunc("/reset", apiCfg.handleReset)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	server.ListenAndServe()
}
