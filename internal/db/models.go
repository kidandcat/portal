package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

type User struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type Project struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	OwnerID   int64  `json:"owner_id"`
	CreatedAt string `json:"created_at"`
}

type Element struct {
	ID        int64   `json:"id"`
	ProjectID int64   `json:"project_id"`
	Type      string  `json:"type"`
	Content   string  `json:"content"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Width     float64 `json:"width"`
	Height    float64 `json:"height"`
	Completed bool    `json:"completed"`
	Style     string  `json:"style"`
	ZIndex    int     `json:"z_index"`
}

type Connection struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	FromID    int64  `json:"from_id"`
	ToID      int64  `json:"to_id"`
	Color     string `json:"color"`
}

type Comment struct {
	ID        int64   `json:"id"`
	ProjectID int64   `json:"project_id"`
	ElementID *int64  `json:"element_id"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Content   string  `json:"content"`
	Author    string  `json:"author"`
	Resolved  bool    `json:"resolved"`
	CreatedAt string  `json:"created_at"`
}

// Users

func GetOrCreateUser(email string) (*User, error) {
	var u User
	err := DB.QueryRow("SELECT id, email, created_at FROM users WHERE email = ?", email).
		Scan(&u.ID, &u.Email, &u.CreatedAt)
	if err == sql.ErrNoRows {
		res, err := DB.Exec("INSERT INTO users (email) VALUES (?)", email)
		if err != nil {
			return nil, fmt.Errorf("insert user: %w", err)
		}
		u.ID, _ = res.LastInsertId()
		u.Email = email
		u.CreatedAt = time.Now()
		return &u, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}
	return &u, nil
}

