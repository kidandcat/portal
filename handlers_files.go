package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func handleUploadFile(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	u := currentUser(r)
	p, role := getProjectForUser(slug, u)
	if p == nil || role == "client" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	r.ParseMultipartForm(32 << 20) // 32MB max
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "File required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	var folderID *int64
	if fid := r.FormValue("folder_id"); fid != "" {
		id, _ := strconv.ParseInt(fid, 10, 64)
		if id > 0 {
			folderID = &id
		}
	}

	// Store file on disk
	dir := filepath.Join(cfg.UploadDir, fmt.Sprintf("%d", p.ID))
	os.MkdirAll(dir, 0755)
	filename := fmt.Sprintf("%d_%s", p.ID, header.Filename)
	diskPath := filepath.Join(dir, filename)

	dst, err := os.Create(diskPath)
	if err != nil {
		http.Error(w, "Failed to save file", 500)
		return
	}
	defer dst.Close()
	size, _ := io.Copy(dst, file)

	mime := header.Header.Get("Content-Type")
	if mime == "" {
		mime = "application/octet-stream"
	}

	db.Exec(`INSERT INTO files (project_id, folder_id, name, path, size, mime_type, uploaded_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, p.ID, folderID, header.Filename, diskPath, size, mime, u.ID)

	folderQuery := ""
	if folderID != nil {
		folderQuery = fmt.Sprintf("&folder=%d", *folderID)
	}
	http.Redirect(w, r, "/projects/"+slug+"?tab=files"+folderQuery, http.StatusSeeOther)
}

func handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	u := currentUser(r)
	p, role := getProjectForUser(slug, u)
	if p == nil || role == "client" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Name required", http.StatusBadRequest)
		return
	}

	var parentID *int64
	if pid := r.FormValue("parent_id"); pid != "" {
		id, _ := strconv.ParseInt(pid, 10, 64)
		if id > 0 {
			parentID = &id
		}
	}

	db.Exec("INSERT INTO folders (project_id, parent_id, name) VALUES (?, ?, ?)", p.ID, parentID, name)

	folderQuery := ""
	if parentID != nil {
		folderQuery = fmt.Sprintf("&folder=%d", *parentID)
	}
	http.Redirect(w, r, "/projects/"+slug+"?tab=files"+folderQuery, http.StatusSeeOther)
}

func handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	u := currentUser(r)
	p, _ := getProjectForUser(slug, u)
	if p == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var f File
	err := db.QueryRow("SELECT id, name, path, mime_type FROM files WHERE id = ? AND project_id = ?", id, p.ID).
		Scan(&f.ID, &f.Name, &f.Path, &f.MimeType)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", f.Name))
	w.Header().Set("Content-Type", f.MimeType)
	http.ServeFile(w, r, f.Path)
}

func handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	u := currentUser(r)
	p, role := getProjectForUser(slug, u)
	if p == nil || role == "client" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var path string
	db.QueryRow("SELECT path FROM files WHERE id = ? AND project_id = ?", id, p.ID).Scan(&path)
	if path != "" {
		os.Remove(path)
	}
	db.Exec("DELETE FROM files WHERE id = ? AND project_id = ?", id, p.ID)
	http.Redirect(w, r, "/projects/"+slug+"?tab=files", http.StatusSeeOther)
}

func projectFiles(projectID int64, folderIDStr string) ([]Folder, []File) {
	var folders []Folder
	var files []File

	var folderID *int64
	if folderIDStr != "" {
		id, _ := strconv.ParseInt(folderIDStr, 10, 64)
		if id > 0 {
			folderID = &id
		}
	}

	// Folders
	var folderRows interface{ Close() error }
	if folderID == nil {
		rows, err := db.Query("SELECT id, project_id, parent_id, name, created_at FROM folders WHERE project_id = ? AND parent_id IS NULL ORDER BY name", projectID)
		if err == nil {
			folderRows = rows
			for rows.Next() {
				var f Folder
				rows.Scan(&f.ID, &f.ProjectID, &f.ParentID, &f.Name, &f.CreatedAt)
				folders = append(folders, f)
			}
		}
	} else {
		rows, err := db.Query("SELECT id, project_id, parent_id, name, created_at FROM folders WHERE project_id = ? AND parent_id = ? ORDER BY name", projectID, *folderID)
		if err == nil {
			folderRows = rows
			for rows.Next() {
				var f Folder
				rows.Scan(&f.ID, &f.ProjectID, &f.ParentID, &f.Name, &f.CreatedAt)
				folders = append(folders, f)
			}
		}
	}
	if folderRows != nil {
		folderRows.Close()
	}

	// Files
	if folderID == nil {
		rows, err := db.Query(`
			SELECT f.id, f.project_id, f.folder_id, f.name, f.path, f.size, f.mime_type, f.uploaded_by, f.created_at,
				u.id, u.name
			FROM files f LEFT JOIN users u ON u.id = f.uploaded_by
			WHERE f.project_id = ? AND f.folder_id IS NULL ORDER BY f.name`, projectID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var f File
				var uid *int64
				var uname *string
				rows.Scan(&f.ID, &f.ProjectID, &f.FolderID, &f.Name, &f.Path, &f.Size, &f.MimeType, &f.UploadedBy, &f.CreatedAt, &uid, &uname)
				if uid != nil {
					f.Uploader = &User{ID: *uid, Name: deref(uname)}
				}
				files = append(files, f)
			}
		}
	} else {
		rows, err := db.Query(`
			SELECT f.id, f.project_id, f.folder_id, f.name, f.path, f.size, f.mime_type, f.uploaded_by, f.created_at,
				u.id, u.name
			FROM files f LEFT JOIN users u ON u.id = f.uploaded_by
			WHERE f.project_id = ? AND f.folder_id = ? ORDER BY f.name`, projectID, *folderID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var f File
				var uid *int64
				var uname *string
				rows.Scan(&f.ID, &f.ProjectID, &f.FolderID, &f.Name, &f.Path, &f.Size, &f.MimeType, &f.UploadedBy, &f.CreatedAt, &uid, &uname)
				if uid != nil {
					f.Uploader = &User{ID: *uid, Name: deref(uname)}
				}
				files = append(files, f)
			}
		}
	}
	return folders, files
}

func folderBreadcrumbs(projectID int64, folderIDStr string) []Folder {
	if folderIDStr == "" {
		return nil
	}
	id, _ := strconv.ParseInt(folderIDStr, 10, 64)
	if id == 0 {
		return nil
	}
	var crumbs []Folder
	for {
		var f Folder
		err := db.QueryRow("SELECT id, project_id, parent_id, name FROM folders WHERE id = ? AND project_id = ?", id, projectID).
			Scan(&f.ID, &f.ProjectID, &f.ParentID, &f.Name)
		if err != nil {
			break
		}
		crumbs = append([]Folder{f}, crumbs...)
		if f.ParentID == nil {
			break
		}
		id = *f.ParentID
	}
	return crumbs
}
