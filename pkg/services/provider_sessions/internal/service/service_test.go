package service_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"
	internalservice "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/service"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestNewForRootsSatisfiesPublishedProviderSessionsService(t *testing.T) {
	t.Parallel()

	codexRoot := writeCodexSessionFixture(t, "internal-root-1")
	service, err := internalservice.NewForRoots(
		platformfilesystem.Local{},
		providersessionsinternal.CodexWalkDirectory(filepath.WalkDir),
		providersessionsinternal.CodexResolveSymlinks(filepath.EvalSymlinks),
		providersessionsinternal.CursorWalkDirectory(filepath.WalkDir),
		providersessionsinternal.CursorResolveSymlinks(filepath.EvalSymlinks),
		providersessionsinternal.CursorOpenSQLDatabase(sql.Open),
		codexRoot,
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("NewForRoots() error = %v", err)
	}
	var root providersessions.Service = service

	ref := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providersessions.SessionIDKind,
		ID:       "internal-root-1",
	}
	result, err := root.Inspect(providersessions.InspectRequest{Session: ref})
	if err != nil {
		t.Fatalf("Inspect() = %v", err)
	}
	if result.Session != ref {
		t.Fatalf("InspectResult.Session = %#v, want %#v", result.Session, ref)
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
