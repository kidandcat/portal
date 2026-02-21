package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
	Addr           string    `json:"addr"`
	MasterPassword string    `json:"master_password"`
	Projects       []Project `json:"projects"`
}

var (
	config    Config
	secretKey []byte

	loginAttempts   = make(map[string][]time.Time)
	loginAttemptsMu sync.Mutex
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
	http.HandleFunc("/admin/hub", handleAdminHub)
	http.HandleFunc("/admin/enter/", handleAdminEnter)
	http.HandleFunc("/admin/pull/", handleAdminPull)
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
	if slug == "_admin" {
		return slug
	}
	if findProject(slug) == nil {
		return ""
	}
	return slug
}

func handleRoute(w http.ResponseWriter, r *http.Request) {
	slug := getSessionSlug(r)

	if slug == "" {
		errMsg := r.URL.Query().Get("error")
		renderLogin(w, errMsg)
		return
	}

	if slug == "_admin" {
		renderAdmin(w)
		return
	}

	project := findProject(slug)
	if project == nil {
		clearSessionCookies(w)
		renderLogin(w, "")
		return
	}

	filePath := strings.TrimPrefix(r.URL.Path, "/")
	if filePath == "" {
		filePath = "index.html"
	}

	fullPath := project.Path + "/" + filePath
	if filepath.Ext(filePath) == ".html" {
		serveHTMLWithBar(w, fullPath, project.Name, isAdminOrigin(r))
		return
	}
	http.ServeFile(w, r, fullPath)
}

