package contract

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionsservice "github.com/portpowered/infinite-you/pkg/services/provider_sessions/service"
)

func TestProviderSessionsRoot_InspectAndProjectThroughProductionService(t *testing.T) {
	root := writeCodexSessionFixture(t, "session-functional-root-1")
	svc := newProviderSessionsService(t, root)

	ref := providersessions.SessionRef{
		Provider: providersessions.ProviderCodex,
		Kind:     providersessions.SessionIDKind,
		ID:       "session-functional-root-1",
	}

	inspected, err := svc.Inspect(providersessions.InspectRequest{Session: ref})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if inspected.Session != ref {
		t.Fatalf("InspectResult.Session = %#v, want %#v", inspected.Session, ref)
	}

	projected, err := svc.Project(providersessions.ProjectRequest{Session: ref})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if projected.Session != ref {
		t.Fatalf("ProjectResult.Session = %#v, want %#v", projected.Session, ref)
	}
	if projected.Detail.ProviderSession.ID != ref.ID {
		t.Fatalf("ProjectResult.Detail.ProviderSession.ID = %q, want %q", projected.Detail.ProviderSession.ID, ref.ID)
	}

	if _, err := svc.Inspect(providersessions.InspectRequest{Session: providersessions.SessionRef{
		Provider: providersessions.ProviderCodex,
		Kind:     providersessions.SessionIDKind,
		ID:       "",
	}}); !errors.Is(err, providersessions.ErrInvalidIdentifier) {
		t.Fatalf("Inspect empty id = %v, want ErrInvalidIdentifier", err)
	}
	if _, err := svc.Project(providersessions.ProjectRequest{Session: providersessions.SessionRef{
		Provider: providersessions.ProviderCodex,
		Kind:     providersessions.SessionIDKind,
		ID:       "missing-session",
	}}); !errors.Is(err, providersessions.ErrSessionNotFound) {
		t.Fatalf("Project missing = %v, want ErrSessionNotFound", err)
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

func newProviderSessionsService(t *testing.T, codexRoot string) providersessions.Service {
	t.Helper()
	svc, err := providersessionsservice.NewForRoots(
		platformfilesystem.Local{},
		providersessions.CodexWalkDirectory(filepath.WalkDir),
		providersessions.CodexResolveSymlinks(filepath.EvalSymlinks),
		providersessions.CursorWalkDirectory(filepath.WalkDir),
		providersessions.CursorResolveSymlinks(filepath.EvalSymlinks),
		providersessions.CursorOpenSQLDatabase(sql.Open),
		codexRoot,
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("NewForRoots: %v", err)
	}
	return svc
}
