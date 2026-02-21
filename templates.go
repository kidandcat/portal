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

var templates map[string]*template.Template

var funcMap = template.FuncMap{
	"upper": strings.ToUpper,
	"lower": strings.ToLower,
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
			"todo":        "Por hacer",
			"in_progress": "En progreso",
			"review":      "Revisión",
			"done":        "Hecho",
		}
		if l, ok := labels[s]; ok {
			return l
		}
		return s
	},
	"priorityLabel": func(s string) string {
		labels := map[string]string{
			"low":    "Baja",
			"medium": "Media",
			"high":   "Alta",
			"urgent": "Urgente",
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
	"milestoneProgress": func(total, done int) int {
		if total == 0 {
			return 0
		}
		return done * 100 / total
	},
	"progressCircle": func(pct int) template.HTML {
		r := 36.0
		c := 2 * 3.14159 * r
		offset := c - (float64(pct)/100.0)*c
		color := "#0d5c84"
		if pct == 100 {
			color = "#16a34a"
		}
		return template.HTML(fmt.Sprintf(
			`<svg width="80" height="80" viewBox="0 0 80 80">
				<circle cx="40" cy="40" r="%.0f" fill="none" stroke="#e5e7eb" stroke-width="6"/>
				<circle cx="40" cy="40" r="%.0f" fill="none" stroke="%s" stroke-width="6"
					stroke-dasharray="%.2f" stroke-dashoffset="%.2f"
					stroke-linecap="round" transform="rotate(-90 40 40)"/>
				<text x="40" y="44" text-anchor="middle" font-size="14" font-weight="600" fill="%s">%d%%</text>
			</svg>`, r, r, color, c, offset, color, pct))
	},
	"isImage": func(mime string) bool {
		return strings.HasPrefix(mime, "image/")
	},
	"split": strings.Split,
	"derefStr": func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	},
}

func mustParsePage(files ...string) *template.Template {
	return template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, files...))
}

func initTemplates() {
	shared := []string{"templates/layout.html", "templates/sidebar.html"}

	templates = map[string]*template.Template{
		"dashboard.html":        mustParsePage(append(shared, "templates/dashboard.html")...),
		"project.html":          mustParsePage(append(shared, "templates/project.html", "templates/issues_tab.html", "templates/files_tab.html", "templates/milestones_tab.html")...),
		"project_settings.html": mustParsePage(append(shared, "templates/project_settings.html")...),
		"login.html":            mustParsePage("templates/layout.html", "templates/login.html"),
		"login_sent.html":       mustParsePage("templates/layout.html", "templates/login_sent.html"),
		"approve.html":          mustParsePage("templates/layout.html", "templates/approve.html"),
		"admin.html":            mustParsePage(append(shared, "templates/admin.html", "templates/admin_users_table.html")...),

		// HTMX partials
		"issues_table":      mustParsePage("templates/issues_tab.html"),
		"admin_users_table": mustParsePage("templates/admin_users_table.html"),
	}
}

func renderTemplate(w http.ResponseWriter, name string, data any) {
	t, ok := templates[name]
	if !ok {
		http.Error(w, "Template not found: "+name, 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, name, data); err != nil {
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