func handleAdminHub(w http.ResponseWriter, r *http.Request) {
	if !isAdminOrigin(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	slug := "_admin"
	sig := signSlug(slug)
	http.SetCookie(w, &http.Cookie{
		Name:     "portal_slug",
		Value:    slug,
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

func handleAdminEnter(w http.ResponseWriter, r *http.Request) {
	if getSessionSlug(r) != "_admin" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	slug := strings.TrimPrefix(r.URL.Path, "/admin/enter/")
	project := findProject(slug)
	if project == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	sig := signSlug(slug)
	http.SetCookie(w, &http.Cookie{
		Name:     "portal_slug",
		Value:    slug,
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
	http.SetCookie(w, &http.Cookie{
		Name:     "portal_admin",
		Value:    "1",
		Path:     "/",
		MaxAge:   7 * 24 * 60 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func isAdminOrigin(r *http.Request) bool {
	c, err := r.Cookie("portal_admin")
	return err == nil && c.Value == "1"
}

func handleAdminPull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || getSessionSlug(r) != "_admin" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	slug := strings.TrimPrefix(r.URL.Path, "/admin/pull/")
	project := findProject(slug)
	if project == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	cmd := exec.Command("git", "pull")
	cmd.Dir = project.Path
	output, err := cmd.CombinedOutput()
	result := string(output)
	if err != nil {
		result = "Error: " + err.Error() + "\n" + result
	}

	tmpl := template.Must(template.New("pull").Parse(pullResultHTML))
	tmpl.Execute(w, map[string]any{
		"Project": project,
		"Output":  result,
	})
}

func renderAdmin(w http.ResponseWriter) {
	tmpl := template.Must(template.New("admin").Parse(adminHTML))
	tmpl.Execute(w, map[string]any{
		"Projects": config.Projects,
	})
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.SplitN(fwd, ",", 2)[0]
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func checkRateLimit(ip string) bool {
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()

	now := time.Now()
	cutoff := now.Add(-time.Minute)

	// Filter to only keep attempts within the last minute
	recent := loginAttempts[ip][:0]
	for _, t := range loginAttempts[ip] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	loginAttempts[ip] = recent

	if len(recent) >= 10 {
		return false
	}

	loginAttempts[ip] = append(loginAttempts[ip], now)
	return true
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if !checkRateLimit(clientIP(r)) {
		http.Error(w, "Demasiados intentos. Espera un minuto.", http.StatusTooManyRequests)
		return
	}

	password := r.FormValue("password")

	var slug string
	if config.MasterPassword != "" && password == config.MasterPassword {
		slug = "_admin"
	} else if project := findProjectByPassword(password); project != nil {
		slug = project.Slug
	} else {
		http.Redirect(w, r, "/?error=Contrase%C3%B1a+incorrecta", http.StatusSeeOther)
		return
	}

	sig := signSlug(slug)
	http.SetCookie(w, &http.Cookie{
		Name:     "portal_slug",
		Value:    slug,
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
	http.SetCookie(w, &http.Cookie{
		Name:   "portal_admin",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

func serveHTMLWithBar(w http.ResponseWriter, filePath, projectName string, isAdmin bool) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	content := string(data)
	idx := strings.LastIndex(content, "</body>")
	if idx == -1 {
		idx = strings.LastIndex(content, "</BODY>")
	}
	if idx == -1 {
		// No </body> tag, serve as-is
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
		return
	}

	var rightButtons string
	if isAdmin {
		rightButtons = `<a href="/admin/hub" style="color:#fff;text-decoration:none;margin-right:12px;font-size:13px">Volver al Hub</a>` +
			`<a href="/logout" style="color:rgba(255,255,255,0.8);text-decoration:none;font-size:13px">Cerrar sesión</a>`
	} else {
		rightButtons = `<a href="/logout" style="color:rgba(255,255,255,0.8);text-decoration:none;font-size:13px">Cerrar sesión</a>`
	}

	label := html.EscapeString(projectName)
	if isAdmin {
		label = "Admin — " + label
	}

	bar := fmt.Sprintf(`<style>
.portal-bar{position:fixed;top:0;left:0;right:0;height:35px;background:#0d5c84;color:#fff;display:flex;align-items:center;justify-content:space-between;padding:0 16px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;font-size:14px;font-weight:500;z-index:999999;box-shadow:0 1px 3px rgba(0,0,0,0.15)}
.portal-bar a:hover{opacity:0.8}
html{padding-top:35px !important}
</style>
<div class="portal-bar"><span>%s</span><span>%s</span></div>`, label, rightButtons)

	injected := content[:idx] + bar + content[idx:]
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, injected)
}

func renderLogin(w http.ResponseWriter, errMsg string) {
	tmpl := template.Must(template.New("login").Parse(loginHTML))
	tmpl.Execute(w, map[string]interface{}{
		"Error": errMsg,
	})
}

var adminHTML = `<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Admin - Portal de Clientes</title>
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

  .admin-card {
    background: #fff;
    border-radius: 12px;
    box-shadow: 0 4px 24px rgba(13, 92, 132, 0.12);
    padding: 40px;
    width: 100%;
    max-width: 500px;
    margin: 20px;
  }

  .header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 28px;
  }

  .header h1 {
    color: #0d5c84;
    font-size: 22px;
    font-weight: 700;
  }

  .header a {
    color: #6b7c8a;
    font-size: 14px;
    text-decoration: none;
  }

  .header a:hover {
    color: #991b1b;
  }

  .project-list {
    list-style: none;
  }

  .project-list li {
    border: 1.5px solid #d1dbe5;
    border-radius: 8px;
    margin-bottom: 12px;
    transition: border-color 0.2s;
  }

  .project-list li:hover {
    border-color: #0d5c84;
  }

  .project-list a {
    display: inline-block;
    padding: 16px 20px;
    text-decoration: none;
    color: #1e293b;
    font-size: 16px;
    font-weight: 500;
    flex: 1;
  }

  .project-list .slug {
    color: #6b7c8a;
    font-size: 13px;
    font-weight: 400;
    margin-left: 8px;
  }

  .project-list li {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .btn-pull {
    background: #e2e8f0;
    border: none;
    border-radius: 6px;
    padding: 8px 14px;
    font-size: 13px;
    font-weight: 500;
    color: #334155;
    cursor: pointer;
    margin-right: 12px;
    transition: background 0.2s;
    white-space: nowrap;
  }

  .btn-pull:hover {
    background: #cbd5e1;
  }
</style>
</head>
<body>
  <div class="admin-card">
    <div class="header">
      <h1>Proyectos</h1>
      <a href="/logout">Cerrar sesión</a>
    </div>
    <ul class="project-list">
      {{range .Projects}}
      <li>
        <a href="/admin/enter/{{.Slug}}">{{.Name}} <span class="slug">/{{.Slug}}</span></a>
        <form method="POST" action="/admin/pull/{{.Slug}}" style="margin:0">
          <button type="submit" class="btn-pull">git pull</button>
        </form>
      </li>
      {{end}}
    </ul>
  </div>
</body>
</html>`

var pullResultHTML = `<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Git Pull - {{.Project.Name}}</title>
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
  .card {
    background: #fff;
    border-radius: 12px;
    box-shadow: 0 4px 24px rgba(13, 92, 132, 0.12);
    padding: 40px;
    width: 100%;
    max-width: 500px;
    margin: 20px;
  }
  h1 { color: #0d5c84; font-size: 20px; margin-bottom: 16px; }
  pre {
    background: #1e293b;
    color: #e2e8f0;
    padding: 16px;
    border-radius: 8px;
    font-size: 13px;
    overflow-x: auto;
    white-space: pre-wrap;
    word-break: break-all;
    margin-bottom: 20px;
  }
  a {
    display: inline-block;
    color: #0d5c84;
    text-decoration: none;
    font-weight: 500;
    font-size: 14px;
  }
  a:hover { text-decoration: underline; }
</style>
</head>
<body>
  <div class="card">
    <h1>git pull — {{.Project.Name}}</h1>
    <pre>{{.Output}}</pre>
    <a href="/">← Volver al panel</a>
  </div>
</body>
</html>`

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
