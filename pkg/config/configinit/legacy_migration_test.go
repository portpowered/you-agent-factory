package configinit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	factorypackages "github.com/portpowered/infinite-you/pkg/factory/packages"
)

func TestInit_MigratesLegacyFactoriesWithoutChangingEditableLayout(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	legacyRoot := defaultpaths.LegacyNamedFactoriesRoot(homeDir)
	definition, ok := factorypackages.Lookup("@you/goal")
	if !ok {
		t.Fatal("expected @you/goal packaged definition")
	}
	legacyDir, err := factoryconfig.PersistNamedFactory(legacyRoot, definition.Name, definition.JSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory(legacy): %v", err)
	}
	markerPath := filepath.Join(legacyDir, "customer-edit.txt")
	if err := os.WriteFile(markerPath, []byte("preserve this customer edit\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(customer edit): %v", err)
	}
	beforeSnapshot := snapshotDirectoryContents(t, legacyDir)

	if _, err := Init(homeDir); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	canonicalDir, err := factoryconfig.MapNamedFactoryDir(defaultpaths.NamedFactoriesRoot(homeDir), definition.Name)
	if err != nil {
		t.Fatalf("MapNamedFactoryDir(canonical): %v", err)
	}
	if _, err := os.Stat(legacyDir); !os.IsNotExist(err) {
		t.Fatalf("legacy factory still exists after migration: %v", err)
	}
	assertDirectorySnapshotUnchanged(t, canonicalDir, beforeSnapshot)
	if _, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(canonicalDir, nil); err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(migrated): %v", err)
	}
	resolution, err := factoryconfig.ResolveNamedFactoryAcrossRoots(t.TempDir(), defaultpaths.NamedFactoriesRoot(homeDir), definition.Name)
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots(migrated): %v", err)
	}
	if resolution.FactoryDir != canonicalDir {
		t.Fatalf("resolved migrated factory dir = %q, want %q", resolution.FactoryDir, canonicalDir)
	}
}

func TestInit_LegacyFactoryConflictPreservesBothCopies(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	definition, ok := factorypackages.Lookup("@you/goal")
	if !ok {
		t.Fatal("expected @you/goal packaged definition")
	}
	legacyDir, err := factoryconfig.PersistNamedFactory(defaultpaths.LegacyNamedFactoriesRoot(homeDir), definition.Name, definition.JSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory(legacy): %v", err)
	}
	canonicalDir, err := factoryconfig.PersistNamedFactory(defaultpaths.NamedFactoriesRoot(homeDir), definition.Name, definition.JSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory(canonical): %v", err)
	}
	legacyBefore := snapshotDirectoryContents(t, legacyDir)
	canonicalBefore := snapshotDirectoryContents(t, canonicalDir)

	_, err = Init(homeDir)
	if err == nil {
		t.Fatal("expected legacy migration conflict")
	}
	for _, want := range []string{"migrate legacy factory", definition.Name, legacyDir, canonicalDir, "without overwriting"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want substring %q", err, want)
		}
	}
	assertDirectorySnapshotUnchanged(t, legacyDir, legacyBefore)
	assertDirectorySnapshotUnchanged(t, canonicalDir, canonicalBefore)
}

func TestInit_LegacyMigrationIsIdempotentAndPreservesCanonicalEdits(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	definition, ok := factorypackages.Lookup("@you/goal")
	if !ok {
		t.Fatal("expected @you/goal packaged definition")
	}
	if _, err := factoryconfig.PersistNamedFactory(defaultpaths.LegacyNamedFactoriesRoot(homeDir), definition.Name, definition.JSON); err != nil {
		t.Fatalf("PersistNamedFactory(legacy): %v", err)
	}
	if _, err := Init(homeDir); err != nil {
		t.Fatalf("first Init(): %v", err)
	}
	canonicalDir, err := factoryconfig.MapNamedFactoryDir(defaultpaths.NamedFactoriesRoot(homeDir), definition.Name)
	if err != nil {
		t.Fatalf("MapNamedFactoryDir(canonical): %v", err)
	}
	editPath := filepath.Join(canonicalDir, "customer-edit.txt")
	if err := os.WriteFile(editPath, []byte("keep on repeat init\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(customer edit): %v", err)
	}
	beforeSnapshot := snapshotDirectoryContents(t, canonicalDir)

	if _, err := Init(homeDir); err != nil {
		t.Fatalf("second Init(): %v", err)
	}
	assertDirectorySnapshotUnchanged(t, canonicalDir, beforeSnapshot)
}
