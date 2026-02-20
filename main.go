package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
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

var (
	config    Config
	secretKey []byte
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

	// Derive a secret key from all project passwords for cookie signing
	h := sha256.New()
	for _, p := range config.Projects {
		h.Write([]byte(p.Password))
	}
	secretKey = h.Sum(nil)

	http.HandleFunc("/logout", handleLogout)
	http.HandleFunc("/login", handleLogin)
	http.HandleFunc("/", handleRoute)

	log.Printf("Portal listening on %s", config.Addr)
	log.Fatal(http.ListenAndServe(config.Addr, nil))
}

func signSlug(slug string) string {
	mac := hmac.New(sha256.New, secretKey)
	mac.Write([]byte(slug))
	return hex.EncodeToString(mac.Sum(nil))
}

func findProjectByPassword(password string) *Project {
	for _, p := range config.Projects {
		if p.Password == password {
			return &p
		}
	}
	return nil
}

func findProject(slug string) *Project {
	for _, p := range config.Projects {
		if p.Slug == slug {
			return &p
		}
	}
	return nil
}

func getSessionSlug(r *http.Request) string {
	slugCookie, err := r.Cookie("portal_slug")
	if err != nil {
		return ""
	}
	sigCookie, err := r.Cookie("portal_sig")
	if err != nil {
		return ""
	}
	slug := slugCookie.Value
	sig := sigCookie.Value
	if signSlug(slug) != sig {
		return ""
	}
	if findProject(slug) == nil {
		return ""
	}
	return slug
}

func handleRoute(w http.ResponseWriter, r *http.Request) {
	slug := getSessionSlug(r)

	if slug == "" {
		// No valid session — show login
		errMsg := r.URL.Query().Get("error")
		renderLogin(w, errMsg)
		return
	}

	project := findProject(slug)
	if project == nil {
		// Project no longer exists, clear session
		clearSessionCookies(w)
		renderLogin(w, "")
		return
	}

	// Serve project files from root
	filePath := strings.TrimPrefix(r.URL.Path, "/")
	if filePath == "" {
		filePath = "index.html"
	}

	http.ServeFile(w, r, project.Path+"/"+filePath)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	password := r.FormValue("password")
	project := findProjectByPassword(password)
	if project == nil {
		http.Redirect(w, r, "/?error=Contrase%C3%B1a+incorrecta", http.StatusSeeOther)
		return
	}

	sig := signSlug(project.Slug)
	http.SetCookie(w, &http.Cookie{
		Name:     "portal_slug",
		Value:    project.Slug,
		Path:     "/",
		MaxAge:   7 * 24 * 60 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "portal_sig",
		Value:    sig,
		Path:     "/",
		MaxAge:   7 * 24 * 60 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	clearSessionCookies(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func clearSessionCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   "portal_slug",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.SetCookie(w, &http.Cookie{
		Name:   "portal_sig",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

func renderLogin(w http.ResponseWriter, errMsg string) {
	tmpl := template.Must(template.New("login").Parse(loginHTML))
	tmpl.Execute(w, map[string]interface{}{
		"Error": errMsg,
	})
}

var loginHTML = `<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Portal de Clientes - Menta Systems</title>
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
    font-size: 22px;
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

  input[type="password"] {
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

  input[type="password"]:focus {
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
      <h1>Portal de Clientes</h1>
      <p>Menta Systems</p>
    </div>

    {{if .Error}}
    <div class="error">{{.Error}}</div>
    {{end}}

    <form method="POST" action="/login">
      <div class="form-group">
        <label for="password">Contraseña</label>
        <input type="password" name="password" id="password" placeholder="Introduce la contraseña" required autofocus>
      </div>

      <button type="submit">Acceder</button>
    </form>
  </div>
</body>
</html>`
