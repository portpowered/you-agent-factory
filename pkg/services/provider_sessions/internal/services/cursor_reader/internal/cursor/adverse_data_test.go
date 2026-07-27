package cursor

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

func withTestLimits(t *testing.T, mutate func()) {
	t.Helper()
	t.Cleanup(func() { testLimitOverrides = struct {
		storeWalkEntries   int
		storeCandidates    int
		queriedRows        int
		blobBytes          int
		inspectionBytes    int
		protobufNesting    int
		protobufDecodeWork int
		transcriptFacts    int
		parseDiagnostics   int
	}{} })
	mutate()
}

func TestLoadDetails_SkipsMalformedAndOversizedBlobsRetainsValidRows(t *testing.T) {
	root, sessionID := writeAdverseBlobFixture(t)
	withTestLimits(t, func() { testLimitOverrides.blobBytes = 256 })

	first, err := LoadDetails(context.Background(), testFiles, testWalkDirectory, testResolveSymlinks, testOpenSQLDatabase, AgentStorageRoot(root), sessionID)
	if err != nil {
		t.Fatalf("first LoadDetails: %v", err)
	}
	second, err := LoadDetails(context.Background(), testFiles, testWalkDirectory, testResolveSymlinks, testOpenSQLDatabase, AgentStorageRoot(root), sessionID)
	if err != nil {
		t.Fatalf("second LoadDetails: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated adverse reads differ")
	}
	if len(first.Transcript) != 1 || first.Transcript[0].Text == nil || *first.Transcript[0].Text != "valid message" {
		t.Fatalf("Transcript = %#v, want one valid message", first.Transcript)
	}
	if first.Parse.MalformedLineCount < 2 && len(first.Parse.ParseErrors) < 2 {
		t.Fatalf("Parse summary = %#v, want malformed and oversized blobs summarized", first.Parse)
	}
	assertDiagnosticsRedacted(t, first.Parse.ParseErrors)
}

func TestLoadDetails_UnreadableDatabaseFailsSafelyWithoutPathLeak(t *testing.T) {
	root, sessionID := writeUnreadableDatabaseFixture(t)
	_, err := LoadDetails(context.Background(), testFiles, testWalkDirectory, testResolveSymlinks, testOpenSQLDatabase, AgentStorageRoot(root), sessionID)
	if err == nil {
		t.Fatal("LoadDetails error = nil, want unreadable database failure")
	}
	assertErrorRedacted(t, err)
}

func TestLoadDetails_CancellationDuringDiscoveryAndLoad(t *testing.T) {
	root, sessionID := writeReadableCursorAgentStorageFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := LoadDetails(ctx, testFiles, testWalkDirectory, testResolveSymlinks, testOpenSQLDatabase, AgentStorageRoot(root), sessionID)
	if !errors.Is(err, providersessions.ErrOperationCanceled) {
		t.Fatalf("canceled discovery error = %v, want ErrOperationCanceled", err)
	}

	ctx, cancel = context.WithCancel(context.Background())
	walks := 0
	_, err = LoadDetails(ctx, testFiles, func(root string, walkFn fs.WalkDirFunc) error {
		return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			walks++
			if walks > 1 {
				cancel()
			}
			return walkFn(path, entry, walkErr)
		})
	}, testResolveSymlinks, testOpenSQLDatabase, AgentStorageRoot(root), sessionID)
	if !errors.Is(err, providersessions.ErrOperationCanceled) {
		t.Fatalf("canceled walk error = %v, want ErrOperationCanceled", err)
	}
}

