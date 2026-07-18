package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractstaging"
)

func TestRunPassesCleanStagingWithoutWriting(t *testing.T) {
	root := commandFixture(t)
	before := commandTree(t, root)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}

	if status := run(root, stdout, stderr); status != 0 {
		t.Fatalf("run() status = %d, want 0; stderr = %q", status, stderr)
	}
	if got := stdout.String(); got != successMessage+"\n" || stderr.Len() != 0 {
		t.Fatalf("run() stdout = %q, stderr = %q", got, stderr.String())
	}
	if after := commandTree(t, root); !reflect.DeepEqual(after, before) {
		t.Fatal("run() changed repository bytes on success")
	}
}

func TestRunReportsCombinedDriftDeterministicallyWithoutWriting(t *testing.T) {
	root := commandFixture(t)
	allowed := contractstaging.AllowedArtifacts()
	writeCommandFixture(t, root, allowed[0], "stale-z")
	writeCommandFixture(t, root, allowed[1], "stale-a")
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(allowed[2]))); err != nil {
		t.Fatalf("remove expected artifact: %v", err)
	}
	for _, path := range []string{
		"packages/api/generated/ui/openapi.ts",
		"packages/api/generated/client.gen.go",
		"packages/api/generated/bin/you.exe",
		"packages/api/generated/.cache/contracts/data",
		"packages/api/generated/unrelated.txt",
	} {
		writeCommandFixture(t, root, path, "unexpected")
	}
	before := commandTree(t, root)
	want := strings.Join([]string{
		"[agent-factory:contracts-check] stale:",
		"  " + allowed[0],
		"  " + allowed[1],
		"[agent-factory:contracts-check] missing:",
		"  " + allowed[2],
		"[agent-factory:contracts-check] unexpected:",
		"  packages/api/generated/.cache/contracts/data",
		"  packages/api/generated/bin/you.exe",
		"  packages/api/generated/client.gen.go",
		"  packages/api/generated/ui/openapi.ts",
		"  packages/api/generated/unrelated.txt",
		"[agent-factory:contracts-check] contract staging differs from canonical sources; run `make contracts-generate` and remove every unexpected file from staging",
		"",
	}, "\n")

	for runIndex := 0; runIndex < 2; runIndex++ {
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		if status := run(root, stdout, stderr); status != 1 {
			t.Fatalf("run %d status = %d, want 1", runIndex, status)
		}
		if stdout.Len() != 0 || stderr.String() != want {
			t.Fatalf("run %d stdout = %q, stderr = %q, want %q", runIndex, stdout, stderr, want)
		}
	}
	if after := commandTree(t, root); !reflect.DeepEqual(after, before) {
		t.Fatal("run() changed repository bytes on failure")
	}
}

func commandFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeCommandFixture(t, root, "contracts/common/documentation.schema.json", `{"$id":"https://schemas.portpowered.com/you/contracts/common/documentation.schema.json","$defs":{"itemId":{"type":"string"}}}`)
	writeCommandFixture(t, root, "contracts/common/deprecations.schema.json", `{"$id":"https://schemas.portpowered.com/you/contracts/common/deprecations.schema.json","properties":{"itemId":{"$ref":"https://schemas.portpowered.com/you/contracts/common/documentation.schema.json#/$defs/itemId"}}}`)
	writeCommandFixture(t, root, "contracts/manifest.schema.json", `{"$id":"https://schemas.portpowered.com/you/contracts/manifest.schema.json","properties":{"packageId":{"$ref":"https://schemas.portpowered.com/you/contracts/common/documentation.schema.json#/$defs/itemId"}}}`)
	for _, artifact := range contractstaging.RawArtifacts() {
		contents := "canonical:" + artifact.Source
		if artifact.Source == "api/openapi.yaml" {
			contents = "components:\n  schemas:\n    Factory:\n      type: object\n      properties:\n        child:\n          $ref: '#/components/schemas/Child'\n    Child:\n      type: string\n    FactoryEvent:\n      type: object\n      required: [schemaVersion, id, type, context, payload]\n      properties:\n        schemaVersion:\n          type: string\n        id:\n          type: string\n        type:\n          type: string\n          enum: [TEST]\n        context:\n          type: object\n        payload:\n          $ref: '#/components/schemas/Child'\n      discriminator:\n        propertyName: type\n        mapping:\n          TEST: '#/components/schemas/Child'\n    FactoryRecording:\n      type: object\n      required: [schemaVersion, sessionId, events]\n      properties:\n        schemaVersion:\n          type: string\n        sessionId:\n          type: string\n        events:\n          type: array\n          items:\n            $ref: '#/components/schemas/FactoryEvent'\n"
		}
		writeCommandFixture(t, root, artifact.Source, contents)
	}
	writeCommandFixture(t, root, "unrelated.txt", "unrelated")
	initCommandGitRepo(t, root)
	if err := contractstaging.Generate(root); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	return root
}

func commandTree(t *testing.T, root string) map[string]string {
	t.Helper()
	result := make(map[string]string)
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(relative)] = string(payload)
		return nil
	}); err != nil {
		t.Fatalf("walk repository: %v", err)
	}
	return result
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

func initCommandGitRepo(t *testing.T, root string) {
	t.Helper()
	commands := [][]string{
		{"git", "-C", root, "init"},
		{"git", "-C", root, "config", "user.email", "contracts-check@test"},
		{"git", "-C", root, "config", "user.name", "contracts-check"},
		{"git", "-C", root, "add", "-A"},
		{"git", "-C", root, "commit", "-m", "contract check fixture"},
	}
	for _, command := range commands {
		if output, err := exec.Command(command[0], command[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", command, output)
		}
	}
}
