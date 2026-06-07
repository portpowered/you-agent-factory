package runtimetests

import (
	"errors"
	. "github.com/portpowered/infinite-you/pkg/config"
	"path/filepath"
	"testing"
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
	if !errors.Is(err, ErrFactoryLayoutNotFound) {
		t.Fatalf("error = %v, want ErrFactoryLayoutNotFound", err)
	}
	if got := err.Error(); !containsAll(got, `resolve named factory "missing"`, "project root", "global root") {
		t.Fatalf("expected cross-root not-found context, got %v", err)
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
