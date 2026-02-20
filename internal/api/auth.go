package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/kidandcat/portal/internal/auth"
	"github.com/kidandcat/portal/internal/config"
	"github.com/kidandcat/portal/internal/db"
)

func RegisterAuthRoutes(mux *http.ServeMux, cfg config.Config) {
	mux.HandleFunc("POST /api/auth/magic-link", handleMagicLink(cfg))
	mux.HandleFunc("GET /api/auth/check-status", handleCheckStatus)
	mux.HandleFunc("POST /api/auth/logout", handleLogout)
	mux.HandleFunc("GET /api/auth/me", handleMe)

	// Server-side rendered pages for email verification
	mux.HandleFunc("GET /auth/verify", handleVerifyPage(cfg))
	mux.HandleFunc("POST /auth/verify", handleVerifyApprove(cfg))
}

func handleMagicLink(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		email := strings.TrimSpace(req.Email)
		if email == "" {
			writeError(w, http.StatusBadRequest, "email required")
			return
		}

		token, err := db.CreateMagicToken(email)
		if err != nil {
			log.Printf("error creating magic token: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		if err := auth.SendMagicLink(email, token, cfg); err != nil {
			log.Printf("error sending magic link: %v", err)
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"token":  token,
			"status": "pending",
		})
	}
}

func handleCheckStatus(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "token required")
		return
	}

	status, email, err := db.CheckMagicTokenStatus(token)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "invalid"})
		return
	}

	if status == "expired" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "expired"})
		return
	}

	if status != "approved" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "pending"})
		return
	}

	// Approved - create session
	if err := db.MarkMagicTokenUsed(token); err != nil {
		log.Printf("error marking token used: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	user, err := db.GetOrCreateUser(email)
	if err != nil {
		log.Printf("error getting/creating user: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	sessionToken, err := db.CreateSession(user.ID)
	if err != nil {
		log.Printf("error creating session: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	auth.SetSessionCookie(w, sessionToken)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "approved",
		"redirect": "/",
		"user":     map[string]any{"id": user.ID, "email": user.Email},
	})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	auth.Logout(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleMe(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r)
	if user == nil {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"user":          map[string]any{"id": user.ID, "email": user.Email},
	})
}

// Server-rendered verification pages (opened from email)
func handleVerifyPage(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "Missing token", http.StatusBadRequest)
			return
		}

		email, err := db.ValidateMagicToken(token)
		if err != nil {
			http.Error(w, "Invalid or expired link", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Approve Sign-in - ` + cfg.Branding.AppName + `</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#0f1117;color:#e2e8f0;display:flex;align-items:center;justify-content:center;min-height:100vh}
.card{background:#1a1d27;border:1px solid #2a2d3e;border-radius:16px;padding:40px;max-width:400px;width:90%;text-align:center}
h1{font-size:20px;margin-bottom:8px}
.email{color:#3b82f6;font-size:14px;margin-bottom:24px}
.btn{display:inline-block;background:#3b82f6;color:#fff;border:none;border-radius:10px;padding:12px 32px;font-size:15px;cursor:pointer;text-decoration:none;transition:background .15s}
.btn:hover{background:#2563eb}
</style></head><body>
<div class="card">
<h1>Approve sign-in</h1>
<p class="email">` + email + `</p>
<form method="POST" action="/auth/verify">
<input type="hidden" name="token" value="` + token + `">
<button type="submit" class="btn">Approve session</button>
</form>
</div></body></html>`))
	}
}

func handleVerifyApprove(_ config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.FormValue("token")
		if token == "" {
			http.Error(w, "Missing token", http.StatusBadRequest)
			return
		}

		_, err := db.ApproveMagicToken(token)
		if err != nil {
			http.Error(w, "Invalid or expired link", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Session Approved</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#0f1117;color:#e2e8f0;display:flex;align-items:center;justify-content:center;min-height:100vh}
.card{background:#1a1d27;border:1px solid #2a2d3e;border-radius:16px;padding:40px;max-width:400px;width:90%;text-align:center}
.check{font-size:48px;margin-bottom:16px}
h1{font-size:20px;margin-bottom:8px}
p{color:#94a3b8;font-size:14px}
</style></head><body>
<div class="card">
<div class="check">&#10003;</div>
<h1>Session approved</h1>
<p>You can close this tab now.</p>
</div></body></html>`))
	}
}
