package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
		renderTemplate(w, "login.html", map[string]any{"Error": "El email es obligatorio"})
		return
	}

	var exists bool
	db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE email = ?)", email).Scan(&exists)
	if !exists {
		renderTemplate(w, "login.html", map[string]any{"Error": "No existe una cuenta con ese email"})
		return
	}

	token := generateToken()
	db.Exec("INSERT INTO magic_tokens (email, token) VALUES (?, ?)", email, token)

	link := fmt.Sprintf("%s/auth/approve?token=%s", cfg.BaseURL, token)
	go sendMagicEmail(email, link)
	log.Printf("Magic link for %s: %s", email, link)

	renderTemplate(w, "login_sent.html", map[string]any{"Email": email, "Token": token})
}

func handleApprove(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Token inválido", http.StatusBadRequest)
		return
	}

	var mt MagicToken
	err := db.QueryRow(
		"SELECT id, email, token, created_at, approved_at FROM magic_tokens WHERE token = ?", token,
	).Scan(&mt.ID, &mt.Email, &mt.Token, &mt.CreatedAt, &mt.ApprovedAt)
	if err != nil {
		renderTemplate(w, "approve.html", map[string]any{"Error": "Enlace inválido o expirado"})
		return
	}
	if mt.ApprovedAt != nil {
		renderTemplate(w, "approve.html", map[string]any{"Error": "Este enlace ya ha sido utilizado"})
		return
	}
	if time.Since(mt.CreatedAt) > 15*time.Minute {
		renderTemplate(w, "approve.html", map[string]any{"Error": "Este enlace ha expirado (15 min)"})
		return
	}

	if r.Method == "GET" {
		renderTemplate(w, "approve.html", map[string]any{"Token": token, "Email": mt.Email})
		return
	}

	db.Exec("UPDATE magic_tokens SET approved_at = CURRENT_TIMESTAMP WHERE id = ?", mt.ID)

	var u User
	err = db.QueryRow("SELECT id, email, name, role FROM users WHERE email = ?", mt.Email).Scan(&u.ID, &u.Email, &u.Name, &u.Role)
	if err != nil {
		http.Error(w, "Usuario no encontrado", http.StatusInternalServerError)
		return
	}

	sessionToken := generateToken()
	expires := time.Now().Add(30 * 24 * time.Hour)
	db.Exec("INSERT INTO sessions (user_id, token, expires_at) VALUES (?, ?, ?)", u.ID, sessionToken, expires)

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionToken,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	renderTemplate(w, "approve.html", map[string]any{"Approved": true, "Email": mt.Email})
}

func handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	token := r.URL.Query().Get("token")
	if token == "" {
		w.Write([]byte(`{"approved":false}`))
		return
	}

	// If this browser already has a valid session, just return approved
	if cookie, err := r.Cookie("session"); err == nil {
		var count int
		db.QueryRow("SELECT COUNT(*) FROM sessions WHERE token = ?", cookie.Value).Scan(&count)
		if count > 0 {
			w.Write([]byte(`{"approved":true}`))
			return
		}
	}

	var mt MagicToken
	err := db.QueryRow(
		"SELECT id, email, token, created_at, approved_at FROM magic_tokens WHERE token = ?", token,
	).Scan(&mt.ID, &mt.Email, &mt.Token, &mt.CreatedAt, &mt.ApprovedAt)
	if err != nil || mt.ApprovedAt == nil {
		w.Write([]byte(`{"approved":false}`))
		return
	}

	var u User
	err = db.QueryRow("SELECT id, email, name, role FROM users WHERE email = ?", mt.Email).Scan(&u.ID, &u.Email, &u.Name, &u.Role)
	if err != nil {
		w.Write([]byte(`{"approved":false}`))
		return
	}

	sessionToken := generateToken()
	expires := time.Now().Add(30 * 24 * time.Hour)
	db.Exec("INSERT INTO sessions (user_id, token, expires_at) VALUES (?, ?, ?)", u.ID, sessionToken, expires)

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionToken,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	w.Write([]byte(`{"approved":true}`))
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
	if cfg.ResendAPIKey == "" {
		log.Printf("RESEND_API_KEY not configured, magic link: %s", link)
		return
	}

	htmlBody := fmt.Sprintf(`<div style="font-family:sans-serif;max-width:480px;margin:0 auto;padding:24px">
<h2 style="color:#333">Acceso a Portal</h2>
<p>Haz clic en el siguiente botón para iniciar sesión:</p>
<a href="%s" style="display:inline-block;background:#2563eb;color:#fff;padding:12px 24px;border-radius:6px;text-decoration:none;font-weight:bold">Iniciar sesión</a>
<p style="margin-top:24px;color:#666;font-size:14px">Este enlace caduca en 15 minutos.</p>
<p style="color:#999;font-size:12px">Si no solicitaste este enlace, ignora este mensaje.</p>
</div>`, link)

	payload, _ := json.Marshal(map[string]any{
		"from":    "Portal <portal@mentasystems.com>",
		"to":      []string{to},
		"subject": "Tu enlace de acceso",
		"html":    htmlBody,
	})

	req, _ := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+cfg.ResendAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("Failed to send email to %s: %v", to, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var resErr map[string]any
		json.NewDecoder(resp.Body).Decode(&resErr)
		log.Printf("Resend API error (%d) sending to %s: %v", resp.StatusCode, to, resErr)
	}
}