func TestLoadDetails_EnforcesQueriedRowLimitDeterministically(t *testing.T) {
	root, sessionID := writeManyBlobRowsFixture(t, 8)
	withTestLimits(t, func() { testLimitOverrides.queriedRows = 3 })

	first, err := LoadDetails(context.Background(), testFiles, testWalkDirectory, testResolveSymlinks, testOpenSQLDatabase, AgentStorageRoot(root), sessionID)
	if err != nil {
		t.Fatalf("first LoadDetails: %v", err)
	}
	second, err := LoadDetails(context.Background(), testFiles, testWalkDirectory, testResolveSymlinks, testOpenSQLDatabase, AgentStorageRoot(root), sessionID)
	if err != nil {
		t.Fatalf("second LoadDetails: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("row-limit reads differ")
	}
	if len(first.Transcript) == 0 {
		t.Fatalf("Transcript empty, want partial rows before limit")
	}
	assertDiagnosticsRedacted(t, first.Parse.ParseErrors)
}

func TestLoadDetails_EnforcesTranscriptFactLimitWithPartialDetail(t *testing.T) {
	root, sessionID := writeManyMessageFixture(t, 6)
	withTestLimits(t, func() { testLimitOverrides.transcriptFacts = 2 })

	detail, err := LoadDetails(context.Background(), testFiles, testWalkDirectory, testResolveSymlinks, testOpenSQLDatabase, AgentStorageRoot(root), sessionID)
	if err != nil {
		t.Fatalf("LoadDetails: %v", err)
	}
	if len(detail.Transcript) != 2 {
		t.Fatalf("Transcript = %#v, want two bounded facts", detail.Transcript)
	}
	assertDiagnosticsRedacted(t, detail.Parse.ParseErrors)
}

func TestResolveStoreDB_EnforcesWalkEntryLimitDeterministically(t *testing.T) {
	root := t.TempDir()
	sessionID := "walk-limit-session"
	for i := 0; i < 20; i++ {
		if err := os.MkdirAll(filepath.Join(root, fmt.Sprintf("workspace-%02d", i)), 0o755); err != nil {
			t.Fatalf("mkdir walk filler: %v", err)
		}
	}
	writeReadableAgentStorageFixtureAt(t, root, "target-workspace", sessionID)
	withTestLimits(t, func() { testLimitOverrides.storeWalkEntries = 3 })

	_, err := ResolveStoreDB(newInspection(context.Background()), testFiles, testWalkDirectory, testResolveSymlinks, AgentStorageRoot(root), sessionID)
	if !errors.Is(err, providersessions.ErrResourceLimitExceeded) {
		t.Fatalf("walk limit error = %v, want ErrResourceLimitExceeded", err)
	}
	assertErrorRedacted(t, err)
}

func TestResolveStoreDB_EnforcesCandidateLimitDeterministically(t *testing.T) {
	root, sessionID := writeMultipleStoreCandidateFixture(t, 4)
	withTestLimits(t, func() { testLimitOverrides.storeCandidates = 1 })

	_, err := ResolveStoreDB(newInspection(context.Background()), testFiles, testWalkDirectory, testResolveSymlinks, AgentStorageRoot(root), sessionID)
	if !errors.Is(err, providersessions.ErrResourceLimitExceeded) {
		t.Fatalf("candidate limit error = %v, want ErrResourceLimitExceeded", err)
	}
	assertErrorRedacted(t, err)
}

func TestLoadDetails_EnforcesInspectionByteLimitDeterministically(t *testing.T) {
	root, sessionID := writeCumulativeBlobBytesFixture(t, 6, 80)
	withTestLimits(t, func() { testLimitOverrides.inspectionBytes = 200 })

	first, err := LoadDetails(context.Background(), testFiles, testWalkDirectory, testResolveSymlinks, testOpenSQLDatabase, AgentStorageRoot(root), sessionID)
	if err != nil {
		t.Fatalf("first LoadDetails: %v", err)
	}
	second, err := LoadDetails(context.Background(), testFiles, testWalkDirectory, testResolveSymlinks, testOpenSQLDatabase, AgentStorageRoot(root), sessionID)
	if err != nil {
		t.Fatalf("second LoadDetails: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("inspection-byte limit reads differ")
	}
	if len(first.Transcript) == 0 {
		t.Fatalf("Transcript empty, want partial rows before byte limit")
	}
	assertDiagnosticsRedacted(t, first.Parse.ParseErrors)
}

func TestExtractProtobufFields_EnforcesNestingLimitDeterministically(t *testing.T) {
	withTestLimits(t, func() { testLimitOverrides.protobufNesting = 2 })
	ins := newInspection(context.Background())
	_, err := extractProtobufFields(ins, []byte{0x0a, 0x01, 'x'}, 3)
	if err == nil || !strings.Contains(err.Error(), "nesting limit") {
		t.Fatalf("nesting limit error = %v, want nesting limit failure", err)
	}
	_, repeatErr := extractProtobufFields(newInspection(context.Background()), []byte{0x0a, 0x01, 'x'}, 3)
	if repeatErr == nil || !strings.Contains(repeatErr.Error(), "nesting limit") {
		t.Fatalf("repeated nesting limit error = %v, want nesting limit failure", repeatErr)
	}
}

func TestLoadDetails_EnforcesProtobufDecodeWorkLimitDeterministically(t *testing.T) {
	root, sessionID := writeManyProtobufBlobFixture(t, 8)
	withTestLimits(t, func() { testLimitOverrides.protobufDecodeWork = 2 })

	first, err := LoadDetails(context.Background(), testFiles, testWalkDirectory, testResolveSymlinks, testOpenSQLDatabase, AgentStorageRoot(root), sessionID)
	if err != nil {
		t.Fatalf("first LoadDetails: %v", err)
	}
	second, err := LoadDetails(context.Background(), testFiles, testWalkDirectory, testResolveSymlinks, testOpenSQLDatabase, AgentStorageRoot(root), sessionID)
	if err != nil {
		t.Fatalf("second LoadDetails: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("protobuf decode-work limit reads differ")
	}
	assertDiagnosticsRedacted(t, first.Parse.ParseErrors)
}

func TestLoadDetails_EnforcesParseDiagnosticLimitWithoutLeakage(t *testing.T) {
	root, sessionID := writeManyMalformedBlobFixture(t, 8)
	withTestLimits(t, func() { testLimitOverrides.parseDiagnostics = 2 })

	detail, err := LoadDetails(context.Background(), testFiles, testWalkDirectory, testResolveSymlinks, testOpenSQLDatabase, AgentStorageRoot(root), sessionID)
	if err != nil {
		t.Fatalf("LoadDetails: %v", err)
	}
	if len(detail.Parse.ParseErrors) > 2 {
		t.Fatalf("ParseErrors = %#v, want bounded diagnostic count", detail.Parse.ParseErrors)
	}
	assertDiagnosticsRedacted(t, detail.Parse.ParseErrors)
}

func TestSanitizedDiagnosticsEnforceDiagnosticMessageLengthLimit(t *testing.T) {
	longClass := strings.Repeat("cursor_malformed_blob", 20)
	message := sanitizeDiagnosticMessage(longClass, 42, "extra-detail")
	if len(message) > maxDiagnosticMessage {
		t.Fatalf("message length = %d, want <= %d: %q", len(message), maxDiagnosticMessage, message)
	}
	if !strings.HasSuffix(message, "...") {
		t.Fatalf("message = %q, want truncation suffix", message)
	}
}

func TestLoadDetails_StructuralDatabaseFailureClosesHandle(t *testing.T) {
	root, sessionID := writeUnreadableDatabaseFixture(t)
	opens := 0
	open := func(driverName, dsn string) (*sql.DB, error) {
		opens++
		return sql.Open(driverName, dsn)
	}
	_, err := LoadDetails(context.Background(), testFiles, testWalkDirectory, testResolveSymlinks, open, AgentStorageRoot(root), sessionID)
	if err == nil {
		t.Fatal("LoadDetails error = nil, want failure")
	}
	if opens == 0 {
		t.Fatal("database was never opened")
	}
	assertErrorRedacted(t, err)
}

func assertDiagnosticsRedacted(t *testing.T, diagnostics []providersessions.LineError) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if len(diagnostic.Message) > maxDiagnosticMessage {
			t.Fatalf("diagnostic too long: %q", diagnostic.Message)
		}
		lower := strings.ToLower(diagnostic.Message)
		for _, forbidden := range []string{"/", "\\", "select ", "pragma ", "password", "prompt", "reasoning", "secret-session"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("diagnostic leaked sensitive content: %q", diagnostic.Message)
			}
		}
	}
}

