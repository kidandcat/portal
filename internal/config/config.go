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
	Branding BrandingConfig
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

type BrandingConfig struct {
	AppName      string
	PrimaryColor string
	LogoURL      string
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
		Branding: BrandingConfig{
			AppName:      getEnv("PORTAL_APP_NAME", "Portal"),
			PrimaryColor: getEnv("PORTAL_PRIMARY_COLOR", "#1a1a1a"),
			LogoURL:      getEnv("PORTAL_LOGO_URL", ""),
		},
	}
}
