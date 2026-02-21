package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
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

	// Seed admin user if no users exist
	var count int
	db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if count == 0 && cfg.AdminEmail != "" {
		db.Exec("INSERT INTO users (email, name, role) VALUES (?, 'Admin', 'admin')", cfg.AdminEmail)
		log.Printf("Created admin user: %s", cfg.AdminEmail)
	}

	mux := http.NewServeMux()

	// Static files
	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Auth routes (public)
	mux.HandleFunc("GET /login", handleLogin)
	mux.HandleFunc("POST /login", handleLogin)
	mux.HandleFunc("GET /auth/approve", handleApprove)
	mux.HandleFunc("POST /auth/approve", handleApprove)
	mux.HandleFunc("POST /logout", handleLogout)

	// Authenticated routes
	app := http.NewServeMux()
	app.HandleFunc("GET /", handleDashboard)
	app.HandleFunc("POST /projects", handleCreateProject)
	app.HandleFunc("GET /projects/{slug}", handleProject)
	app.HandleFunc("GET /projects/{slug}/settings", handleProjectSettings)
	app.HandleFunc("POST /projects/{slug}/settings", handleProjectSettings)

	// Issues
	app.HandleFunc("POST /projects/{slug}/issues", handleCreateIssue)
	app.HandleFunc("PUT /projects/{slug}/issues/{id}", handleUpdateIssue)
	app.HandleFunc("POST /projects/{slug}/issues/{id}", handleUpdateIssue)
	app.HandleFunc("DELETE /projects/{slug}/issues/{id}", handleDeleteIssue)

	// Files
	app.HandleFunc("POST /projects/{slug}/files", handleUploadFile)
	app.HandleFunc("POST /projects/{slug}/folders", handleCreateFolder)
	app.HandleFunc("GET /projects/{slug}/files/{id}/download", handleDownloadFile)
	app.HandleFunc("DELETE /projects/{slug}/files/{id}", handleDeleteFile)

	// Chat
	app.HandleFunc("POST /projects/{slug}/chat", handleSendMessage)
	app.HandleFunc("GET /projects/{slug}/chat/poll", handleChatPoll)

	// Admin
	app.HandleFunc("GET /admin", requireAdmin(handleAdmin))
	app.HandleFunc("POST /admin/users", requireAdmin(handleAdminCreateUser))
	app.HandleFunc("DELETE /admin/users/{id}", requireAdmin(handleAdminDeleteUser))

	mux.Handle("/", authMiddleware(app))

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("Portal running on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
