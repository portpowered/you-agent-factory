package providersessions_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionswire "github.com/portpowered/infinite-you/pkg/services/provider_sessions/wire"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// TestRootWireBehavioralBoundary_PublishedServicePreservesObservables constructs
// Provider Sessions exclusively through provider_sessions/wire and proves Details,
// Inspect, and Project preserve observable outcomes for Codex- and Cursor-backed
// reader fixtures on the published providersessions.Service peer surface.
func TestRootWireBehavioralBoundary_PublishedServicePreservesObservables(t *testing.T) {
	t.Run("codex success", func(t *testing.T) {
		root := writeRootCodexSessionFixture(t, "root-wire-behavioral-codex")
		svc := wireServiceForRoots(t, root, "")

		ref := providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "root-wire-behavioral-codex",
		}

		detail, err := svc.Details("codex", providers.SessionIDKind, ref.ID)
		if err != nil {
			t.Fatalf("Details: %v", err)
		}
		if detail.ProviderSession.Provider != providersessions.ProviderCodex ||
			detail.ProviderSession.ID != ref.ID ||
			detail.Source.RelativePath == "" {
			t.Fatalf("Details detail = %#v, want codex session with source metadata", detail)
		}

		inspected, err := svc.Inspect(providersessions.InspectRequest{Session: ref})
		if err != nil {
			t.Fatalf("Inspect: %v", err)
		}
		if inspected.Session != ref {
			t.Fatalf("InspectResult.Session = %#v, want %#v", inspected.Session, ref)
		}
		if inspected.Source.RelativePath != detail.Source.RelativePath ||
			inspected.Source.SizeBytes != detail.Source.SizeBytes {
			t.Fatalf("Inspect source = %#v, want Details source %#v", inspected.Source, detail.Source)
		}

		projected, err := svc.Project(providersessions.ProjectRequest{Session: ref})
		if err != nil {
			t.Fatalf("Project: %v", err)
		}
		if projected.Session != ref {
			t.Fatalf("ProjectResult.Session = %#v, want %#v", projected.Session, ref)
		}
		if projected.Detail.ProviderSession != detail.ProviderSession ||
			projected.Detail.Source.RelativePath != detail.Source.RelativePath {
			t.Fatalf("Project detail = %#v, want Details detail %#v", projected.Detail, detail)
		}
	})

	t.Run("cursor success", func(t *testing.T) {
		root, sessionID := writeRootCursorSessionFixture(t)
		svc := wireServiceForRoots(t, t.TempDir(), root)

		ref := providers.SessionRef{
			Provider: providers.IDCursor,
			Kind:     providers.SessionIDKind,
			ID:       sessionID,
		}

		detail, err := svc.Details("cursor", providers.SessionIDKind, sessionID)
		if err != nil {
			t.Fatalf("Details: %v", err)
		}
		if detail.ProviderSession.Provider != providersessions.ProviderCursor {
			t.Fatalf("Details ProviderSession = %#v, want Cursor", detail.ProviderSession)
		}
		assertRootBoundaryCursorProjection(t, detail)

		inspected, err := svc.Inspect(providersessions.InspectRequest{Session: ref})
		if err != nil {
			t.Fatalf("Inspect: %v", err)
		}
		if inspected.Session != ref {
			t.Fatalf("InspectResult.Session = %#v, want %#v", inspected.Session, ref)
		}
		if inspected.Source.RelativePath == "" {
			t.Fatalf("InspectResult.Source.RelativePath empty")
		}

		projected, err := svc.Project(providersessions.ProjectRequest{Session: ref})
		if err != nil {
			t.Fatalf("Project: %v", err)
		}
		if projected.Session != ref {
			t.Fatalf("ProjectResult.Session = %#v, want %#v", projected.Session, ref)
		}
		assertRootBoundaryCursorProjection(t, projected.Detail)
	})

	t.Run("typed failures", func(t *testing.T) {
		codexRoot := writeRootCodexSessionFixture(t, "root-wire-behavioral-typed-codex")
		cursorRoot, cursorSessionID := writeRootCursorSessionFixture(t)
		svc := wireServiceForRoots(t, codexRoot, cursorRoot)

		codexRef := providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "root-wire-behavioral-typed-codex",
		}
		cursorRef := providers.SessionRef{
			Provider: providers.IDCursor,
			Kind:     providers.SessionIDKind,
			ID:       cursorSessionID,
		}

		if _, err := svc.Details("openai", providers.SessionIDKind, "session-1"); !errors.Is(err, providersessions.ErrUnsupportedProvider) {
			t.Fatalf("Details unsupported provider = %v, want ErrUnsupportedProvider", err)
		}
		if _, err := svc.Inspect(providersessions.InspectRequest{Session: providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     "path",
			ID:       "session-1",
		}}); !errors.Is(err, providersessions.ErrUnsupportedKind) {
			t.Fatalf("Inspect unsupported kind = %v, want ErrUnsupportedKind", err)
		}
		if _, err := svc.Project(providersessions.ProjectRequest{Session: providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "../secret",
		}}); !errors.Is(err, providersessions.ErrInvalidIdentifier) {
			t.Fatalf("Project invalid identifier = %v, want ErrInvalidIdentifier", err)
		}
		if _, err := svc.Details("codex", providers.SessionIDKind, "missing-session"); !errors.Is(err, providersessions.ErrSessionNotFound) {
			t.Fatalf("Details missing codex session = %v, want ErrSessionNotFound", err)
		}
		if _, err := svc.Inspect(providersessions.InspectRequest{Session: providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "missing-session",
		}}); !errors.Is(err, providersessions.ErrSessionNotFound) {
			t.Fatalf("Inspect missing codex session = %v, want ErrSessionNotFound", err)
		}
		if _, err := svc.Project(providersessions.ProjectRequest{Session: codexRef}); err != nil {
			t.Fatalf("Project codex success sanity = %v", err)
		}
		if _, err := svc.Details("cursor", providers.SessionIDKind, "missing-cursor-session"); !errors.Is(err, providersessions.ErrSessionNotFound) {
			t.Fatalf("Details missing cursor session = %v, want ErrSessionNotFound", err)
		}
		if _, err := svc.Inspect(providersessions.InspectRequest{Session: providers.SessionRef{
			Provider: providers.IDCursor,
			Kind:     providers.SessionIDKind,
			ID:       "missing-cursor-session",
		}}); !errors.Is(err, providersessions.ErrSessionNotFound) {
			t.Fatalf("Inspect missing cursor session = %v, want ErrSessionNotFound", err)
		}
		if _, err := svc.Project(providersessions.ProjectRequest{Session: cursorRef}); err != nil {
			t.Fatalf("Project cursor success sanity = %v", err)
		}
	})
}

