package providersessioncursor

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cursorstorage "github.com/portpowered/infinite-you/pkg/platform/cursors"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	_ "modernc.org/sqlite"
)

func TestLoadDetails_ReadsReadableSessionFromConfiguredRoot(t *testing.T) {
	root, sessionID := writeReadableCursorAgentStorageFixture(t)

	resp, err := LoadDetails(cursorstorage.AgentStorageRoot(root), sessionID)
	if err != nil {
		t.Fatalf("LoadDetails: %v", err)
	}
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
	assertReadableFixtureTokenUsage(t, resp.Parse.TokenUsage)
}

func assertReadableFixtureTokenUsage(t *testing.T, usage *factoryapi.ProviderSessionTokenUsage) {
	t.Helper()
	if usage == nil {
		t.Fatal("token usage = nil, want usage from fixture meta")
	}
	if usage.InputTokens == nil || *usage.InputTokens != 100 {
		t.Fatalf("token usage = %#v, want 100 input tokens", usage)
	}
	if usage.CacheWriteTokens == nil || *usage.CacheWriteTokens != 10 {
		t.Fatalf("token usage = %#v, want 10 cache-write tokens", usage)
	}
	if usage.TotalTokens == nil || *usage.TotalTokens != 175 {
		t.Fatalf("token usage = %#v, want 175 total tokens", usage)
	}
}

func TestLoadDetails_ReadsUUIDShapedSessionFromConfiguredRoot(t *testing.T) {
	root, sessionID := writeUUIDShapedCursorAgentStorageFixture(t)

	resp, err := LoadDetails(cursorstorage.AgentStorageRoot(root), sessionID)
	if err != nil {
		t.Fatalf("LoadDetails: %v", err)
	}
	if resp.ProviderSession.Id != sessionID || string(resp.ProviderSession.Provider) != "cursor" {
		t.Fatalf("provider session = %#v, want cursor session_id %s", resp.ProviderSession, sessionID)
	}
	wantRelativePath := "d2191e81bfe68d31807c1e354ea83571/" + sessionID + "/store.db"
	if resp.Source.RelativePath != wantRelativePath {
		t.Fatalf("source relative path = %q, want %q", resp.Source.RelativePath, wantRelativePath)
	}
	if len(resp.Transcript) != 1 || resp.Transcript[0].Text == nil || *resp.Transcript[0].Text != "Hello from API fixture" {
		t.Fatalf("transcript = %#v, want readable UUID session transcript", resp.Transcript)
	}
}

func TestLoadDetails_UnavailableContentHasNoPlaintextTranscript(t *testing.T) {
	root, sessionID := writeUnavailableCursorAgentStorageFixture(t)

	resp, err := LoadDetails(cursorstorage.AgentStorageRoot(root), sessionID)
	if err != nil {
		t.Fatalf("LoadDetails: %v", err)
	}
	if len(resp.Transcript) != 0 {
		t.Fatalf("transcript = %#v, want no decrypted plaintext", resp.Transcript)
	}
	if resp.Parse.UnknownEventCount != 1 || len(resp.Parse.UnknownEvents) != 1 {
		t.Fatalf("parse summary = %#v, want unavailable unknown events", resp.Parse)
	}
}

func TestLoadDetails_NotFoundIsDistinguishable(t *testing.T) {
	_, err := LoadDetails(cursorstorage.AgentStorageRoot(t.TempDir()), "missing-session")
	if !errors.Is(err, ErrProviderSessionNotFound) {
		t.Fatalf("err = %v, want ErrProviderSessionNotFound", err)
	}
}

func TestLoadDetails_ReturnsNotFoundForEmptyRoot(t *testing.T) {
	_, err := LoadDetails(cursorstorage.AgentStorageRoot(""), "missing-session")
	if !errors.Is(err, ErrProviderSessionNotFound) {
		t.Fatalf("err = %v, want ErrProviderSessionNotFound", err)
	}
}

func TestLoadDetails_ReturnsNotFoundForMissingRootDirectory(t *testing.T) {
	missingRoot := filepath.Join(t.TempDir(), "does-not-exist")
	_, err := LoadDetails(cursorstorage.AgentStorageRoot(missingRoot), "missing-session")
	if !errors.Is(err, ErrProviderSessionNotFound) {
		t.Fatalf("err = %v, want ErrProviderSessionNotFound", err)
	}
}

func TestLoadDetails_RejectsPathLikeIdentifiers(t *testing.T) {
	for _, id := range []string{"../secret", "/tmp/store.db", "session.with.dot"} {
		t.Run(id, func(t *testing.T) {
			_, err := LoadDetails(cursorstorage.AgentStorageRoot(t.TempDir()), id)
			if !errors.Is(err, ErrInvalidProviderSessionIdentifier) {
				t.Fatalf("err = %v, want ErrInvalidProviderSessionIdentifier", err)
			}
		})
	}
}

func writeReadableCursorAgentStorageFixture(t *testing.T) (root string, sessionID string) {
	t.Helper()
	root = t.TempDir()
	sessionID = "cursor-api-readable"
	return writeReadableCursorAgentStorageFixtureAt(t, root, "workspace-hash", sessionID)
}

func writeUUIDShapedCursorAgentStorageFixture(t *testing.T) (root string, sessionID string) {
	t.Helper()
	root = t.TempDir()
	sessionID = "ed332681-38eb-485f-b3d3-d8b6df3a450b"
	return writeReadableCursorAgentStorageFixtureAt(t, root, "d2191e81bfe68d31807c1e354ea83571", sessionID)
}

func writeReadableCursorAgentStorageFixtureAt(t *testing.T, root, workspaceHash, sessionID string) (string, string) {
	t.Helper()
	dbPath := filepath.Join(root, workspaceHash, sessionID, "store.db")
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
		`{"createdAt":1000,"agentId":"`+sessionID+`","name":"API fixture session"}`,
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
