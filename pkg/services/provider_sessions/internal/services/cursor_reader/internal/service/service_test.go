package service

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestReadDiscoversOnlyCanonicalContainedSession(t *testing.T) {
	root, sessionID := writeSessionFixture(t)
	reader, err := New(
		platformfilesystem.Local{},
		filepath.WalkDir,
		filepath.EvalSymlinks,
		sql.Open,
		root,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	detail, err := reader.Read(context.Background(),providers.SessionRef{
		Provider: providers.IDCursor,
		Kind:     providers.SessionIDKind,
		ID:       sessionID,
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if detail.ProviderSession != (providersessions.Ref{
		Provider: providersessions.ProviderCursor,
		Kind:     providersessions.SessionIDKind,
		ID:       sessionID,
	}) {
		t.Fatalf("ProviderSession = %#v", detail.ProviderSession)
	}
	if got, want := detail.Source.RelativePath, filepath.ToSlash(filepath.Join("workspace", sessionID, "store.db")); got != want {
		t.Fatalf("RelativePath = %q, want %q", got, want)
	}
	if filepath.IsAbs(detail.Source.RelativePath) {
		t.Fatalf("RelativePath exposed absolute storage path: %q", detail.Source.RelativePath)
	}
}

func TestReadReconstructsDeterministicDetachedNormalizedDetail(t *testing.T) {
	root, sessionID := writeNormalizedSessionFixture(t)
	reader, err := New(
		platformfilesystem.Local{},
		filepath.WalkDir,
		filepath.EvalSymlinks,
		sql.Open,
		root,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ref := providers.SessionRef{
		Provider: providers.IDCursor,
		Kind:     providers.SessionIDKind,
		ID:       sessionID,
	}

	first, err := reader.Read(context.Background(),ref)
	if err != nil {
		t.Fatalf("first Read: %v", err)
	}
	second, err := reader.Read(context.Background(),ref)
	if err != nil {
		t.Fatalf("second Read: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated reads differ:\nfirst:  %#v\nsecond: %#v", first, second)
	}
	assertNormalizedDetail(t, first)

	*first.Transcript[0].Text = "mutated"
	*first.Parse.FunctionCalls[0].Output = "mutated"
	*first.Parse.TokenUsage.InputTokens = 999
	first.Transcript = append(first.Transcript, providersessions.TranscriptEntry{})

	third, err := reader.Read(context.Background(),ref)
	if err != nil {
		t.Fatalf("third Read: %v", err)
	}
	if !reflect.DeepEqual(second, third) {
		t.Fatalf("mutating one result affected a later read:\nwant: %#v\ngot:  %#v", second, third)
	}
}

func assertNormalizedDetail(t *testing.T, detail providersessions.Detail) {
	t.Helper()
	assertNormalizedTranscript(t, detail.Transcript)
	assertNormalizedFunctionCalls(t, detail.Parse.FunctionCalls)
	assertNormalizedReasoning(t, detail.Parse.Reasoning)
	assertNormalizedTurns(t, detail.Parse.Turns)
	assertNormalizedUsage(t, detail.Parse.TokenUsage)
}

func assertNormalizedTranscript(t *testing.T, transcript []providersessions.TranscriptEntry) {
	t.Helper()
	wantTypes := []providersessions.TranscriptEntryType{
		providersessions.TranscriptUserMessage,
		providersessions.TranscriptAssistantMessage,
		providersessions.TranscriptReasoning,
		providersessions.TranscriptToolCall,
		providersessions.TranscriptToolOutput,
		providersessions.TranscriptReasoning,
	}
	if len(transcript) != len(wantTypes) {
		t.Fatalf("Transcript = %#v, want %d normalized entries", transcript, len(wantTypes))
	}
	for index, want := range wantTypes {
		if transcript[index].Type != want || transcript[index].Order != index+1 {
			t.Fatalf("Transcript[%d] = %#v, want type %q order %d", index, transcript[index], want, index+1)
		}
	}
}

func assertNormalizedFunctionCalls(t *testing.T, calls []providersessions.FunctionCallSummary) {
	t.Helper()
	if len(calls) != 1 {
		t.Fatalf("FunctionCalls = %#v, want one deduplicated call", calls)
	}
	call := calls[0]
	if stringValue(call.CallID) != "call-1" || stringValue(call.Name) != "search" ||
		stringValue(call.Arguments) != `{"q":"docs"}` || stringValue(call.Output) != "found" ||
		stringValue(call.Status) != "completed" {
		t.Fatalf("FunctionCalls[0] = %#v", call)
	}
}

func assertNormalizedReasoning(t *testing.T, reasoning []providersessions.ReasoningSummary) {
	t.Helper()
	if len(reasoning) != 2 {
		t.Fatalf("Reasoning = %#v, want readable and encrypted facts", reasoning)
	}
	encrypted := reasoning[1]
	if encrypted.Encrypted == nil || !*encrypted.Encrypted || encrypted.Text != nil || encrypted.EncryptedContent != nil {
		t.Fatalf("encrypted reasoning = %#v, want unavailable plaintext and ciphertext", encrypted)
	}
}

func assertNormalizedTurns(t *testing.T, turns []providersessions.TurnSummary) {
	t.Helper()
	if len(turns) != 1 || turns[0].FunctionCallCount != 1 ||
		turns[0].ReasoningCount != 2 || turns[0].EventCount != 6 {
		t.Fatalf("Turns = %#v", turns)
	}
}

func assertNormalizedUsage(t *testing.T, usage *providersessions.TokenUsage) {
	t.Helper()
	if usage == nil || intValue(usage.InputTokens) != 10 || intValue(usage.OutputTokens) != 5 ||
		intValue(usage.CachedInputTokens) != 2 || intValue(usage.CacheWriteTokens) != 1 ||
		intValue(usage.TotalTokens) != 18 {
		t.Fatalf("TokenUsage = %#v", usage)
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func TestReadRejectsInvalidReferencesBeforeStorageIO(t *testing.T) {
	var walks, opens int
	reader, err := New(
		platformfilesystem.Local{},
		func(string, fs.WalkDirFunc) error {
			walks++
			return nil
		},
		filepath.EvalSymlinks,
		func(string, string) (*sql.DB, error) {
			opens++
			return nil, errors.New("unexpected database open")
		},
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tests := []struct {
		name string
		ref  providers.SessionRef
		want error
	}{
		{
			name: "unsupported provider",
			ref:  providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "session-1"},
			want: providersessions.ErrUnsupportedProvider,
		},
		{
			name: "legacy cursor alias is not canonical",
			ref:  providers.SessionRef{Provider: providers.ID("cursor"), Kind: providers.SessionIDKind, ID: "session-1"},
			want: providersessions.ErrUnsupportedProvider,
		},
		{
			name: "wrong kind",
			ref:  providers.SessionRef{Provider: providers.IDCursor, Kind: "path", ID: "session-1"},
			want: providersessions.ErrUnsupportedKind,
		},
		{
			name: "empty id",
			ref:  providers.SessionRef{Provider: providers.IDCursor, Kind: providers.SessionIDKind, ID: "  "},
			want: providersessions.ErrInvalidIdentifier,
		},
		{
			name: "path-like id",
			ref:  providers.SessionRef{Provider: providers.IDCursor, Kind: providers.SessionIDKind, ID: "../other-session"},
			want: providersessions.ErrInvalidIdentifier,
		},
		{
			name: "malformed id",
			ref:  providers.SessionRef{Provider: providers.IDCursor, Kind: providers.SessionIDKind, ID: "session.with.dot"},
			want: providersessions.ErrInvalidIdentifier,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := reader.Read(context.Background(),test.ref); !errors.Is(err, test.want) {
				t.Fatalf("Read error = %v, want %v", err, test.want)
			}
		})
	}
	if walks != 0 || opens != 0 {
		t.Fatalf("rejected references performed storage IO: walks=%d opens=%d", walks, opens)
	}
}

func TestReadMissingAndAmbiguousSessionsNeverOpenDatabase(t *testing.T) {
	for _, test := range []struct {
		name    string
		prepare func(*testing.T) string
		want    error
	}{
		{
			name:    "missing",
			prepare: func(t *testing.T) string { return t.TempDir() },
			want:    providersessions.ErrSessionNotFound,
		},
		{
			name: "ambiguous",
			prepare: func(t *testing.T) string {
				root := t.TempDir()
				for _, workspace := range []string{"a", "b"} {
					path := filepath.Join(root, workspace, "same-session", "store.db")
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
						t.Fatalf("mkdir: %v", err)
					}
					if err := os.WriteFile(path, nil, 0o600); err != nil {
						t.Fatalf("write store: %v", err)
					}
				}
				return root
			},
			want: providersessions.ErrAmbiguousSessionFile,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			opens := 0
			reader, err := New(
				platformfilesystem.Local{},
				filepath.WalkDir,
				filepath.EvalSymlinks,
				func(string, string) (*sql.DB, error) {
					opens++
					return nil, errors.New("unexpected database open")
				},
				test.prepare(t),
			)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			_, err = reader.Read(context.Background(),providers.SessionRef{
				Provider: providers.IDCursor,
				Kind:     providers.SessionIDKind,
				ID:       "same-session",
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("Read error = %v, want %v", err, test.want)
			}
			if opens != 0 {
				t.Fatalf("database opens = %d, want 0", opens)
			}
		})
	}
}

func TestReadRejectsCandidateResolvedOutsideRootBeforeDatabaseOpen(t *testing.T) {
	root := t.TempDir()
	sessionID := "replaced-session"
	candidate := filepath.Join(root, "workspace", sessionID, "store.db")
	if err := os.MkdirAll(filepath.Dir(candidate), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(candidate, nil, 0o600); err != nil {
		t.Fatalf("write candidate: %v", err)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs root: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "replacement.db")
	opens := 0
	resolve := func(path string) (string, error) {
		if filepath.Clean(path) == filepath.Clean(absoluteRoot) {
			return absoluteRoot, nil
		}
		return outside, nil
	}
	reader, err := New(
		platformfilesystem.Local{},
		filepath.WalkDir,
		resolve,
		func(string, string) (*sql.DB, error) {
			opens++
			return nil, errors.New("unexpected database open")
		},
		root,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = reader.Read(context.Background(),providers.SessionRef{
		Provider: providers.IDCursor,
		Kind:     providers.SessionIDKind,
		ID:       sessionID,
	})
	if !errors.Is(err, providersessions.ErrInvalidIdentifier) {
		t.Fatalf("Read error = %v, want ErrInvalidIdentifier", err)
	}
	if opens != 0 {
		t.Fatalf("database opens = %d, want 0", opens)
	}
}

func writeSessionFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	sessionID := "canonical-cursor-session"
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
INSERT INTO blobs (key, value) VALUES ('bubble-1', '{"bubbleId":"bubble-1","text":"hello","timestamp":1000,"type":1}');
INSERT INTO meta (key, value) VALUES ('0', '{"agentId":"canonical-cursor-session","createdAt":1000}');
`); err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	return root, sessionID
}

func writeNormalizedSessionFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	sessionID := "normalized-cursor-session"
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
`); err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	user := `{"id":"user-1","role":"user","timestamp":2000,"content":[{"type":"input_text","text":"question"}]}`
	assistant := `{"id":"assistant-1","role":"assistant","timestamp":2000,"content":[{"type":"output_text","text":"answer"},{"type":"reasoning","text":"considered","summary":"brief"},{"type":"tool_call","name":"search","tool_call_id":"call-1","arguments":{"q":"docs"},"status":"started"},{"type":"tool","name":"search","tool_call_id":"call-1","content":"found","status":"completed"},{"type":"redacted-reasoning","data":"sensitive-ciphertext"}]}`
	composer := `{"composerId":"composer-1","createdAt":1000,"fullConversationHeadersOnly":[{"bubbleId":"user-1","type":1},{"bubbleId":"assistant-1","type":2}]}`
	usage := `{"usage":{"inputTokens":10,"outputTokens":5,"cacheReadTokens":2,"cacheWriteTokens":1}}`
	for _, row := range []struct {
		key   string
		value string
	}{
		{key: "03-assistant-protobuf-duplicate", value: protobufJSON(assistant)},
		{key: "02-assistant-json", value: assistant},
		{key: "01-user", value: user},
		{key: "04-composer", value: composer},
	} {
		if _, err := db.Exec(`INSERT INTO blobs (key, value) VALUES (?, ?)`, row.key, row.value); err != nil {
			t.Fatalf("insert blob %s: %v", row.key, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO meta (key, value) VALUES ('usage', ?)`, usage); err != nil {
		t.Fatalf("insert usage: %v", err)
	}
	return root, sessionID
}

func protobufJSON(value string) string {
	encoded := []byte{0x0a}
	encoded = binary.AppendUvarint(encoded, uint64(len(value)))
	encoded = append(encoded, value...)
	return string(encoded)
}

func TestReadHonorsCancellationAndRedactsLookupFailures(t *testing.T) {
	root, sessionID := writeSessionFixture(t)
	reader, err := New(
		platformfilesystem.Local{},
		filepath.WalkDir,
		filepath.EvalSymlinks,
		sql.Open,
		root,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ref := providers.SessionRef{
		Provider: providers.IDCursor,
		Kind:     providers.SessionIDKind,
		ID:       sessionID,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reader.Read(ctx, ref); !errors.Is(err, providersessions.ErrOperationCanceled) {
		t.Fatalf("canceled Read error = %v, want ErrOperationCanceled", err)
	}

	_, err = reader.Read(context.Background(), providers.SessionRef{
		Provider: providers.IDCursor,
		Kind:     providers.SessionIDKind,
		ID:       "missing-session",
	})
	if !errors.Is(err, providersessions.ErrSessionNotFound) {
		t.Fatalf("missing session error = %v, want ErrSessionNotFound", err)
	}
	var lookupErr *providersessions.LookupError
	if !errors.As(err, &lookupErr) {
		t.Fatalf("error = %T, want LookupError", err)
	}
	if strings.Contains(lookupErr.Error(), string(filepath.Separator)) && strings.Contains(lookupErr.Error(), "store.db") {
		t.Fatalf("lookup error leaked absolute storage path: %v", lookupErr)
	}
}
