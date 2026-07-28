package providersessions_test

import (
	"database/sql"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"
	providersessionswire "github.com/portpowered/infinite-you/pkg/services/provider_sessions/wire"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestDetailsLoadsCodexSessionThroughService(t *testing.T) {
	root := writeCodexSessionFixture(t, "session-123")

	detail, err := newServiceForRoots(t, root, "").Details("codex", "session_id", "session-123")
	if err != nil {
		t.Fatalf("Details: %v", err)
	}
	if detail.ProviderSession.Provider != providersessions.ProviderCodex ||
		detail.ProviderSession.ID != "session-123" {
		t.Fatalf("provider session = %#v, want codex session-123", detail.ProviderSession)
	}
}

func TestDetailsReconstructsNormalizedCodexJSONLThroughRoot(t *testing.T) {
	root := writeRichCodexSessionFixture(t, "session-reconstruct-root")
	svc := newServiceForRoots(t, root, "")

	first, err := svc.Details("codex", providersessions.SessionIDKind, "session-reconstruct-root")
	if err != nil {
		t.Fatalf("Details: %v", err)
	}
	second, err := svc.Details("codex", providersessions.SessionIDKind, "session-reconstruct-root")
	if err != nil {
		t.Fatalf("Details repeat: %v", err)
	}
	if first.ProviderSession.ID != "session-reconstruct-root" ||
		first.Source.RelativePath != "2026/07/27/rollout-session-reconstruct-root.jsonl" {
		t.Fatalf("detail identity = %#v, want normalized codex session and source", first)
	}
	if len(first.Transcript) < 4 || len(first.Parse.FunctionCalls) != 1 || len(first.Parse.Reasoning) != 1 {
		t.Fatalf("detail = %#v, want transcript, tool, and reasoning facts", first)
	}
	if first.Parse.TokenUsage == nil || first.Parse.TokenUsage.TotalTokens == nil ||
		*first.Parse.TokenUsage.TotalTokens != 130 {
		t.Fatalf("token usage = %#v, want total 130", first.Parse.TokenUsage)
	}
	if first.Transcript[0].Text == nil || !strings.Contains(*first.Transcript[0].Text, "Inspect the failing run") {
		t.Fatalf("transcript = %#v, want user message text", first.Transcript)
	}
	*first.Transcript[0].Text = "mutated"
	if *second.Transcript[0].Text == "mutated" {
		t.Fatalf("mutating first inspection affected second inspection transcript")
	}
}

func TestInspectLoadsTypedSessionRefThroughService(t *testing.T) {
	root := writeCodexSessionFixture(t, "session-inspect-1")
	svc := newServiceForRoots(t, root, "")
	ref := providersessions.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providersessions.SessionIDKind,
		ID:       "session-inspect-1",
	}

	result, err := svc.Inspect(providersessions.InspectRequest{Session: ref})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if result.Session != ref {
		t.Fatalf("InspectResult.Session = %#v, want %#v", result.Session, ref)
	}
	if strings.TrimSpace(result.Source.RelativePath) == "" {
		t.Fatalf("InspectResult.Source.RelativePath empty")
	}
}

func TestInspectLoadsCanonicalCursorProvidersRefThroughService(t *testing.T) {
	root, sessionID := writeCursorSessionFixture(t)
	svc := newServiceForRoots(t, t.TempDir(), root)
	ref := providers.SessionRef{
		Provider: providers.IDCursor,
		Kind:     providers.SessionIDKind,
		ID:       sessionID,
	}

	result, err := svc.Inspect(providersessions.InspectRequest{Session: ref})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if result.Session != ref {
		t.Fatalf("InspectResult.Session = %#v, want %#v", result.Session, ref)
	}
	if result.Source.RelativePath != filepath.ToSlash(filepath.Join("workspace", sessionID, "store.db")) {
		t.Fatalf("InspectResult.Source = %#v", result.Source)
	}
}

