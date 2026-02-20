package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type Project struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Password string `json:"password"`
	Path     string `json:"path"`
}

type Config struct {
	Addr     string    `json:"addr"`
	Projects []Project `json:"projects"`
}

type Session struct {
	Token     string
	Slug      string
	ExpiresAt time.Time
}

var (
	config   Config
	sessions = struct {
		sync.RWMutex
		m map[string]Session
	}{m: make(map[string]Session)}
)

func main() {
	configPath := "config.json"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("Failed to read config: %v", err)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		log.Fatalf("Failed to parse config: %v", err)
	}
	if config.Addr == "" {
		config.Addr = ":8080"
	}

	http.HandleFunc("/login", handleLogin)
	http.HandleFunc("/logout", handleLogout)
	http.HandleFunc("/", handleProject)

	log.Printf("Portal listening on %s", config.Addr)
	log.Fatal(http.ListenAndServe(config.Addr, nil))
}

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func createSession(slug string) string {
	token := generateToken()
	sessions.Lock()
	sessions.m[token] = Session{
		Token:     token,
		Slug:      slug,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	sessions.Unlock()
	return token
}

func getSession(r *http.Request) *Session {
	cookie, err := r.Cookie("portal_session")
	if err != nil {
		return nil
	}
	sessions.RLock()
	s, ok := sessions.m[cookie.Value]
	sessions.RUnlock()
	if !ok || time.Now().After(s.ExpiresAt) {
		if ok {
			sessions.Lock()
			delete(sessions.m, cookie.Value)
			sessions.Unlock()
		}
		return nil
	}
	return &s
}

func findProject(slug string) *Project {
	for _, p := range config.Projects {
		if p.Slug == slug {
			return &p
		}
	}
	return nil
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		errMsg := r.URL.Query().Get("error")
		renderLogin(w, errMsg)
	case http.MethodPost:
		slug := r.FormValue("project")
		password := r.FormValue("password")

		project := findProject(slug)
		if project == nil || project.Password != password {
			http.Redirect(w, r, "/login?error=Invalid+credentials", http.StatusSeeOther)
			return
		}

		token := createSession(slug)
		http.SetCookie(w, &http.Cookie{
			Name:     "portal_session",
			Value:    token,
			Path:     "/",
			MaxAge:   7 * 24 * 60 * 60,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, fmt.Sprintf("/%s/", slug), http.StatusSeeOther)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("portal_session")
	if err == nil {
		sessions.Lock()
		delete(sessions.m, cookie.Value)
		sessions.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:   "portal_session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func handleProject(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	parts := strings.SplitN(path, "/", 2)
	slug := parts[0]

	project := findProject(slug)
	if project == nil {
		http.NotFound(w, r)
		return
	}

	session := getSession(r)
	if session == nil || session.Slug != slug {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	http.StripPrefix("/"+slug, http.FileServer(http.Dir(project.Path))).ServeHTTP(w, r)
}

func renderLogin(w http.ResponseWriter, errMsg string) {
	tmpl := template.Must(template.New("login").Parse(loginHTML))
	tmpl.Execute(w, map[string]interface{}{
		"Projects": config.Projects,
		"Error":    errMsg,
	})
}

var loginHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Portal - Menta Systems</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }

  body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: #f0f4f8;
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .login-card {
    background: #fff;
    border-radius: 12px;
    box-shadow: 0 4px 24px rgba(13, 92, 132, 0.12);
    padding: 40px;
    width: 100%;
    max-width: 400px;
    margin: 20px;
  }

  .logo {
    text-align: center;
    margin-bottom: 32px;
  }

  .logo h1 {
    color: #0d5c84;
    font-size: 24px;
    font-weight: 700;
    letter-spacing: -0.5px;
  }

  .logo p {
    color: #6b7c8a;
    font-size: 14px;
    margin-top: 4px;
  }

  .form-group {
    margin-bottom: 20px;
  }

  label {
    display: block;
    color: #334155;
    font-size: 14px;
    font-weight: 500;
    margin-bottom: 6px;
  }

  select, input[type="password"] {
    width: 100%;
    padding: 10px 14px;
    border: 1.5px solid #d1dbe5;
    border-radius: 8px;
    font-size: 15px;
    color: #1e293b;
    background: #fff;
    transition: border-color 0.2s;
    outline: none;
  }

  select:focus, input[type="password"]:focus {
    border-color: #0d5c84;
    box-shadow: 0 0 0 3px rgba(13, 92, 132, 0.1);
  }

  button {
    width: 100%;
    padding: 12px;
    background: #0d5c84;
    color: #fff;
    border: none;
    border-radius: 8px;
    font-size: 15px;
    font-weight: 600;
    cursor: pointer;
    transition: background 0.2s;
  }

  button:hover {
    background: #094a6b;
  }

  .error {
    background: #fef2f2;
    color: #991b1b;
    padding: 10px 14px;
    border-radius: 8px;
    font-size: 14px;
    margin-bottom: 20px;
    border: 1px solid #fecaca;
  }
</style>
</head>
<body>
  <div class="login-card">
    <div class="logo">
      <h1>Portal</h1>
      <p>Menta Systems</p>
    </div>

    {{if .Error}}
    <div class="error">{{.Error}}</div>
    {{end}}

    <form method="POST" action="/login">
      <div class="form-group">
        <label for="project">Project</label>
        <select name="project" id="project" required>
          <option value="" disabled selected>Select a project</option>
          {{range .Projects}}
          <option value="{{.Slug}}">{{.Name}}</option>
          {{end}}
        </select>
      </div>

      <div class="form-group">
        <label for="password">Password</label>
        <input type="password" name="password" id="password" placeholder="Enter password" required>
      </div>

      <button type="submit">Sign In</button>
    </form>
  </div>
</body>
</html>`
