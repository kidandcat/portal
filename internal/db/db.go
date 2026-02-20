package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

func Init(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "portal.db")
	var err error
	DB, err = sql.Open("sqlite", dbPath+"?_pragma=journal_mode(wal)&_pragma=foreign_keys(on)")
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	if err := migrate(); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	return nil
}

func migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT NOT NULL UNIQUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			slug TEXT NOT NULL UNIQUE,
			owner_id INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS elements (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			type TEXT NOT NULL DEFAULT 'note',
			content TEXT NOT NULL DEFAULT '',
			x REAL NOT NULL DEFAULT 0,
			y REAL NOT NULL DEFAULT 0,
			width REAL NOT NULL DEFAULT 200,
			height REAL NOT NULL DEFAULT 60,
			completed INTEGER NOT NULL DEFAULT 0,
			style TEXT NOT NULL DEFAULT '{}',
			z_index INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS connections (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			from_id INTEGER NOT NULL REFERENCES elements(id) ON DELETE CASCADE,
			to_id INTEGER NOT NULL REFERENCES elements(id) ON DELETE CASCADE,
			color TEXT NOT NULL DEFAULT '#3b82f6'
		)`,
		`CREATE TABLE IF NOT EXISTS comments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			element_id INTEGER REFERENCES elements(id) ON DELETE SET NULL,
			x REAL NOT NULL DEFAULT 0,
			y REAL NOT NULL DEFAULT 0,
			content TEXT NOT NULL,
			author TEXT NOT NULL DEFAULT '',
			resolved INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS magic_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT NOT NULL,
			token TEXT NOT NULL UNIQUE,
			expires_at DATETIME NOT NULL,
			used INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending'
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id),
			token TEXT NOT NULL UNIQUE,
			expires_at DATETIME NOT NULL
		)`,
	}

	for _, m := range migrations {
		if _, err := DB.Exec(m); err != nil {
			return fmt.Errorf("exec migration: %w", err)
		}
	}

	// Add columns that may not exist on old tables
	DB.Exec("ALTER TABLE elements ADD COLUMN completed INTEGER NOT NULL DEFAULT 0")
	DB.Exec("ALTER TABLE elements ADD COLUMN z_index INTEGER NOT NULL DEFAULT 0")

	return nil
}

func Close() {
	if DB != nil {
		DB.Close()
	}
}
