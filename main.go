package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/kidandcat/portal/internal/api"
	"github.com/kidandcat/portal/internal/config"
	"github.com/kidandcat/portal/internal/db"
	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	baseURL := flag.String("base-url", "http://localhost:8080", "base URL for magic links")
	dataDir := flag.String("data", "./data", "data directory")
	flag.Parse()

	cfg := config.Load(*addr, *baseURL, *dataDir)

	if err := db.Init(cfg.DataDir); err != nil {
		log.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	if err := db.SeedDefaultProject(); err != nil {
		log.Printf("warning: could not seed default project: %v", err)
	}

	mux := http.NewServeMux()

	// API routes
	api.RegisterRoutes(mux, cfg)

	// go-app handler serves the WASM app
	appHandler := &app.Handler{
		Name:        "Portal",
		ShortName:   "Portal",
		Description: "Canvas collaboration tool",
		Styles:      []string{"/web/app.css"},
		Title:       "Portal",
	}
	mux.Handle("/", appHandler)

	log.Printf("listening on %s (base URL: %s)", cfg.Addr, cfg.BaseURL)
	if err := http.ListenAndServe(cfg.Addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
