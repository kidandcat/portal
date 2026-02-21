package main

import (
	"net/http"
	"strconv"
	"strings"
)

func handleCreateIssue(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	u := currentUser(r)
	p, role := getProjectForUser(slug, u)
	if p == nil || role == "client" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Error(w, "Title required", http.StatusBadRequest)
		return
	}
	desc := r.FormValue("description")
	status := r.FormValue("status")
	if status == "" {
		status = "backlog"
	}
	priority := r.FormValue("priority")
	if priority == "" {
		priority = "medium"
	}
	var assigneeID *int64
	if aid := r.FormValue("assignee_id"); aid != "" {
		id, _ := strconv.ParseInt(aid, 10, 64)
		if id > 0 {
			assigneeID = &id
		}
	}
	var milestoneID *int64
	if mid := r.FormValue("milestone_id"); mid != "" {
		id, _ := strconv.ParseInt(mid, 10, 64)
		if id > 0 {
			milestoneID = &id
		}
	}
	var dueDate *string
	if dd := r.FormValue("due_date"); dd != "" {
		dueDate = &dd
	}

	var maxPos int
	db.QueryRow("SELECT COALESCE(MAX(position), 0) FROM issues WHERE project_id = ? AND status = ?", p.ID, status).Scan(&maxPos)

	db.Exec(`INSERT INTO issues (project_id, title, description, status, priority, assignee_id, due_date, milestone_id, position, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, title, desc, status, priority, assigneeID, dueDate, milestoneID, maxPos+1, u.ID)

	if isHTMX(r) {
		renderIssuesTable(w, p, role)
		return
	}
	http.Redirect(w, r, "/projects/"+slug+"?tab=issues", http.StatusSeeOther)
}

func handleUpdateIssue(w http.ResponseWriter, r *http.Request) {
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
	case "status":
		db.Exec("UPDATE issues SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND project_id = ?", value, id, p.ID)
	case "priority":
		db.Exec("UPDATE issues SET priority = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND project_id = ?", value, id, p.ID)
	case "title":
		db.Exec("UPDATE issues SET title = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND project_id = ?", value, id, p.ID)
	case "assignee_id":
		var assignee *int64
		if value != "" {
			aid, _ := strconv.ParseInt(value, 10, 64)
			if aid > 0 {
				assignee = &aid
			}
		}
		db.Exec("UPDATE issues SET assignee_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND project_id = ?", assignee, id, p.ID)
	case "due_date":
		var dd *string
		if value != "" {
			dd = &value
		}
		db.Exec("UPDATE issues SET due_date = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND project_id = ?", dd, id, p.ID)
	case "milestone_id":
		var mid *int64
		if value != "" {
			midVal, _ := strconv.ParseInt(value, 10, 64)
			if midVal > 0 {
				mid = &midVal
			}
		}
		db.Exec("UPDATE issues SET milestone_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND project_id = ?", mid, id, p.ID)
	}

	if isHTMX(r) {
		renderIssuesTable(w, p, role)
		return
	}
	http.Redirect(w, r, "/projects/"+slug+"?tab=issues", http.StatusSeeOther)
}

func handleDeleteIssue(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	u := currentUser(r)
	p, role := getProjectForUser(slug, u)
	if p == nil || role == "client" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	db.Exec("DELETE FROM issues WHERE id = ? AND project_id = ?", id, p.ID)

	if isHTMX(r) {
		renderIssuesTable(w, p, role)
		return
	}
	http.Redirect(w, r, "/projects/"+slug+"?tab=issues", http.StatusSeeOther)
}

func renderIssuesTable(w http.ResponseWriter, p *Project, role string) {
	data := map[string]any{
		"Issues":     projectIssues(p.ID),
		"Members":    projectMembers(p.ID),
		"Milestones": projectMilestones(p.ID),
		"Project":    p,
		"IsClient":   role == "client",
		"Statuses":   []string{"backlog", "todo", "in_progress", "review", "done"},
		"Priorities": []string{"low", "medium", "high", "urgent"},
	}
	renderTemplate(w, "issues_table", data)
}

func projectIssues(projectID int64) []Issue {
	rows, err := db.Query(`
		SELECT i.id, i.project_id, i.title, i.description, i.status, i.priority,
			i.assignee_id, i.due_date, i.milestone_id, i.position, i.created_by, i.created_at, i.updated_at,
			u.id, u.email, u.name,
			m.id, m.name, m.position
		FROM issues i
		LEFT JOIN users u ON u.id = i.assignee_id
		LEFT JOIN milestones m ON m.id = i.milestone_id
		WHERE i.project_id = ?
		ORDER BY COALESCE(m.position, 999999), i.position, i.created_at`, projectID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var issues []Issue
	for rows.Next() {
		var issue Issue
		var aID, aEmail, aName *string
		var assigneeID, createdBy, milestoneID *int64
		var mID *int64
		var mName *string
		var mPos *int
		rows.Scan(
			&issue.ID, &issue.ProjectID, &issue.Title, &issue.Description,
			&issue.Status, &issue.Priority, &assigneeID, &issue.DueDate,
			&milestoneID, &issue.Position, &createdBy, &issue.CreatedAt, &issue.UpdatedAt,
			&aID, &aEmail, &aName,
			&mID, &mName, &mPos,
		)
		issue.AssigneeID = assigneeID
		issue.CreatedBy = createdBy
		issue.MilestoneID = milestoneID
		if aID != nil {
			id, _ := strconv.ParseInt(*aID, 10, 64)
			issue.Assignee = &User{ID: id, Email: deref(aEmail), Name: deref(aName)}
		}
		if mID != nil {
			pos := 0
			if mPos != nil {
				pos = *mPos
			}
			issue.Milestone = &Milestone{ID: *mID, Name: deref(mName), Position: pos}
		}
		issues = append(issues, issue)
	}
	return issues
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
