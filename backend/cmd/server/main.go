package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

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

	var maxCostUSD float64
	if v := os.Getenv("MAX_COST_USD"); v != "" {
		if c, err := strconv.ParseFloat(v, 64); err == nil && c > 0 {
			maxCostUSD = c
			log.Printf("Cost limit: $%.4f", maxCostUSD)
		} else {
			log.Printf("warn: invalid MAX_COST_USD %q — no limit applied", v)
		}
	}

	allowedOrigin := os.Getenv("ALLOWED_ORIGIN")
	if allowedOrigin == "" {
		allowedOrigin = "*"
		log.Printf("warn: ALLOWED_ORIGIN not set — using wildcard CORS (set it in production)")
	} else {
		log.Printf("CORS origin: %s", allowedOrigin)
	}

	// Data directory — configurable, default to ./data
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" { dataDir = "data" }
	storePath := filepath.Join(dataDir, "history.json")

	st, err := store.New(storePath)
	if err != nil { log.Fatalf("failed to open store at %s: %v", storePath, err) }
	log.Printf("Store loaded from %s", storePath)

	authMgr := auth.NewManager(username, password)
	handler := api.NewHandler(authMgr, anthropicKey, maxCostUSD, st)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	log.Printf("Server starting on :%s", port)
	if err := http.ListenAndServe(":"+port, corsMiddleware(mux, allowedOrigin)); err != nil {
		log.Fatal(err)
	}
}

func corsMiddleware(next http.Handler, allowedOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		if allowedOrigin != "*" {
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions { w.WriteHeader(http.StatusNoContent); return }
		next.ServeHTTP(w, r)
	})
}
