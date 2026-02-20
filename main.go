package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/kidandcat/portal/internal/config"
	"github.com/kidandcat/portal/internal/db"
	"github.com/kidandcat/portal/internal/handlers"
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

	handlers.Init("templates")

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	handlers.RegisterRoutes(mux, cfg)

	log.Printf("listening on %s (base URL: %s, SMTP: %v)", cfg.Addr, cfg.BaseURL, cfg.Email.SMTPEnabled)
	if err := http.ListenAndServe(cfg.Addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
