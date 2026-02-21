package main

import (
	"encoding/json"
	"log"
	"os"
)

type Config struct {
	Port         string `json:"port"`
	DBPath       string `json:"db_path"`
	BaseURL      string `json:"base_url"`
	UploadDir    string `json:"upload_dir"`
	ResendAPIKey string `json:"resend_api_key"`
	AdminEmail   string `json:"admin_email"`
}

var cfg Config

func loadConfig(path string) {
	cfg = Config{
		Port:      "8080",
		DBPath:    "portal.db",
		BaseURL:   "http://localhost:8080",
		UploadDir: "uploads",
	}

	f, err := os.Open(path)
	if err != nil {
		log.Printf("No config file found at %s, using defaults + env", path)
	} else {
		defer f.Close()
		json.NewDecoder(f).Decode(&cfg)
	}

	// Env vars override config file
	if v := os.Getenv("PORT"); v != "" {
		cfg.Port = v
	}
	if v := os.Getenv("DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("UPLOAD_DIR"); v != "" {
		cfg.UploadDir = v
	}
	if v := os.Getenv("RESEND_API_KEY"); v != "" {
		cfg.ResendAPIKey = v
	}
	if v := os.Getenv("ADMIN_EMAIL"); v != "" {
		cfg.AdminEmail = v
	}
}
