package handlers

import (
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

func RegisterRoutes(mux *http.ServeMux, cfg config.Config) {
	mux.HandleFunc("GET /{$}", handleIndex)
	mux.HandleFunc("POST /auth/magic-link", handleMagicLink(cfg))
	mux.HandleFunc("GET /auth/verify", handleVerify)
	mux.HandleFunc("POST /auth/logout", handleLogout)
	mux.HandleFunc("GET /p/{slug}", handleProject)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	user := auth.CurrentUser(r)
	if user == nil {
		templates.ExecuteTemplate(w, "login.html", nil)
		return
	}

	projects, err := db.GetProjectsByOwner(user.ID)
	if err != nil {
		log.Printf("error getting projects: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	templates.ExecuteTemplate(w, "projects.html", map[string]any{
		"User":     user,
		"Projects": projects,
	})
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
		w.Write([]byte(`<p class="success">Check your email for the login link.</p>`))
	}
}

func handleVerify(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Missing token", http.StatusBadRequest)
		return
	}

	email, err := db.ValidateMagicToken(token)
	if err != nil {
		log.Printf("invalid token: %v", err)
		http.Error(w, "Invalid or expired link", http.StatusBadRequest)
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
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	auth.Logout(w, r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleProject(w http.ResponseWriter, r *http.Request) {
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

	templates.ExecuteTemplate(w, "project.html", map[string]any{
		"Project": project,
		"Pages":   pages,
	})
}
