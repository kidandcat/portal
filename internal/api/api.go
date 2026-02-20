package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/kidandcat/portal/internal/config"
	"github.com/kidandcat/portal/internal/db"
)

func RegisterRoutes(mux *http.ServeMux, _ config.Config) {
	// Projects
	mux.HandleFunc("GET /api/projects", handleGetProjects)
	mux.HandleFunc("POST /api/projects", handleCreateProject)
	mux.HandleFunc("PUT /api/projects/{id}", handleUpdateProject)
	mux.HandleFunc("DELETE /api/projects/{id}", handleDeleteProject)

	// Elements
	mux.HandleFunc("GET /api/elements", handleGetElements)
	mux.HandleFunc("POST /api/elements", handleCreateElement)
	mux.HandleFunc("PUT /api/elements/{id}", handleUpdateElement)
	mux.HandleFunc("DELETE /api/elements/{id}", handleDeleteElement)

	// Connections
	mux.HandleFunc("GET /api/connections", handleGetConnections)
	mux.HandleFunc("POST /api/connections", handleCreateConnection)
	mux.HandleFunc("DELETE /api/connections/{id}", handleDeleteConnection)

	// Comments
	mux.HandleFunc("GET /api/comments", handleGetComments)
	mux.HandleFunc("POST /api/comments", handleCreateComment)
	mux.HandleFunc("PUT /api/comments/{id}", handleUpdateComment)
	mux.HandleFunc("DELETE /api/comments/{id}", handleDeleteComment)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// Projects

func handleGetProjects(w http.ResponseWriter, r *http.Request) {
	slug := r.URL.Query().Get("slug")
	if slug != "" {
		p, err := db.GetProjectBySlug(slug)
		if err != nil {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		writeJSON(w, http.StatusOK, []db.Project{*p})
		return
	}

	projects, err := db.GetProjects()
	if err != nil {
		log.Printf("error getting projects: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if projects == nil {
		projects = []db.Project{}
	}
	writeJSON(w, http.StatusOK, projects)
}

func handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" || req.Slug == "" {
		writeError(w, http.StatusBadRequest, "name and slug required")
		return
	}

	p, err := db.CreateProject(req.Name, req.Slug)
	if err != nil {
		log.Printf("error creating project: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	p, err := db.UpdateProject(id, req.Name, req.Slug)
	if err != nil {
		log.Printf("error updating project: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := db.DeleteProject(id); err != nil {
		log.Printf("error deleting project: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Elements

func handleGetElements(w http.ResponseWriter, r *http.Request) {
	pidStr := r.URL.Query().Get("project_id")
	if pidStr == "" {
		writeError(w, http.StatusBadRequest, "project_id required")
		return
	}
	pid, err := strconv.ParseInt(pidStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	elements, err := db.GetElementsByProject(pid)
	if err != nil {
		log.Printf("error getting elements: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if elements == nil {
		elements = []db.Element{}
	}
	writeJSON(w, http.StatusOK, elements)
}

func handleCreateElement(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID int64   `json:"project_id"`
		Type      string  `json:"type"`
		Content   string  `json:"content"`
		X         float64 `json:"x"`
		Y         float64 `json:"y"`
		Width     float64 `json:"width"`
		Height    float64 `json:"height"`
		Style     string  `json:"style"`
		ZIndex    int     `json:"z_index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.ProjectID == 0 {
		writeError(w, http.StatusBadRequest, "project_id required")
		return
	}
	if req.Type == "" {
		req.Type = "note"
	}
	if req.Width == 0 {
		req.Width = 200
	}
	if req.Height == 0 {
		req.Height = 60
	}
	if req.Style == "" {
		req.Style = "{}"
	}

	el, err := db.CreateElement(req.ProjectID, req.Type, req.Content, req.X, req.Y, req.Width, req.Height, req.Style, req.ZIndex)
	if err != nil {
		log.Printf("error creating element: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, el)
}

func handleUpdateElement(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	existing, err := db.GetElementByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "element not found")
		return
	}

	var req struct {
		Type      *string  `json:"type"`
		Content   *string  `json:"content"`
		X         *float64 `json:"x"`
		Y         *float64 `json:"y"`
		Width     *float64 `json:"width"`
		Height    *float64 `json:"height"`
		Completed *bool    `json:"completed"`
		Style     *string  `json:"style"`
		ZIndex    *int     `json:"z_index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	typ := existing.Type
	content := existing.Content
	x := existing.X
	y := existing.Y
	w2 := existing.Width
	h := existing.Height
	completed := existing.Completed
	style := existing.Style
	zIndex := existing.ZIndex

	if req.Type != nil {
		typ = *req.Type
	}
	if req.Content != nil {
		content = *req.Content
	}
	if req.X != nil {
		x = *req.X
	}
	if req.Y != nil {
		y = *req.Y
	}
	if req.Width != nil {
		w2 = *req.Width
	}
	if req.Height != nil {
		h = *req.Height
	}
	if req.Completed != nil {
		completed = *req.Completed
	}
	if req.Style != nil {
		style = *req.Style
	}
	if req.ZIndex != nil {
		zIndex = *req.ZIndex
	}

	el, err := db.UpdateElement(id, typ, content, x, y, w2, h, completed, style, zIndex)
	if err != nil {
		log.Printf("error updating element: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, el)
}

func handleDeleteElement(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := db.DeleteElement(id); err != nil {
		log.Printf("error deleting element: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Connections

func handleGetConnections(w http.ResponseWriter, r *http.Request) {
	pidStr := r.URL.Query().Get("project_id")
	if pidStr == "" {
		writeError(w, http.StatusBadRequest, "project_id required")
		return
	}
	pid, err := strconv.ParseInt(pidStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	conns, err := db.GetConnectionsByProject(pid)
	if err != nil {
		log.Printf("error getting connections: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if conns == nil {
		conns = []db.Connection{}
	}
	writeJSON(w, http.StatusOK, conns)
}

func handleCreateConnection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID int64  `json:"project_id"`
		FromID    int64  `json:"from_id"`
		ToID      int64  `json:"to_id"`
		Color     string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Color == "" {
		req.Color = "#3b82f6"
	}

	conn, err := db.CreateConnection(req.ProjectID, req.FromID, req.ToID, req.Color)
	if err != nil {
		log.Printf("error creating connection: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, conn)
}

func handleDeleteConnection(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := db.DeleteConnection(id); err != nil {
		log.Printf("error deleting connection: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Comments

func handleGetComments(w http.ResponseWriter, r *http.Request) {
	pidStr := r.URL.Query().Get("project_id")
	if pidStr == "" {
		writeError(w, http.StatusBadRequest, "project_id required")
		return
	}
	pid, err := strconv.ParseInt(pidStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	comments, err := db.GetCommentsByProject(pid)
	if err != nil {
		log.Printf("error getting comments: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if comments == nil {
		comments = []db.Comment{}
	}
	writeJSON(w, http.StatusOK, comments)
}

func handleCreateComment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID int64  `json:"project_id"`
		ElementID *int64 `json:"element_id"`
		X         float64 `json:"x"`
		Y         float64 `json:"y"`
		Content   string `json:"content"`
		Author    string `json:"author"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	c, err := db.CreateComment(req.ProjectID, req.ElementID, req.X, req.Y, req.Content, req.Author)
	if err != nil {
		log.Printf("error creating comment: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func handleUpdateComment(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req struct {
		Content  string `json:"content"`
		Resolved bool   `json:"resolved"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if err := db.UpdateComment(id, req.Content, req.Resolved); err != nil {
		log.Printf("error updating comment: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusOK)
}

func handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := db.DeleteComment(id); err != nil {
		log.Printf("error deleting comment: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
