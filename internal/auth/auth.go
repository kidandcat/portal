package auth

import (
	"net/http"

	"github.com/kidandcat/portal/internal/db"
)

const sessionCookie = "portal_session"

func CurrentUser(r *http.Request) *db.User {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil
	}
	user, err := db.GetUserBySession(cookie.Value)
	if err != nil {
		return nil
	}
	return user
}

func SetSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   sessionCookie,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

func Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookie)
	if err == nil {
		db.DeleteSession(cookie.Value)
	}
	ClearSessionCookie(w)
}