func assertErrorRedacted(t *testing.T, err error) {
	t.Helper()
	text := strings.ToLower(err.Error())
	for _, forbidden := range []string{"select ", "pragma ", "c:\\", "/users/", "password"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("error leaked sensitive content: %v", err)
		}
	}
}

func writeAdverseBlobFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	sessionID := "adverse-blob-session"
	path := filepath.Join(root, "workspace", sessionID, "store.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`
CREATE TABLE blobs (key TEXT PRIMARY KEY, value TEXT);
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);
INSERT INTO blobs (key, value) VALUES ('valid', '{"bubbleId":"bubble-valid","text":"valid message","timestamp":1000,"type":1}');
INSERT INTO blobs (key, value) VALUES ('malformed', 'not-json-secret-prompt');
INSERT INTO blobs (key, value) VALUES ('oversized', ?);
INSERT INTO meta (key, value) VALUES ('0', '{"agentId":"adverse-blob-session","createdAt":1000}');
`, strings.Repeat("x", 128)); err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	return root, sessionID
}

func writeUnreadableDatabaseFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	sessionID := "unreadable-session"
	path := filepath.Join(root, "workspace", sessionID, "store.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("not-a-sqlite-database"), 0o600); err != nil {
		t.Fatalf("write unreadable db: %v", err)
	}
	return root, sessionID
}

func writeManyBlobRowsFixture(t *testing.T, count int) (string, string) {
	t.Helper()
	root := t.TempDir()
	sessionID := "many-rows-session"
	path := filepath.Join(root, "workspace", sessionID, "store.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE blobs (key TEXT PRIMARY KEY, value TEXT); CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);`); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("row-%02d", i)
		value := fmt.Sprintf(`{"bubbleId":"bubble-%d","text":"row %d","timestamp":%d,"type":1}`, i, i, 1000+i)
		if _, err := db.Exec(`INSERT INTO blobs (key, value) VALUES (?, ?)`, key, value); err != nil {
			t.Fatalf("insert blob: %v", err)
		}
	}
	if _, err := db.Exec(`INSERT INTO meta (key, value) VALUES ('0', '{"agentId":"many-rows-session","createdAt":1000}')`); err != nil {
		t.Fatalf("insert meta: %v", err)
	}
	return root, sessionID
}

