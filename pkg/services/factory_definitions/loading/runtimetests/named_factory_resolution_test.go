package runtimetests

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestResolveNamedFactoryAcrossRoots_ReturnsLocalFactory(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()
	projectFactoryDir := persistRuntimeNamedFactory(t, projectRoot, "alpha", "project-alpha")

	resolution, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "alpha")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots(local): %v", err)
	}

	assertNamedFactoryResolution(t, resolution, "alpha", projectFactoryDir, interfaces.NamedFactoryResolutionSourceProjectLocal, projectRoot, globalRoot)
	if resolution.PrecedenceDecision != interfaces.NamedFactoryPrecedenceDecisionNone {
		t.Fatalf("resolution precedence = %q, want %q", resolution.PrecedenceDecision, interfaces.NamedFactoryPrecedenceDecisionNone)
	}
}

func TestResolveNamedFactoryAcrossRoots_PrefersLocalFactoryOverGlobal(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()
	projectFactoryDir := persistRuntimeNamedFactory(t, projectRoot, "alpha", "project-alpha")
	persistRuntimeNamedFactory(t, globalRoot, "alpha", "global-alpha")

	resolution, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "alpha")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots(conflict): %v", err)
	}

	assertNamedFactoryResolution(t, resolution, "alpha", projectFactoryDir, interfaces.NamedFactoryResolutionSourceProjectLocal, projectRoot, globalRoot)
	if resolution.PrecedenceDecision != interfaces.NamedFactoryPrecedenceDecisionProjectOverGlobal {
		t.Fatalf("resolution precedence = %q, want %q", resolution.PrecedenceDecision, interfaces.NamedFactoryPrecedenceDecisionProjectOverGlobal)
	}
	loaded, err := LoadRuntimeConfigFromFactoryDir(resolution.FactoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(resolved local): %v", err)
	}
	if loaded.FactoryConfig().Project != "project-alpha" {
		t.Fatalf("resolved project = %q, want project-alpha", loaded.FactoryConfig().Project)
	}
}

func TestResolveNamedFactoryAcrossRoots_ReturnsGlobalWhenLocalMissing(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()
	globalFactoryDir := persistRuntimeNamedFactory(t, globalRoot, "@you/tts", "global-tts")

	factoryDir, err := ResolveNamedFactoryDirAcrossRoots(projectRoot, globalRoot, "@you/tts")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryDirAcrossRoots(global): %v", err)
	}
	if factoryDir != globalFactoryDir {
		t.Fatalf("resolved factory dir = %q, want %q", factoryDir, globalFactoryDir)
	}
}

func TestResolveNamedFactoryAcrossRoots_SelectsOneFactoryFromMultipleNamedEntries(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()
	persistRuntimeNamedFactory(t, projectRoot, "gamma", "project-gamma")
	wantFactoryDir := persistRuntimeNamedFactory(t, projectRoot, "beta", "project-beta")
	persistRuntimeNamedFactory(t, projectRoot, "alpha", "project-alpha")

	entries, err := namedFactoryCatalog.ListNamedFactories(projectRoot)
	if err != nil {
		t.Fatalf("ListNamedFactories(project root): %v", err)
	}
	if got := namedFactoryEntryNames(entries); len(got) != 3 || got[0] != "alpha" || got[1] != "beta" || got[2] != "gamma" {
		t.Fatalf("project named factories = %#v, want deterministic alpha/beta/gamma ordering", got)
	}

	resolution, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "beta")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots(multiple): %v", err)
	}
	assertNamedFactoryResolution(t, resolution, "beta", wantFactoryDir, interfaces.NamedFactoryResolutionSourceProjectLocal, projectRoot, globalRoot)
}

func TestResolveNamedFactoryAcrossRoots_ReturnsNotFoundWhenBothRootsMiss(t *testing.T) {
	_, err := ResolveNamedFactoryAcrossRoots(t.TempDir(), t.TempDir(), "missing")
	if err == nil {
		t.Fatal("expected missing named factory to fail")
	}
	if !errors.Is(err, ErrNamedFactoryNotFound) {
		t.Fatalf("error = %v, want ErrNamedFactoryNotFound", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, want os.ErrNotExist", err)
	}
	if got := err.Error(); !containsAll(got, `resolve named factory "missing"`, "project root", "global root") {
		t.Fatalf("expected cross-root not-found context, got %v", err)
	}
}

func TestResolveNamedFactoryAcrossRoots_RejectsInvalidCanonicalName(t *testing.T) {
	_, err := ResolveNamedFactoryAcrossRoots(t.TempDir(), t.TempDir(), "@you")
	if err == nil {
		t.Fatal("expected invalid named factory name to fail")
	}
	if !errors.Is(err, ErrInvalidNamedFactoryName) {
		t.Fatalf("error = %v, want ErrInvalidNamedFactoryName", err)
	}
	if errors.Is(err, ErrNamedFactoryNotFound) {
		t.Fatalf("error = %v, did not want ErrNamedFactoryNotFound", err)
	}
	if got := err.Error(); !containsAll(got, `invalid named factory name "@you"`, `must be scoped as @scope/name`) {
		t.Fatalf("expected invalid-name error context, got %v", err)
	}
}

func TestResolveNamedFactoryAcrossRoots_UninitializedPackagedFactoryIsNotFoundWithoutWrites(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()

	projectBefore := snapshotDirectoryContents(t, projectRoot)
	globalBefore := snapshotDirectoryContents(t, globalRoot)
	_, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "@you/goal")
	if !errors.Is(err, ErrNamedFactoryNotFound) {
		t.Fatalf("error = %v, want ErrNamedFactoryNotFound", err)
	}
	assertDirectorySnapshotUnchanged(t, projectRoot, projectBefore)
	assertDirectorySnapshotUnchanged(t, globalRoot, globalBefore)
}

