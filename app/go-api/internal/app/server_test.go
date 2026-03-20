package app

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"mdhub/go-api/internal/db"
)

func newTestServer(t *testing.T) (*Server, *sql.DB, string) {
	t.Helper()
	baseDir := t.TempDir()
	dbPath := filepath.Join(baseDir, "data", "db", "app.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db failed: %v", err)
	}
	if err := db.InitSchema(database); err != nil {
		t.Fatalf("init schema failed: %v", err)
	}
	return NewServer(database, filepath.Join(baseDir, "data", "files")), database, baseDir
}

func TestAuthMiddlewareRequiresBearerWhenTokenConfigured(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "auth.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	if err := db.InitSchema(database); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	srv := NewServer(database, t.TempDir(), "secret-token")

	unauthReq := httptest.NewRequest(http.MethodGet, "/api/v1/sync/pending", nil)
	unauthW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(unauthW, unauthReq)
	if unauthW.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthW.Code)
	}

	authReq := httptest.NewRequest(http.MethodGet, "/api/v1/sync/pending", nil)
	authReq.Header.Set("Authorization", "Bearer secret-token")
	authW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(authW, authReq)
	if authW.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", authW.Code, authW.Body.String())
	}
}

func TestCreateMarkdownSuccess(t *testing.T) {
	t.Parallel()
	srv, database, _ := newTestServer(t)
	defer database.Close()

	body := map[string]string{
		"title":    "测试文章",
		"fileName": "demo.md",
		"content":  "# hello",
	}
	buf, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/markdown", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	uuid, _ := resp["uuid"].(string)
	if uuid == "" {
		t.Fatalf("expected uuid in response")
	}

	var syncStatus, relPath string
	err := database.QueryRow(`SELECT sync_status, relative_path FROM files WHERE uuid = ?`, uuid).Scan(&syncStatus, &relPath)
	if err != nil {
		t.Fatalf("query inserted row failed: %v", err)
	}
	if syncStatus != "pending" {
		t.Fatalf("expected pending, got %s", syncStatus)
	}

	fullPath := filepath.Join(srv.dataDir, relPath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("read markdown file failed: %v", err)
	}
	if string(content) != "# hello" {
		t.Fatalf("unexpected content: %s", string(content))
	}
}

func TestCreateMarkdownMissingContent(t *testing.T) {
	t.Parallel()
	srv, database, _ := newTestServer(t)
	defer database.Close()

	body := map[string]string{
		"fileName": "demo.md",
		"content":  "",
	}
	buf, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/markdown", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateMarkdownRejectPathTraversal(t *testing.T) {
	t.Parallel()
	srv, database, _ := newTestServer(t)
	defer database.Close()

	body := map[string]string{
		"fileName": "../evil.md",
		"content":  "hack",
	}
	buf, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/markdown", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateMarkdownWithSourceURL(t *testing.T) {
	t.Parallel()
	srv, database, _ := newTestServer(t)
	defer database.Close()

	body := map[string]string{
		"fileName":  "demo.md",
		"content":   "ok",
		"sourceUrl": "https://example.com/a",
	}
	buf, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/markdown", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	uuid, _ := resp["uuid"].(string)

	var sourceURL sql.NullString
	if err := database.QueryRow(`SELECT source_url FROM files WHERE uuid = ?`, uuid).Scan(&sourceURL); err != nil {
		t.Fatalf("query source_url failed: %v", err)
	}
	if !sourceURL.Valid || sourceURL.String != "https://example.com/a" {
		t.Fatalf("expected source_url stored, got %#v", sourceURL)
	}
}

func TestCreateMarkdownRequiresJSON(t *testing.T) {
	t.Parallel()
	srv, database, _ := newTestServer(t)
	defer database.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/markdown", strings.NewReader("x=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", w.Code)
	}
}