func TestProjectReconstructsNormalizedCursorDetailThroughRoot(t *testing.T) {
	root, sessionID := writeCursorSessionFixture(t)
	svc := newServiceForRoots(t, t.TempDir(), root)
	ref := providers.SessionRef{
		Provider: providers.IDCursor,
		Kind:     providers.SessionIDKind,
		ID:       sessionID,
	}

	result, err := svc.Project(providersessions.ProjectRequest{Session: ref})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if result.Session != ref || result.Detail.ProviderSession.Provider != providersessions.ProviderCursor {
		t.Fatalf("ProjectResult = %#v, want canonical Cursor identity", result)
	}
	assertRootCursorProjection(t, result.Detail)
}

func assertRootCursorProjection(t *testing.T, detail providersessions.Detail) {
	t.Helper()
	assertRootCursorTranscript(t, detail.Transcript)
	assertRootCursorFacts(t, detail.Parse)
}

func assertRootCursorTranscript(t *testing.T, transcript []providersessions.TranscriptEntry) {
	t.Helper()
	wantTypes := []providersessions.TranscriptEntryType{
		providersessions.TranscriptUserMessage,
		providersessions.TranscriptAssistantMessage,
		providersessions.TranscriptReasoning,
		providersessions.TranscriptToolCall,
		providersessions.TranscriptToolOutput,
	}
	if len(transcript) != len(wantTypes) {
		t.Fatalf("Transcript = %#v, want %d entries", transcript, len(wantTypes))
	}
	for index, want := range wantTypes {
		if transcript[index].Type != want {
			t.Fatalf("Transcript[%d] = %#v, want %q", index, transcript[index], want)
		}
	}
}

func assertRootCursorFacts(t *testing.T, summary providersessions.ParseSummary) {
	t.Helper()
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

func TestInspectValidatesCanonicalProviderSessionRefBeforeOpeningNativeContent(t *testing.T) {
	files := &openRecordingFileSystem{base: platformfilesystem.Local{}}
	svc, err := providersessionswire.NewForRoots(
		files,
		filepath.WalkDir,
		filepath.EvalSymlinks,
		filepath.WalkDir,
		filepath.EvalSymlinks,
		sql.Open,
		t.TempDir(),
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("NewForRoots: %v", err)
	}

	tests := []struct {
		name string
		ref  providers.SessionRef
		want error
	}{
		{
			name: "provider",
			ref:  providers.SessionRef{Provider: "openai", Kind: providers.SessionIDKind, ID: "session-1"},
			want: providersessions.ErrUnsupportedProvider,
		},
		{
			name: "kind",
			ref:  providers.SessionRef{Provider: providers.IDCodex, Kind: "path", ID: "session-1"},
			want: providersessions.ErrUnsupportedKind,
		},
		{
			name: "identifier",
			ref:  providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "../secret"},
			want: providersessions.ErrInvalidIdentifier,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, inspectErr := svc.Inspect(providersessions.InspectRequest{Session: test.ref})
			if !errors.Is(inspectErr, test.want) {
				t.Fatalf("Inspect error = %v, want %v", inspectErr, test.want)
			}
		})
	}
	if files.opens != 0 {
		t.Fatalf("native file opens = %d, want 0", files.opens)
	}
}

func TestInspectRejectsInvalidIdentifierAndPropagatesTypedFailures(t *testing.T) {
	svc := newServiceForRoots(t, t.TempDir(), "")
	if _, err := svc.Inspect(providersessions.InspectRequest{Session: providersessions.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providersessions.SessionIDKind,
		ID:       "   ",
	}}); !errors.Is(err, providersessions.ErrInvalidIdentifier) {
		t.Fatalf("empty id err = %v, want ErrInvalidIdentifier", err)
	}
	if _, err := svc.Inspect(providersessions.InspectRequest{Session: providersessions.SessionRef{
		Provider: "openai",
		Kind:     providersessions.SessionIDKind,
		ID:       "session-1",
	}}); !errors.Is(err, providersessions.ErrUnsupportedProvider) {
		t.Fatalf("provider err = %v, want ErrUnsupportedProvider", err)
	}
	if _, err := svc.Inspect(providersessions.InspectRequest{Session: providersessions.SessionRef{
		Provider: providers.IDCodex,
		Kind:     "path",
		ID:       "session-1",
	}}); !errors.Is(err, providersessions.ErrUnsupportedKind) {
		t.Fatalf("kind err = %v, want ErrUnsupportedKind", err)
	}
}

