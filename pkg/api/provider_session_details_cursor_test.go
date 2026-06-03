package api

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	_ "modernc.org/sqlite"
)

func TestGetProviderSessionDetails_LoadsCursorSessionFromConfiguredRoot(t *testing.T) {
	root, sessionID := writeReadableCursorAgentStorageFixture(t)
	srv := newTestServerWithCursorRoot(root)

	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=cursor&kind=session_id&id="+sessionID, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	if string(resp.ProviderSession.Provider) != "cursor" || string(resp.ProviderSession.Kind) != "session_id" || resp.ProviderSession.Id != sessionID {
		t.Fatalf("provider session = %#v, want cursor session_id %s", resp.ProviderSession, sessionID)
	}
	if !strings.Contains(resp.Source.RelativePath, "store.db") || resp.Source.SizeBytes == 0 {
		t.Fatalf("source = %#v, want store.db path with metadata", resp.Source)
	}
	if resp.Parse.EventCount != 1 || resp.Parse.LineCount < 1 {
		t.Fatalf("parse summary = %#v, want readable blob counts", resp.Parse)
	}
}

func TestGetProviderSessionDetails_CursorNotFoundIsDistinguishable(t *testing.T) {
	srv := newTestServerWithCursorRoot(t.TempDir())
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=cursor&kind=session_id&id=missing-session", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "provider session not found")
}

func TestGetProviderSessionDetails_RejectsPathLikeCursorIdentifiers(t *testing.T) {
	for _, target := range []string{
		"/provider-sessions/detail?provider=cursor&kind=session_id&id=../secret",
		"/provider-sessions/detail?provider=cursor&kind=session_id&id=/tmp/store.db",
		"/provider-sessions/detail?provider=cursor&kind=session_id&id=session.with.dot",
	} {
		t.Run(target, func(t *testing.T) {
			srv := newTestServerWithCursorRoot(t.TempDir())
			req := httptest.NewRequest("GET", target, nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "provider session must be a cursor session_id identifier without path separators")
		})
	}
}

func writeReadableCursorAgentStorageFixture(t *testing.T) (root string, sessionID string) {
	t.Helper()
	root = t.TempDir()
	sessionID = "cursor-api-readable"
	dbPath := filepath.Join(root, "workspace-hash", sessionID, "store.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	schema := `
CREATE TABLE blobs (key TEXT PRIMARY KEY, value TEXT);
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO blobs (key, value) VALUES (?, ?)`,
		"bubble1",
		`{"bubbleId":"bubble1","chatId":"chat1","text":"Hello from API fixture","timestamp":1000,"type":1}`,
	); err != nil {
		t.Fatalf("insert bubble: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO meta (key, value) VALUES (?, ?)`,
		"0",
		`{"createdAt":1000,"agentId":"cursor-api-readable","name":"API fixture session"}`,
	); err != nil {
		t.Fatalf("insert meta: %v", err)
	}
	return root, sessionID
}