func TestResolveNamedFactoryAcrossRoots_PreservesLegacyPromptAndCustomerEdits(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()
	factoryDir := persistRuntimeNamedFactory(t, globalRoot, "@you/goal", "customer-goal")

	workstationPath := filepath.Join(factoryDir, interfaces.WorkstationsDir, "execute-customer-goal", interfaces.FactoryAgentsFileName)
	legacyEditedBody := "Customer prompt for {{ .WorkID }}.\n"
	if err := os.WriteFile(workstationPath, []byte(legacyEditedBody), 0o640); err != nil {
		t.Fatalf("WriteFile(customer prompt): %v", err)
	}
	before := snapshotDirectoryContents(t, factoryDir)
	beforeInfo, err := os.Stat(workstationPath)
	if err != nil {
		t.Fatalf("Stat(customer prompt): %v", err)
	}

	resolution, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "@you/goal")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots: %v", err)
	}
	assertNamedFactoryResolution(t, resolution, "@you/goal", factoryDir, interfaces.NamedFactoryResolutionSourceGlobal, projectRoot, globalRoot)
	assertDirectorySnapshotUnchanged(t, factoryDir, before)
	afterInfo, err := os.Stat(workstationPath)
	if err != nil {
		t.Fatalf("Stat(customer prompt after resolution): %v", err)
	}
	if afterInfo.Mode() != beforeInfo.Mode() || !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
		t.Fatalf("customer prompt metadata changed: before mode=%v mtime=%v; after mode=%v mtime=%v", beforeInfo.Mode(), beforeInfo.ModTime(), afterInfo.Mode(), afterInfo.ModTime())
	}
}

func TestResolveNamedFactoryAcrossRoots_ReturnsNotFoundForUnknownBuiltInName(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()

	_, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "@you/missing")
	if err == nil {
		t.Fatal("expected unknown built-in name to fail")
	}
	if !errors.Is(err, ErrNamedFactoryNotFound) {
		t.Fatalf("error = %v, want ErrNamedFactoryNotFound", err)
	}
	if got := err.Error(); strings.Contains(got, "materialize built-in named factory") || !containsAll(got, `resolve named factory "@you/missing"`, "project root", "global root") {
		t.Fatalf("expected deterministic not-found error, got %v", err)
	}
}

func persistRuntimeNamedFactory(t *testing.T, rootDir, name, project string) string {
	t.Helper()

	factoryDir, err := factorydefinitioncomposition.PersistNamedFactory(rootDir, name, namedFactoryPayload(t, project), ownerFactoryDefinitionValidator())
	if err != nil {
		t.Fatalf("PersistNamedFactory(%s): %v", name, err)
	}
	return factoryDir
}

func assertNamedFactoryResolution(
	t *testing.T,
	resolution *NamedFactoryResolution,
	name string,
	factoryDir string,
	source interfaces.NamedFactoryResolutionSource,
	projectRoot string,
	globalRoot string,
) {
	t.Helper()

	if resolution == nil {
		t.Fatal("expected named factory resolution")
	}
	if resolution.Name != name {
		t.Fatalf("resolution name = %q, want %q", resolution.Name, name)
	}
	if resolution.FactoryDir != factoryDir {
		t.Fatalf("resolution factory dir = %q, want %q", resolution.FactoryDir, factoryDir)
	}
	if resolution.Source != source {
		t.Fatalf("resolution source = %q, want %q", resolution.Source, source)
	}
	if resolution.ProjectRoot != filepath.Clean(projectRoot) {
		t.Fatalf("resolution project root = %q, want %q", resolution.ProjectRoot, filepath.Clean(projectRoot))
	}
	if resolution.GlobalRoot != filepath.Clean(globalRoot) {
		t.Fatalf("resolution global root = %q, want %q", resolution.GlobalRoot, filepath.Clean(globalRoot))
	}
}

func namedFactoryEntryNames(entries []interfaces.NamedFactoryListEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	return names
}

func TestFreshNamedGoalPersist_NeverCreatesEncodedPaths(t *testing.T) {
	rootDir := t.TempDir()

	assertNoEncodedScopedFactoryLeaf(t, rootDir, "@you/goal")

	factoryDir, err := factorydefinitioncomposition.PersistNamedFactory(rootDir, "@you/goal", namedFactoryPayload(t, "fresh-goal"), ownerFactoryDefinitionValidator())
	if err != nil {
		t.Fatalf("PersistNamedFactory(@you/goal): %v", err)
	}

	wantDir := filepath.Join(rootDir, "@you", "goal")
	if factoryDir != wantDir {
		t.Fatalf("factory dir = %q, want hierarchical %q", factoryDir, wantDir)
	}
	assertNoEncodedScopedFactoryLeaf(t, rootDir, "@you/goal")
	assertFactoriesRootHasNoPercentEncodedLeaves(t, rootDir)

	if err := factorydefinitioncomposition.NamedPaths().WriteCurrentPointer(rootDir, "@you/goal"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}
	assertCurrentFactoryPointerWritesCanonicalName(t, rootDir, "@you/goal")
}

func assertNoEncodedScopedFactoryLeaf(t *testing.T, factoriesRoot, scopedName string) {
	t.Helper()

	segment := legacyEncodedNamedFactorySegment(scopedName)
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

	name, err := factorydefinitioncomposition.NamedPaths().ReadCurrentPointer(factoriesRoot)
	if err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	}
	if name != wantName {
		t.Fatalf("read current factory = %q, want %q", name, wantName)
	}
}
