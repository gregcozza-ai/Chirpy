package main

import "net/http"

func main() {
	mux := http.NewServeMux()
	// Serve current directory's index.html at root path
	mux.Handle("/", http.FileServer(http.Dir(".")))
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	server.ListenAndServe()
}
