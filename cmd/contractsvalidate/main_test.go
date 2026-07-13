package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

func TestRunEmitsDeterministicDiagnosticsAndFailureStatus(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "schema.json", `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","required":["name"]}`)
	writeFixture(t, root, "z.json", `{}`)
	writeFixture(t, root, "a.json", `{}`)
	registry := contractvalidator.NewRegistry(contractvalidator.Entry{
		Family: "test", FormatVersion: "1.0.0",
		Schemas: []contractvalidator.Schema{{ID: "https://example.test/schema.json", Path: "schema.json"}},
		Documents: []contractvalidator.Document{
			{Path: `z.json`, SchemaID: "https://example.test/schema.json"},
			{Path: `a.json`, SchemaID: "https://example.test/schema.json"},
		},
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if status := run(root, registry, stdout, stderr); status != 1 {
		t.Fatalf("run() status = %d, want 1", status)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() stdout = %q, want empty", stdout.String())
	}
	want := strings.Join([]string{
		`{"code":"schema.validation","path":"/","message":"document does not conform to its registered schema","document":"a.json"}`,
		`{"code":"schema.validation","path":"/","message":"document does not conform to its registered schema","document":"z.json"}`,
		"",
	}, "\n")
	if got := stderr.String(); got != want {
		t.Fatalf("run() stderr = %q, want %q", got, want)
	}
}

func TestContractsValidateMakeTargetPassesRegisteredRepositoryContracts(t *testing.T) {
	root := repositoryRoot(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "make", "-C", root, "contracts-validate")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("make contracts-validate failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), successMessage) {
		t.Fatalf("make contracts-validate output = %q, want success message", output)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	return root
}

func writeFixture(t *testing.T, root, path, contents string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(contents), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