func writeManyMessageFixture(t *testing.T, count int) (string, string) {
	t.Helper()
	root := t.TempDir()
	sessionID := "many-messages-session"
	path := filepath.Join(root, "workspace", sessionID, "store.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE blobs (key TEXT PRIMARY KEY, value TEXT); CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);`); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("msg-%02d", i)
		value := fmt.Sprintf(`{"id":"user-%d","role":"user","timestamp":%d,"content":[{"type":"input_text","text":"message %d"}]}`, i, 1000+i, i)
		if _, err := db.Exec(`INSERT INTO blobs (key, value) VALUES (?, ?)`, key, value); err != nil {
			t.Fatalf("insert blob: %v", err)
		}
	}
	if _, err := db.Exec(`INSERT INTO meta (key, value) VALUES ('0', '{"agentId":"many-messages-session","createdAt":1000}')`); err != nil {
		t.Fatalf("insert meta: %v", err)
	}
	return root, sessionID
}

func writeMultipleStoreCandidateFixture(t *testing.T, count int) (string, string) {
	t.Helper()
	root := t.TempDir()
	sessionID := "multi-candidate-session"
	for i := 0; i < count; i++ {
		path := filepath.Join(root, fmt.Sprintf("workspace-%d", i), sessionID, "store.db")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir candidate store: %v", err)
		}
		if err := os.WriteFile(path, []byte("placeholder"), 0o600); err != nil {
			t.Fatalf("write candidate store: %v", err)
		}
	}
	return root, sessionID
}

