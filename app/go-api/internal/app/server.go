package app

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Server struct {
	db       *sql.DB
	dataDir  string
	apiToken string
}

func NewServer(db *sql.DB, dataDir string, apiToken ...string) *Server {
	token := ""
	if len(apiToken) > 0 {
		token = strings.TrimSpace(apiToken[0])
	}
	return &Server{db: db, dataDir: dataDir, apiToken: token}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/files/markdown", s.handleCreateMarkdown)
	mux.HandleFunc("POST /api/v1/files/image", s.handleUploadImage)
	mux.HandleFunc("GET /api/v1/files", s.handleListFiles)
	mux.HandleFunc("GET /api/v1/files/{uuid}", s.handleGetFileDetail)
	mux.HandleFunc("DELETE /api/v1/files/{uuid}", s.handleSoftDeleteFile)
	mux.HandleFunc("GET /api/v1/sync/pending", s.handleGetSyncPending)
	mux.HandleFunc("POST /api/v1/sync/ack", s.handleSyncAck)
	return s.authMiddleware(mux)
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	if s.apiToken == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")) != s.apiToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleCreateMarkdown(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		http.Error(w, "content-type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	var req struct {
		Title     string `json:"title"`
		FileName  string `json:"fileName"`
		Content   string `json:"content"`
		SourceURL string `json:"sourceUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.FileName) == "" || strings.TrimSpace(req.Content) == "" {
		http.Error(w, "fileName and content are required", http.StatusBadRequest)
		return
	}
	if !isSafeFileName(req.FileName) {
		http.Error(w, "invalid fileName", http.StatusBadRequest)
		return
	}

	uuid, err := newUUIDLike()
	if err != nil {
		http.Error(w, "failed to generate uuid", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	relDir := filepath.Join("articles", now.Format("2006"), now.Format("01"))
	relPath := filepath.Join(relDir, fmt.Sprintf("%s-%s", uuid, req.FileName))
	fullPath := filepath.Join(s.dataDir, relPath)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		http.Error(w, "failed to prepare storage", http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(fullPath, []byte(req.Content), 0o644); err != nil {
		http.Error(w, "failed to write file", http.StatusInternalServerError)
		return
	}

	if err := execWithRetry(func() error {
		_, err := s.db.Exec(`
INSERT INTO files (uuid, title, file_name, relative_path, content_type, ext, size_bytes, source_url, sync_status)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending')
`, uuid, req.Title, req.FileName, relPath, "markdown", filepath.Ext(req.FileName), len(req.Content), nullIfEmpty(req.SourceURL))
		return err
	}); err != nil {
		http.Error(w, "failed to persist metadata", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"uuid":       uuid,
		"fileName":   req.FileName,
		"syncStatus": "pending",
		"relativePath": relPath,
	})
}

const maxImageUploadBytes = 10 << 20

func (s *Server) handleUploadImage(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxImageUploadBytes + (1 << 20)); err != nil {
		http.Error(w, "invalid multipart form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	if header.Size <= 0 {
		http.Error(w, "empty file", http.StatusBadRequest)
		return
	}
	if header.Size > maxImageUploadBytes {
		http.Error(w, "file too large", http.StatusBadRequest)
		return
	}
	if !isSafeFileName(header.Filename) {
		http.Error(w, "invalid fileName", http.StatusBadRequest)
		return
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == ".jpeg" {
		ext = ".jpg"
	}
	if ext != ".png" && ext != ".jpg" && ext != ".webp" {
		http.Error(w, "unsupported image format", http.StatusBadRequest)
		return
	}

	uuid, err := newUUIDLike()
	if err != nil {
		http.Error(w, "failed to generate uuid", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	relDir := filepath.Join("images", now.Format("2006"), now.Format("01"))
	relPath := filepath.Join(relDir, fmt.Sprintf("%s%s", uuid, ext))
	fullPath := filepath.Join(s.dataDir, relPath)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		http.Error(w, "failed to prepare storage", http.StatusInternalServerError)
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "failed to read upload", http.StatusInternalServerError)
		return
	}
	if len(data) == 0 {
		http.Error(w, "empty file", http.StatusBadRequest)
		return
	}
	if len(data) > maxImageUploadBytes {
		http.Error(w, "file too large", http.StatusBadRequest)
		return
	}
	if !isAllowedImageData(ext, data) {
		http.Error(w, "image content does not match extension", http.StatusBadRequest)
		return
	}
	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		http.Error(w, "failed to write file", http.StatusInternalServerError)
		return
	}

	if _, err := s.db.Exec(`
INSERT INTO files (uuid, title, file_name, relative_path, content_type, ext, size_bytes, sync_status)
VALUES (?, ?, ?, ?, ?, ?, ?, 'pending')
`, uuid, header.Filename, header.Filename, relPath, "image", strings.TrimPrefix(ext, "."), len(data)); err != nil {
		http.Error(w, "failed to persist metadata", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"uuid":         uuid,
		"relativePath": relPath,
		"url":          "/data/files/" + filepath.ToSlash(relPath),
	})
}

func isAllowedImageData(ext string, data []byte) bool {
	if len(data) < 12 {
		return false
	}
	sniffed := http.DetectContentType(data)
	switch ext {
	case ".png":
		return sniffed == "image/png"
	case ".jpg":
		return sniffed == "image/jpeg"
	case ".webp":
		return sniffed == "image/webp"
	default:
		return false
	}
}

func isSafeFileName(name string) bool {
	if strings.Contains(name, "..") {
		return false
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == string(filepath.Separator) {
		return false
	}
	if strings.HasPrefix(clean, string(filepath.Separator)) {
		return false
	}
	if strings.ContainsRune(clean, '\x00') {
		return false
	}
	return filepath.Base(clean) == clean
}

func newUUIDLike() (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	h := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32]), nil
}

func execWithRetry(fn func() error) error {
	var last error
	for i := 0; i < 10; i++ {
		err := fn()
		if err == nil {
			return nil
		}
		last = err
		if !strings.Contains(strings.ToLower(err.Error()), "database is locked") {
			return err
		}
		time.Sleep(time.Duration(i+1) * 10 * time.Millisecond)
	}
	return last
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	typeFilter := strings.TrimSpace(r.URL.Query().Get("type"))
	syncFilter := strings.TrimSpace(r.URL.Query().Get("syncStatus"))

	query := `
SELECT uuid, title, file_name, content_type, sync_status, deleted
FROM files
WHERE deleted = 0
`
	args := make([]any, 0)
	if typeFilter != "" {
		query += " AND content_type = ?"
		args = append(args, typeFilter)
	}
	if syncFilter != "" {
		query += " AND sync_status = ?"
		args = append(args, syncFilter)
	}
	query += " ORDER BY created_at DESC, id DESC LIMIT 200"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		http.Error(w, "failed to list files", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		var uuid string
		var title sql.NullString
		var fileName string
		var contentType string
		var syncStatus sql.NullString
		var deleted int
		if err := rows.Scan(&uuid, &title, &fileName, &contentType, &syncStatus, &deleted); err != nil {
			http.Error(w, "failed to scan files", http.StatusInternalServerError)
			return
		}
		items = append(items, map[string]any{
			"uuid":       uuid,
			"title":      title.String,
			"fileName":   fileName,
			"type":       contentType,
			"syncStatus": syncStatus.String,
		})
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read files", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleGetFileDetail(w http.ResponseWriter, r *http.Request) {
	uuid := strings.TrimSpace(r.PathValue("uuid"))
	if uuid == "" {
		http.Error(w, "uuid is required", http.StatusBadRequest)
		return
	}

	var row struct {
		UUID        string
		Title       sql.NullString
		FileName    string
		Relative    string
		ContentType string
		SyncStatus  sql.NullString
		Deleted     int
	}
	err := s.db.QueryRow(`
SELECT uuid, title, file_name, relative_path, content_type, sync_status, deleted
FROM files
WHERE uuid = ?
`, uuid).Scan(&row.UUID, &row.Title, &row.FileName, &row.Relative, &row.ContentType, &row.SyncStatus, &row.Deleted)
	if err == sql.ErrNoRows {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load file", http.StatusInternalServerError)
		return
	}
	if row.Deleted == 1 {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	resp := map[string]any{
		"uuid":        row.UUID,
		"title":       row.Title.String,
		"fileName":    row.FileName,
		"relativePath": row.Relative,
		"contentType": row.ContentType,
		"syncStatus":  row.SyncStatus.String,
	}

	if row.ContentType == "markdown" {
		content, readErr := os.ReadFile(filepath.Join(s.dataDir, row.Relative))
		if readErr == nil {
			resp["contentPreview"] = previewText(string(content), 300)
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSoftDeleteFile(w http.ResponseWriter, r *http.Request) {
	uuid := strings.TrimSpace(r.PathValue("uuid"))
	if uuid == "" {
		http.Error(w, "uuid is required", http.StatusBadRequest)
		return
	}

	var deleted int
	err := s.db.QueryRow(`SELECT deleted FROM files WHERE uuid = ?`, uuid).Scan(&deleted)
	if err == sql.ErrNoRows {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load file", http.StatusInternalServerError)
		return
	}

	alreadyDeleted := deleted == 1
	if !alreadyDeleted {
		if _, err := s.db.Exec(`
UPDATE files
SET deleted = 1, updated_at = CURRENT_TIMESTAMP
WHERE uuid = ?
`, uuid); err != nil {
			http.Error(w, "failed to delete file", http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"uuid":           uuid,
		"deleted":        true,
		"alreadyDeleted": alreadyDeleted,
	})
}

func (s *Server) handleGetSyncPending(w http.ResponseWriter, r *http.Request) {
	type row struct {
		UUID     string
		FileName string
		Relative string
	}

	rows, err := s.db.Query(`
SELECT uuid, file_name, relative_path
FROM files
WHERE sync_status = 'pending' AND deleted = 0 AND content_type = 'markdown'
ORDER BY created_at ASC, id ASC
`)
	if err != nil {
		http.Error(w, "failed to query pending items", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		var it row
		if err := rows.Scan(&it.UUID, &it.FileName, &it.Relative); err != nil {
			http.Error(w, "failed to scan pending items", http.StatusInternalServerError)
			return
		}

		content, readErr := os.ReadFile(filepath.Join(s.dataDir, it.Relative))
		if readErr != nil {
			continue
		}

		items = append(items, map[string]any{
			"uuid":     it.UUID,
			"fileName": it.FileName,
			"content":  string(content),
		})
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read pending items", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleSyncAck(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		http.Error(w, "content-type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	var req struct {
		UUIDs  []string `json:"uuids"`
		Failed []struct {
			UUID    string `json:"uuid"`
			Message string `json:"message"`
		} `json:"failed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if len(req.UUIDs) == 0 {
		http.Error(w, "uuids is required", http.StatusBadRequest)
		return
	}

	success := 0
	failed := 0
	for _, raw := range req.UUIDs {
		uuid := strings.TrimSpace(raw)
		if uuid == "" {
			failed++
			_ = s.logSync("", "ack", "failed", "empty uuid in uuids")
			continue
		}
		res, err := s.db.Exec(`
UPDATE files
SET sync_status = 'synced', synced_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE uuid = ?
`, uuid)
		if err != nil {
			failed++
			_ = s.logSync(uuid, "ack", "failed", "db update failed")
			continue
		}
		affected, _ := res.RowsAffected()
		if affected > 0 {
			success++
			_ = s.logSync(uuid, "ack", "success", "synced")
		} else {
			failed++
			_ = s.logSync(uuid, "ack", "failed", "uuid not found")
		}
	}

	failedMarked := 0
	for _, item := range req.Failed {
		uuid := strings.TrimSpace(item.UUID)
		if uuid == "" {
			continue
		}
		res, err := s.db.Exec(`
UPDATE files
SET sync_status = 'failed', updated_at = CURRENT_TIMESTAMP
WHERE uuid = ?
`, uuid)
		if err != nil {
			_ = s.logSync(uuid, "ack-failed", "failed", "db update failed")
			continue
		}
		affected, _ := res.RowsAffected()
		if affected > 0 {
			failedMarked++
			_ = s.logSync(uuid, "ack-failed", "failed", item.Message)
		} else {
			_ = s.logSync(uuid, "ack-failed", "failed", "uuid not found")
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":      success,
		"failed":       failed,
		"failedMarked": failedMarked,
	})
}

func (s *Server) logSync(fileUUID, action, status, message string) error {
	if len(message) > 500 {
		message = message[:500]
	}
	_, err := s.db.Exec(`
INSERT INTO sync_logs (file_uuid, action, status, message)
VALUES (?, ?, ?, ?)
`, nullIfEmpty(fileUUID), action, status, message)
	return err
}

func previewText(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
