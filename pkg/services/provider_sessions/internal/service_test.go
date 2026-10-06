package internal_test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"
	providersessionswire "github.com/portpowered/infinite-you/pkg/services/provider_sessions/wire"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

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
		sql.Open,
		t.TempDir(),
		emptyCapturedReader{},
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

func TestDetailsUsesCanonicalCursorAndWrapsLookupContext(t *testing.T) {
	root := t.TempDir()
	for _, provider := range []string{"cursor"} {
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
	for _, provider := range []string{"agent", "cursor-agent"} {
		if _, err := newServiceForRoots(t, t.TempDir(), string(root)).Details(provider, "session_id", "missing-session"); !errors.Is(err, providersessions.ErrUnsupportedProvider) {
			t.Fatalf("Details(%q) error = %v, want ErrUnsupportedProvider", provider, err)
		}
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
	if !errors.Is(err, providersessions.ErrUnsupportedProvider) {
		t.Fatalf("error = %v, want ErrUnsupportedProvider", err)
	}
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
	if !errors.Is(err, providersessions.ErrUnsupportedProvider) {
		t.Fatalf("error = %v, want ErrUnsupportedProvider", err)
	}
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
	if !errors.As(err, &lookupErr) || lookupErr.Provider != provider || lookupErr.Root != root || lookupErr.SessionID != "missing-session" {
		t.Fatalf("lookup error = %#v, want provider=%q root=%q session=missing-session", lookupErr, provider, root)
	}
}

func TestNewRejectsMissingProcessEdges(t *testing.T) {
	resolveHome := providersessionsinternal.ResolveHomeDirectory(func() (string, error) { return t.TempDir(), nil })
	tests := []struct {
		name                  string
		files                 providersessionsinternal.FileSystem
		home                  providersessionsinternal.ResolveHomeDirectory
		cursorWalk            providersessionsinternal.CursorWalkDirectory
		cursorSymlinks        providersessionsinternal.CursorResolveSymlinks
		cursorDatabase        providersessionsinternal.CursorOpenSQLDatabase
		cursorOperatingSystem providersessionsinternal.OperatingSystem
	}{
		{name: "filesystem", home: resolveHome, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open, cursorOperatingSystem: providersessionsinternal.OperatingSystem(runtime.GOOS)},
		{name: "home resolver", files: platformfilesystem.Local{}, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open, cursorOperatingSystem: providersessionsinternal.OperatingSystem(runtime.GOOS)},
		{name: "Cursor walker", files: platformfilesystem.Local{}, home: resolveHome, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open, cursorOperatingSystem: providersessionsinternal.OperatingSystem(runtime.GOOS)},
		{name: "Cursor symlink resolver", files: platformfilesystem.Local{}, home: resolveHome, cursorWalk: filepath.WalkDir, cursorDatabase: sql.Open, cursorOperatingSystem: providersessionsinternal.OperatingSystem(runtime.GOOS)},
		{name: "Cursor database opener", files: platformfilesystem.Local{}, home: resolveHome, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorOperatingSystem: providersessionsinternal.OperatingSystem(runtime.GOOS)},
		{name: "operating system", files: platformfilesystem.Local{}, home: resolveHome, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := providersessionswire.NewService(test.files, test.home, test.cursorWalk, test.cursorSymlinks, test.cursorDatabase, test.cursorOperatingSystem, emptyCapturedReader{})
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
		sql.Open,
		providersessionsinternal.OperatingSystem(runtime.GOOS),
		emptyCapturedReader{},
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
		sql.Open,
		providersessionsinternal.OperatingSystem(runtime.GOOS),
		emptyCapturedReader{},
	)
	if err == nil {
		t.Fatal("New() error = nil, want empty home rejection")
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
		sql.Open,
		providersessionsinternal.OperatingSystem(runtime.GOOS),
		emptyCapturedReader{},
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
		sql.Open,
		t.TempDir(),
		emptyCapturedReader{},
	)
	if err == nil {
		t.Fatal("NewForRoots() error = nil, want missing filesystem dependency")
	}
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

func newServiceForRoots(t *testing.T, codexRoot, cursorRoot string) providersessions.Service {
	t.Helper()
	service, err := providersessionswire.NewForRoots(
		platformfilesystem.Local{},
		providersessionsinternal.CursorWalkDirectory(filepath.WalkDir),
		providersessionsinternal.CursorResolveSymlinks(filepath.EvalSymlinks),
		providersessionsinternal.CursorOpenSQLDatabase(sql.Open),
		cursorRoot,
		emptyCapturedReader{},
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

type emptyCapturedReader struct{}

func (emptyCapturedReader) ListWorkerSessionCaptures(context.Context, recordings.WorkerCapturedCatalogRequest) (recordings.WorkerCapturedCatalogPage, error) {
	return recordings.WorkerCapturedCatalogPage{}, nil
}
func (emptyCapturedReader) ReadWorkerCapturedActivity(context.Context, recordings.WorkerCapturedActivityRequest) (recordings.WorkerCapturedActivityPage, error) {
	panic("unexpected activity read")
}
func (emptyCapturedReader) LookupWorkerSessionCapture(context.Context, string) (recordings.WorkerSessionCatalogEntry, error) {
	panic("unexpected lookup")
}
