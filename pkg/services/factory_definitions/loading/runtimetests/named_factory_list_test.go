package runtimetests

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	. "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestListNamedFactories_ReturnsPersistedFactoriesSorted(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := factorydefinitioncomposition.PersistNamedFactory(rootDir, "beta", namedFactoryPayload(t, "beta"), ownerFactoryDefinitionValidator()); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	if _, err := factorydefinitioncomposition.PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha"), ownerFactoryDefinitionValidator()); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}

	entries, err := namedFactoryCatalog.ListNamedFactories(rootDir)
	if err != nil {
		t.Fatalf("ListNamedFactories: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(entries))
	}
	if entries[0].Name != "alpha" || entries[1].Name != "beta" {
		t.Fatalf("entries = %#v, want alpha then beta", entries)
	}
	if entries[0].FactoryDir != filepath.Join(rootDir, "alpha") {
		t.Fatalf("alpha directory = %q, want %q", entries[0].FactoryDir, filepath.Join(rootDir, "alpha"))
	}
}

func TestListNamedFactories_MarksCurrentPointer(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := factorydefinitioncomposition.PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha"), ownerFactoryDefinitionValidator()); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if _, err := factorydefinitioncomposition.PersistNamedFactory(rootDir, "beta", namedFactoryPayload(t, "beta"), ownerFactoryDefinitionValidator()); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	if err := factorydefinitioncomposition.NamedPaths().WriteCurrentPointer(rootDir, "beta"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}

	entries, err := namedFactoryCatalog.ListNamedFactories(rootDir)
	if err != nil {
		t.Fatalf("ListNamedFactories: %v", err)
	}

	currentCount := 0
	for _, entry := range entries {
		if entry.Current {
			currentCount++
			if entry.Name != "beta" {
				t.Fatalf("unexpected current entry %#v", entry)
			}
		}
	}
	if currentCount != 1 {
		t.Fatalf("current count = %d, want 1", currentCount)
	}
}

func TestListNamedFactories_NoPointerMarksNothingCurrent(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := factorydefinitioncomposition.PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha"), ownerFactoryDefinitionValidator()); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}

	entries, err := namedFactoryCatalog.ListNamedFactories(rootDir)
	if err != nil {
		t.Fatalf("ListNamedFactories: %v", err)
	}
	for _, entry := range entries {
		if entry.Current {
			t.Fatalf("expected no current entry, got %#v", entry)
		}
	}
}

func TestListNamedFactories_RejectsMissingRoot(t *testing.T) {
	_, err := namedFactoryCatalog.ListNamedFactories(filepath.Join(t.TempDir(), "missing-root"))
	if err == nil {
		t.Fatal("expected missing factory root to fail")
	}
}

func TestListNamedFactories_RejectsNonDirectoryRoot(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(rootPath, []byte("factory"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := namedFactoryCatalog.ListNamedFactories(rootPath)
	if err == nil {
		t.Fatal("expected non-directory factory root to fail")
	}
}

func TestListNamedFactories_RejectsInvalidCurrentPointer(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir, ".current-factory"), []byte("../beta\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := namedFactoryCatalog.ListNamedFactories(rootDir)
	if err == nil {
		t.Fatal("expected malformed current-factory pointer to fail")
	}
}

func TestListNamedFactories_IgnoresLegacyEncodedLeafDirectories(t *testing.T) {
	rootDir := t.TempDir()

	segment := legacyEncodedNamedFactorySegment("@you/goal")
	encodedDir := filepath.Join(rootDir, segment)
	if err := os.MkdirAll(encodedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(encoded legacy dir): %v", err)
	}
	writeRuntimeFactoryJSON(t, encodedDir, map[string]any{
		"name":    "legacy-encoded-only",
		"project": "legacy-encoded-only",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"workers": []map[string]any{{
			"name": "legacy-executor",
			"type": "MODEL_WORKER",
		}},
		"workstations": []map[string]any{{
			"name":    "legacy-execute",
			"worker":  "legacy-executor",
			"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
			"outputs": []map[string]string{{"workType": "task", "state": "complete"}},
			"type":    "MODEL_WORKSTATION",
		}},
	})

	entries, err := namedFactoryCatalog.ListNamedFactories(rootDir)
	if err != nil {
		t.Fatalf("ListNamedFactories: %v", err)
	}
	for _, entry := range entries {
		if entry.Name == "@you/goal" {
			t.Fatalf("ListNamedFactories must ignore legacy encoded leaf, got %#v", entry)
		}
	}
}

const legacyEncodedGoalMarkerFile = "legacy-encoded-sentinel.txt"

func TestHierarchicalOperations_LeaveLegacyEncodedDirectoryUntouched(t *testing.T) {
	t.Run("resolution miss", func(t *testing.T) {
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
	})

	t.Run("fresh named persist", func(t *testing.T) {
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
		assertDirectorySnapshotUnchanged(t, encodedDir, beforeSnapshot)
	})
}

func seedLegacyEncodedGoalFactory(t *testing.T, factoriesRoot string) string {
	t.Helper()

	segment := legacyEncodedNamedFactorySegment("@you/goal")
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
