package main

import (
	"net/http"
	"regexp"
	"strings"
)

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func makeSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	projects := userProjects(u)
	renderTemplate(w, "dashboard.html", map[string]any{
		"User":     u,
		"Projects": projects,
	})
}

func handleCreateProject(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u.Role == "client" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	desc := strings.TrimSpace(r.FormValue("description"))
	if name == "" {
		http.Error(w, "Name required", http.StatusBadRequest)
		return
	}
	slug := makeSlug(name)

	res, err := db.Exec("INSERT INTO projects (name, slug, description) VALUES (?, ?, ?)", name, slug, desc)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	pid, _ := res.LastInsertId()
	db.Exec("INSERT INTO project_members (project_id, user_id, role) VALUES (?, ?, 'owner')", pid, u.ID)

	http.Redirect(w, r, "/projects/"+slug, http.StatusSeeOther)
}

func handleProject(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	u := currentUser(r)
	p, role := getProjectForUser(slug, u)
	if p == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	p.MemberRole = role
	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "issues"
	}

	data := map[string]any{
		"User":     u,
		"Project":  p,
		"Projects": userProjects(u),
		"Tab":      tab,
		"IsClient": role == "client",
	}

	switch tab {
	case "issues":
		data["Issues"] = projectIssues(p.ID)
		data["Members"] = projectMembers(p.ID)
		data["Statuses"] = []string{"backlog", "todo", "in_progress", "review", "done"}
		data["Priorities"] = []string{"low", "medium", "high", "urgent"}
	case "files":
		folderID := r.URL.Query().Get("folder")
		data["Folders"], data["Files"] = projectFiles(p.ID, folderID)
		data["CurrentFolder"] = folderID
		data["Breadcrumbs"] = folderBreadcrumbs(p.ID, folderID)
	case "chat":
		data["Messages"] = projectMessages(p.ID, 100)
	}

	renderTemplate(w, "project.html", data)
}

func handleProjectSettings(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	u := currentUser(r)
	p, role := getProjectForUser(slug, u)
	if p == nil || (role != "owner" && u.Role != "admin") {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if r.Method == "POST" {
		action := r.FormValue("action")
		switch action {
		case "add_member":
			email := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
			memberRole := r.FormValue("role")
			if memberRole != "member" && memberRole != "client" && memberRole != "owner" {
				memberRole = "member"
			}
			var uid int64
			err := db.QueryRow("SELECT id FROM users WHERE email = ?", email).Scan(&uid)
			if err != nil {
				http.Error(w, "User not found", http.StatusBadRequest)
				return
			}
			db.Exec("INSERT OR REPLACE INTO project_members (project_id, user_id, role) VALUES (?, ?, ?)", p.ID, uid, memberRole)
		case "remove_member":
			uid := r.FormValue("user_id")
			db.Exec("DELETE FROM project_members WHERE project_id = ? AND user_id = ?", p.ID, uid)
		}
		http.Redirect(w, r, "/projects/"+slug+"?tab=settings", http.StatusSeeOther)
		return
	}

	members := projectMembers(p.ID)
	allUsers, _ := listUsers()
	renderTemplate(w, "project_settings.html", map[string]any{
		"User":     u,
		"Project":  p,
		"Projects": userProjects(u),
		"Members":  members,
		"AllUsers": allUsers,
	})
}

func userProjects(u *User) []Project {
	var query string
	var args []any
	if u.Role == "admin" {
		query = `SELECT p.id, p.name, p.slug, p.description, p.created_at, COALESCE(pm.role, 'admin')
			FROM projects p
			LEFT JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = ?
			ORDER BY p.name`
		args = []any{u.ID}
	} else {
		query = `SELECT p.id, p.name, p.slug, p.description, p.created_at, pm.role
			FROM projects p
			JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = ?
			ORDER BY p.name`
		args = []any{u.ID}
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var projects []Project
	for rows.Next() {
		var p Project
		rows.Scan(&p.ID, &p.Name, &p.Slug, &p.Description, &p.CreatedAt, &p.MemberRole)
		projects = append(projects, p)
	}
	return projects
}

func getProjectForUser(slug string, u *User) (*Project, string) {
	var p Project
	err := db.QueryRow("SELECT id, name, slug, description, created_at FROM projects WHERE slug = ?", slug).
		Scan(&p.ID, &p.Name, &p.Slug, &p.Description, &p.CreatedAt)
	if err != nil {
		return nil, ""
	}
	if u.Role == "admin" {
		var role string
		err := db.QueryRow("SELECT role FROM project_members WHERE project_id = ? AND user_id = ?", p.ID, u.ID).Scan(&role)
		if err != nil {
			role = "admin"
		}
		return &p, role
	}
	var role string
	err = db.QueryRow("SELECT role FROM project_members WHERE project_id = ? AND user_id = ?", p.ID, u.ID).Scan(&role)
	if err != nil {
		return nil, ""
	}
	return &p, role
}

func projectMembers(projectID int64) []ProjectMember {
	rows, err := db.Query(`
		SELECT pm.id, pm.project_id, pm.user_id, pm.role, u.id, u.email, u.name, u.role
		FROM project_members pm
		JOIN users u ON u.id = pm.user_id
		WHERE pm.project_id = ?
		ORDER BY pm.role, u.name`, projectID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var members []ProjectMember
	for rows.Next() {
		var m ProjectMember
		var u User
		rows.Scan(&m.ID, &m.ProjectID, &m.UserID, &m.Role, &u.ID, &u.Email, &u.Name, &u.Role)
		m.User = &u
		members = append(members, m)
	}
	return members
}
