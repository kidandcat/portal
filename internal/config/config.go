package config

import (
	"os"
	"strings"
)

type Config struct {
	Addr     string
	BaseURL  string
	DataDir  string
	Email    EmailConfig
}

type EmailConfig struct {
	FromEmail    string
	ResendAPIKey string
	SMTPEnabled  bool
	SMTPHost     string
	SMTPPort     string
	SMTPUser     string
	SMTPPass     string
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func Load(flagAddr, flagBaseURL, flagDataDir string) Config {
	return Config{
		Addr:    flagAddr,
		BaseURL: getEnv("PORTAL_BASE_URL", flagBaseURL),
		DataDir: flagDataDir,
		Email: EmailConfig{
			FromEmail:    getEnv("PORTAL_FROM_EMAIL", "Portal <portal@resend.dev>"),
			ResendAPIKey: getEnv("RESEND_API_KEY", ""),
			SMTPEnabled:  strings.EqualFold(getEnv("SMTP_ENABLED", "false"), "true"),
			SMTPHost:     getEnv("SMTP_HOST", ""),
			SMTPPort:     getEnv("SMTP_PORT", "587"),
			SMTPUser:     getEnv("SMTP_USER", ""),
			SMTPPass:     getEnv("SMTP_PASS", ""),
		},
	}
}
