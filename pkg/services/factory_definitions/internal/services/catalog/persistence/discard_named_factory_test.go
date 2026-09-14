package persistence_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	catalogpersistence "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/persistence"
)

type namedFactoryDiscarder interface {
	DiscardNamedFactory(string, string) error
}

func TestDiscardNamedFactoryRemovesCandidateAndIsIdempotent(t *testing.T) {
	t.Parallel()

	service := replacementService(nil)
	discarder, ok := service.(namedFactoryDiscarder)
	if !ok {
		t.Fatal("persistence service does not expose named Factory discard capability")
	}
	rootDir := t.TempDir()
	factoryDir := filepath.Join(rootDir, "@you", "goal")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(candidate): %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile), []byte(`{"name":"@you/goal"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "temporary.txt"), []byte("candidate"), 0o644); err != nil {
		t.Fatalf("WriteFile(candidate artifact): %v", err)
	}

	if err := discarder.DiscardNamedFactory(rootDir, "@you/goal"); err != nil {
		t.Fatalf("DiscardNamedFactory: %v", err)
	}
	if _, err := os.Stat(factoryDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("candidate stat error = %v, want not-exist", err)
	}
	if err := discarder.DiscardNamedFactory(rootDir, "@you/goal"); err != nil {
		t.Fatalf("DiscardNamedFactory(missing): %v", err)
	}
}

func TestDiscardNamedFactoryRejectsInvalidRootAndName(t *testing.T) {
	t.Parallel()

	discarder, ok := replacementService(nil).(namedFactoryDiscarder)
	if !ok {
		t.Fatal("persistence service does not expose named Factory discard capability")
	}
	if err := discarder.DiscardNamedFactory(" ", "alpha"); err == nil || !strings.Contains(err.Error(), "factory root is required") {
		t.Fatalf("DiscardNamedFactory(empty root) = %v, want required-root error", err)
	}
	if err := discarder.DiscardNamedFactory(t.TempDir(), "../escape"); err == nil || !strings.Contains(err.Error(), "invalid named factory name") {
		t.Fatalf("DiscardNamedFactory(invalid name) = %v, want invalid-name error", err)
	}
}

func TestDiscardNamedFactoryReportsFilesystemFailure(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("remove unavailable")
	service, err := catalogpersistence.New(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		removeAllErrorFileSystem{err: wantErr},
		persistenceTestNamedPaths.RequireDefinitionDir,
		nil,
	)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	discarder, ok := service.(namedFactoryDiscarder)
	if !ok {
		t.Fatal("persistence service does not expose named Factory discard capability")
	}
	if err := discarder.DiscardNamedFactory(t.TempDir(), "alpha"); !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "discard named Factory") {
		t.Fatalf("DiscardNamedFactory(filesystem failure) = %v, want wrapped removal error", err)
	}
}

func TestPersistenceExposesValidationAndPackagedPreparationErrors(t *testing.T) {
	t.Parallel()

	service := replacementService(nil)
	if err := service.ValidateFactoryLayout(t.TempDir()); err != nil {
		t.Fatalf("ValidateFactoryLayout(configured): %v", err)
	}
	packaged, ok := service.(factorydefinitions.PackagedFactoryPersistence)
	if !ok {
		t.Fatal("persistence service does not expose packaged preparation capability")
	}
	if _, err := packaged.PreparePackagedFactoryLayout(context.Background(), "alpha", []byte(`{}`)); err == nil || !strings.Contains(err.Error(), "layout preparer is required") {
		t.Fatalf("PreparePackagedFactoryLayout(unconfigured) = %v, want required-preparer error", err)
	}
}

type removeAllErrorFileSystem struct {
	platformfilesystem.Local
	err error
}

func (f removeAllErrorFileSystem) RemoveAll(string) error { return f.err }
