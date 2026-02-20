package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

var resendAPIKey = "re_Fs5dKc3n_JLiZmQzeK5VnGa9rzdRJtRso"

type resendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

func SendMagicLink(email, token, baseURL string) error {
	link := fmt.Sprintf("%s/auth/verify?token=%s", baseURL, token)

	body := resendRequest{
		From:    "Portal <portal@resend.dev>",
		To:      []string{email},
		Subject: "Your login link",
		HTML:    fmt.Sprintf(`<p>Click the link below to sign in:</p><p><a href="%s">Sign in to Portal</a></p><p>This link expires in 15 minutes.</p>`, link),
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
	req.Header.Set("Authorization", "Bearer "+resendAPIKey)

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
