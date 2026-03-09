package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const apiProjectIDKey contextKey = "api_project_id"
const apiProjectSlugKey contextKey = "api_project_slug"

func apiKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"missing Authorization header"}`, http.StatusUnauthorized)
			return
		}
		key := strings.TrimPrefix(auth, "Bearer ")

		var projectID int64
		var slug string
		err := db.QueryRow(`
			SELECT ak.project_id, p.slug
			FROM api_keys ak
			JOIN projects p ON p.id = ak.project_id
			WHERE ak.key = ?`, key).Scan(&projectID, &slug)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), apiProjectIDKey, projectID)
		ctx = context.WithValue(ctx, apiProjectSlugKey, slug)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func handleAPIPushDashboard(w http.ResponseWriter, r *http.Request) {
	slug := r.Context().Value(apiProjectSlugKey).(string)
	reqSlug := r.PathValue("slug")
	if reqSlug != slug {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"API key does not match project"}`, http.StatusForbidden)
		return
	}

	path := r.PathValue("path")
	if path == "" {
		path = "index.html"
	}

	// Sanitize path
	clean := filepath.Clean(path)
	if strings.Contains(clean, "..") {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return
	}

	dir := filepath.Join(cfg.DashboardDir, slug)
	fullPath := filepath.Join(dir, clean)

	os.MkdirAll(filepath.Dir(fullPath), 0755)

	body, err := io.ReadAll(io.LimitReader(r.Body, 50<<20)) // 50MB limit
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}

	if err := os.WriteFile(fullPath, body, 0644); err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"failed to write file"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ok":true,"path":%q,"size":%d}`, clean, len(body))
}

func handleAPIPushStatus(w http.ResponseWriter, r *http.Request) {
	projectID := r.Context().Value(apiProjectIDKey).(int64)
	slug := r.Context().Value(apiProjectSlugKey).(string)
	if r.PathValue("slug") != slug {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"API key does not match project"}`, http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}

	if _, err := db.Exec("UPDATE projects SET status_md = ? WHERE id = ?", string(body), projectID); err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"failed to update status"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ok":true,"size":%d}`, len(body))
}

func handleAPIPushRoadmap(w http.ResponseWriter, r *http.Request) {
	projectID := r.Context().Value(apiProjectIDKey).(int64)
	slug := r.Context().Value(apiProjectSlugKey).(string)
	if r.PathValue("slug") != slug {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"API key does not match project"}`, http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}

	if _, err := db.Exec("UPDATE projects SET roadmap_md = ? WHERE id = ?", string(body), projectID); err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"failed to update roadmap"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ok":true,"size":%d}`, len(body))
}

func handleAPICreateIssue(w http.ResponseWriter, r *http.Request) {
	projectID := r.Context().Value(apiProjectIDKey).(int64)
	slug := r.Context().Value(apiProjectSlugKey).(string)
	if r.PathValue("slug") != slug {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"API key does not match project"}`, http.StatusForbidden)
		return
	}

	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
		Priority    string `json:"priority"`
		MilestoneID *int64 `json:"milestone_id"`
	}

	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"title is required"}`, http.StatusBadRequest)
		return
	}

	status := req.Status
	if status == "" {
		status = "backlog"
	}
	priority := req.Priority
	if priority == "" {
		priority = "medium"
	}

	var maxPos int
	db.QueryRow("SELECT COALESCE(MAX(position), 0) FROM issues WHERE project_id = ? AND status = ?", projectID, status).Scan(&maxPos)

	result, err := db.Exec(`INSERT INTO issues (project_id, title, description, status, priority, milestone_id, position)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		projectID, title, req.Description, status, priority, req.MilestoneID, maxPos+1)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"failed to create issue"}`, http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ok":true,"id":%d}`, id)
}

func handleProjectDashboard(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	var exists bool
	db.QueryRow("SELECT EXISTS(SELECT 1 FROM projects WHERE slug = ?)", slug).Scan(&exists)
	if !exists {
		http.NotFound(w, r)
		return
	}

	indexPath := filepath.Join(cfg.DashboardDir, slug, "index.html")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}

	http.ServeFile(w, r, indexPath)
}

func handleProjectDashboardAsset(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	path := r.PathValue("path")

	clean := filepath.Clean(path)
	if strings.Contains(clean, "..") {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	var exists bool
	db.QueryRow("SELECT EXISTS(SELECT 1 FROM projects WHERE slug = ?)", slug).Scan(&exists)
	if !exists {
		http.NotFound(w, r)
		return
	}

	filePath := filepath.Join(cfg.DashboardDir, slug, clean)
	http.ServeFile(w, r, filePath)
}

func generateAPIKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return "pk_" + hex.EncodeToString(b)
}

func handleLlmsTxt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, `# Portal Menta Systems — API for LLMs

Base URL: %s

## Authentication

All API endpoints require a project API key sent as a Bearer token:

    Authorization: Bearer pk_your_api_key_here

## Endpoints

### Push Dashboard File

Upload or update a file in the project's client-facing dashboard.

    PUT /api/projects/{slug}/dashboard/{path}

- Body: raw file content
- Content-Type: matches the file type
- Path: relative file path (e.g., index.html, style.css, images/logo.png)
- Max size: 50MB

Example:

    curl -X PUT \
      -H "Authorization: Bearer pk_..." \
      -H "Content-Type: text/html" \
      --data-binary @project/index.html \
      %s/api/projects/myproject/dashboard/index.html

### Push Status

Update the project status document (Markdown).

    PUT /api/projects/{slug}/status

- Body: raw Markdown content
- Max size: 1MB

Example:

    curl -X PUT \
      -H "Authorization: Bearer pk_..." \
      --data-binary @STATUS.md \
      %s/api/projects/myproject/status

### Push Roadmap

Update the project roadmap document (Markdown).

    PUT /api/projects/{slug}/roadmap

- Body: raw Markdown content
- Max size: 1MB

Example:

    curl -X PUT \
      -H "Authorization: Bearer pk_..." \
      --data-binary @ROADMAP.md \
      %s/api/projects/myproject/roadmap

### Create Issue

Create a new issue/task in the project.

    POST /api/projects/{slug}/issues

- Body: JSON object
- Fields: title (required), description, status (backlog|todo|in_progress|review|done), priority (low|medium|high|urgent), milestone_id

Example:

    curl -X POST \
      -H "Authorization: Bearer pk_..." \
      -H "Content-Type: application/json" \
      -d '{"title":"My task","description":"Details","status":"backlog","priority":"medium"}' \
      %s/api/projects/myproject/issues

## Dashboard Serving

Project dashboards are served at (requires session authentication):

    GET /{slug}              → index.html
    GET /{slug}/{path...}    → static assets

## Response Format

Success:

    {"ok": true, "path": "index.html", "size": 1234}

Error:

    {"error": "description of what went wrong"}
`, cfg.BaseURL, cfg.BaseURL, cfg.BaseURL, cfg.BaseURL, cfg.BaseURL)
}
