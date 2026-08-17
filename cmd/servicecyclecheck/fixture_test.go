package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureFile describes one Go file in a synthetic pkg/services-shaped tree.
// Imports are written as repository-relative package paths such as
// "pkg/services/beta"; the fixture writer expands them to module paths.
type fixtureFile struct {
	path    string
	imports []string
}

// writeFixtureRepo materializes a synthetic repository root containing the
// named services and files, and returns its path. Fixtures are built under
// t.TempDir() rather than committed testdata so that no synthetic import ever
// reaches a repository-wide checker.
func writeFixtureRepo(t *testing.T, services []string, files []fixtureFile) string {
	t.Helper()

	root := t.TempDir()
	for _, service := range services {
		if err := os.MkdirAll(filepath.Join(root, "pkg", "services", service), 0o755); err != nil {
			t.Fatalf("create fixture service %s: %v", service, err)
		}
	}
	for _, file := range files {
		writeFixtureFile(t, root, file)
	}
	return root
}

func writeFixtureFile(t *testing.T, root string, file fixtureFile) {
	t.Helper()

	fullPath := filepath.Join(root, filepath.FromSlash(file.path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("create fixture directory for %s: %v", file.path, err)
	}

	var builder strings.Builder
	builder.WriteString("package fixture\n")
	for _, importPath := range file.imports {
		fmt.Fprintf(&builder, "\nimport %q\n", repositoryImportPrefix+importPath)
	}
	if err := os.WriteFile(fullPath, []byte(builder.String()), 0o600); err != nil {
		t.Fatalf("write fixture file %s: %v", file.path, err)
	}
}

// writeFixtureCeiling writes a ceiling baseline next to a fixture root and
// returns its path.
func writeFixtureCeiling(t *testing.T, root string, ceiling int) string {
	t.Helper()

	path := filepath.Join(root, "ceiling.json")
	content, err := json.Marshal(cycleCeiling{Description: "fixture ceiling", Ceiling: ceiling})
	if err != nil {
		t.Fatalf("encode fixture ceiling: %v", err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write fixture ceiling: %v", err)
	}
	return path
}

// ratchetFixtureServices is the service roster shared by the ratchet fixtures.
var ratchetFixtureServices = []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta"}

// ratchetFixtureFiles is a hand-checkable synthetic graph with two arc-disjoint
// cycles, alpha<->beta and gamma<->delta, plus the one-way arc epsilon->zeta.
// Breaking each cycle needs one arc and neither arc can serve both, so the
// minimum feedback arc weight is exactly 2.
func ratchetFixtureFiles() []fixtureFile {
	return []fixtureFile{
		{path: "pkg/services/alpha/internal/client.go", imports: []string{"pkg/services/beta"}},
		{path: "pkg/services/beta/transports/http/adapter.go", imports: []string{"pkg/services/alpha"}},
		{path: "pkg/services/gamma/contract.go", imports: []string{"pkg/services/delta"}},
		{path: "pkg/services/delta/wire/provider.go", imports: []string{"pkg/services/gamma"}},
		{path: "pkg/services/epsilon/reader.go", imports: []string{"pkg/services/zeta"}},
	}
}

// withAddedBackEdge returns the ratchet fixture plus zeta->epsilon, which
// closes a third arc-disjoint cycle and raises the minimum weight to 3.
func withAddedBackEdge() []fixtureFile {
	return append(ratchetFixtureFiles(), fixtureFile{
		path:    "pkg/services/zeta/internal/adapter/callback.go",
		imports: []string{"pkg/services/epsilon"},
	})
}

// withRemovedBackEdge returns the ratchet fixture minus delta->gamma, which
// opens the gamma/delta cycle and lowers the minimum weight to 1.
func withRemovedBackEdge() []fixtureFile {
	var kept []fixtureFile
	for _, file := range ratchetFixtureFiles() {
		if file.path == "pkg/services/delta/wire/provider.go" {
			continue
		}
		kept = append(kept, file)
	}
	return kept
}
