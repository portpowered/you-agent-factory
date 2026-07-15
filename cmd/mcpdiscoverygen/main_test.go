package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/mcp/discoverygen"
)

func TestRunGeneratesDiscoveryArtifact(t *testing.T) {
	root := t.TempDir()
	writeAuthoredCatalogFixture(t, root)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if status := run(root, false, stdout, stderr); status != 0 {
		t.Fatalf("run() = %d, stderr = %q", status, stderr.String())
	}
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte("MCP discovery metadata generated")) {
		t.Fatalf("stdout = %q, want success message", got)
	}

	drift, err := discoverygen.Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("drift after generation = %#v", drift)
	}
}

func TestRunCheckFailsOnStaleArtifact(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, filepath.FromSlash(discoverygen.DiscoveryJSONPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create artifact directory: %v", err)
	}
	if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write stale artifact: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if status := run(root, true, stdout, stderr); status == 0 {
		t.Fatalf("run(check) = 0, want failure; stderr = %q", stderr.String())
	}
}

func writeAuthoredCatalogFixture(t *testing.T, root string) {
	t.Helper()
	catalog, err := os.ReadFile(filepath.Join("..", "..", discoverygen.AuthoredCatalogPath))
	if err != nil {
		t.Fatalf("read authored catalog fixture: %v", err)
	}
	target := filepath.Join(root, filepath.FromSlash(discoverygen.AuthoredCatalogPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create catalog directory: %v", err)
	}
	if err := os.WriteFile(target, catalog, 0o644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
}
