package main

import "time"

type User struct {
	ID        int64
	Email     string
	Name      string
	Role      string
	CreatedAt time.Time
}

type MagicToken struct {
	ID         int64
	Email      string
	Token      string
	CreatedAt  time.Time
	ApprovedAt *time.Time
}

type Session struct {
	ID        int64
	UserID    int64
	Token     string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type Project struct {
	ID          int64
	Name        string
	Slug        string
	Description string
	CreatedAt   time.Time
	MemberRole  string // populated by query context
}

type ProjectMember struct {
	ID        int64
	ProjectID int64
	UserID    int64
	Role      string
	User      *User // joined
}

type Milestone struct {
	ID          int64
	ProjectID   int64
	Name        string
	Description string
	TargetDate  *string
	Position    int
	CreatedAt   time.Time
	TotalIssues int // computed
	DoneIssues  int // computed
}

type Issue struct {
	ID          int64
	ProjectID   int64
	Title       string
	Description string
	Status      string
	Priority    string
	AssigneeID  *int64
	DueDate     *string
	MilestoneID *int64
	Position    int
	CreatedBy   *int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Assignee    *User      // joined
	Milestone   *Milestone // joined
}

type Folder struct {
	ID        int64
	ProjectID int64
	ParentID  *int64
	Name      string
	CreatedAt time.Time
}

type File struct {
	ID         int64
	ProjectID  int64
	FolderID   *int64
	Name       string
	Path       string
	Size       int64
	MimeType   string
	UploadedBy *int64
	CreatedAt  time.Time
	Uploader   *User // joined
}

type Message struct {
	ID        int64
	ProjectID int64
	UserID    int64
	Content   string
	CreatedAt time.Time
	User      *User // joined
}
