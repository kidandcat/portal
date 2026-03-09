package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
)

//go:embed static/*
var staticFS embed.FS

func main() {
	configPath := "config.json"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}
	loadConfig(configPath)
	initDB(cfg.DBPath)
	initTemplates()
	os.MkdirAll(cfg.UploadDir, 0755)
	os.MkdirAll(cfg.DashboardDir, 0755)

	// Sync admin users from config
	for _, email := range cfg.AdminEmails {
		email = strings.TrimSpace(strings.ToLower(email))
		if email == "" {
			continue
		}
		var exists bool
		db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE email = ?)", email).Scan(&exists)
		if !exists {
			name := strings.Split(email, "@")[0]
			db.Exec("INSERT INTO users (email, name, role) VALUES (?, ?, 'admin')", email, name)
			log.Printf("Created admin user: %s", email)
		} else {
			db.Exec("UPDATE users SET role = 'admin' WHERE email = ?", email)
		}
	}

	mux := http.NewServeMux()

	// Static files
	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Public routes
	mux.HandleFunc("GET /login", handleLogin)
	mux.HandleFunc("POST /login", handleLogin)
	mux.HandleFunc("GET /auth/approve", handleApprove)
	mux.HandleFunc("POST /auth/approve", handleApprove)
	mux.HandleFunc("GET /auth/status", handleAuthStatus)
	mux.HandleFunc("POST /logout", handleLogout)
	mux.HandleFunc("GET /llms.txt", handleLlmsTxt)

	// API routes (API key auth)
	api := http.NewServeMux()
	api.HandleFunc("PUT /projects/{slug}/dashboard/{path...}", handleAPIPushDashboard)
	api.HandleFunc("PUT /projects/{slug}/status", handleAPIPushStatus)
	api.HandleFunc("PUT /projects/{slug}/roadmap", handleAPIPushRoadmap)
	api.HandleFunc("POST /projects/{slug}/issues", handleAPICreateIssue)
	mux.Handle("/api/", http.StripPrefix("/api", apiKeyAuth(api)))

	// Authenticated routes
	app := http.NewServeMux()
	app.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	})
	app.HandleFunc("GET /dashboard", handleDashboard)
	app.HandleFunc("POST /projects", handleCreateProject)
	app.HandleFunc("GET /projects/{slug}", handleProject)
	app.HandleFunc("GET /projects/{slug}/settings", handleProjectSettings)
	app.HandleFunc("POST /projects/{slug}/settings", handleProjectSettings)

	// Issues
	app.HandleFunc("POST /projects/{slug}/issues", handleCreateIssue)
	app.HandleFunc("PUT /projects/{slug}/issues/{id}", handleUpdateIssue)
	app.HandleFunc("POST /projects/{slug}/issues/{id}", handleUpdateIssue)
	app.HandleFunc("DELETE /projects/{slug}/issues/{id}", handleDeleteIssue)

	// Milestones
	app.HandleFunc("POST /projects/{slug}/milestones", handleCreateMilestone)
	app.HandleFunc("POST /projects/{slug}/milestones/{id}", handleUpdateMilestone)
	app.HandleFunc("DELETE /projects/{slug}/milestones/{id}", handleDeleteMilestone)

	// Files
	app.HandleFunc("POST /projects/{slug}/files", handleUploadFile)
	app.HandleFunc("POST /projects/{slug}/folders", handleCreateFolder)
	app.HandleFunc("GET /projects/{slug}/files/{id}/download", handleDownloadFile)
	app.HandleFunc("DELETE /projects/{slug}/files/{id}", handleDeleteFile)

	// Project dashboards (served at /{slug})
	app.HandleFunc("GET /{slug}", handleProjectDashboard)
	app.HandleFunc("GET /{slug}/{path...}", handleProjectDashboardAsset)

	mux.Handle("/", authMiddleware(app))

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("Portal running on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
