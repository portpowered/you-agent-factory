package runtimetests

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	. "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/namedfactorypath"
)

const legacyEncodedGoalMarkerFile = "legacy-encoded-sentinel.txt"

func TestHierarchicalOperations_LeaveLegacyEncodedDirectoryUntouched(t *testing.T) {
	t.Run("fresh builtin materialization", func(t *testing.T) {
		projectRoot := t.TempDir()
		globalRoot := t.TempDir()

		encodedDir := seedLegacyEncodedGoalFactory(t, globalRoot)
		beforeSnapshot := snapshotDirectoryContents(t, encodedDir)

		resolution, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "@you/goal")
		if err != nil {
			t.Fatalf("ResolveNamedFactoryAcrossRoots(@you/goal): %v", err)
		}

		wantDir := filepath.Join(globalRoot, "@you", "goal")
		assertNamedFactoryResolution(t, resolution, "@you/goal", wantDir, NamedFactoryResolutionSourceBuiltin, projectRoot, globalRoot)
		assertBuiltInGoalMaterializedLayout(t, wantDir)
		assertDirectorySnapshotUnchanged(t, encodedDir, beforeSnapshot)

		entries, err := ListNamedFactories(globalRoot)
		if err != nil {
			t.Fatalf("ListNamedFactories: %v", err)
		}
		assertListIncludesHierarchicalGoalOnly(t, entries, wantDir, encodedDir)
		assertDirectorySnapshotUnchanged(t, encodedDir, beforeSnapshot)

		resolvedDir, err := ResolveNamedFactoryDir(globalRoot, "@you/goal")
		if err != nil {
			t.Fatalf("ResolveNamedFactoryDir(@you/goal): %v", err)
		}
		if resolvedDir != wantDir {
			t.Fatalf("resolved dir = %q, want hierarchical %q", resolvedDir, wantDir)
		}
		assertDirectorySnapshotUnchanged(t, encodedDir, beforeSnapshot)
	})

	t.Run("fresh named persist", func(t *testing.T) {
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
		assertDirectorySnapshotUnchanged(t, encodedDir, beforeSnapshot)
	})
}

func seedLegacyEncodedGoalFactory(t *testing.T, factoriesRoot string) string {
	t.Helper()

	segment, err := namedfactorypath.LegacyLayoutSegment("@you/goal")
	if err != nil {
		t.Fatalf("LegacyLayoutSegment(@you/goal): %v", err)
	}
	encodedDir := filepath.Join(factoriesRoot, segment)
	if err := os.MkdirAll(encodedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(encoded legacy dir): %v", err)
	}

	writeRuntimeFactoryJSON(t, encodedDir, map[string]any{
		"name":    "legacy-encoded-marker",
		"project": "legacy-encoded-marker",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
		},
		"workers": []map[string]any{
			{
				"name": "legacy-executor",
				"type": "MODEL_WORKER",
				"body": "legacy encoded factory worker",
			},
		},
		"workstations": []map[string]any{
			{
				"name":    "legacy-execute",
				"worker":  "legacy-executor",
				"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
				"outputs": []map[string]string{{"workType": "task", "state": "complete"}},
				"type":    "MODEL_WORKSTATION",
				"body":    "legacy encoded factory workstation",
			},
		},
	})

	markerPath := filepath.Join(encodedDir, legacyEncodedGoalMarkerFile)
	if err := os.WriteFile(markerPath, []byte("do-not-touch-legacy-encoded-goal\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(legacy marker): %v", err)
	}
	return encodedDir
}

func snapshotDirectoryContents(t *testing.T, root string) map[string][]byte {
	t.Helper()

	snapshot := make(map[string][]byte)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snapshot[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		t.Fatalf("snapshotDirectoryContents(%s): %v", root, err)
	}
	return snapshot
}

func assertDirectorySnapshotUnchanged(t *testing.T, root string, before map[string][]byte) {
	t.Helper()

	after := snapshotDirectoryContents(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("directory %s changed after hierarchical operation:\nbefore=%#v\nafter=%#v", root, before, after)
	}
}

func assertListIncludesHierarchicalGoalOnly(
	t *testing.T,
	entries []NamedFactoryListEntry,
	wantHierarchicalDir string,
	encodedDir string,
) {
	t.Helper()

	var goalEntry *NamedFactoryListEntry
	for i := range entries {
		if entries[i].Name == "@you/goal" {
			goalEntry = &entries[i]
			break
		}
	}
	if goalEntry == nil {
		t.Fatalf("ListNamedFactories missing @you/goal entry: %#v", entries)
	}
	if goalEntry.FactoryDir != wantHierarchicalDir {
		t.Fatalf("@you/goal factory dir = %q, want hierarchical %q", goalEntry.FactoryDir, wantHierarchicalDir)
	}
	if goalEntry.FactoryDir == encodedDir {
		t.Fatalf("ListNamedFactories must not resolve @you/goal to legacy encoded dir %q", encodedDir)
	}
}
