package service_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"
	internalservice "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/service"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestCodexRequiredEffectsAreRejectedAtOwnerConstruction(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, missing := range []string{"filesystem", "Codex directory walker", "Codex symlink resolver"} {
		t.Run(missing, func(t *testing.T) {
			var files providersessionsinternal.FileSystem = platformfilesystem.Local{}
			walk := providersessionsinternal.CodexWalkDirectory(filepath.WalkDir)
			resolve := providersessionsinternal.CodexResolveSymlinks(filepath.EvalSymlinks)
			switch missing {
			case "filesystem":
				files = nil
			case "Codex directory walker":
				walk = nil
			case "Codex symlink resolver":
				resolve = nil
			}
			constructors := []func() (providersessions.Service, error){
				func() (providersessions.Service, error) {
					return internalservice.NewForRoots(files, walk, resolve, filepath.WalkDir, filepath.EvalSymlinks, sql.Open, root, root)
				},
				func() (providersessions.Service, error) {
					return internalservice.New(files, func() (string, error) { t.Fatal("home lookup before required-effect rejection"); return "", nil }, walk, resolve, filepath.WalkDir, filepath.EvalSymlinks, sql.Open, providersessionsinternal.OperatingSystem(runtime.GOOS))
				},
			}
			for _, construct := range constructors {
				service, err := construct()
				if service != nil || err == nil || err.Error() != "provider-session "+missing+" is required" {
					t.Fatalf("construction = %#v, %v; want nil service and exact missing-effect error", service, err)
				}
			}
		})
	}
}

func TestHomeFailurePreservesCauseAndNilOwnerService(t *testing.T) {
	t.Parallel()
	cause := errors.New("controlled home failure")
	service, err := internalservice.New(platformfilesystem.Local{}, func() (string, error) { return "", cause }, filepath.WalkDir, filepath.EvalSymlinks, filepath.WalkDir, filepath.EvalSymlinks, sql.Open, providersessionsinternal.OperatingSystem(runtime.GOOS))
	if service != nil || !errors.Is(err, cause) || err.Error() != "home directory: controlled home failure" {
		t.Fatalf("New = %#v, %v; want nil service and wrapped home cause", service, err)
	}
}

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
