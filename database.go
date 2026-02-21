package main

import (
	"database/sql"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

func initDB(path string) {
	var err error
	db, err = sql.Open("sqlite3", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		log.Fatal(err)
	}
	migrate()
}

func migrate() {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT UNIQUE NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT 'user' CHECK(role IN ('admin','user','client')),
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS magic_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT NOT NULL,
			token TEXT UNIQUE NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			approved_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id),
			token TEXT UNIQUE NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			slug TEXT UNIQUE NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS project_members (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			role TEXT NOT NULL DEFAULT 'member' CHECK(role IN ('owner','member','client')),
			UNIQUE(project_id, user_id)
		)`,
		`CREATE TABLE IF NOT EXISTS issues (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'backlog' CHECK(status IN ('backlog','todo','in_progress','review','done')),
			priority TEXT NOT NULL DEFAULT 'medium' CHECK(priority IN ('low','medium','high','urgent')),
			assignee_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
			due_date DATE,
			position INTEGER NOT NULL DEFAULT 0,
			created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS folders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			parent_id INTEGER REFERENCES folders(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS files (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			folder_id INTEGER REFERENCES folders(id) ON DELETE SET NULL,
			name TEXT NOT NULL,
			path TEXT NOT NULL,
			size INTEGER NOT NULL DEFAULT 0,
			mime_type TEXT NOT NULL DEFAULT 'application/octet-stream',
			uploaded_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			content TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			log.Fatalf("migration error: %v\nSQL: %s", err, s)
		}
	}
}
