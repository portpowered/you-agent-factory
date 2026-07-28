package cursor

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"

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
		if err := ValidateSessionID(id); err == nil {
			t.Fatalf("ValidateSessionID(%q) = nil, want error", id)
		}
	}
	if err := ValidateSessionID("cursor-session-abc"); err != nil {
		t.Fatalf("ValidateSessionID(safe id) = %v, want nil", err)
	}
	if err := ValidateSessionID("ed332681-38eb-485f-b3d3-d8b6df3a450b"); err != nil {
		t.Fatalf("ValidateSessionID(UUID session_id) = %v, want nil", err)
	}
}

func TestResolveStoreDB_ReturnsNotFoundForEmptyRoot(t *testing.T) {
	t.Parallel()

	_, err := ResolveStoreDB(newInspection(context.Background()), testFiles, testWalkDirectory, testResolveSymlinks, AgentStorageRoot(""), "missing-session")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
}

func TestResolveStoreDB_ReturnsNotFoundForMissingRootDirectory(t *testing.T) {
	t.Parallel()

	missingRoot := filepath.Join(t.TempDir(), "does-not-exist")
	_, err := ResolveStoreDB(newInspection(context.Background()), testFiles, testWalkDirectory, testResolveSymlinks, AgentStorageRoot(missingRoot), "missing-session")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
}

