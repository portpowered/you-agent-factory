package runtimetests

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	"github.com/portpowered/infinite-you/internal/retiredsurfaceguard"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	. "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/wire"
)

var settledScopedNamedFactoryPaths = retiredsurfaceguard.SettledScopedNamedFactoryPaths()

func TestRetiredEncodedPathResolution_ProductionMappingUsesHierarchicalLayout(t *testing.T) {
	rootDir := t.TempDir()

	for _, name := range settledScopedNamedFactoryPaths {
		name := name
		t.Run(name, func(t *testing.T) {
			mappedDir, err := factorydefinitions.MapDir(rootDir, name)
			if err != nil {
				t.Fatalf("MapNamedFactoryDir(%q): %v", name, err)
			}
			if strings.Contains(mappedDir, "%2F") {
				t.Fatalf("MapNamedFactoryDir(%q) = %q, must not use percent-encoded scoped leaf names", name, mappedDir)
			}

			segments, err := factorydefinitions.PathSegments(name)
			if err != nil {
				t.Fatalf("NamedFactoryPathSegments(%q): %v", name, err)
			}
			wantDir := filepath.Join(append([]string{rootDir}, segments...)...)
			if mappedDir != wantDir {
				t.Fatalf("MapNamedFactoryDir(%q) = %q, want hierarchical %q", name, mappedDir, wantDir)
			}
			if mappedDir == legacyEncodedFactoryDir(t, rootDir, name) {
				t.Fatalf("MapNamedFactoryDir(%q) must not resolve to legacy encoded layout %q", name, mappedDir)
			}
		})
	}
}

func TestRetiredEncodedPathResolution_ResolveMissLeavesEncodedSiblingUntouched(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()

	encodedDir := seedLegacyEncodedGoalFactory(t, globalRoot)
	beforeSnapshot := snapshotDirectoryContents(t, encodedDir)

	_, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "@you/goal")
	if !errors.Is(err, ErrNamedFactoryNotFound) {
		t.Fatalf("ResolveNamedFactoryAcrossRoots(@you/goal) error = %v, want ErrNamedFactoryNotFound", err)
	}
	assertDirectorySnapshotUnchanged(t, encodedDir, beforeSnapshot)

	entries, err := namedFactoryCatalog.ListNamedFactories(globalRoot)
	if err != nil {
		t.Fatalf("ListNamedFactories: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %#v, want encoded legacy leaf ignored", entries)
	}
	assertDirectorySnapshotUnchanged(t, encodedDir, beforeSnapshot)
}

func TestRetiredEncodedPathResolution_PersistLeavesEncodedSiblingUntouched(t *testing.T) {
	rootDir := t.TempDir()

	encodedDir := seedLegacyEncodedGoalFactory(t, rootDir)
	beforeSnapshot := snapshotDirectoryContents(t, encodedDir)

	factoryDir, err := factorydefinitioncomposition.PersistNamedFactory(rootDir, "@you/goal", namedFactoryPayload(t, "hierarchical-goal-persist"), ownerFactoryDefinitionValidator())
	if err != nil {
		t.Fatalf("PersistNamedFactory(@you/goal): %v", err)
	}

	wantDir := filepath.Join(rootDir, "@you", "goal")
	if factoryDir != wantDir {
		t.Fatalf("factory dir = %q, want hierarchical %q", factoryDir, wantDir)
	}
	if strings.Contains(factoryDir, "%2F") {
		t.Fatalf("factory dir = %q, must not use percent-encoded scoped leaf names", factoryDir)
	}
	assertDirectorySnapshotUnchanged(t, encodedDir, beforeSnapshot)
}

func TestRetiredEncodedPathResolution_ResolveUsesHierarchicalFactoryNotEncodedSibling(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()

	encodedDir := seedLegacyEncodedGoalFactory(t, globalRoot)
	encodedBefore := snapshotDirectoryContents(t, encodedDir)
	hierarchicalDir := persistRuntimeNamedFactory(t, globalRoot, "@you/goal", "hierarchical-goal")

	resolution, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "@you/goal")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots(@you/goal): %v", err)
	}
	if resolution.FactoryDir != hierarchicalDir {
		t.Fatalf("resolution factory dir = %q, want hierarchical %q", resolution.FactoryDir, hierarchicalDir)
	}
	if resolution.FactoryDir == encodedDir {
		t.Fatalf("resolution must not dual-read legacy encoded sibling %q", encodedDir)
	}
	if strings.Contains(resolution.FactoryDir, "%2F") {
		t.Fatalf("resolution factory dir = %q, must not use percent-encoded scoped leaf names", resolution.FactoryDir)
	}

	loaded, err := LoadRuntimeConfigFromFactoryDir(resolution.FactoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(hierarchical): %v", err)
	}
	if loaded.FactoryConfig().Project != "hierarchical-goal" {
		t.Fatalf("resolved project = %q, want hierarchical-goal", loaded.FactoryConfig().Project)
	}
	assertDirectorySnapshotUnchanged(t, encodedDir, encodedBefore)
}

