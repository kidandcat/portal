package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"

	"github.com/kidandcat/portal/internal/config"
)

type resendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

func SendMagicLink(email, token string, cfg config.Config) error {
	link := fmt.Sprintf("%s/auth/verify?token=%s", cfg.BaseURL, token)
	appName := cfg.Branding.AppName
	subject := "Your login link"
	html := fmt.Sprintf(
		`<p>Click the link below to approve your sign-in:</p>`+
			`<p><a href="%s">Approve sign-in to %s</a></p>`+
			`<p>This link expires in 15 minutes.</p>`,
		link, appName,
	)

	if cfg.Email.SMTPEnabled {
		return sendViaSMTP(cfg.Email, email, subject, html)
	}
	return sendViaResend(cfg.Email, email, subject, html)
}

func sendViaResend(ecfg config.EmailConfig, to, subject, html string) error {
	body := resendRequest{
		From:    ecfg.FromEmail,
		To:      []string{to},
		Subject: subject,
		HTML:    html,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ecfg.ResendAPIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("resend API error: status %d", resp.StatusCode)
	}

	return nil
}

func sendViaSMTP(ecfg config.EmailConfig, to, subject, html string) error {
	addr := ecfg.SMTPHost + ":" + ecfg.SMTPPort

	msg := "From: " + ecfg.FromEmail + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=\"UTF-8\"\r\n" +
		"\r\n" +
		html

	var auth smtp.Auth
	if ecfg.SMTPUser != "" {
		auth = smtp.PlainAuth("", ecfg.SMTPUser, ecfg.SMTPPass, ecfg.SMTPHost)
	}

	if err := smtp.SendMail(addr, auth, ecfg.SMTPUser, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}

	return nil
}
