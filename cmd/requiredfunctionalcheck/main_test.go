package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunChecksRepositoryRequiredScenarioManifest(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	var stdout bytes.Buffer
	if err := run(config{root: root, manifestPath: defaultManifestPath}, &stdout); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got := stdout.String(); got != "[agent-factory:required-functional] 1 required short customer-boundary scenario(s) are current\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestRunReturnsStableGuardDiagnostic(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifestPath := filepath.Join(root, defaultManifestPath)
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("create manifest directory: %v", err)
	}
	manifest := `{"formatVersion":"required-functional-scenarios/v1","scenarios":[{"stableId":"cli/missing","test":"tests/functional/missing_test.go::TestMissing","interface":"cli","lane":"short","executionClass":"deterministic","customerBoundary":true}]}`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	err := run(config{root: root, manifestPath: defaultManifestPath}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), `required functional scenario "cli/missing" [missing-test]`) {
		t.Fatalf("run() error = %v, want stable missing-test diagnostic", err)
	}
}
