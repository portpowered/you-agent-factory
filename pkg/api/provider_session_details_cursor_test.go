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

// pkgmaintcheck:ignore-cyclomatic-complexity fixture-backed HTTP test keeps cursor detail response assertions together.
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
	if len(resp.Transcript) != 1 || resp.Transcript[0].Text == nil || *resp.Transcript[0].Text != "Hello from API fixture" {
		t.Fatalf("transcript = %#v, want one readable bubble entry", resp.Transcript)
	}
	if resp.Parse.TokenUsage == nil || resp.Parse.TokenUsage.InputTokens == nil || *resp.Parse.TokenUsage.InputTokens != 100 {
		t.Fatalf("token usage = %#v, want input tokens from fixture meta", resp.Parse.TokenUsage)
	}
	if resp.Parse.TokenUsage.CacheWriteTokens == nil || *resp.Parse.TokenUsage.CacheWriteTokens != 10 {
		t.Fatalf("token usage = %#v, want cacheWriteTokens", resp.Parse.TokenUsage)
	}
	if resp.Parse.TokenUsage.TotalTokens == nil || *resp.Parse.TokenUsage.TotalTokens != 175 {
		t.Fatalf("token usage total = %#v, want 175", resp.Parse.TokenUsage.TotalTokens)
	}
}

func TestGetProviderSessionDetails_CursorUnavailableContentHasNoPlaintextTranscript(t *testing.T) {
	root, sessionID := writeUnavailableCursorAgentStorageFixture(t)
	srv := newTestServerWithCursorRoot(root)

	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=cursor&kind=session_id&id="+sessionID, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	if len(resp.Transcript) != 0 {
		t.Fatalf("transcript = %#v, want no decrypted plaintext", resp.Transcript)
	}
	if resp.Parse.UnknownEventCount != 1 || len(resp.Parse.UnknownEvents) != 1 {
		t.Fatalf("parse summary = %#v, want unavailable unknown events", resp.Parse)
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
	if _, err := db.Exec(
		`INSERT INTO meta (key, value) VALUES (?, ?)`,
		"1",
		`{"usage":{"inputTokens":100,"outputTokens":25,"cacheReadTokens":40,"cacheWriteTokens":10}}`,
	); err != nil {
		t.Fatalf("insert usage meta: %v", err)
	}
	return root, sessionID
}

func writeUnavailableCursorAgentStorageFixture(t *testing.T) (root string, sessionID string) {
	t.Helper()
	root = t.TempDir()
	sessionID = "cursor-api-unavailable"
	dbPath := filepath.Join(root, "workspace-hash", sessionID, "store.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(`CREATE TABLE blobs (key TEXT PRIMARY KEY, value TEXT)`); err != nil {
		t.Fatalf("create blobs table: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO blobs (key, value) VALUES (?, ?)`,
		"encrypted-blob",
		string([]byte{0x00, 0x01, 0x02, 0x03, 0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa}),
	); err != nil {
		t.Fatalf("insert encrypted blob: %v", err)
	}
	return root, sessionID
}
