package cursorstorage_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/internal/cursorstorage"
	_ "modernc.org/sqlite"
)

func TestValidateSessionID_RejectsPathLikeIdentifiers(t *testing.T) {
	t.Parallel()
	cases := []string{
		"",
		"../escape",
		"foo/bar",
		"foo\\bar",
		"has space",
	}
	for _, id := range cases {
		if err := cursorstorage.ValidateSessionID(id); err == nil {
			t.Fatalf("ValidateSessionID(%q) = nil, want error", id)
		}
	}
	if err := cursorstorage.ValidateSessionID("cursor-session-abc"); err != nil {
		t.Fatalf("ValidateSessionID(safe id) = %v, want nil", err)
	}
}

func TestResolveAndLoadReadableSessionFixture(t *testing.T) {
	root, sessionID := writeReadableAgentStorageFixture(t)

	resolved, err := cursorstorage.ResolveStoreDB(cursorstorage.AgentStorageRoot(root), sessionID)
	if err != nil {
		t.Fatalf("ResolveStoreDB() = %v", err)
	}
	if resolved.RelativePath == "" {
		t.Fatal("expected non-empty relative path")
	}

	session, err := cursorstorage.LoadSessionData(resolved)
	if err != nil {
		t.Fatalf("LoadSessionData() = %v", err)
	}
	if len(session.Bubbles) == 0 {
		t.Fatal("expected readable bubbles from fixture")
	}
	if session.ParseStats.ReadableBlobCount == 0 {
		t.Fatalf("parse stats = %#v, want readable blobs", session.ParseStats)
	}
	ordered := session.OrderedBubbles()
	if len(ordered) == 0 || ordered[0].Text == "" {
		t.Fatalf("ordered bubbles = %#v, want readable text", ordered)
	}
}

func TestLoadEncryptedOrUnavailableContentFixture(t *testing.T) {
	root, sessionID := writeUnavailableAgentStorageFixture(t)

	resolved, err := cursorstorage.ResolveStoreDB(cursorstorage.AgentStorageRoot(root), sessionID)
	if err != nil {
		t.Fatalf("ResolveStoreDB() = %v", err)
	}
	session, err := cursorstorage.LoadSessionData(resolved)
	if err != nil {
		t.Fatalf("LoadSessionData() = %v", err)
	}
	if session.ParseStats.UnavailableBlobCount == 0 {
		t.Fatalf("parse stats = %#v, want unavailable blob count", session.ParseStats)
	}
	if len(session.Bubbles) != 0 {
		t.Fatalf("bubbles = %#v, want no decrypted plaintext from binary blob", session.Bubbles)
	}
}

func writeReadableAgentStorageFixture(t *testing.T) (root string, sessionID string) {
	t.Helper()
	root = t.TempDir()
	sessionID = "cursor-fixture-readable"
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
		`{"bubbleId":"bubble1","chatId":"chat1","text":"Hello from fixture","timestamp":1000,"type":1}`,
	); err != nil {
		t.Fatalf("insert bubble: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO meta (key, value) VALUES (?, ?)`,
		"0",
		`{"createdAt":1000,"agentId":"cursor-fixture-readable","name":"Fixture session"}`,
	); err != nil {
		t.Fatalf("insert meta: %v", err)
	}
	return root, sessionID
}

func writeUnavailableAgentStorageFixture(t *testing.T) (root string, sessionID string) {
	t.Helper()
	root = t.TempDir()
	sessionID = "cursor-fixture-unavailable"
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
