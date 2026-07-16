package runtimetests

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/retiredsurfaceguard"
	. "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/configinit"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/config/namedfactorypath"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

var settledScopedNamedFactoryPaths = retiredsurfaceguard.SettledScopedNamedFactoryPaths()

func TestRetiredEncodedPathResolution_ProductionMappingUsesHierarchicalLayout(t *testing.T) {
	rootDir := t.TempDir()

	for _, name := range settledScopedNamedFactoryPaths {
		name := name
		t.Run(name, func(t *testing.T) {
			mappedDir, err := MapNamedFactoryDir(rootDir, name)
			if err != nil {
				t.Fatalf("MapNamedFactoryDir(%q): %v", name, err)
			}
			if strings.Contains(mappedDir, "%2F") {
				t.Fatalf("MapNamedFactoryDir(%q) = %q, must not use percent-encoded scoped leaf names", name, mappedDir)
			}

			segments, err := NamedFactoryPathSegments(name)
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

	entries, err := ListNamedFactories(globalRoot)
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

	factoryDir, err := PersistNamedFactory(rootDir, "@you/goal", namedFactoryPayload(t, "hierarchical-goal-persist"))
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
	namedFactoriesRoot := defaultpaths.NamedFactoriesRoot(homeDir)
	if err := os.MkdirAll(namedFactoriesRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(namedFactoriesRoot): %v", err)
	}

	encodedDir := seedLegacyEncodedGoalFactory(t, namedFactoriesRoot)
	beforeSnapshot := snapshotDirectoryContents(t, encodedDir)

	result, err := configinit.Init(homeDir)
	if err != nil {
		t.Fatalf("configinit.Init(): %v", err)
	}
	assertDirectorySnapshotUnchanged(t, encodedDir, beforeSnapshot)

	wantGoalDir, err := MapNamedFactoryDir(namedFactoriesRoot, "@you/goal")
	if err != nil {
		t.Fatalf("MapNamedFactoryDir(@you/goal): %v", err)
	}
	if wantGoalDir == encodedDir {
		t.Fatalf("materialize must not target legacy encoded dir %q", encodedDir)
	}
	if strings.Contains(wantGoalDir, "%2F") {
		t.Fatalf("materialize target dir = %q, must not use percent-encoded scoped leaf names", wantGoalDir)
	}

	var goalResult *configinit.PackagedFactoryResult
	for i := range result.PackagedFactories {
		if result.PackagedFactories[i].Name == "@you/goal" {
			goalResult = &result.PackagedFactories[i]
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
		factoryDir, err := PersistNamedFactory(rootDir, "@you/goal", namedFactoryPayload(t, "fixture-goal"))
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
		entries, err := ListNamedFactories(rootDir)
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

	encodedSegment, err := namedfactorypath.LegacyLayoutSegment("@you/goal")
	if err != nil {
		t.Fatalf("LegacyLayoutSegment(@you/goal): %v", err)
	}
	pointerPath := filepath.Join(rootDir, interfaces.CurrentFactoryPointerFile)
	if err := os.WriteFile(pointerPath, []byte(encodedSegment+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(current pointer): %v", err)
	}

	if _, err := ReadCurrentFactoryPointer(rootDir); err == nil {
		t.Fatal("expected legacy encoded current-factory pointer to be rejected")
	}
}

func TestRetiredSurfaceResidue_ReadCurrentFactoryPointerAcceptsCanonicalScopedName(t *testing.T) {
	rootDir := t.TempDir()
	if _, err := PersistNamedFactory(rootDir, "@you/goal", namedFactoryPayload(t, "goal")); err != nil {
		t.Fatalf("PersistNamedFactory(@you/goal): %v", err)
	}
	if err := WriteCurrentFactoryPointer(rootDir, "@you/goal"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}

	current, err := ReadCurrentFactoryPointer(rootDir)
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

	result, err := configinit.Init(homeDir)
	if err != nil {
		t.Fatalf("configinit.Init: %v", err)
	}
	if len(result.PackagedFactories) == 0 {
		t.Fatal("expected packaged factories to be ensured during init")
	}

	assertDirectorySnapshotUnchanged(t, encodedDir, beforeSnapshot)
	for _, factory := range result.PackagedFactories {
		if factory.Name == "@you/goal" && strings.Contains(factory.FactoryDir, "%2F") {
			t.Fatalf("packaged factory dir = %q, must not use percent-encoded scoped leaf names", factory.FactoryDir)
		}
	}
}

func legacyEncodedFactoryDir(t *testing.T, factoriesRoot, scopedName string) string {
	t.Helper()

	segment, err := namedfactorypath.LegacyLayoutSegment(scopedName)
	if err != nil {
		t.Fatalf("LegacyLayoutSegment(%q): %v", scopedName, err)
	}
	return filepath.Join(factoriesRoot, segment)
}

func seedLegacyEncodedGoalFactoryForResidueTest(t *testing.T, factoriesRoot string) string {
	t.Helper()

	segment, err := namedfactorypath.LegacyLayoutSegment("@you/goal")
	if err != nil {
		t.Fatalf("LegacyLayoutSegment(@you/goal): %v", err)
	}
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