func TestProjectLoadsNormalizedDetailThroughService(t *testing.T) {
	root := writeCodexSessionFixture(t, "session-project-1")
	svc := newServiceForRoots(t, root, "")
	ref := providersessions.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providersessions.SessionIDKind,
		ID:       "session-project-1",
	}

	result, err := svc.Project(providersessions.ProjectRequest{Session: ref})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if result.Session != ref {
		t.Fatalf("ProjectResult.Session = %#v, want %#v", result.Session, ref)
	}
	if result.Detail.ProviderSession.Provider != providersessions.ProviderCodex ||
		result.Detail.ProviderSession.ID != ref.ID {
		t.Fatalf("ProjectResult.Detail.ProviderSession = %#v", result.Detail.ProviderSession)
	}
}

func TestProjectRejectsInvalidIdentifierAndPropagatesTypedFailures(t *testing.T) {
	svc := newServiceForRoots(t, t.TempDir(), "")
	if _, err := svc.Project(providersessions.ProjectRequest{Session: providersessions.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providersessions.SessionIDKind,
		ID:       "",
	}}); !errors.Is(err, providersessions.ErrInvalidIdentifier) {
		t.Fatalf("empty id err = %v, want ErrInvalidIdentifier", err)
	}
	if _, err := svc.Project(providersessions.ProjectRequest{Session: providersessions.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providersessions.SessionIDKind,
		ID:       "missing-session",
	}}); !errors.Is(err, providersessions.ErrSessionNotFound) {
		t.Fatalf("missing err = %v, want ErrSessionNotFound", err)
	}
}

func TestDetailsNormalizesCursorAliasesAndWrapsLookupContext(t *testing.T) {
	root := t.TempDir()
	for _, provider := range []string{"cursor", "agent", "cursor-agent"} {
		t.Run(provider, func(t *testing.T) {
			_, err := newServiceForRoots(t, t.TempDir(), string(root)).Details(provider, "session_id", "missing-session")
			if !errors.Is(err, providersessions.ErrSessionNotFound) {
				t.Fatalf("err = %v, want ErrSessionNotFound", err)
			}
			var lookupErr *providersessions.LookupError
			if !errors.As(err, &lookupErr) {
				t.Fatalf("err = %T, want LookupError", err)
			}
			if lookupErr.Provider != providersessions.ProviderCursor || lookupErr.Root != string(root) {
				t.Fatalf("lookup error = %#v, want normalized cursor root context", lookupErr)
			}
		})
	}
}

func TestDetailsRejectsUnsupportedProviderAndKind(t *testing.T) {
	svc := newServiceForRoots(t, t.TempDir(), "")
	if _, err := svc.Details("openai", "session_id", "session-123"); !errors.Is(err, providersessions.ErrUnsupportedProvider) {
		t.Fatalf("provider err = %v, want ErrUnsupportedProvider", err)
	}
	if _, err := svc.Details("codex", "path", "session-123"); !errors.Is(err, providersessions.ErrUnsupportedKind) {
		t.Fatalf("kind err = %v, want ErrUnsupportedKind", err)
	}
}

func TestGetProviderSessionDetails_LegacyAgentCursorNotFoundIsDistinguishable(t *testing.T) {
	root := t.TempDir()
	_, err := newServiceForRoots(t, t.TempDir(), root).Details("agent", "session_id", "missing-session")
	assertCursorLookupContext(t, err, root)
}

func TestGetProviderSessionDetails_RejectsUnsupportedProviderOrKindByContract(t *testing.T) {
	service := newServiceForRoots(t, t.TempDir(), t.TempDir())
	if _, err := service.Details("openai", "session_id", "session-123"); !errors.Is(err, providersessions.ErrUnsupportedProvider) {
		t.Fatalf("provider error = %v, want ErrUnsupportedProvider", err)
	}
	if _, err := service.Details("codex", "path", "session-123"); !errors.Is(err, providersessions.ErrUnsupportedKind) {
		t.Fatalf("kind error = %v, want ErrUnsupportedKind", err)
	}
}

