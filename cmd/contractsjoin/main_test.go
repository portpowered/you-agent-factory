package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractjoiner"
)

func TestRunGeneratesJoinedOutput(t *testing.T) {
	root := t.TempDir()
	writeCommandFixture(t, root, "contracts/root.json", `{"$id":"root","type":"object"}`)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	status := run(contractjoiner.Input{RepositoryRoot: root, Roots: []string{"contracts/root.json"}}, stdout, stderr)

	if status != 0 || stdout.String() != successMessage+"\n" || stderr.Len() != 0 {
		t.Fatalf("run() = status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "packages", "api", "generated", "joined", "contracts", "root.json")); err != nil {
		t.Fatalf("generated output: %v", err)
	}
}

func TestRunReportsStableJoinDiagnosticAndFailure(t *testing.T) {
	root := t.TempDir()
	writeCommandFixture(t, root, "contracts/root.json", `{"$ref":"missing.json"}`)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	status := run(contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          []string{"contracts/root.json"},
		Components:     []string{"contracts/missing.json"},
	}, stdout, stderr)

	if status != 1 || stdout.Len() != 0 {
		t.Fatalf("run() = status %d, stdout %q; want failure and empty stdout", status, stdout)
	}
	want := "{\"code\":\"reference.missing\",\"path\":\"/$ref\",\"message\":\"referenced document does not exist\",\"document\":\"contracts/root.json\"}\n"
	if stderr.String() != want {
		t.Fatalf("run() stderr = %q, want %q", stderr, want)
	}
	if _, err := os.Stat(filepath.Join(root, "packages", "api", "generated", "joined")); !os.IsNotExist(err) {
		t.Fatalf("failed command created joined output: %v", err)
	}
}

func writeCommandFixture(t *testing.T, root, path, contents string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(target, []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
