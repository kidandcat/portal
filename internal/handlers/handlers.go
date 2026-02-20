package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/kidandcat/portal/internal/auth"
	"github.com/kidandcat/portal/internal/config"
	"github.com/kidandcat/portal/internal/db"
)

var templates *template.Template

func Init(templateDir string) {
	funcMap := template.FuncMap{
		"json": func(v any) template.JS {
			b, _ := json.Marshal(v)
			return template.JS(b)
		},
		"inc": func(i int) int {
			return i + 1
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
	mux.HandleFunc("GET /auth/verify", handleVerifyPage(cfg))
	mux.HandleFunc("POST /auth/verify", handleVerifyApprove(cfg))
	mux.HandleFunc("GET /auth/check-status", handleCheckStatus(cfg))
	mux.HandleFunc("POST /auth/logout", handleLogout)
	mux.HandleFunc("GET /p/{slug}", handleProject(cfg))
	mux.HandleFunc("POST /p/{slug}/comment", handleCreateComment(cfg))
	mux.HandleFunc("POST /p/{slug}/comment/{id}/resolve", handleResolveComment(cfg))
	mux.HandleFunc("POST /p/{slug}/contact", handleContact(cfg))
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

		if len(projects) == 1 {
			http.Redirect(w, r, "/p/"+projects[0].Slug, http.StatusFound)
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

func handleVerifyPage(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		data := brandingData(cfg)
		data["Token"] = token
		data["Email"] = email
		templates.ExecuteTemplate(w, "verify.html", data)
	}
}

func handleVerifyApprove(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.FormValue("token")
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

func handleContact(_ config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		project, err := db.GetProjectBySlug(slug)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		name := strings.TrimSpace(r.FormValue("name"))
		email := strings.TrimSpace(r.FormValue("email"))
		content := strings.TrimSpace(r.FormValue("message"))

		if content == "" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<p class="contact-error">Message is required.</p>`))
			return
		}

		if err := db.CreateMessage(project.ID, name, email, content); err != nil {
			log.Printf("error saving message: %v", err)
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<p class="contact-error">Error sending message. Please try again.</p>`))
			return
		}

		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<p class="contact-success">Message sent successfully!</p>`))
	}
}

func handleProject(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		project, err := db.GetProjectBySlug(slug)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		elements, err := db.GetElementsByProject(project.ID)
		if err != nil {
			log.Printf("error getting elements: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		comments, err := db.GetCommentsByProject(project.ID)
		if err != nil {
			log.Printf("error getting comments: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		data := brandingData(cfg)
		data["Project"] = project
		data["Elements"] = elements
		data["Comments"] = comments
		templates.ExecuteTemplate(w, "project.html", data)
	}
}

func handleCreateComment(_ config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		project, err := db.GetProjectBySlug(slug)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		content := strings.TrimSpace(r.FormValue("content"))
		email := strings.TrimSpace(r.FormValue("email"))
		xStr := r.FormValue("x")
		yStr := r.FormValue("y")

		if content == "" {
			http.Error(w, "Content is required", http.StatusBadRequest)
			return
		}

		x, err := strconv.ParseFloat(xStr, 64)
		if err != nil {
			http.Error(w, "Invalid x coordinate", http.StatusBadRequest)
			return
		}
		y, err := strconv.ParseFloat(yStr, 64)
		if err != nil {
			http.Error(w, "Invalid y coordinate", http.StatusBadRequest)
			return
		}

		id, err := db.CreateComment(project.ID, email, content, x, y)
		if err != nil {
			log.Printf("error creating comment: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		// Return the new comment pin + tooltip as HTML
		comments, _ := db.GetCommentsByProject(project.ID)
		idx := len(comments)
		for i, c := range comments {
			if c.ID == id {
				idx = i + 1
				break
			}
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<div class="comment-pin" data-id="%d" style="left:%.0fpx;top:%.0fpx">
			<span class="pin-number">%d</span>
			<div class="comment-tooltip">
				<div class="comment-meta">%s</div>
				<div class="comment-text">%s</div>
				<form hx-post="/p/%s/comment/%d/resolve" hx-target="closest .comment-pin" hx-swap="outerHTML">
					<button type="submit" class="resolve-btn">Resolve</button>
				</form>
			</div>
		</div>`, id, x, y, idx, template.HTMLEscapeString(email), template.HTMLEscapeString(content), slug, id)
	}
}

func handleResolveComment(_ config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		project, err := db.GetProjectBySlug(slug)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		idStr := r.PathValue("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid comment ID", http.StatusBadRequest)
			return
		}

		if err := db.ResolveComment(id, project.ID); err != nil {
			log.Printf("error resolving comment: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		// Return empty to remove the pin
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(""))
	}
}
