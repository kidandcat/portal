package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		renderTemplate(w, "login.html", nil)
		return
	}
	email := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	if email == "" {
		renderTemplate(w, "login.html", map[string]any{"Error": "Email is required"})
		return
	}

	// Check user exists
	var exists bool
	db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE email = ?)", email).Scan(&exists)
	if !exists {
		renderTemplate(w, "login.html", map[string]any{"Error": "No account with that email"})
		return
	}

	token := generateToken()
	db.Exec("INSERT INTO magic_tokens (email, token) VALUES (?, ?)", email, token)

	link := fmt.Sprintf("%s/auth/approve?token=%s", cfg.BaseURL, token)
	go sendMagicEmail(email, link)
	log.Printf("Magic link for %s: %s", email, link)

	renderTemplate(w, "login_sent.html", map[string]any{"Email": email})
}

func handleApprove(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Invalid token", http.StatusBadRequest)
		return
	}

	var mt MagicToken
	err := db.QueryRow(
		"SELECT id, email, token, created_at, approved_at FROM magic_tokens WHERE token = ?", token,
	).Scan(&mt.ID, &mt.Email, &mt.Token, &mt.CreatedAt, &mt.ApprovedAt)
	if err != nil {
		renderTemplate(w, "approve.html", map[string]any{"Error": "Invalid or expired link"})
		return
	}
	if mt.ApprovedAt != nil {
		renderTemplate(w, "approve.html", map[string]any{"Error": "This link has already been used"})
		return
	}
	if time.Since(mt.CreatedAt) > 15*time.Minute {
		renderTemplate(w, "approve.html", map[string]any{"Error": "This link has expired (15 min)"})
		return
	}

	if r.Method == "GET" {
		renderTemplate(w, "approve.html", map[string]any{"Token": token, "Email": mt.Email})
		return
	}

	// POST: approve the token
	db.Exec("UPDATE magic_tokens SET approved_at = CURRENT_TIMESTAMP WHERE id = ?", mt.ID)

	var u User
	err = db.QueryRow("SELECT id, email, name, role FROM users WHERE email = ?", mt.Email).Scan(&u.ID, &u.Email, &u.Name, &u.Role)
	if err != nil {
		http.Error(w, "User not found", http.StatusInternalServerError)
		return
	}

	sessionToken := generateToken()
	expires := time.Now().Add(30 * 24 * time.Hour) // 30 days
	db.Exec("INSERT INTO sessions (user_id, token, expires_at) VALUES (?, ?, ?)", u.ID, sessionToken, expires)

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionToken,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil {
		db.Exec("DELETE FROM sessions WHERE token = ?", cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "session", MaxAge: -1, Path: "/"})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func sendMagicEmail(to, link string) {
	if cfg.SMTPHost == "" {
		log.Printf("SMTP not configured, magic link: %s", link)
		return
	}
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: Portal Login\r\n\r\nClick to log in:\n%s\n\nThis link expires in 15 minutes.",
		cfg.SMTPFrom, to, link)
	auth := smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPHost)
	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)
	if err := smtp.SendMail(addr, auth, cfg.SMTPFrom, []string{to}, []byte(msg)); err != nil {
		log.Printf("Failed to send email to %s: %v", to, err)
	}
}