func TestResolveAndLoadReadableSessionFixture(t *testing.T) {
	root, sessionID := writeReadableAgentStorageFixture(t)

	resolved, err := ResolveStoreDB(newInspection(context.Background()), testFiles, testWalkDirectory, testResolveSymlinks, AgentStorageRoot(root), sessionID)
	if err != nil {
		t.Fatalf("ResolveStoreDB() = %v", err)
	}
	if resolved.RelativePath == "" {
		t.Fatal("expected non-empty relative path")
	}

	session, err := LoadSessionData(newInspection(context.Background()), testFiles, testOpenSQLDatabase, resolved)
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

func TestResolveAndLoadUUIDShapedSessionFixture(t *testing.T) {
	root, sessionID := writeUUIDShapedAgentStorageFixture(t)

	resolved, err := ResolveStoreDB(newInspection(context.Background()), testFiles, testWalkDirectory, testResolveSymlinks, AgentStorageRoot(root), sessionID)
	if err != nil {
		t.Fatalf("ResolveStoreDB() = %v", err)
	}
	wantRelativePath := "d2191e81bfe68d31807c1e354ea83571/" + sessionID + "/store.db"
	if resolved.RelativePath != wantRelativePath {
		t.Fatalf("relative path = %q, want %q", resolved.RelativePath, wantRelativePath)
	}

	session, err := LoadSessionData(newInspection(context.Background()), testFiles, testOpenSQLDatabase, resolved)
	if err != nil {
		t.Fatalf("LoadSessionData() = %v", err)
	}
	if session.ParseStats.ReadableBlobCount == 0 {
		t.Fatalf("parse stats = %#v, want readable blobs for UUID session_id", session.ParseStats)
	}
}

func TestLoadEncryptedOrUnavailableContentFixture(t *testing.T) {
	root, sessionID := writeUnavailableAgentStorageFixture(t)

	resolved, err := ResolveStoreDB(newInspection(context.Background()), testFiles, testWalkDirectory, testResolveSymlinks, AgentStorageRoot(root), sessionID)
	if err != nil {
		t.Fatalf("ResolveStoreDB() = %v", err)
	}
	session, err := LoadSessionData(newInspection(context.Background()), testFiles, testOpenSQLDatabase, resolved)
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

func TestDefaultAgentStorageRoot_LinuxPrefersExistingConfigRoot(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	configRoot := filepath.Join(home, ".config", "cursor", "chats")
	legacyRoot := filepath.Join(home, ".cursor", "chats")
	mustMkdirAll(t, legacyRoot)
	mustMkdirAll(t, configRoot)

	root, err := defaultAgentStorageRoot("linux", home, os.Stat)
	if err != nil {
		t.Fatalf("defaultAgentStorageRoot() = %v", err)
	}
	if got, want := string(root), configRoot; got != want {
		t.Fatalf("defaultAgentStorageRoot() = %q, want %q", got, want)
	}
}

func TestDefaultAgentStorageRoot_DarwinPrefersExistingCursorRoot(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cursorRoot := filepath.Join(home, ".cursor", "chats")
	configRoot := filepath.Join(home, ".config", "cursor", "chats")
	mustMkdirAll(t, configRoot)
	mustMkdirAll(t, cursorRoot)

	root, err := defaultAgentStorageRoot("darwin", home, os.Stat)
	if err != nil {
		t.Fatalf("defaultAgentStorageRoot() = %v", err)
	}
	if got, want := string(root), cursorRoot; got != want {
		t.Fatalf("defaultAgentStorageRoot() = %q, want %q", got, want)
	}
}

func TestDefaultAgentStorageRoot_WindowsPrefersExistingCursorRoot(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cursorRoot := filepath.Join(home, ".cursor", "chats")
	configRoot := filepath.Join(home, ".config", "cursor", "chats")
	mustMkdirAll(t, configRoot)
	mustMkdirAll(t, cursorRoot)

	root, err := defaultAgentStorageRoot("windows", home, os.Stat)
	if err != nil {
		t.Fatalf("defaultAgentStorageRoot() = %v", err)
	}
	if got, want := string(root), cursorRoot; got != want {
		t.Fatalf("defaultAgentStorageRoot() = %q, want %q", got, want)
	}
}

func TestDefaultAgentStorageRoot_WindowsFallsBackToConfigRoot(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	configRoot := filepath.Join(home, ".config", "cursor", "chats")
	mustMkdirAll(t, configRoot)

	root, err := defaultAgentStorageRoot("windows", home, os.Stat)
	if err != nil {
		t.Fatalf("defaultAgentStorageRoot() = %v", err)
	}
	if got, want := string(root), configRoot; got != want {
		t.Fatalf("defaultAgentStorageRoot() = %q, want %q", got, want)
	}
}

func TestDefaultAgentStorageRoot_ReturnsEmptyWhenNoSupportedDirectoryExists(t *testing.T) {
	t.Parallel()

	home := t.TempDir()

	for _, goos := range []string{"linux", "darwin", "windows"} {
		root, err := defaultAgentStorageRoot(goos, home, os.Stat)
		if err != nil {
			t.Fatalf("defaultAgentStorageRoot(%q) = %v", goos, err)
		}
		if root != "" {
			t.Fatalf("defaultAgentStorageRoot(%q) = %q, want empty root", goos, root)
		}
	}
}

func TestNormalizeAgentStorageRoot_ExplicitRootOverridesDiscovery(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	got, err := NormalizeAgentStorageRoot(func() (string, error) { return t.TempDir(), nil }, testFiles, providersessionsinternal.OperatingSystem("linux"), root)
	if err != nil {
		t.Fatalf("NormalizeAgentStorageRoot() = %v", err)
	}
	if want := AgentStorageRoot(root); got != want {
		t.Fatalf("NormalizeAgentStorageRoot() = %q, want %q", got, want)
	}
}

func writeReadableAgentStorageFixture(t *testing.T) (root string, sessionID string) {
	t.Helper()
	root = t.TempDir()
	sessionID = "cursor-fixture-readable"
	return writeReadableAgentStorageFixtureAt(t, root, "workspace-hash", sessionID)
}

func writeUUIDShapedAgentStorageFixture(t *testing.T) (root string, sessionID string) {
	t.Helper()
	root = t.TempDir()
	sessionID = "ed332681-38eb-485f-b3d3-d8b6df3a450b"
	return writeReadableAgentStorageFixtureAt(t, root, "d2191e81bfe68d31807c1e354ea83571", sessionID)
}

func writeReadableAgentStorageFixtureAt(t *testing.T, root, workspaceHash, sessionID string) (string, string) {
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
		`{"bubbleId":"bubble1","chatId":"chat1","text":"Hello from fixture","timestamp":1000,"type":1}`,
	); err != nil {
		t.Fatalf("insert bubble: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO meta (key, value) VALUES (?, ?)`,
		"0",
		`{"createdAt":1000,"agentId":"`+sessionID+`","name":"Fixture session"}`,
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

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v", path, err)
	}
}

func TestTokenUsageFromData_NestedUsageObject(t *testing.T) {
	t.Parallel()
	data := map[string]interface{}{
		"usage": map[string]interface{}{
			"inputTokens":      float64(120),
			"outputTokens":     float64(25),
			"cacheReadTokens":  float64(40),
			"cacheWriteTokens": float64(10),
		},
	}
	usage := tokenUsageFromData(data)
	if usage.InputTokens == nil || *usage.InputTokens != 120 {
		t.Fatalf("input tokens = %#v, want 120", usage.InputTokens)
	}
	if usage.CacheWriteTokens == nil || *usage.CacheWriteTokens != 10 {
		t.Fatalf("cache write tokens = %#v, want 10", usage.CacheWriteTokens)
	}
}

func TestComputeTotalTokens_SumsPresentFields(t *testing.T) {
	t.Parallel()
	input := 100
	output := 25
	cacheRead := 40
	cacheWrite := 10
	total := computeTotalTokens(SessionTokenUsage{
		InputTokens:      &input,
		OutputTokens:     &output,
		CacheReadTokens:  &cacheRead,
		CacheWriteTokens: &cacheWrite,
	})
	if total == nil || *total != 175 {
		t.Fatalf("total = %#v, want 175", total)
	}
}
