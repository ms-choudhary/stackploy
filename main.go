package main

import (
	"database/sql"
	"embed"
	"log"
	"net/http"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

//go:embed static
var static embed.FS

func main() {
	dataDir := env("DATA_DIR", "/data")
	addr := env("ADDR", ":8080")

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "stackploy.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(schema); err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.FileServerFS(static))
	mux.Handle("GET /{$}", http.RedirectHandler("/stacks", http.StatusSeeOther))
	environmentRoutes(mux, db)
	stackRoutes(mux, db, dataDir)

	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
