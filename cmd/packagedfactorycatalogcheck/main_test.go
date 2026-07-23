package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRunAcceptsCurrentCatalogWithoutWriting(t *testing.T) {
	root := commandCatalogFixture(t)
	before := commandCatalogSnapshot(t, root)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}

	if status := run(root, stdout, stderr); status != 0 {
		t.Fatalf("run() status = %d, stderr = %q", status, stderr)
	}
	if stdout.String() != successMessage+"\n" || stderr.Len() != 0 {
		t.Fatalf("run() stdout = %q, stderr = %q", stdout, stderr)
	}
	if after := commandCatalogSnapshot(t, root); !reflect.DeepEqual(after, before) {
		t.Fatal("run() changed package bytes")
	}
}

func TestRunReportsSortedDriftAndRegenerationRemedyWithoutWriting(t *testing.T) {
	root := commandCatalogFixture(t)
	for _, relative := range []string{
		"generated/factories/tts/factory.json",
		"generated/factories/tts/factory.yaml",
	} {
		target := commandCatalogPath(root, relative)
		if err := os.WriteFile(target, []byte("stale\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(commandCatalogPath(root, "generated/factories/subagent/factory.json")); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"generated/z.txt", "generated/a.txt"} {
		target := commandCatalogPath(root, relative)
		if err := os.WriteFile(target, []byte("unexpected\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	before := commandCatalogSnapshot(t, root)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}

	if status := run(root, stdout, stderr); status != 1 {
		t.Fatalf("run() status = %d, want 1", status)
	}
	want := strings.Join([]string{
		"[agent-factory:packaged-factory-catalog-check] stale:",
		"  generated/factories/tts/factory.json",
		"  generated/factories/tts/factory.yaml",
		"[agent-factory:packaged-factory-catalog-check] missing:",
		"  generated/factories/subagent/factory.json",
		"[agent-factory:packaged-factory-catalog-check] unexpected:",
		"  generated/a.txt",
		"  generated/z.txt",
		"[agent-factory:packaged-factory-catalog-check] generated catalog differs from canonical authored inventory; run `make packaged-factory-catalog-generate` and remove every unexpected generated output",
		"",
	}, "\n")
	if stdout.Len() != 0 || stderr.String() != want {
		t.Fatalf("run() stdout = %q, stderr = %q, want %q", stdout, stderr, want)
	}
	if after := commandCatalogSnapshot(t, root); !reflect.DeepEqual(after, before) {
		t.Fatal("run() changed package bytes")
	}
}

func TestRunReportsProjectionFailure(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if status := run(t.TempDir(), stdout, stderr); status != 1 {
		t.Fatalf("run() status = %d, want 1", status)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "check failed") {
		t.Fatalf("run() stdout = %q, stderr = %q", stdout, stderr)
	}
}

func commandCatalogFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join("..", "..", "packages", "packaged-factories")
	if err := copyCommandCatalogTree(source, commandCatalogPath(root, "")); err != nil {
		t.Fatalf("copy package fixture: %v", err)
	}
	return root
}

func commandCatalogPath(root, relative string) string {
	return filepath.Join(
		root,
		"packages",
		"packaged-factories",
		filepath.FromSlash(relative),
	)
}

func copyCommandCatalogTree(source, destination string) error {
	return filepath.WalkDir(source, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, current)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		payload, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		return os.WriteFile(target, payload, 0o644)
	})
}

func commandCatalogSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	base := commandCatalogPath(root, "")
	snapshot := make(map[string]string)
	if err := filepath.WalkDir(base, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		relative, err := filepath.Rel(base, current)
		if err != nil {
			return err
		}
		payload, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		snapshot[filepath.ToSlash(relative)] = string(payload)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return snapshot
}
