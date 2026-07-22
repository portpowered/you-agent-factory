package persistence_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorypersistence "github.com/portpowered/infinite-you/pkg/services/factory_definitions/persistence"
)

type recordingDirectoryStore struct {
	wantErr   error
	parentDir string
	targetDir string
	staging   string
}

func (s *recordingDirectoryStore) Commit(parentDir, targetDir, stagingDir string) (string, error) {
	s.parentDir = parentDir
	s.targetDir = targetDir
	s.staging = stagingDir
	return "", s.wantErr
}

func (*recordingDirectoryStore) Restore(string, string) {}

func TestReplaceFactoryLayoutRequiresInjectedDirectoryReplacementStore(t *testing.T) {
	t.Parallel()

	targetDir := seedReplacementTarget(t)
	service := replacementService(nil)
	_, err := service.ReplaceFactoryLayout(targetDir, &factorydefinitions.PreparedFactoryLayoutPayload{})
	if err == nil || err.Error() != "directory replacement store is required" {
		t.Fatalf("ReplaceFactoryLayout() error = %v, want required dependency", err)
	}
}

func TestNewRejectsMissingPersistenceFileSystem(t *testing.T) {
	t.Parallel()

	if _, err := factorypersistence.New(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	); err == nil || err.Error() != "Factory Definitions persistence filesystem is required" {
		t.Fatalf("New() error = %v, want required persistence filesystem", err)
	}
}

func TestReplaceFactoryLayoutDelegatesMechanicsAndRetainsFactoryDiagnostics(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("replace unavailable")
	store := &recordingDirectoryStore{wantErr: wantErr}
	targetDir := seedReplacementTarget(t)
	service := replacementService(store)
	_, err := service.ReplaceFactoryLayout(targetDir, &factorydefinitions.PreparedFactoryLayoutPayload{})
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), `commit factory "alpha"`) {
		t.Fatalf("ReplaceFactoryLayout() error = %v, want Factory-owned context wrapping store error", err)
	}
	if store.parentDir != filepath.Dir(targetDir) || store.targetDir != targetDir || store.staging == "" {
		t.Fatalf("store call = parent %q target %q staging %q", store.parentDir, store.targetDir, store.staging)
	}
}

func replacementService(store factorydefinitions.DirectoryReplacementStore) factorydefinitions.Persistence {
	persistence, err := factorypersistence.New(
		nil,
		nil,
		nil,
		func(targetDir string, _ *factorydefinitions.PreparedFactoryLayoutPayload, _ string) error {
			return os.WriteFile(filepath.Join(targetDir, factorydefinitions.FactoryConfigFile), []byte(`{"name":"alpha"}`), 0o644)
		},
		func(string) error { return nil },
		nil,
		nil,
		nil,
		platformfilesystem.Local{},
		persistenceTestNamedPaths.RequireDefinitionDir,
		store,
	)
	if err != nil {
		panic(err)
	}
	return persistence
}

func seedReplacementTarget(t *testing.T) string {
	t.Helper()
	targetDir := filepath.Join(t.TempDir(), "alpha")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, factorydefinitions.FactoryConfigFile), []byte(`{"name":"alpha"}`), 0o644); err != nil {
		t.Fatalf("write target definition: %v", err)
	}
	return targetDir
}