func TestGetProviderSessionDetails_LoadsLegacyAgentCursorSessionFromConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	_, err := newServiceForRoots(t, t.TempDir(), root).Details("agent", "session_id", "missing-session")
	assertCursorLookupContext(t, err, root)
}

func TestGetProviderSessionDetails_RegressionLoadsCodexAndCursorFromConfiguredRoots(t *testing.T) {
	codexRoot, cursorRoot := t.TempDir(), t.TempDir()
	service := newServiceForRoots(t, codexRoot, cursorRoot)
	_, codexErr := service.Details("codex", "session_id", "missing-session")
	assertLookupContext(t, codexErr, providersessions.ProviderCodex, "")
	_, cursorErr := service.Details("cursor", "session_id", "missing-session")
	assertCursorLookupContext(t, cursorErr, cursorRoot)
}

func TestGetProviderSessionDetails_EventRefRoundTripLoadsCursorAndCodex(t *testing.T) {
	codexRoot, cursorRoot := t.TempDir(), t.TempDir()
	service := newServiceForRoots(t, codexRoot, cursorRoot)
	for _, test := range []struct {
		provider string
		want     providersessions.Provider
		root     string
	}{
		{"codex", providersessions.ProviderCodex, ""},
		{"cursor", providersessions.ProviderCursor, cursorRoot},
		{"agent", providersessions.ProviderCursor, cursorRoot},
	} {
		_, err := service.Details(test.provider, "session_id", "missing-session")
		assertLookupContext(t, err, test.want, test.root)
	}
}

func TestGetProviderSessionDetails_CursorNotFoundLogsDiagnostic(t *testing.T) {
	root := t.TempDir()
	_, err := newServiceForRoots(t, t.TempDir(), root).Details("cursor", "session_id", "missing-session")
	assertCursorLookupContext(t, err, root)
}

func TestGetProviderSessionDetails_CursorNotFoundLogsDiagnosticWhenRootUnconfigured(t *testing.T) {
	_, err := newServiceForRoots(t, t.TempDir(), "").Details("cursor", "session_id", "missing-session")
	assertCursorLookupContext(t, err, "")
}

func assertCursorLookupContext(t *testing.T, err error, root string) {
	t.Helper()
	assertLookupContext(t, err, providersessions.ProviderCursor, root)
}

func assertLookupContext(t *testing.T, err error, provider providersessions.Provider, root string) {
	t.Helper()
	if !errors.Is(err, providersessions.ErrSessionNotFound) {
		t.Fatalf("error = %v, want ErrSessionNotFound", err)
	}
	var lookupErr *providersessions.LookupError
	if !errors.As(err, &lookupErr) || lookupErr.Provider != provider || lookupErr.Root != root {
		t.Fatalf("lookup error = %#v, want provider=%q root=%q", lookupErr, provider, root)
	}
}

