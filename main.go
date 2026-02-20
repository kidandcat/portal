package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/kidandcat/portal/internal/db"
	"github.com/kidandcat/portal/internal/handlers"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	baseURL := flag.String("base-url", "http://localhost:8080", "base URL for magic links")
	dataDir := flag.String("data", "./data", "data directory")
	flag.Parse()

	if err := db.Init(*dataDir); err != nil {
		log.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	handlers.Init("templates")

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	handlers.RegisterRoutes(mux, *baseURL)

	log.Printf("listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
