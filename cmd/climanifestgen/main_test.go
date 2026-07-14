package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
)

func TestRunGeneratesCLIFamilyArtifacts(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	root := t.TempDir()
	manifestSource := filepath.Join(repoRoot, climanifestgen.ProductionManifestPath)
	manifestTarget := filepath.Join(root, climanifestgen.ProductionManifestPath)
	if err := copyTestFile(manifestSource, manifestTarget); err != nil {
		t.Fatalf("copy production manifest: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if status := run(root, false, stdout, stderr); status != 0 {
		t.Fatalf("run() = %d, stderr = %q", status, stderr.String())
	}
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte("CLI family metadata generated")) {
		t.Fatalf("stdout = %q, want success message", got)
	}

	drift, err := climanifestgen.Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("drift after generation = %#v", drift)
	}
}

func copyTestFile(source, target string) error {
	payload, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, payload, 0o644)
}

func TestRunCheckFailsOnStaleArtifact(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, filepath.FromSlash(climanifestgen.RepresentativeFamilyJSONPath))
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("create artifact directory: %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write stale artifact: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if status := run(root, true, stdout, stderr); status == 0 {
		t.Fatalf("run(check) = 0, want failure; stderr = %q", stderr.String())
	}
}