func TestRetiredEncodedPathResolution_MaterializeLeavesEncodedSiblingUntouched(t *testing.T) {
	homeDir := t.TempDir()
	namedFactoriesRoot := factorydefinitions.NamedFactoriesRoot(homeDir)
	if err := os.MkdirAll(namedFactoriesRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(namedFactoriesRoot): %v", err)
	}

	encodedDir := seedLegacyEncodedGoalFactory(t, namedFactoriesRoot)
	beforeSnapshot := snapshotDirectoryContents(t, encodedDir)

	installer := distributionwire.NewPackagedFactoryInstaller(ownerFactoryDefinitionPersistence(), platformfilesystem.Local{})
	result, err := installer.EnsurePackagedFactories(t.Context(), namedFactoriesRoot, publishedPackagedDefinitions(t))
	if err != nil {
		t.Fatalf("system initialization: %v", err)
	}
	assertDirectorySnapshotUnchanged(t, encodedDir, beforeSnapshot)

	wantGoalDir, err := factorydefinitions.MapDir(namedFactoriesRoot, "@you/goal")
	if err != nil {
		t.Fatalf("MapNamedFactoryDir(@you/goal): %v", err)
	}
	if wantGoalDir == encodedDir {
		t.Fatalf("materialize must not target legacy encoded dir %q", encodedDir)
	}
	if strings.Contains(wantGoalDir, "%2F") {
		t.Fatalf("materialize target dir = %q, must not use percent-encoded scoped leaf names", wantGoalDir)
	}

	var goalResult *factorydefinitions.PackagedFactoryInstallResult
	for i := range result {
		if result[i].Name == "@you/goal" {
			goalResult = &result[i]
			break
		}
	}
	if goalResult == nil {
		t.Fatal("expected @you/goal in packaged factory results")
	}
	if goalResult.FactoryDir != wantGoalDir {
		t.Fatalf("@you/goal factory dir = %q, want hierarchical %q", goalResult.FactoryDir, wantGoalDir)
	}
	assertDirectorySnapshotUnchanged(t, encodedDir, beforeSnapshot)
}

func TestRetiredEncodedPathResolution_EncodedDirectoryUntouchedFixture(t *testing.T) {
	rootDir := t.TempDir()
	encodedDir := seedLegacyEncodedGoalFactory(t, rootDir)
	beforeSnapshot := snapshotDirectoryContents(t, encodedDir)

	t.Run("resolve miss", func(t *testing.T) {
		_, err := ResolveNamedFactoryAcrossRoots(t.TempDir(), rootDir, "@you/goal")
		if !errors.Is(err, ErrNamedFactoryNotFound) {
			t.Fatalf("ResolveNamedFactoryAcrossRoots(@you/goal) error = %v, want ErrNamedFactoryNotFound", err)
		}
		assertDirectorySnapshotUnchanged(t, encodedDir, beforeSnapshot)
	})

	t.Run("persist hierarchical sibling", func(t *testing.T) {
		factoryDir, err := factorydefinitioncomposition.PersistNamedFactory(rootDir, "@you/goal", namedFactoryPayload(t, "fixture-goal"), ownerFactoryDefinitionValidator())
		if err != nil {
			t.Fatalf("PersistNamedFactory(@you/goal): %v", err)
		}
		wantDir := filepath.Join(rootDir, "@you", "goal")
		if factoryDir != wantDir {
			t.Fatalf("factory dir = %q, want hierarchical %q", factoryDir, wantDir)
		}
		assertDirectorySnapshotUnchanged(t, encodedDir, beforeSnapshot)
	})

	t.Run("list ignores encoded leaf", func(t *testing.T) {
		entries, err := namedFactoryCatalog.ListNamedFactories(rootDir)
		if err != nil {
			t.Fatalf("ListNamedFactories: %v", err)
		}
		for _, entry := range entries {
			if entry.FactoryDir == encodedDir {
				t.Fatalf("ListNamedFactories must ignore legacy encoded leaf, got %#v", entry)
			}
			if strings.Contains(entry.FactoryDir, "%2F") {
				t.Fatalf("listed factory dir %q must not use percent-encoded scoped leaf names", entry.FactoryDir)
			}
		}
		assertDirectorySnapshotUnchanged(t, encodedDir, beforeSnapshot)
	})
}

