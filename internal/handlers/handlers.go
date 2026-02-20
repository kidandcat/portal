package handlers

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/kidandcat/portal/internal/auth"
	"github.com/kidandcat/portal/internal/config"
	"github.com/kidandcat/portal/internal/db"
	"github.com/yuin/goldmark"
)

var templates *template.Template

func Init(templateDir string) {
	funcMap := template.FuncMap{
		"markdown": func(content string) template.HTML {
			var buf strings.Builder
			if err := goldmark.Convert([]byte(content), &buf); err != nil {
				return template.HTML("<p>Error rendering markdown</p>")
			}
			return template.HTML(buf.String())
		},
	}
	templates = template.Must(template.New("").Funcs(funcMap).ParseGlob(templateDir + "/*.html"))
}

func brandingData(cfg config.Config) map[string]any {
	return map[string]any{
		"AppName":      cfg.Branding.AppName,
		"PrimaryColor": cfg.Branding.PrimaryColor,
		"LogoURL":      cfg.Branding.LogoURL,
	}
}

func RegisterRoutes(mux *http.ServeMux, cfg config.Config) {
	mux.HandleFunc("GET /{$}", handleIndex(cfg))
	mux.HandleFunc("POST /auth/magic-link", handleMagicLink(cfg))
	mux.HandleFunc("GET /auth/verify", handleVerify(cfg))
	mux.HandleFunc("GET /auth/check-status", handleCheckStatus(cfg))
	mux.HandleFunc("POST /auth/logout", handleLogout)
	mux.HandleFunc("GET /p/{slug}", handleProject(cfg))
}

func handleIndex(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.CurrentUser(r)
		if user == nil {
			data := brandingData(cfg)
			templates.ExecuteTemplate(w, "login.html", data)
			return
		}

		projects, err := db.GetProjectsByOwner(user.ID)
		if err != nil {
			log.Printf("error getting projects: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		data := brandingData(cfg)
		data["User"] = user
		data["Projects"] = projects
		templates.ExecuteTemplate(w, "projects.html", data)
	}
}

func handleMagicLink(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := strings.TrimSpace(r.FormValue("email"))
		if email == "" {
			http.Error(w, "Email is required", http.StatusBadRequest)
			return
		}

		token, err := db.CreateMagicToken(email)
		if err != nil {
			log.Printf("error creating magic token: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		if err := auth.SendMagicLink(email, token, cfg); err != nil {
			log.Printf("error sending magic link: %v", err)
		}

		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<div id="waiting" hx-get="/auth/check-status?token=` + token + `" hx-trigger="every 2s" hx-swap="innerHTML" hx-target="#result">` +
			`<p class="success">Check your email and click the login link.</p>` +
			`<p class="waiting-hint">Waiting for approval…</p>` +
			`</div>`))
	}
}

func handleVerify(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "Missing token", http.StatusBadRequest)
			return
		}

		_, err := db.ApproveMagicToken(token)
		if err != nil {
			log.Printf("invalid token: %v", err)
			http.Error(w, "Invalid or expired link", http.StatusBadRequest)
			return
		}

		data := brandingData(cfg)
		templates.ExecuteTemplate(w, "approved.html", data)
	}
}

func handleCheckStatus(_ config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "Missing token", http.StatusBadRequest)
			return
		}

		status, email, err := db.CheckMagicTokenStatus(token)
		if err != nil {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<p class="error">Link expired or invalid.</p>`))
			return
		}

		if status == "expired" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<p class="error">Link expired. Please request a new one.</p>`))
			return
		}

		if status != "approved" {
			// Still pending - keep polling
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<div hx-get="/auth/check-status?token=` + token + `" hx-trigger="every 2s" hx-swap="innerHTML" hx-target="#result">` +
				`<p class="success">Check your email and click the login link.</p>` +
				`<p class="waiting-hint">Waiting for approval…</p>` +
				`</div>`))
			return
		}

		// Approved! Create session
		if err := db.MarkMagicTokenUsed(token); err != nil {
			log.Printf("error marking token used: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		user, err := db.GetOrCreateUser(email)
		if err != nil {
			log.Printf("error getting/creating user: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		sessionToken, err := db.CreateSession(user.ID)
		if err != nil {
			log.Printf("error creating session: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		auth.SetSessionCookie(w, sessionToken)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "approved", "redirect": "/"})
	}
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	auth.Logout(w, r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleProject(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		project, err := db.GetProjectBySlug(slug)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		pages, err := db.GetPagesByProject(project.ID)
		if err != nil {
			log.Printf("error getting pages: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		data := brandingData(cfg)
		data["Project"] = project
		data["Pages"] = pages
		templates.ExecuteTemplate(w, "project.html", data)
	}
}
