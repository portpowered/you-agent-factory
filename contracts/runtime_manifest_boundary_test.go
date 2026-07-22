package contracts_test

import (
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractstaging"
	"github.com/portpowered/infinite-you/internal/testpath"
)

func TestJavaScriptAuthoredCatalogBoundary(t *testing.T) {
	t.Parallel()

	repositoryRoot := testpath.MustRepoRootFromCaller(t, 0)
	authoredSchema := testpath.MustRepoPathFromCaller(t, 0, "contracts", "javascript", "runtime-manifest.schema.json")
	authoredCatalog := testpath.MustRepoPathFromCaller(t, 0, "contracts", "javascript", "runtime-api.json")

	if _, err := os.Stat(authoredSchema); err != nil {
		t.Fatalf("authored runtime-manifest schema missing: %v", err)
	}
	if _, err := os.Stat(authoredCatalog); err != nil {
		t.Fatalf("authored JavaScript runtime API catalog missing: %v", err)
	}

	allowed := map[string]struct{}{
		"runtime-manifest.schema.json": {},
		"runtime-api.json":             {},
	}
	entries, err := os.ReadDir(testpath.MustRepoPathFromCaller(t, 0, "contracts", "javascript"))
	if err != nil {
		t.Fatalf("read contracts/javascript: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("contracts/javascript must contain only authored contract files, found directory %s", entry.Name())
		}
		if _, ok := allowed[entry.Name()]; !ok {
			t.Fatalf("unexpected authored javascript contract %s under %s", entry.Name(), repositoryRoot)
		}
	}
}

func TestJavaScriptStagedRuntimeAPIProjectsFromAuthoredCatalog(t *testing.T) {
	t.Parallel()

	const (
		wantSource = "contracts/javascript/runtime-api.json"
		wantTarget = "packages/api/generated/javascript/runtime-api.json"
	)

	found := false
	for _, artifact := range contractstaging.RawArtifacts() {
		if artifact.Target != wantTarget {
			continue
		}
		found = true
		if artifact.Source != wantSource {
			t.Fatalf("staged runtime-api source = %q, want authored catalog %q", artifact.Source, wantSource)
		}
	}
	if !found {
		t.Fatalf("missing staged runtime-api projection in RawArtifacts()")
	}
}
