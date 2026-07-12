package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureBuiltInNamedFactories_FreshRootMaterializesHierarchicalPackagedDefaults(t *testing.T) {
	t.Parallel()

	globalRoot := t.TempDir()
	results, err := EnsureBuiltInNamedFactories(globalRoot)
	if err != nil {
		t.Fatalf("EnsureBuiltInNamedFactories() error = %v", err)
	}

	wantNames := BuiltInNamedFactoryNames()
	if len(results) != len(wantNames) {
		t.Fatalf("result count = %d, want %d (%#v)", len(results), len(wantNames), wantNames)
	}

	created := 0
	for i, result := range results {
		if result.Name != wantNames[i] {
			t.Fatalf("results[%d].Name = %q, want %q", i, result.Name, wantNames[i])
		}
		if result.Outcome != BuiltInNamedFactoryCreated {
			t.Fatalf("results[%d].Outcome = %q, want %q", i, result.Outcome, BuiltInNamedFactoryCreated)
		}
		created++

		wantDir, err := MapNamedFactoryDir(globalRoot, result.Name)
		if err != nil {
			t.Fatalf("MapNamedFactoryDir(%q): %v", result.Name, err)
		}
		if result.FactoryDir != wantDir {
			t.Fatalf("results[%d].FactoryDir = %q, want %q", i, result.FactoryDir, wantDir)
		}
		if _, err := os.Stat(wantDir); err != nil {
			t.Fatalf("Stat(%q): %v", wantDir, err)
		}
		if _, err := LoadRuntimeConfigFromFactoryDir(wantDir, nil); err != nil {
			t.Fatalf("LoadRuntimeConfigFromFactoryDir(%q): %v", wantDir, err)
		}
	}
	if created != len(wantNames) {
		t.Fatalf("created count = %d, want %d", created, len(wantNames))
	}

	assertNoLegacyEncodedFactoryLeaves(t, globalRoot)
}

func TestEnsureBuiltInNamedFactories_ReusesExistingFactoriesWithoutRewrite(t *testing.T) {
	t.Parallel()

	globalRoot := t.TempDir()
	first, err := EnsureBuiltInNamedFactories(globalRoot)
	if err != nil {
		t.Fatalf("first EnsureBuiltInNamedFactories(): %v", err)
	}

	second, err := EnsureBuiltInNamedFactories(globalRoot)
	if err != nil {
		t.Fatalf("second EnsureBuiltInNamedFactories(): %v", err)
	}
	if len(second) != len(first) {
		t.Fatalf("second result count = %d, want %d", len(second), len(first))
	}
	for i, result := range second {
		if result.Outcome != BuiltInNamedFactorySkipped {
			t.Fatalf("second results[%d].Outcome = %q, want %q", i, result.Outcome, BuiltInNamedFactorySkipped)
		}
		if result.FactoryDir != first[i].FactoryDir {
			t.Fatalf("second results[%d].FactoryDir = %q, want %q", i, result.FactoryDir, first[i].FactoryDir)
		}
	}
}

func assertNoLegacyEncodedFactoryLeaves(t *testing.T, globalRoot string) {
	t.Helper()

	err := filepath.WalkDir(globalRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if path == globalRoot {
			return nil
		}
		if strings.Contains(entry.Name(), "%2F") {
			t.Fatalf("found legacy encoded factory directory %q under %s", path, globalRoot)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%s): %v", globalRoot, err)
	}
}
