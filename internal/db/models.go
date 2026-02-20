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

type Page struct {
	ID        int64
	ProjectID int64
	Title     string
	Content   string
	SortOrder int
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

// Pages

func GetPagesByProject(projectID int64) ([]Page, error) {
	rows, err := DB.Query(
		"SELECT id, project_id, title, content, sort_order, created_at FROM pages WHERE project_id = ? ORDER BY sort_order ASC",
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pages []Page
	for rows.Next() {
		var p Page
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.Title, &p.Content, &p.SortOrder, &p.CreatedAt); err != nil {
			return nil, err
		}
		pages = append(pages, p)
	}
	return pages, nil
}
