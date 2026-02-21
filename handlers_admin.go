package main

import (
	"net/http"
	"strconv"
	"strings"
)

func handleAdmin(w http.ResponseWriter, r *http.Request) {
	users, err := listUsers()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	renderTemplate(w, "admin.html", map[string]any{
		"User":  currentUser(r),
		"Users": users,
	})
}

func handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	name := strings.TrimSpace(r.FormValue("name"))
	role := r.FormValue("role")
	if email == "" || name == "" {
		http.Error(w, "Email and name are required", http.StatusBadRequest)
		return
	}
	if role != "user" && role != "client" && role != "admin" {
		role = "user"
	}
	_, err := db.Exec("INSERT INTO users (email, name, role) VALUES (?, ?, ?)", email, name, role)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			http.Error(w, "User already exists", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}

	if isHTMX(r) {
		users, _ := listUsers()
		renderTemplate(w, "admin_users_table.html", map[string]any{"Users": users})
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	u := currentUser(r)
	if id == u.ID {
		http.Error(w, "Cannot delete yourself", http.StatusBadRequest)
		return
	}
	db.Exec("DELETE FROM users WHERE id = ?", id)

	if isHTMX(r) {
		users, _ := listUsers()
		renderTemplate(w, "admin_users_table.html", map[string]any{"Users": users})
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func listUsers() ([]User, error) {
	rows, err := db.Query("SELECT id, email, name, role, created_at FROM users ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var u User
		rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.CreatedAt)
		users = append(users, u)
	}
	return users, nil
}
