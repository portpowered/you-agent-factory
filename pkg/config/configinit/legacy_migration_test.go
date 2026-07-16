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

func TestInit_MigratesInvalidLegacyFactoryBeforePackagedInstallation(t *testing.T) {
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
	invalidConfig := []byte("{ customer edited but temporarily invalid\n")
	if err := os.WriteFile(filepath.Join(legacyDir, "factory.json"), invalidConfig, 0o600); err != nil {
		t.Fatalf("WriteFile(invalid legacy factory): %v", err)
	}
	markerPath := filepath.Join(legacyDir, "customer-edit.txt")
	if err := os.WriteFile(markerPath, []byte("must not be replaced\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(customer marker): %v", err)
	}
	legacyBefore := snapshotDirectoryContents(t, legacyDir)

	_, err = Init(homeDir)
	if err == nil {
		t.Fatal("expected invalid migrated factory error")
	}
	canonicalDir, mapErr := factoryconfig.MapNamedFactoryDir(defaultpaths.NamedFactoriesRoot(homeDir), definition.Name)
	if mapErr != nil {
		t.Fatalf("MapNamedFactoryDir(canonical): %v", mapErr)
	}
	for _, want := range []string{"install packaged factory", definition.Name, canonicalDir, "existing target", "invalid"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Init() error = %q, want substring %q", err, want)
		}
	}
	if _, statErr := os.Stat(legacyDir); !os.IsNotExist(statErr) {
		t.Fatalf("invalid legacy factory still exists after lossless migration: %v", statErr)
	}
	assertDirectorySnapshotUnchanged(t, canonicalDir, legacyBefore)
}

func TestInit_RejectsLegacyRootThatIsNotADirectory(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	legacyRoot := defaultpaths.LegacyNamedFactoriesRoot(homeDir)
	if err := os.MkdirAll(filepath.Dir(legacyRoot), 0o755); err != nil {
		t.Fatalf("MkdirAll(legacy parent): %v", err)
	}
	if err := os.WriteFile(legacyRoot, []byte("not a factory directory\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(legacy root): %v", err)
	}

	_, err := Init(homeDir)
	if err == nil {
		t.Fatal("expected legacy root validation error")
	}
	if !strings.Contains(err.Error(), "path is not a directory") {
		t.Fatalf("Init() error = %q, want non-directory guidance", err)
	}
}

func TestInit_ReportsInvalidLegacyFactoryInventory(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	legacyRoot := defaultpaths.LegacyNamedFactoriesRoot(homeDir)
	if err := os.MkdirAll(legacyRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(legacy root): %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, ".current-factory"), []byte("../outside-root\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(legacy current pointer): %v", err)
	}

	_, err := Init(homeDir)
	if err == nil {
		t.Fatal("expected legacy inventory validation error")
	}
	if !strings.Contains(err.Error(), "list legacy global factories") {
		t.Fatalf("Init() error = %q, want legacy inventory guidance", err)
	}
}

func TestInit_ReportsCanonicalDestinationInspectionFailure(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	definition, ok := factorypackages.Lookup("@you/goal")
	if !ok {
		t.Fatal("expected @you/goal packaged definition")
	}
	if _, err := factoryconfig.PersistNamedFactory(defaultpaths.LegacyNamedFactoriesRoot(homeDir), definition.Name, definition.JSON); err != nil {
		t.Fatalf("PersistNamedFactory(legacy): %v", err)
	}
	canonicalRoot := defaultpaths.NamedFactoriesRoot(homeDir)
	if err := os.WriteFile(canonicalRoot, []byte("not a factory root directory\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(canonical root): %v", err)
	}

	_, err := Init(homeDir)
	if err == nil {
		t.Fatal("expected canonical destination inspection error")
	}
	if !strings.Contains(err.Error(), canonicalRoot) || !strings.Contains(err.Error(), "legacy factory") {
		t.Fatalf("Init() error = %q, want actionable canonical-root migration guidance", err)
	}
}
