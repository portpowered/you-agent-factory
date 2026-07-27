package providersessions_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providercursor "github.com/portpowered/infinite-you/pkg/services/provider_sessions/cursor"
	providersessionsservice "github.com/portpowered/infinite-you/pkg/services/provider_sessions/service"
	"github.com/portpowered/infinite-you/pkg/services/providers"
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
	root := providercursor.AgentStorageRoot(t.TempDir())
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
	assertLookupContext(t, codexErr, providersessions.ProviderCodex, codexRoot)
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
		{"codex", providersessions.ProviderCodex, codexRoot},
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
	resolveHome := providersessions.ResolveHomeDirectory(func() (string, error) { return t.TempDir(), nil })
	tests := []struct {
		name                  string
		files                 providersessions.FileSystem
		home                  providersessions.ResolveHomeDirectory
		codexWalk             providersessions.CodexWalkDirectory
		codexSymlinks         providersessions.CodexResolveSymlinks
		cursorWalk            providersessions.CursorWalkDirectory
		cursorSymlinks        providersessions.CursorResolveSymlinks
		cursorDatabase        providersessions.CursorOpenSQLDatabase
		cursorOperatingSystem providersessions.OperatingSystem
	}{
		{name: "filesystem", home: resolveHome, codexWalk: filepath.WalkDir, codexSymlinks: filepath.EvalSymlinks, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open, cursorOperatingSystem: providersessions.OperatingSystem(runtime.GOOS)},
		{name: "home resolver", files: platformfilesystem.Local{}, codexWalk: filepath.WalkDir, codexSymlinks: filepath.EvalSymlinks, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open, cursorOperatingSystem: providersessions.OperatingSystem(runtime.GOOS)},
		{name: "Codex walker", files: platformfilesystem.Local{}, home: resolveHome, codexSymlinks: filepath.EvalSymlinks, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open, cursorOperatingSystem: providersessions.OperatingSystem(runtime.GOOS)},
		{name: "Codex symlink resolver", files: platformfilesystem.Local{}, home: resolveHome, codexWalk: filepath.WalkDir, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open, cursorOperatingSystem: providersessions.OperatingSystem(runtime.GOOS)},
		{name: "Cursor walker", files: platformfilesystem.Local{}, home: resolveHome, codexWalk: filepath.WalkDir, codexSymlinks: filepath.EvalSymlinks, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open, cursorOperatingSystem: providersessions.OperatingSystem(runtime.GOOS)},
		{name: "Cursor symlink resolver", files: platformfilesystem.Local{}, home: resolveHome, codexWalk: filepath.WalkDir, codexSymlinks: filepath.EvalSymlinks, cursorWalk: filepath.WalkDir, cursorDatabase: sql.Open, cursorOperatingSystem: providersessions.OperatingSystem(runtime.GOOS)},
		{name: "Cursor database opener", files: platformfilesystem.Local{}, home: resolveHome, codexWalk: filepath.WalkDir, codexSymlinks: filepath.EvalSymlinks, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorOperatingSystem: providersessions.OperatingSystem(runtime.GOOS)},
		{name: "operating system", files: platformfilesystem.Local{}, home: resolveHome, codexWalk: filepath.WalkDir, codexSymlinks: filepath.EvalSymlinks, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := providersessionsservice.New(test.files, test.home, test.codexWalk, test.codexSymlinks, test.cursorWalk, test.cursorSymlinks, test.cursorDatabase, test.cursorOperatingSystem)
			if err == nil {
				t.Fatalf("New() error = nil, want missing %s dependency", test.name)
			}
		})
	}
}

func TestNewConstructsServiceWithValidDependencies(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".cursor", "chats"), 0o755); err != nil {
		t.Fatalf("mkdir cursor chats: %v", err)
	}
	resolveHome := providersessions.ResolveHomeDirectory(func() (string, error) { return home, nil })
	svc, err := providersessionsservice.New(
		platformfilesystem.Local{},
		resolveHome,
		filepath.WalkDir,
		filepath.EvalSymlinks,
		filepath.WalkDir,
		filepath.EvalSymlinks,
		sql.Open,
		providersessions.OperatingSystem(runtime.GOOS),
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
	_, err := providersessionsservice.NewForRoots(
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
INSERT INTO meta (key, value) VALUES ('0', '{"agentId":"cursor-root-inspect","createdAt":1000}');
`); err != nil {
		t.Fatalf("create Cursor fixture: %v", err)
	}
	return root, sessionID
}

func newServiceForRoots(t *testing.T, codexRoot, cursorRoot string) providersessions.Service {
	t.Helper()
	service, err := providersessionsservice.NewForRoots(
		platformfilesystem.Local{},
		providersessions.CodexWalkDirectory(filepath.WalkDir),
		providersessions.CodexResolveSymlinks(filepath.EvalSymlinks),
		providersessions.CursorWalkDirectory(filepath.WalkDir),
		providersessions.CursorResolveSymlinks(filepath.EvalSymlinks),
		providersessions.CursorOpenSQLDatabase(sql.Open),
		codexRoot,
		cursorRoot,
	)
	if err != nil {
		t.Fatalf("NewForRoots: %v", err)
	}
	return service
}