func TestNewRejectsMissingProcessEdges(t *testing.T) {
	resolveHome := providersessionsinternal.ResolveHomeDirectory(func() (string, error) { return t.TempDir(), nil })
	tests := []struct {
		name                  string
		files                 providersessionsinternal.FileSystem
		home                  providersessionsinternal.ResolveHomeDirectory
		codexWalk             providersessionsinternal.CodexWalkDirectory
		codexSymlinks         providersessionsinternal.CodexResolveSymlinks
		cursorWalk            providersessionsinternal.CursorWalkDirectory
		cursorSymlinks        providersessionsinternal.CursorResolveSymlinks
		cursorDatabase        providersessionsinternal.CursorOpenSQLDatabase
		cursorOperatingSystem providersessionsinternal.OperatingSystem
	}{
		{name: "filesystem", home: resolveHome, codexWalk: filepath.WalkDir, codexSymlinks: filepath.EvalSymlinks, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open, cursorOperatingSystem: providersessionsinternal.OperatingSystem(runtime.GOOS)},
		{name: "home resolver", files: platformfilesystem.Local{}, codexWalk: filepath.WalkDir, codexSymlinks: filepath.EvalSymlinks, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open, cursorOperatingSystem: providersessionsinternal.OperatingSystem(runtime.GOOS)},
		{name: "Codex walker", files: platformfilesystem.Local{}, home: resolveHome, codexSymlinks: filepath.EvalSymlinks, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open, cursorOperatingSystem: providersessionsinternal.OperatingSystem(runtime.GOOS)},
		{name: "Codex symlink resolver", files: platformfilesystem.Local{}, home: resolveHome, codexWalk: filepath.WalkDir, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open, cursorOperatingSystem: providersessionsinternal.OperatingSystem(runtime.GOOS)},
		{name: "Cursor walker", files: platformfilesystem.Local{}, home: resolveHome, codexWalk: filepath.WalkDir, codexSymlinks: filepath.EvalSymlinks, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open, cursorOperatingSystem: providersessionsinternal.OperatingSystem(runtime.GOOS)},
		{name: "Cursor symlink resolver", files: platformfilesystem.Local{}, home: resolveHome, codexWalk: filepath.WalkDir, codexSymlinks: filepath.EvalSymlinks, cursorWalk: filepath.WalkDir, cursorDatabase: sql.Open, cursorOperatingSystem: providersessionsinternal.OperatingSystem(runtime.GOOS)},
		{name: "Cursor database opener", files: platformfilesystem.Local{}, home: resolveHome, codexWalk: filepath.WalkDir, codexSymlinks: filepath.EvalSymlinks, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorOperatingSystem: providersessionsinternal.OperatingSystem(runtime.GOOS)},
		{name: "operating system", files: platformfilesystem.Local{}, home: resolveHome, codexWalk: filepath.WalkDir, codexSymlinks: filepath.EvalSymlinks, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := providersessionswire.NewService(test.files, test.home, test.codexWalk, test.codexSymlinks, test.cursorWalk, test.cursorSymlinks, test.cursorDatabase, test.cursorOperatingSystem)
			if err == nil {
				t.Fatalf("New() error = nil, want missing %s dependency", test.name)
			}
		})
	}
}

func TestNewPropagatesHomeResolverFailure(t *testing.T) {
	_, err := providersessionswire.NewService(
		platformfilesystem.Local{},
		func() (string, error) { return "", errors.New("home unavailable") },
		filepath.WalkDir,
		filepath.EvalSymlinks,
		filepath.WalkDir,
		filepath.EvalSymlinks,
		sql.Open,
		providersessionsinternal.OperatingSystem(runtime.GOOS),
	)
	if err == nil {
		t.Fatal("New() error = nil, want home resolver failure")
	}
}

func TestNewRejectsEmptyHomeDirectory(t *testing.T) {
	_, err := providersessionswire.NewService(
		platformfilesystem.Local{},
		func() (string, error) { return "   ", nil },
		filepath.WalkDir,
		filepath.EvalSymlinks,
		filepath.WalkDir,
		filepath.EvalSymlinks,
		sql.Open,
		providersessionsinternal.OperatingSystem(runtime.GOOS),
	)
	if err == nil {
		t.Fatal("New() error = nil, want empty home rejection")
	}
}

func TestInspectSucceedsWithNilRequestContext(t *testing.T) {
	root := writeCodexSessionFixture(t, "session-nil-ctx")
	svc := newServiceForRoots(t, root, "")
	ref := providersessions.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providersessions.SessionIDKind,
		ID:       "session-nil-ctx",
	}

	result, err := svc.Inspect(providersessions.InspectRequest{Session: ref})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if result.Session != ref {
		t.Fatalf("InspectResult.Session = %#v, want %#v", result.Session, ref)
	}
}

