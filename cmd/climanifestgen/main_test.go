package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
)

func TestRunGeneratesCLIFamilyArtifacts(t *testing.T) {
	root := t.TempDir()
	writeProductionManifestFixture(t, root)

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

func TestRunCheckNamesAffectedStableCommandIDs(t *testing.T) {
	root := t.TempDir()
	writeProductionManifestFixture(t, root)
	if err := climanifestgen.Generate(root); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, filepath.FromSlash(climanifestgen.WorkflowCompatibilityFamilyJSONPath))
	if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if status := run(root, true, stdout, stderr); status == 0 {
		t.Fatal("run(check) = 0, want stale-artifact failure")
	}
	for _, id := range climanifestgen.WorkflowCompatibilityFamilyCommandIDs {
		if !bytes.Contains(stderr.Bytes(), []byte(id)) {
			t.Errorf("stderr = %q, want affected stable ID %q", stderr.String(), id)
		}
	}
}

func writeProductionManifestFixture(t *testing.T, root string) {
	t.Helper()
	repositoryRoot := testutil.MustRepoPath(t, ".")
	for _, relativePath := range []string{
		climanifestgen.ProductionManifestPath,
		climanifestgen.CompatibilityManifestPath,
	} {
		manifest, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatalf("read %s fixture: %v", relativePath, err)
		}
		manifestPath := filepath.Join(root, filepath.FromSlash(relativePath))
		if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
			t.Fatalf("create manifest directory: %v", err)
		}
		if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
			t.Fatalf("write %s: %v", relativePath, err)
		}
	}
}
