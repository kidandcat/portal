package main

import (
	"context"
	"net/http"
	"time"
)

type contextKey string

const userKey contextKey = "user"

func currentUser(r *http.Request) *User {
	if u, ok := r.Context().Value(userKey).(*User); ok {
		return u
	}
	return nil
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		var userID int64
		var expiresAt time.Time
		err = db.QueryRow(
			"SELECT user_id, expires_at FROM sessions WHERE token = ?",
			cookie.Value,
		).Scan(&userID, &expiresAt)
		if err != nil || time.Now().After(expiresAt) {
			http.SetCookie(w, &http.Cookie{Name: "session", MaxAge: -1, Path: "/"})
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		var u User
		err = db.QueryRow(
			"SELECT id, email, name, role FROM users WHERE id = ?", userID,
		).Scan(&u.ID, &u.Email, &u.Name, &u.Role)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), userKey, &u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if u == nil || u.Role != "admin" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}