func wireServiceForRoots(t *testing.T, codexRoot, cursorRoot string) providersessions.Service {
	t.Helper()
	service, err := providersessionswire.NewForRoots(
		platformfilesystem.Local{},
		filepath.WalkDir,
		filepath.EvalSymlinks,
		filepath.WalkDir,
		filepath.EvalSymlinks,
		sql.Open,
		codexRoot,
		cursorRoot,
	)
	if err != nil {
		t.Fatalf("NewForRoots: %v", err)
	}
	return service
}

func writeRootCodexSessionFixture(t *testing.T, sessionID string) string {
	t.Helper()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "2026", "07", "16")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session fixture: %v", err)
	}
	path := filepath.Join(sessionDir, "rollout-"+sessionID+".jsonl")
	if err := os.WriteFile(path, []byte("{\"type\":\"session_meta\"}\n"), 0o600); err != nil {
		t.Fatalf("write session fixture: %v", err)
	}
	return root
}

func writeRootCursorSessionFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	sessionID := "cursor-root-boundary"
	path := filepath.Join(root, "workspace", sessionID, "store.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir Cursor fixture: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open Cursor fixture: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`
CREATE TABLE blobs (key TEXT PRIMARY KEY, value TEXT);
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);
INSERT INTO blobs (key, value) VALUES ('bubble-1', '{"bubbleId":"bubble-1","text":"hello","timestamp":1000,"type":1}');
INSERT INTO blobs (key, value) VALUES ('message-2', '{"id":"assistant-1","role":"assistant","timestamp":2000,"content":[{"type":"output_text","text":"answer"},{"type":"reasoning","text":"reasoning","summary":"summary"},{"type":"tool_call","name":"read","tool_call_id":"call-1","arguments":{"path":"file.go"}},{"type":"tool","name":"read","tool_call_id":"call-1","content":"tool result"}]}');
INSERT INTO meta (key, value) VALUES ('0', '{"agentId":"cursor-root-boundary","createdAt":1000}');
INSERT INTO meta (key, value) VALUES ('usage', '{"usage":{"inputTokens":4,"outputTokens":3}}');
`); err != nil {
		t.Fatalf("create Cursor fixture: %v", err)
	}
	return root, sessionID
}

func assertRootBoundaryCursorProjection(t *testing.T, detail providersessions.Detail) {
	t.Helper()
	wantTypes := []providersessions.TranscriptEntryType{
		providersessions.TranscriptUserMessage,
		providersessions.TranscriptAssistantMessage,
		providersessions.TranscriptReasoning,
		providersessions.TranscriptToolCall,
		providersessions.TranscriptToolOutput,
	}
	if len(detail.Transcript) != len(wantTypes) {
		t.Fatalf("Transcript = %#v, want %d entries", detail.Transcript, len(wantTypes))
	}
	for index, want := range wantTypes {
		if detail.Transcript[index].Type != want {
			t.Fatalf("Transcript[%d] = %#v, want %q", index, detail.Transcript[index], want)
		}
	}
	summary := detail.Parse
	if len(summary.FunctionCalls) != 1 ||
		summary.FunctionCalls[0].Output == nil ||
		*summary.FunctionCalls[0].Output != "tool result" {
		t.Fatalf("FunctionCalls = %#v, want attached tool output", summary.FunctionCalls)
	}
	if len(summary.Reasoning) != 1 ||
		summary.Reasoning[0].Text == nil ||
		*summary.Reasoning[0].Text != "reasoning" {
		t.Fatalf("Reasoning = %#v", summary.Reasoning)
	}
	if summary.TokenUsage == nil ||
		summary.TokenUsage.TotalTokens == nil ||
		*summary.TokenUsage.TotalTokens != 7 {
		t.Fatalf("TokenUsage = %#v, want total 7", summary.TokenUsage)
	}
}
