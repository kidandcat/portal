package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

type User struct {
	ID        int64
	Email     string
	CreatedAt time.Time
}

type Project struct {
	ID        int64
	Name      string
	Slug      string
	OwnerID   int64
	CreatedAt time.Time
}

type Element struct {
	ID        int64
	ProjectID int64
	Type      string
	Content   string
	X         float64
	Y         float64
	Width     float64
	Height    float64
	Style     string
	CreatedAt time.Time
}

type Comment struct {
	ID        int64
	ProjectID int64
	UserEmail string
	Content   string
	X         float64
	Y         float64
	Resolved  bool
	CreatedAt time.Time
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

func GetProjectsByOwner(ownerID int64) ([]Project, error) {
	rows, err := DB.Query(
		"SELECT id, name, slug, owner_id, created_at FROM projects WHERE owner_id = ? ORDER BY created_at DESC",
		ownerID,
	)
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

// Messages

type Message struct {
	ID        int64
	ProjectID int64
	Name      string
	Email     string
	Content   string
	CreatedAt time.Time
}

func CreateMessage(projectID int64, name, email, content string) error {
	_, err := DB.Exec(
		"INSERT INTO messages (project_id, name, email, content) VALUES (?, ?, ?, ?)",
		projectID, name, email, content,
	)
	return err
}

func GetMessagesByProject(projectID int64) ([]Message, error) {
	rows, err := DB.Query(
		"SELECT id, project_id, name, email, content, created_at FROM messages WHERE project_id = ? ORDER BY created_at DESC",
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.Name, &m.Email, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, nil
}

// Elements

func GetElementsByProject(projectID int64) ([]Element, error) {
	rows, err := DB.Query(
		"SELECT id, project_id, type, content, x, y, width, height, style, created_at FROM elements WHERE project_id = ? ORDER BY created_at ASC",
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var elements []Element
	for rows.Next() {
		var e Element
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Type, &e.Content, &e.X, &e.Y, &e.Width, &e.Height, &e.Style, &e.CreatedAt); err != nil {
			return nil, err
		}
		elements = append(elements, e)
	}
	return elements, nil
}

// Comments

func GetCommentsByProject(projectID int64) ([]Comment, error) {
	rows, err := DB.Query(
		"SELECT id, project_id, user_email, content, x, y, resolved, created_at FROM comments WHERE project_id = ? ORDER BY created_at ASC",
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.UserEmail, &c.Content, &c.X, &c.Y, &c.Resolved, &c.CreatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, nil
}

func CreateComment(projectID int64, userEmail, content string, x, y float64) (int64, error) {
	res, err := DB.Exec(
		"INSERT INTO comments (project_id, user_email, content, x, y) VALUES (?, ?, ?, ?, ?)",
		projectID, userEmail, content, x, y,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func ResolveComment(id, projectID int64) error {
	_, err := DB.Exec("UPDATE comments SET resolved = 1 WHERE id = ? AND project_id = ?", id, projectID)
	return err
}