func GetUserByID(id int64) (*User, error) {
	var u User
	err := DB.QueryRow("SELECT id, email, created_at FROM users WHERE id = ?", id).
		Scan(&u.ID, &u.Email, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// Magic tokens

func CreateMagicToken(email string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	expires := time.Now().Add(15 * time.Minute)

	_, err := DB.Exec(
		"INSERT INTO magic_tokens (email, token, expires_at, status) VALUES (?, ?, ?, 'pending')",
		email, token, expires,
	)
	if err != nil {
		return "", fmt.Errorf("insert token: %w", err)
	}
	return token, nil
}

func ValidateMagicToken(token string) (string, error) {
	var email string
	var used int
	var expiresAt time.Time
	err := DB.QueryRow(
		"SELECT email, used, expires_at FROM magic_tokens WHERE token = ?", token,
	).Scan(&email, &used, &expiresAt)
	if err != nil {
		return "", fmt.Errorf("token not found")
	}
	if used != 0 {
		return "", fmt.Errorf("token already used")
	}
	if time.Now().After(expiresAt) {
		return "", fmt.Errorf("token expired")
	}
	return email, nil
}

func ApproveMagicToken(token string) (string, error) {
	var email string
	var used int
	var status string
	var expiresAt time.Time
	err := DB.QueryRow(
		"SELECT email, used, status, expires_at FROM magic_tokens WHERE token = ?", token,
	).Scan(&email, &used, &status, &expiresAt)
	if err != nil {
		return "", fmt.Errorf("token not found")
	}
	if used != 0 {
		return "", fmt.Errorf("token already used")
	}
	if time.Now().After(expiresAt) {
		return "", fmt.Errorf("token expired")
	}
	_, err = DB.Exec("UPDATE magic_tokens SET status = 'approved' WHERE token = ?", token)
	if err != nil {
		return "", fmt.Errorf("approve token: %w", err)
	}
	return email, nil
}

func CheckMagicTokenStatus(token string) (status string, email string, err error) {
	var used int
	var expiresAt time.Time
	err = DB.QueryRow(
		"SELECT email, used, status, expires_at FROM magic_tokens WHERE token = ?", token,
	).Scan(&email, &used, &status, &expiresAt)
	if err != nil {
		return "", "", fmt.Errorf("token not found")
	}
	if used != 0 {
		return "used", email, nil
	}
	if time.Now().After(expiresAt) {
		return "expired", email, nil
	}
	return status, email, nil
}

func MarkMagicTokenUsed(token string) error {
	_, err := DB.Exec("UPDATE magic_tokens SET used = 1 WHERE token = ?", token)
	return err
}

// Sessions

func CreateSession(userID int64) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	expires := time.Now().Add(30 * 24 * time.Hour)

	_, err := DB.Exec(
		"INSERT INTO sessions (user_id, token, expires_at) VALUES (?, ?, ?)",
		userID, token, expires,
	)
	if err != nil {
		return "", fmt.Errorf("insert session: %w", err)
	}
	return token, nil
}

func GetUserBySession(token string) (*User, error) {
	var userID int64
	var expiresAt time.Time
	err := DB.QueryRow(
		"SELECT user_id, expires_at FROM sessions WHERE token = ?", token,
	).Scan(&userID, &expiresAt)
	if err != nil {
		return nil, fmt.Errorf("session not found")
	}
	if time.Now().After(expiresAt) {
		DB.Exec("DELETE FROM sessions WHERE token = ?", token)
		return nil, fmt.Errorf("session expired")
	}
	return GetUserByID(userID)
}

func DeleteSession(token string) {
	DB.Exec("DELETE FROM sessions WHERE token = ?", token)
}

// Projects

func GetProjects() ([]Project, error) {
	rows, err := DB.Query("SELECT id, name, slug, owner_id, created_at FROM projects ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.OwnerID, &p.CreatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, nil
}

func GetProjectBySlug(slug string) (*Project, error) {
	var p Project
	err := DB.QueryRow(
		"SELECT id, name, slug, owner_id, created_at FROM projects WHERE slug = ?", slug,
	).Scan(&p.ID, &p.Name, &p.Slug, &p.OwnerID, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func GetProjectByID(id int64) (*Project, error) {
	var p Project
	err := DB.QueryRow(
		"SELECT id, name, slug, owner_id, created_at FROM projects WHERE id = ?", id,
	).Scan(&p.ID, &p.Name, &p.Slug, &p.OwnerID, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func CreateProject(name, slug string) (*Project, error) {
	res, err := DB.Exec("INSERT INTO projects (name, slug) VALUES (?, ?)", name, slug)
	if err != nil {
		return nil, fmt.Errorf("insert project: %w", err)
	}
	id, _ := res.LastInsertId()
	return GetProjectByID(id)
}

func UpdateProject(id int64, name, slug string) (*Project, error) {
	_, err := DB.Exec("UPDATE projects SET name = ?, slug = ? WHERE id = ?", name, slug, id)
	if err != nil {
		return nil, fmt.Errorf("update project: %w", err)
	}
	return GetProjectByID(id)
}

func DeleteProject(id int64) error {
	_, err := DB.Exec("DELETE FROM projects WHERE id = ?", id)
	return err
}

// Elements

func GetElementsByProject(projectID int64) ([]Element, error) {
	rows, err := DB.Query(
		"SELECT id, project_id, type, content, x, y, width, height, completed, style, z_index FROM elements WHERE project_id = ? ORDER BY z_index ASC, id ASC",
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var elements []Element
	for rows.Next() {
		var e Element
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Type, &e.Content, &e.X, &e.Y, &e.Width, &e.Height, &e.Completed, &e.Style, &e.ZIndex); err != nil {
			return nil, err
		}
		elements = append(elements, e)
	}
	return elements, nil
}

func CreateElement(projectID int64, typ, content string, x, y, w, h float64, style string, zIndex int) (*Element, error) {
	res, err := DB.Exec(
		"INSERT INTO elements (project_id, type, content, x, y, width, height, style, z_index) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		projectID, typ, content, x, y, w, h, style, zIndex,
	)
	if err != nil {
		return nil, fmt.Errorf("insert element: %w", err)
	}
	id, _ := res.LastInsertId()
	return GetElementByID(id)
}

func GetElementByID(id int64) (*Element, error) {
	var e Element
	err := DB.QueryRow(
		"SELECT id, project_id, type, content, x, y, width, height, completed, style, z_index FROM elements WHERE id = ?", id,
	).Scan(&e.ID, &e.ProjectID, &e.Type, &e.Content, &e.X, &e.Y, &e.Width, &e.Height, &e.Completed, &e.Style, &e.ZIndex)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func UpdateElement(id int64, typ, content string, x, y, w, h float64, completed bool, style string, zIndex int) (*Element, error) {
	_, err := DB.Exec(
		"UPDATE elements SET type = ?, content = ?, x = ?, y = ?, width = ?, height = ?, completed = ?, style = ?, z_index = ? WHERE id = ?",
		typ, content, x, y, w, h, completed, style, zIndex, id,
	)
	if err != nil {
		return nil, fmt.Errorf("update element: %w", err)
	}
	return GetElementByID(id)
}

func DeleteElement(id int64) error {
	_, err := DB.Exec("DELETE FROM elements WHERE id = ?", id)
	return err
}

// Connections

func GetConnectionsByProject(projectID int64) ([]Connection, error) {
	rows, err := DB.Query(
		"SELECT id, project_id, from_id, to_id, color FROM connections WHERE project_id = ?",
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conns []Connection
	for rows.Next() {
		var c Connection
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.FromID, &c.ToID, &c.Color); err != nil {
			return nil, err
		}
		conns = append(conns, c)
	}
	return conns, nil
}

func CreateConnection(projectID, fromID, toID int64, color string) (*Connection, error) {
	res, err := DB.Exec(
		"INSERT INTO connections (project_id, from_id, to_id, color) VALUES (?, ?, ?, ?)",
		projectID, fromID, toID, color,
	)
	if err != nil {
		return nil, fmt.Errorf("insert connection: %w", err)
	}
	id, _ := res.LastInsertId()
	var c Connection
	err = DB.QueryRow("SELECT id, project_id, from_id, to_id, color FROM connections WHERE id = ?", id).
		Scan(&c.ID, &c.ProjectID, &c.FromID, &c.ToID, &c.Color)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func DeleteConnection(id int64) error {
	_, err := DB.Exec("DELETE FROM connections WHERE id = ?", id)
	return err
}

// Comments

func GetCommentsByProject(projectID int64) ([]Comment, error) {
	rows, err := DB.Query(
		"SELECT id, project_id, element_id, x, y, content, author, resolved, created_at FROM comments WHERE project_id = ? ORDER BY created_at ASC",
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.ElementID, &c.X, &c.Y, &c.Content, &c.Author, &c.Resolved, &c.CreatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, nil
}

func CreateComment(projectID int64, elementID *int64, x, y float64, content, author string) (*Comment, error) {
	res, err := DB.Exec(
		"INSERT INTO comments (project_id, element_id, x, y, content, author) VALUES (?, ?, ?, ?, ?, ?)",
		projectID, elementID, x, y, content, author,
	)
	if err != nil {
		return nil, fmt.Errorf("insert comment: %w", err)
	}
	id, _ := res.LastInsertId()
	var c Comment
	err = DB.QueryRow(
		"SELECT id, project_id, element_id, x, y, content, author, resolved, created_at FROM comments WHERE id = ?", id,
	).Scan(&c.ID, &c.ProjectID, &c.ElementID, &c.X, &c.Y, &c.Content, &c.Author, &c.Resolved, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func UpdateComment(id int64, content string, resolved bool) error {
	_, err := DB.Exec("UPDATE comments SET content = ?, resolved = ? WHERE id = ?", content, resolved, id)
	return err
}

func DeleteComment(id int64) error {
	_, err := DB.Exec("DELETE FROM comments WHERE id = ?", id)
	return err
}

// Seed default project if none exists
func SeedDefaultProject() error {
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM projects").Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		_, err := DB.Exec("INSERT INTO projects (name, slug) VALUES ('My Project', 'default')")
		if err != nil {
			return fmt.Errorf("seed project: %w", err)
		}
	}
	return nil
}