func TestRetiredSurfaceResidue_ReadCurrentFactoryPointerRejectsLegacyEncodedSegment(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	encodedSegment := legacyEncodedNamedFactorySegment("@you/goal")
	pointerPath := filepath.Join(rootDir, factorydefinitions.CurrentFactoryPointerFile)
	if err := os.WriteFile(pointerPath, []byte(encodedSegment+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(current pointer): %v", err)
	}

	if _, err := factorydefinitioncomposition.NamedPaths().ReadCurrentPointer(rootDir); err == nil {
		t.Fatal("expected legacy encoded current-factory pointer to be rejected")
	}
}

func TestRetiredSurfaceResidue_ReadCurrentFactoryPointerAcceptsCanonicalScopedName(t *testing.T) {
	rootDir := t.TempDir()
	if _, err := factorydefinitioncomposition.PersistNamedFactory(rootDir, "@you/goal", namedFactoryPayload(t, "goal"), ownerFactoryDefinitionValidator()); err != nil {
		t.Fatalf("PersistNamedFactory(@you/goal): %v", err)
	}
	if err := factorydefinitioncomposition.NamedPaths().WriteCurrentPointer(rootDir, "@you/goal"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}

	current, err := factorydefinitioncomposition.NamedPaths().ReadCurrentPointer(rootDir)
	if err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	}
	if current != "@you/goal" {
		t.Fatalf("current = %q, want canonical scoped display name @you/goal", current)
	}
	if strings.Contains(current, "%2F") {
		t.Fatalf("current = %q, must not use percent-encoded layout segments", current)
	}
}

func TestRetiredSurfaceResidue_ConfigInitLeavesLegacyEncodedSiblingUntouched(t *testing.T) {
	homeDir := t.TempDir()
	namedFactoriesRoot := filepath.Join(homeDir, ".you-agent-factory", "you-agent-factories")
	encodedDir := seedLegacyEncodedGoalFactoryForResidueTest(t, namedFactoriesRoot)
	beforeSnapshot := snapshotDirectoryContents(t, encodedDir)

	installer := distributionwire.NewPackagedFactoryInstaller(ownerFactoryDefinitionPersistence(), platformfilesystem.Local{})
	result, err := installer.EnsurePackagedFactories(t.Context(), namedFactoriesRoot, publishedPackagedDefinitions(t))
	if err != nil {
		t.Fatalf("system initialization: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("expected packaged factories to be ensured during init")
	}

	assertDirectorySnapshotUnchanged(t, encodedDir, beforeSnapshot)
	for _, factory := range result {
		if factory.Name == "@you/goal" && strings.Contains(factory.FactoryDir, "%2F") {
			t.Fatalf("packaged factory dir = %q, must not use percent-encoded scoped leaf names", factory.FactoryDir)
		}
	}
}

func publishedPackagedDefinitions(t *testing.T) []factorydefinitions.PackagedDefinition {
	t.Helper()
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog: %v", err)
	}
	return catalog.All()
}

func legacyEncodedFactoryDir(t *testing.T, factoriesRoot, scopedName string) string {
	t.Helper()

	segment := legacyEncodedNamedFactorySegment(scopedName)
	return filepath.Join(factoriesRoot, segment)
}

func seedLegacyEncodedGoalFactoryForResidueTest(t *testing.T, factoriesRoot string) string {
	t.Helper()

	segment := legacyEncodedNamedFactorySegment("@you/goal")
	encodedDir := filepath.Join(factoriesRoot, segment)
	if err := os.MkdirAll(encodedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(encoded legacy dir): %v", err)
	}
	markerPath := filepath.Join(encodedDir, legacyEncodedGoalMarkerFile)
	if err := os.WriteFile(markerPath, []byte("do-not-touch-legacy-encoded-goal\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(marker): %v", err)
	}
	return encodedDir
}