func TestUploadImageSuccessPNG(t *testing.T) {
	t.Parallel()
	srv, database, _ := newTestServer(t)
	defer database.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "a.png")
	if err != nil {
		t.Fatalf("create form file failed: %v", err)
	}
	_, _ = part.Write(samplePNGBytes())
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/image", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	relPath, _ := resp["relativePath"].(string)
	if relPath == "" || !strings.HasPrefix(relPath, "images/") {
		t.Fatalf("unexpected relativePath: %s", relPath)
	}

	var contentType string
	err = database.QueryRow(`SELECT content_type FROM files WHERE relative_path = ?`, relPath).Scan(&contentType)
	if err != nil {
		t.Fatalf("query file row failed: %v", err)
	}
	if contentType != "image" {
		t.Fatalf("expected content_type=image, got %s", contentType)
	}
}

func TestUploadImageRejectUnsupportedFormat(t *testing.T) {
	t.Parallel()
	srv, database, _ := newTestServer(t)
	defer database.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "a.gif")
	_, _ = part.Write([]byte("gif89a"))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/image", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestUploadImageRejectMismatchedContentType(t *testing.T) {
	t.Parallel()
	srv, database, _ := newTestServer(t)
	defer database.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "a.png")
	_, _ = part.Write([]byte("not-a-real-png"))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/image", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestIsSafeFileNameRejectTraversal(t *testing.T) {
	t.Parallel()
	if isSafeFileName("../a.png") {
		t.Fatalf("expected unsafe file name to be rejected")
	}
}

