package main

type Project struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
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
