package runtimetests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/namedfactorypath"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestFreshBuiltinGoalMaterialization_NeverCreatesEncodedPaths(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()

	assertNoEncodedScopedFactoryLeaf(t, globalRoot, "@you/goal")

	resolution, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "@you/goal")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots(@you/goal): %v", err)
	}

	wantDir := filepath.Join(globalRoot, "@you", "goal")
	assertNamedFactoryResolution(t, resolution, "@you/goal", wantDir, NamedFactoryResolutionSourceBuiltin, projectRoot, globalRoot)
	assertBuiltInGoalMaterializedLayout(t, wantDir)
	assertNoEncodedScopedFactoryLeaf(t, globalRoot, "@you/goal")
	assertFactoriesRootHasNoPercentEncodedLeaves(t, globalRoot)

	resolvedDir, err := ResolveNamedFactoryDir(globalRoot, "@you/goal")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryDir after fresh materialization: %v", err)
	}
	if resolvedDir != wantDir {
		t.Fatalf("resolved dir = %q, want hierarchical %q", resolvedDir, wantDir)
	}

	loaded, err := LoadRuntimeConfigFromFactoryDir(resolvedDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	if loaded.FactoryConfig().Project != "builtin-goal" {
		t.Fatalf("project = %q, want builtin-goal", loaded.FactoryConfig().Project)
	}
}

func TestFreshNamedGoalPersist_NeverCreatesEncodedPaths(t *testing.T) {
	rootDir := t.TempDir()

	assertNoEncodedScopedFactoryLeaf(t, rootDir, "@you/goal")

	factoryDir, err := PersistNamedFactory(rootDir, "@you/goal", namedFactoryPayload(t, "fresh-goal"))
	if err != nil {
		t.Fatalf("PersistNamedFactory(@you/goal): %v", err)
	}

	wantDir := filepath.Join(rootDir, "@you", "goal")
	if factoryDir != wantDir {
		t.Fatalf("factory dir = %q, want hierarchical %q", factoryDir, wantDir)
	}
	assertNoEncodedScopedFactoryLeaf(t, rootDir, "@you/goal")
	assertFactoriesRootHasNoPercentEncodedLeaves(t, rootDir)

	if err := WriteCurrentFactoryPointer(rootDir, "@you/goal"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}
	assertCurrentFactoryPointerWritesCanonicalName(t, rootDir, "@you/goal")
}

func assertNoEncodedScopedFactoryLeaf(t *testing.T, factoriesRoot, scopedName string) {
	t.Helper()

	segment, err := namedfactorypath.LegacyLayoutSegment(scopedName)
	if err != nil {
		t.Fatalf("LegacyLayoutSegment(%q): %v", scopedName, err)
	}
	encodedDir := filepath.Join(factoriesRoot, segment)
	if _, err := os.Stat(encodedDir); err == nil {
		t.Fatalf("encoded factory leaf %q must not exist under %s", encodedDir, factoriesRoot)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat encoded factory leaf %q: %v", encodedDir, err)
	}
}

func assertFactoriesRootHasNoPercentEncodedLeaves(t *testing.T, factoriesRoot string) {
	t.Helper()

	entries, err := os.ReadDir(factoriesRoot)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", factoriesRoot, err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "%2F") {
			t.Fatalf("factories root %s contains percent-encoded leaf %q", factoriesRoot, entry.Name())
		}
	}
}

func assertCurrentFactoryPointerWritesCanonicalName(t *testing.T, factoriesRoot, wantName string) {
	t.Helper()

	path := filepath.Join(factoriesRoot, interfaces.CurrentFactoryPointerFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	got := strings.TrimSpace(string(data))
	if got != wantName {
		t.Fatalf("current pointer = %q, want canonical %q", got, wantName)
	}
	if strings.Contains(got, "%2F") {
		t.Fatalf("current pointer must not contain encoded slash leaf: %q", got)
	}

	name, err := ReadCurrentFactoryPointer(factoriesRoot)
	if err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	}
	if name != wantName {
		t.Fatalf("read current factory = %q, want %q", name, wantName)
	}
}
