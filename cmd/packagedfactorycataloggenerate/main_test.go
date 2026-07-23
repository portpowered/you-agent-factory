package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunReportsGenerationFailure(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(filepath.Join(t.TempDir(), "missing"), &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "generation failed") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunGeneratesCompleteCatalog(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := filepath.Join("..", "..", "packages", "packaged-factories")
	destination := filepath.Join(root, "packages", "packaged-factories")
	if err := copyTree(source, destination); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run(root, &stdout, &stderr); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	if stdout.String() != successMessage+"\n" {
		t.Fatalf("stdout=%q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(destination, "generated", "manifest.json")); err != nil {
		t.Fatalf("generated manifest: %v", err)
	}
}

func copyTree(source, destination string) error {
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