func TestNewConstructsServiceWithValidDependencies(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".cursor", "chats"), 0o755); err != nil {
		t.Fatalf("mkdir cursor chats: %v", err)
	}
	resolveHome := providersessionsinternal.ResolveHomeDirectory(func() (string, error) { return home, nil })
	svc, err := providersessionswire.NewService(
		platformfilesystem.Local{},
		resolveHome,
		filepath.WalkDir,
		filepath.EvalSymlinks,
		filepath.WalkDir,
		filepath.EvalSymlinks,
		sql.Open,
		providersessionsinternal.OperatingSystem(runtime.GOOS),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if svc == nil {
		t.Fatal("New returned nil Service")
	}
	if _, err := svc.Inspect(providersessions.InspectRequest{Session: providersessions.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providersessions.SessionIDKind,
		ID:       "missing-from-default-root",
	}}); !errors.Is(err, providersessions.ErrSessionNotFound) {
		t.Fatalf("Inspect via New service = %v, want ErrSessionNotFound", err)
	}
}

func TestNewForRootsRejectsMissingProcessEdges(t *testing.T) {
	_, err := providersessionswire.NewForRoots(
		nil,
		filepath.WalkDir,
		filepath.EvalSymlinks,
		filepath.WalkDir,
		filepath.EvalSymlinks,
		sql.Open,
		t.TempDir(),
		t.TempDir(),
	)
	if err == nil {
		t.Fatal("NewForRoots() error = nil, want missing filesystem dependency")
	}
}

func writeCodexSessionFixture(t *testing.T, sessionID string) string {
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

func writeCursorSessionFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	sessionID := "cursor-root-inspect"
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
INSERT INTO meta (key, value) VALUES ('0', '{"agentId":"cursor-root-inspect","createdAt":1000}');
INSERT INTO meta (key, value) VALUES ('usage', '{"usage":{"inputTokens":4,"outputTokens":3}}');
`); err != nil {
		t.Fatalf("create Cursor fixture: %v", err)
	}
	return root, sessionID
}

func writeRichCodexSessionFixture(t *testing.T, sessionID string) string {
	t.Helper()
	root := t.TempDir()
	sessionDir := filepath.Join(root, "2026", "07", "27")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session fixture: %v", err)
	}
	content := strings.Join([]string{
		`{"timestamp":"2026-05-18T10:00:00Z","type":"turn_context"}`,
		`{"timestamp":"2026-05-18T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Inspect the failing run."}]}}`,
		`{"timestamp":"2026-05-18T10:00:02Z","type":"response_item","payload":{"type":"reasoning","summary":["Checking tool output"]}}`,
		`{"timestamp":"2026-05-18T10:00:03Z","type":"response_item","payload":{"type":"function_call","call_id":"call-1","name":"exec_command","arguments":"go test ./pkg/api"}}`,
		`{"timestamp":"2026-05-18T10:00:04Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-1","output":"ok"}}`,
		`{"timestamp":"2026-05-18T10:00:05Z","type":"event_msg","payload":{"type":"agent_message","message":"The package tests passed."}}`,
		`{"timestamp":"2026-05-18T10:00:06Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":25,"reasoning_output_tokens":5,"total_tokens":130}}}}`,
	}, "\n") + "\n"
	path := filepath.Join(sessionDir, "rollout-"+sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write session fixture: %v", err)
	}
	return root
}

func newServiceForRoots(t *testing.T, codexRoot, cursorRoot string) providersessions.Service {
	t.Helper()
	service, err := providersessionswire.NewForRoots(
		platformfilesystem.Local{},
		providersessionsinternal.CodexWalkDirectory(filepath.WalkDir),
		providersessionsinternal.CodexResolveSymlinks(filepath.EvalSymlinks),
		providersessionsinternal.CursorWalkDirectory(filepath.WalkDir),
		providersessionsinternal.CursorResolveSymlinks(filepath.EvalSymlinks),
		providersessionsinternal.CursorOpenSQLDatabase(sql.Open),
		codexRoot,
		cursorRoot,
	)
	if err != nil {
		t.Fatalf("NewForRoots: %v", err)
	}
	return service
}

type openRecordingFileSystem struct {
	base  providersessionsinternal.FileSystem
	opens int
}

func (f *openRecordingFileSystem) Open(path string) (io.ReadCloser, error) {
	f.opens++
	return f.base.Open(path)
}

func (f *openRecordingFileSystem) Stat(path string) (fs.FileInfo, error) {
	return f.base.Stat(path)
}
