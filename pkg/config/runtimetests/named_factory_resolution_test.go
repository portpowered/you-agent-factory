package runtimetests

import (
	"errors"
	. "github.com/portpowered/infinite-you/pkg/config"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestResolveNamedFactoryAcrossRoots_ReturnsLocalFactory(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()
	projectFactoryDir := persistRuntimeNamedFactory(t, projectRoot, "alpha", "project-alpha")

	resolution, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "alpha")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots(local): %v", err)
	}

	assertNamedFactoryResolution(t, resolution, "alpha", projectFactoryDir, NamedFactoryResolutionSourceProjectLocal, projectRoot, globalRoot)
	if resolution.PrecedenceDecision != NamedFactoryPrecedenceDecisionNone {
		t.Fatalf("resolution precedence = %q, want %q", resolution.PrecedenceDecision, NamedFactoryPrecedenceDecisionNone)
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

	assertNamedFactoryResolution(t, resolution, "alpha", projectFactoryDir, NamedFactoryResolutionSourceProjectLocal, projectRoot, globalRoot)
	if resolution.PrecedenceDecision != NamedFactoryPrecedenceDecisionProjectOverGlobal {
		t.Fatalf("resolution precedence = %q, want %q", resolution.PrecedenceDecision, NamedFactoryPrecedenceDecisionProjectOverGlobal)
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

	entries, err := ListNamedFactories(projectRoot)
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
	assertNamedFactoryResolution(t, resolution, "beta", wantFactoryDir, NamedFactoryResolutionSourceProjectLocal, projectRoot, globalRoot)
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

func TestResolveNamedFactoryAcrossRoots_MaterializesBuiltInIntoGlobalRoot(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()

	resolution, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "@you/tts")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots(builtin): %v", err)
	}

	wantDir := filepath.Join(globalRoot, "@you%2Ftts")
	assertNamedFactoryResolution(t, resolution, "@you/tts", wantDir, NamedFactoryResolutionSourceBuiltin, projectRoot, globalRoot)
	if resolution.PrecedenceDecision != NamedFactoryPrecedenceDecisionNone {
		t.Fatalf("resolution precedence = %q, want %q", resolution.PrecedenceDecision, NamedFactoryPrecedenceDecisionNone)
	}
	assertBuiltInMaterializedLayout(t, wantDir)

	loaded, err := LoadRuntimeConfigFromFactoryDir(resolution.FactoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(materialized builtin): %v", err)
	}
	if loaded.FactoryConfig().Project != "builtin-tts" {
		t.Fatalf("materialized builtin project = %q, want builtin-tts", loaded.FactoryConfig().Project)
	}
}

func TestResolveNamedFactoryAcrossRoots_RepeatedBuiltinResolutionReusesMaterializedDir(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()

	first, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "@you/tts")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots(first): %v", err)
	}
	second, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "@you/tts")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots(second): %v", err)
	}
	third, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "@you/tts")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots(third): %v", err)
	}

	if first.Source != NamedFactoryResolutionSourceBuiltin {
		t.Fatalf("first resolution source = %q, want builtin materialization", first.Source)
	}
	for idx, resolution := range []*NamedFactoryResolution{first, second, third} {
		if resolution.FactoryDir != first.FactoryDir {
			t.Fatalf("resolution[%d] dir = %q, want stable %q", idx, resolution.FactoryDir, first.FactoryDir)
		}
	}
	if second.Source != NamedFactoryResolutionSourceGlobal {
		t.Fatalf("second resolution source = %q, want global reuse of materialized builtin", second.Source)
	}
	if third.Source != NamedFactoryResolutionSourceGlobal {
		t.Fatalf("third resolution source = %q, want global reuse of materialized builtin", third.Source)
	}

	loaded, err := LoadRuntimeConfigFromFactoryDir(first.FactoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(stable builtin): %v", err)
	}
	if loaded.FactoryConfig().Project != "builtin-tts" {
		t.Fatalf("stable builtin project = %q, want builtin-tts", loaded.FactoryConfig().Project)
	}
}

func TestResolveNamedFactoryAcrossRoots_UsesEditedMaterializedBuiltInOnNextLoad(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()

	resolution, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "@you/tts")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots(initial builtin): %v", err)
	}

	workerPath := filepath.Join(resolution.FactoryDir, interfaces.WorkersDir, "tts-executor", interfaces.FactoryAgentsFileName)
	editedBody := "You are the customer-edited @you/tts built-in.\n"
	if err := os.WriteFile(workerPath, []byte(editedBody), 0o644); err != nil {
		t.Fatalf("WriteFile(materialized worker body): %v", err)
	}

	resolvedDir, err := ResolveNamedFactoryDirAcrossRoots(projectRoot, globalRoot, "@you/tts")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryDirAcrossRoots(edited builtin): %v", err)
	}
	if resolvedDir != resolution.FactoryDir {
		t.Fatalf("resolved dir after edit = %q, want %q", resolvedDir, resolution.FactoryDir)
	}

	loaded, err := LoadRuntimeConfigFromFactoryDir(resolvedDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(edited builtin): %v", err)
	}
	worker, ok := loaded.Worker("tts-executor")
	if !ok {
		t.Fatal("expected materialized builtin worker")
	}
	if worker.Body != strings.TrimSpace(editedBody) {
		t.Fatalf("edited builtin worker body = %q, want %q", worker.Body, strings.TrimSpace(editedBody))
	}
}

func TestResolveNamedFactoryAcrossRoots_ReportsCorruptMaterializedBuiltInTarget(t *testing.T) {
	projectRoot := t.TempDir()
	globalRoot := t.TempDir()
	corruptDir := filepath.Join(globalRoot, "@you%2Ftts")
	if err := os.MkdirAll(corruptDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(corrupt builtin dir): %v", err)
	}

	_, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, "@you/tts")
	if err == nil {
		t.Fatal("expected corrupt materialized builtin to fail")
	}
	if got := err.Error(); !containsAll(got, `materialize built-in named factory "@you/tts"`, "existing target invalid", "find factory config") {
		t.Fatalf("expected corrupt-target resolution error, got %v", err)
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

	factoryDir, err := PersistNamedFactory(rootDir, name, namedFactoryPayload(t, project))
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
	source NamedFactoryResolutionSource,
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

func namedFactoryEntryNames(entries []NamedFactoryListEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	return names
}

func assertBuiltInMaterializedLayout(t *testing.T, factoryDir string) {
	t.Helper()

	for _, path := range []string{
		filepath.Join(factoryDir, interfaces.FactoryConfigFile),
		filepath.Join(factoryDir, interfaces.WorkersDir, "tts-executor", interfaces.FactoryAgentsFileName),
		filepath.Join(factoryDir, interfaces.WorkstationsDir, "execute-tts", interfaces.FactoryAgentsFileName),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected built-in materialized path %s: %v", path, err)
		}
	}
}
