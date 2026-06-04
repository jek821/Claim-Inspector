package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"factchecker/internal/api"
	"factchecker/internal/auth"
	"factchecker/internal/envload"
	"factchecker/internal/store"
)

func main() {
	if err := envload.Load(".env"); err != nil {
		log.Printf("warn: .env load error: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" { port = "8080" }

	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	if anthropicKey == "" { log.Fatal("ANTHROPIC_API_KEY is required") }

	username := os.Getenv("APP_USERNAME")
	password := os.Getenv("APP_PASSWORD")
	if username == "" || password == "" { log.Fatal("APP_USERNAME and APP_PASSWORD are required") }

	newsAPIKey := os.Getenv("NEWS_API_KEY")

	// Data directory — configurable, default to ./data
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" { dataDir = "data" }
	storePath := filepath.Join(dataDir, "history.json")

	st, err := store.New(storePath)
	if err != nil { log.Fatalf("failed to open store at %s: %v", storePath, err) }
	log.Printf("Store loaded from %s", storePath)

	authMgr := auth.NewManager(username, password)
	handler := api.NewHandler(authMgr, anthropicKey, newsAPIKey, st)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	log.Printf("Server starting on :%s", port)
	if err := http.ListenAndServe(":"+port, corsMiddleware(mux)); err != nil {
		log.Fatal(err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions { w.WriteHeader(http.StatusNoContent); return }
		next.ServeHTTP(w, r)
	})
}