func TestUploadImageRejectTooLarge(t *testing.T) {
	t.Parallel()
	srv, database, _ := newTestServer(t)
	defer database.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "big.jpg")
	_, _ = part.Write(append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, bytes.Repeat([]byte("a"), (10<<20)+1)...))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/image", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestUploadImageRejectEmptyFile(t *testing.T) {
	t.Parallel()
	srv, database, _ := newTestServer(t)
	defer database.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_, _ = writer.CreateFormFile("file", "a.jpg")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/image", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestListFilesReturnsVisibleItemsWithFilters(t *testing.T) {
	t.Parallel()
	srv, database, _ := newTestServer(t)
	defer database.Close()

	uuid1 := createMarkdownWithContentForTest(t, srv, "list-a.md", "# A")
	uuid2 := createMarkdownWithContentForTest(t, srv, "list-b.md", "# B")

	if _, err := database.Exec(`UPDATE files SET sync_status = 'synced' WHERE uuid = ?`, uuid2); err != nil {
		t.Fatalf("update sync status failed: %v", err)
	}
	if _, err := database.Exec(`UPDATE files SET deleted = 1 WHERE uuid = ?`, uuid1); err != nil {
		t.Fatalf("soft delete failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files?type=markdown&syncStatus=synced", nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Items []struct {
			UUID       string `json:"uuid"`
			Type       string `json:"type"`
			SyncStatus string `json:"syncStatus"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].UUID != uuid2 {
		t.Fatalf("unexpected list result: %+v", resp.Items)
	}
}

func TestGetFileDetailSuccess(t *testing.T) {
	t.Parallel()
	srv, database, _ := newTestServer(t)
	defer database.Close()

	uuid := createMarkdownForTest(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+uuid, nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if got, _ := resp["uuid"].(string); got != uuid {
		t.Fatalf("expected uuid=%s, got %s", uuid, got)
	}
	if got, _ := resp["contentType"].(string); got != "markdown" {
		t.Fatalf("expected markdown contentType, got %s", got)
	}
	if got, _ := resp["contentPreview"].(string); !strings.Contains(got, "hello") {
		t.Fatalf("expected contentPreview contains markdown content, got %q", got)
	}
}

func TestGetFileDetailNotFound(t *testing.T) {
	t.Parallel()
	srv, database, _ := newTestServer(t)
	defer database.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/not-exist", nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSoftDeleteFileSuccessAndIdempotent(t *testing.T) {
	t.Parallel()
	srv, database, _ := newTestServer(t)
	defer database.Close()

	uuid := createMarkdownForTest(t, srv)

	firstReq := httptest.NewRequest(http.MethodDelete, "/api/v1/files/"+uuid, nil)
	firstW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(firstW, firstReq)
	if firstW.Code != http.StatusOK {
		t.Fatalf("expected first delete 200, got %d body=%s", firstW.Code, firstW.Body.String())
	}

	var deleted int
	if err := database.QueryRow(`SELECT deleted FROM files WHERE uuid = ?`, uuid).Scan(&deleted); err != nil {
		t.Fatalf("query deleted failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected deleted=1, got %d", deleted)
	}

	secondReq := httptest.NewRequest(http.MethodDelete, "/api/v1/files/"+uuid, nil)
	secondW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(secondW, secondReq)
	if secondW.Code != http.StatusOK {
		t.Fatalf("expected second delete 200, got %d body=%s", secondW.Code, secondW.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(secondW.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if got, _ := resp["alreadyDeleted"].(bool); !got {
		t.Fatalf("expected alreadyDeleted=true on second delete")
	}
}

func TestGetSyncPendingReturnsOnlyPendingMarkdown(t *testing.T) {
	t.Parallel()
	srv, database, _ := newTestServer(t)
	defer database.Close()

	uuid1 := createMarkdownWithContentForTest(t, srv, "a.md", "# A")
	uuid2 := createMarkdownWithContentForTest(t, srv, "b.md", "# B")
	uuid3 := createMarkdownWithContentForTest(t, srv, "c.md", "# C")

	if _, err := database.Exec(`UPDATE files SET sync_status = 'synced' WHERE uuid = ?`, uuid2); err != nil {
		t.Fatalf("update synced failed: %v", err)
	}
	if _, err := database.Exec(`UPDATE files SET deleted = 1 WHERE uuid = ?`, uuid3); err != nil {
		t.Fatalf("update deleted failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sync/pending", nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Items []struct {
			UUID     string `json:"uuid"`
			FileName string `json:"fileName"`
			Content  string `json:"content"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	if resp.Items[0].UUID != uuid1 {
		t.Fatalf("expected pending uuid=%s, got %s", uuid1, resp.Items[0].UUID)
	}
	if resp.Items[0].FileName != "a.md" || resp.Items[0].Content != "# A" {
		t.Fatalf("unexpected item payload: %+v", resp.Items[0])
	}
}

func TestGetSyncPendingEmptyReturnsArray(t *testing.T) {
	t.Parallel()
	srv, database, _ := newTestServer(t)
	defer database.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sync/pending", nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	items, ok := resp["items"].([]any)
	if !ok {
		t.Fatalf("expected items array in response")
	}
	if len(items) != 0 {
		t.Fatalf("expected empty items, got %d", len(items))
	}
}

func TestSyncAckSuccessAndPartialFailure(t *testing.T) {
	t.Parallel()
	srv, database, _ := newTestServer(t)
	defer database.Close()

	uuid1 := createMarkdownWithContentForTest(t, srv, "ack-a.md", "# ack A")
	uuid2 := createMarkdownWithContentForTest(t, srv, "ack-b.md", "# ack B")

	payload := map[string]any{"uuids": []string{uuid1, uuid2, "not-found-uuid"}}
	buf, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync/ack", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if int(resp["success"].(float64)) != 2 || int(resp["failed"].(float64)) != 1 {
		t.Fatalf("unexpected ack result: %#v", resp)
	}

	for _, uuid := range []string{uuid1, uuid2} {
		var status string
		var syncedAt sql.NullString
		if err := database.QueryRow(`SELECT sync_status, synced_at FROM files WHERE uuid = ?`, uuid).Scan(&status, &syncedAt); err != nil {
			t.Fatalf("query synced row failed: %v", err)
		}
		if status != "synced" {
			t.Fatalf("expected synced status, got %s", status)
		}
		if !syncedAt.Valid {
			t.Fatalf("expected synced_at set for %s", uuid)
		}
	}

	var logs int
	if err := database.QueryRow(`SELECT COUNT(*) FROM sync_logs WHERE action = 'ack'`).Scan(&logs); err != nil {
		t.Fatalf("query sync_logs failed: %v", err)
	}
	if logs < 3 {
		t.Fatalf("expected at least 3 ack logs, got %d", logs)
	}
}

func TestSyncAckRejectEmptyUUIDs(t *testing.T) {
	t.Parallel()
	srv, database, _ := newTestServer(t)
	defer database.Close()

	buf, _ := json.Marshal(map[string]any{"uuids": []string{}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync/ack", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSyncAckIdempotentForAlreadySynced(t *testing.T) {
	t.Parallel()
	srv, database, _ := newTestServer(t)
	defer database.Close()

	uuid := createMarkdownWithContentForTest(t, srv, "ack-idempotent.md", "# ack")
	if _, err := database.Exec(`UPDATE files SET sync_status='synced', synced_at=CURRENT_TIMESTAMP WHERE uuid = ?`, uuid); err != nil {
		t.Fatalf("preset synced failed: %v", err)
	}

	buf, _ := json.Marshal(map[string]any{"uuids": []string{uuid}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync/ack", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if int(resp["success"].(float64)) != 1 || int(resp["failed"].(float64)) != 0 {
		t.Fatalf("expected idempotent success, got %#v", resp)
	}
}

func TestSyncAckMarksFailedAndLogsMessage(t *testing.T) {
	t.Parallel()
	srv, database, _ := newTestServer(t)
	defer database.Close()

	uuid := createMarkdownWithContentForTest(t, srv, "ack-failed.md", "# fail")
	longMessage := strings.Repeat("x", 700)

	payload := map[string]any{
		"uuids": []string{uuid},
		"failed": []map[string]string{{
			"uuid":    uuid,
			"message": longMessage,
		}},
	}
	buf, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync/ack", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var status string
	if err := database.QueryRow(`SELECT sync_status FROM files WHERE uuid = ?`, uuid).Scan(&status); err != nil {
		t.Fatalf("query status failed: %v", err)
	}
	if status != "failed" {
		t.Fatalf("expected failed status, got %s", status)
	}

	var message string
	if err := database.QueryRow(`SELECT message FROM sync_logs WHERE file_uuid = ? AND action = 'ack-failed' ORDER BY id DESC LIMIT 1`, uuid).Scan(&message); err != nil {
		t.Fatalf("query failed log message failed: %v", err)
	}
	if len(message) != 500 {
		t.Fatalf("expected truncated message length=500, got %d", len(message))
	}
}

func TestConcurrentMarkdownWritesBaseline1000(t *testing.T) {
	srv, database, _ := newTestServer(t)
	defer database.Close()

	const total = 1000
	const workers = 4

	jobs := make(chan int, total)
	errCh := make(chan error, total)
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				payload := map[string]string{
					"title":    fmt.Sprintf("bulk-%d", i),
					"fileName": fmt.Sprintf("bulk-%d.md", i),
					"content":  "# bulk",
				}
				buf, _ := json.Marshal(payload)
				req := httptest.NewRequest(http.MethodPost, "/api/v1/files/markdown", bytes.NewReader(buf))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				srv.Routes().ServeHTTP(w, req)
				if w.Code != http.StatusCreated {
					errCh <- fmt.Errorf("write %d failed code=%d body=%s", i, w.Code, strings.TrimSpace(w.Body.String()))
				}
			}
		}()
	}

	for i := 0; i < total; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM files`).Scan(&count); err != nil {
		t.Fatalf("count rows failed: %v", err)
	}
	if count != total {
		t.Fatalf("expected %d rows, got %d", total, count)
	}
}

func samplePNGBytes() []byte {
	return []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
		0x89, 0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
		0x42, 0x60, 0x82,
	}
}

func createMarkdownForTest(t *testing.T, srv *Server) string {
	t.Helper()
	return createMarkdownWithContentForTest(t, srv, "detail.md", "# hello detail")
}

func createMarkdownWithContentForTest(t *testing.T, srv *Server, fileName, content string) string {
	t.Helper()
	body := map[string]string{
		"title":    "详情测试",
		"fileName": fileName,
		"content":  content,
	}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/markdown", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create markdown failed: code=%d body=%s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid create response: %v", err)
	}
	uuid, _ := resp["uuid"].(string)
	if uuid == "" {
		t.Fatalf("missing uuid in create response")
	}
	return uuid
}