func writeCumulativeBlobBytesFixture(t *testing.T, count, payloadBytes int) (string, string) {
	t.Helper()
	root := t.TempDir()
	sessionID := "cumulative-bytes-session"
	path := filepath.Join(root, "workspace", sessionID, "store.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE blobs (key TEXT PRIMARY KEY, value TEXT); CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);`); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	padding := strings.Repeat("x", payloadBytes)
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("blob-%02d", i)
		value := fmt.Sprintf(`{"bubbleId":"bubble-%d","text":"%s","timestamp":%d,"type":1}`, i, padding, 1000+i)
		if _, err := db.Exec(`INSERT INTO blobs (key, value) VALUES (?, ?)`, key, value); err != nil {
			t.Fatalf("insert blob: %v", err)
		}
	}
	if _, err := db.Exec(`INSERT INTO meta (key, value) VALUES ('0', '{"agentId":"cumulative-bytes-session","createdAt":1000}')`); err != nil {
		t.Fatalf("insert meta: %v", err)
	}
	return root, sessionID
}

func writeManyProtobufBlobFixture(t *testing.T, count int) (string, string) {
	t.Helper()
	root := t.TempDir()
	sessionID := "many-protobuf-session"
	path := filepath.Join(root, "workspace", sessionID, "store.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE blobs (key TEXT PRIMARY KEY, value TEXT); CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);`); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	for i := 0; i < count; i++ {
		jsonPayload := fmt.Sprintf(`{"bubbleId":"bubble-%d","text":"protobuf row %d","timestamp":%d,"type":1}`, i, i, 1000+i)
		payload := nestedProtobufPayload(1, jsonPayload)
		key := fmt.Sprintf("protobuf-%02d", i)
		if _, err := db.Exec(`INSERT INTO blobs (key, value) VALUES (?, ?)`, key, string(payload)); err != nil {
			t.Fatalf("insert protobuf blob: %v", err)
		}
	}
	if _, err := db.Exec(`INSERT INTO meta (key, value) VALUES ('0', '{"agentId":"many-protobuf-session","createdAt":1000}')`); err != nil {
		t.Fatalf("insert meta: %v", err)
	}
	return root, sessionID
}

func writeManyMalformedBlobFixture(t *testing.T, count int) (string, string) {
	t.Helper()
	root := t.TempDir()
	sessionID := "many-malformed-session"
	path := filepath.Join(root, "workspace", sessionID, "store.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE blobs (key TEXT PRIMARY KEY, value TEXT); CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);`); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("malformed-%02d", i)
		value := fmt.Sprintf("not-json-secret-prompt-%d", i)
		if _, err := db.Exec(`INSERT INTO blobs (key, value) VALUES (?, ?)`, key, value); err != nil {
			t.Fatalf("insert malformed blob: %v", err)
		}
	}
	if _, err := db.Exec(`INSERT INTO meta (key, value) VALUES ('0', '{"agentId":"many-malformed-session","createdAt":1000}')`); err != nil {
		t.Fatalf("insert meta: %v", err)
	}
	return root, sessionID
}

func nestedProtobufPayload(nestingDepth int, jsonPayload string) []byte {
	encoded := []byte(jsonPayload)
	for i := 0; i < nestingDepth; i++ {
		wrapped := []byte{0x0a}
		wrapped = binary.AppendUvarint(wrapped, uint64(len(encoded)))
		wrapped = append(wrapped, encoded...)
		encoded = wrapped
	}
	return encoded
}
