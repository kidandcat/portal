package main

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
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
	// Only admins can create projects
	if u.Role != "admin" {
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

	isClient := role == "client"
	isOwnerOrAdmin := role == "owner" || u.Role == "admin"

	data := map[string]any{
		"User":          u,
		"Project":       p,
		"Projects":      userProjects(u),
		"Tab":           tab,
		"IsClient":      isClient,
		"IsOwnerOrAdmin": isOwnerOrAdmin,
	}

	data["Issues"] = projectIssues(p.ID)
	data["Members"] = projectMembers(p.ID)
	data["Milestones"] = projectMilestones(p.ID)
	data["Statuses"] = []string{"backlog", "todo", "in_progress", "review", "done"}
	data["Priorities"] = []string{"low", "medium", "high", "urgent"}

	folderID := r.URL.Query().Get("folder")
	folders, files := projectFiles(p.ID, folderID)
	data["Folders"] = folders
	data["Files"] = files
	data["CurrentFolder"] = folderID
	data["Breadcrumbs"] = folderBreadcrumbs(p.ID, folderID)

	// Check if current folder contains only images (for carousel)
	allImages := len(files) > 0 && len(folders) == 0
	if allImages {
		for _, f := range files {
			if !strings.HasPrefix(f.MimeType, "image/") {
				allImages = false
				break
			}
		}
	}
	data["AllImages"] = allImages

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
				memberRole = "client"
			}
			if email == "" {
				http.Error(w, "Email required", http.StatusBadRequest)
				return
			}
			// Create user if doesn't exist, always as client
			var uid int64
			err := db.QueryRow("SELECT id FROM users WHERE email = ?", email).Scan(&uid)
			if err != nil {
				// User doesn't exist, create as client
				name := strings.Split(email, "@")[0]
				res, err := db.Exec("INSERT INTO users (email, name, role) VALUES (?, ?, 'client')", email, name)
				if err != nil {
					http.Error(w, "Error al crear usuario", http.StatusInternalServerError)
					return
				}
				uid, _ = res.LastInsertId()
				// Send magic link email so they can log in
				token := generateToken()
				db.Exec("INSERT INTO magic_tokens (email, token) VALUES (?, ?)", email, token)
				link := fmt.Sprintf("%s/auth/approve?token=%s", cfg.BaseURL, token)
				go sendMagicEmail(email, link)
			}
			db.Exec("INSERT OR REPLACE INTO project_members (project_id, user_id, role) VALUES (?, ?, ?)", p.ID, uid, memberRole)
		case "remove_member":
			uid := r.FormValue("user_id")
			db.Exec("DELETE FROM project_members WHERE project_id = ? AND user_id = ?", p.ID, uid)
		}
		http.Redirect(w, r, "/projects/"+slug+"/settings", http.StatusSeeOther)
		return
	}

	members := projectMembers(p.ID)
	renderTemplate(w, "project_settings.html", map[string]any{
		"User":     u,
		"Project":  p,
		"Projects": userProjects(u),
		"Members":  members,
	})
}

// Milestone handlers
func handleCreateMilestone(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	u := currentUser(r)
	p, role := getProjectForUser(slug, u)
	if p == nil || role == "client" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Name required", http.StatusBadRequest)
		return
	}
	desc := r.FormValue("description")
	var targetDate *string
	if td := r.FormValue("target_date"); td != "" {
		targetDate = &td
	}

	var maxPos int
	db.QueryRow("SELECT COALESCE(MAX(position), 0) FROM milestones WHERE project_id = ?", p.ID).Scan(&maxPos)

	db.Exec(`INSERT INTO milestones (project_id, name, description, target_date, position)
		VALUES (?, ?, ?, ?, ?)`, p.ID, name, desc, targetDate, maxPos+1)

	http.Redirect(w, r, "/projects/"+slug+"?tab=milestones", http.StatusSeeOther)
}

func handleUpdateMilestone(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	u := currentUser(r)
	p, role := getProjectForUser(slug, u)
	if p == nil || role == "client" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	field := r.FormValue("field")
	value := r.FormValue("value")

	switch field {
	case "name":
		db.Exec("UPDATE milestones SET name = ? WHERE id = ? AND project_id = ?", value, id, p.ID)
	case "description":
		db.Exec("UPDATE milestones SET description = ? WHERE id = ? AND project_id = ?", value, id, p.ID)
	case "target_date":
		var td *string
		if value != "" {
			td = &value
		}
		db.Exec("UPDATE milestones SET target_date = ? WHERE id = ? AND project_id = ?", td, id, p.ID)
	}

	http.Redirect(w, r, "/projects/"+slug+"?tab=milestones", http.StatusSeeOther)
}

func handleDeleteMilestone(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	u := currentUser(r)
	p, role := getProjectForUser(slug, u)
	if p == nil || role == "client" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	// Unlink issues from this milestone
	db.Exec("UPDATE issues SET milestone_id = NULL WHERE milestone_id = ? AND project_id = ?", id, p.ID)
	db.Exec("DELETE FROM milestones WHERE id = ? AND project_id = ?", id, p.ID)

	http.Redirect(w, r, "/projects/"+slug+"?tab=milestones", http.StatusSeeOther)
}

func projectMilestones(projectID int64) []Milestone {
	rows, err := db.Query(`
		SELECT m.id, m.project_id, m.name, m.description, m.target_date, m.position, m.created_at,
			COALESCE((SELECT COUNT(*) FROM issues WHERE milestone_id = m.id), 0),
			COALESCE((SELECT COUNT(*) FROM issues WHERE milestone_id = m.id AND status = 'done'), 0)
		FROM milestones m
		WHERE m.project_id = ?
		ORDER BY m.position, m.created_at`, projectID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var milestones []Milestone
	for rows.Next() {
		var ms Milestone
		rows.Scan(&ms.ID, &ms.ProjectID, &ms.Name, &ms.Description, &ms.TargetDate,
			&ms.Position, &ms.CreatedAt, &ms.TotalIssues, &ms.DoneIssues)
		milestones = append(milestones, ms)
	}
	return milestones
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
