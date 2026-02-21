package main

import (
	"net/http"
	"strconv"
	"strings"
)

func handleSendMessage(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	u := currentUser(r)
	p, _ := getProjectForUser(slug, u)
	if p == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	content := strings.TrimSpace(r.FormValue("content"))
	if content == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	db.Exec("INSERT INTO messages (project_id, user_id, content) VALUES (?, ?, ?)", p.ID, u.ID, content)

	if isHTMX(r) {
		msgs := projectMessages(p.ID, 100)
		renderTemplate(w, "chat_messages.html", map[string]any{
			"Messages": msgs,
			"User":     u,
		})
		return
	}
	http.Redirect(w, r, "/projects/"+slug+"?tab=chat", http.StatusSeeOther)
}

func handleChatPoll(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	u := currentUser(r)
	p, _ := getProjectForUser(slug, u)
	if p == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	afterID, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	msgs := projectMessagesAfter(p.ID, afterID)
	if len(msgs) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	renderTemplate(w, "chat_messages.html", map[string]any{
		"Messages": msgs,
		"User":     u,
	})
}

func projectMessages(projectID int64, limit int) []Message {
	rows, err := db.Query(`
		SELECT m.id, m.project_id, m.user_id, m.content, m.created_at, u.id, u.email, u.name
		FROM messages m JOIN users u ON u.id = m.user_id
		WHERE m.project_id = ?
		ORDER BY m.created_at DESC LIMIT ?`, projectID, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var msgs []Message
	for rows.Next() {
		var m Message
		var u User
		rows.Scan(&m.ID, &m.ProjectID, &m.UserID, &m.Content, &m.CreatedAt, &u.ID, &u.Email, &u.Name)
		m.User = &u
		msgs = append(msgs, m)
	}
	// Reverse to chronological order
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs
}

func projectMessagesAfter(projectID int64, afterID int64) []Message {
	rows, err := db.Query(`
		SELECT m.id, m.project_id, m.user_id, m.content, m.created_at, u.id, u.email, u.name
		FROM messages m JOIN users u ON u.id = m.user_id
		WHERE m.project_id = ? AND m.id > ?
		ORDER BY m.created_at ASC`, projectID, afterID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var msgs []Message
	for rows.Next() {
		var m Message
		var u User
		rows.Scan(&m.ID, &m.ProjectID, &m.UserID, &m.Content, &m.CreatedAt, &u.ID, &u.Email, &u.Name)
		m.User = &u
		msgs = append(msgs, m)
	}
	return msgs
}
