package main

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"strings"
)

//go:embed templates/*
var templateFS embed.FS

var tmpl *template.Template

func initTemplates() {
	funcMap := template.FuncMap{
		"upper":      strings.ToUpper,
		"lower":      strings.ToLower,
		"title": func(s string) string {
			if len(s) == 0 {
				return s
			}
			return strings.ToUpper(s[:1]) + s[1:]
		},
		"contains":   strings.Contains,
		"hasPrefix":  strings.HasPrefix,
		"hasSuffix":  strings.HasSuffix,
		"formatSize": formatSize,
		"statusLabel": func(s string) string {
			labels := map[string]string{
				"backlog":     "Backlog",
				"todo":        "To Do",
				"in_progress": "In Progress",
				"review":      "Review",
				"done":        "Done",
			}
			if l, ok := labels[s]; ok {
				return l
			}
			return s
		},
		"priorityLabel": func(s string) string {
			labels := map[string]string{
				"low":    "Low",
				"medium": "Medium",
				"high":   "High",
				"urgent": "Urgent",
			}
			if l, ok := labels[s]; ok {
				return l
			}
			return s
		},
		"eq": func(a, b any) bool {
			return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
		},
		"dict": func(values ...any) map[string]any {
			d := make(map[string]any)
			for i := 0; i < len(values)-1; i += 2 {
				d[fmt.Sprintf("%v", values[i])] = values[i+1]
			}
			return d
		},
		"derefInt64": func(p *int64) int64 {
			if p == nil {
				return 0
			}
			return *p
		},
		"sub": func(a, b int) int {
			return a - b
		},
	}
	tmpl = template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html"))
}

func renderTemplate(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
