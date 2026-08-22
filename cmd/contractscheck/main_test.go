package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
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
	assertCommandTreeUnchanged(t, "run() changed repository bytes on success", before, commandTree(t, root))
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
	assertCommandTreeUnchanged(t, "run() changed repository bytes on failure", before, commandTree(t, root))
}

func TestRunReportsPackagedFactorySchemaDriftWithRegenerationRemedy(t *testing.T) {
	const (
		jsonPath = "packages/packaged-factories/schemas/factory.schema.json"
		yamlPath = "packages/packaged-factories/schemas/factory.schema.yaml"
	)
	tests := []struct {
		name     string
		path     string
		category string
	}{
		{name: "missing JSON", path: jsonPath, category: "missing"},
		{name: "stale JSON", path: jsonPath, category: "stale"},
		{name: "missing YAML", path: yamlPath, category: "missing"},
		{name: "stale YAML", path: yamlPath, category: "stale"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := commandFixture(t)
			target := filepath.Join(root, filepath.FromSlash(test.path))
			mutatePackagedSchema(t, target, test.category)
			before := commandTree(t, root)
			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}

			if status := run(root, stdout, stderr); status != 1 {
				t.Fatalf("run() status = %d, want 1; stderr = %q", status, stderr)
			}
			if stdout.Len() != 0 {
				t.Fatalf("run() stdout = %q, want empty", stdout)
			}
			for _, fragment := range []string{
				"[agent-factory:contracts-check] " + test.category + ":",
				"  " + test.path,
				"run `make contracts-generate`",
			} {
				if !strings.Contains(stderr.String(), fragment) {
					t.Fatalf("run() stderr = %q, want fragment %q", stderr, fragment)
				}
			}
			assertCommandTreeUnchanged(t, "run() changed repository bytes on failure", before, commandTree(t, root))
		})
	}
}

func mutatePackagedSchema(t *testing.T, path, category string) {
	t.Helper()
	if category == "missing" {
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove schema: %v", err)
		}
		return
	}
	payload := []byte(`{"type":"string"}`)
	if filepath.Ext(path) == ".yaml" {
		payload = []byte("type: string\n")
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write semantically divergent schema: %v", err)
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

func commandTree(t *testing.T, root string) commandTreeSnapshot {
	t.Helper()
	return snapshotCommandTree(t, root, filepath.WalkDir, os.ReadFile)
}

func snapshotCommandTree(
	t *testing.T,
	root string,
	walkDir func(string, fs.WalkDirFunc) error,
	readFile func(string) ([]byte, error),
) commandTreeSnapshot {
	t.Helper()
	result := make(commandTreeSnapshot)
	if err := walkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		payload, err := readFile(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		result[filepath.ToSlash(relative)] = payload
		return nil
	}); err != nil {
		t.Fatalf("walk repository: %v", err)
	}
	return result
}

func TestSnapshotCommandTreeSkipsTransientMissingEntriesAndReportsRemovals(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"kept.txt", "walk-missing.txt", "read-missing.txt"} {
		writeCommandFixture(t, root, path, path)
	}
	before := commandTree(t, root)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read fixture directory: %v", err)
	}
	entryByName := make(map[string]os.DirEntry, len(entries))
	for _, entry := range entries {
		entryByName[entry.Name()] = entry
	}
	wrappedNotExist := func(path string) error {
		return fmt.Errorf("fixture entry disappeared at %s: %w", path, fs.ErrNotExist)
	}

	after := snapshotCommandTree(
		t,
		root,
		func(_ string, walkFn fs.WalkDirFunc) error {
			for _, path := range []string{"kept.txt", "walk-missing.txt", "read-missing.txt"} {
				fullPath := filepath.Join(root, path)
				var callbackErr error
				if path == "walk-missing.txt" {
					callbackErr = wrappedNotExist(path)
				}
				if err := walkFn(fullPath, entryByName[path], callbackErr); err != nil {
					return err
				}
			}
			return nil
		},
		func(path string) ([]byte, error) {
			if filepath.Base(path) == "read-missing.txt" {
				return nil, wrappedNotExist(filepath.Base(path))
			}
			return os.ReadFile(path)
		},
	)

	diff := formatCommandTreeDiff(before, after)
	for _, fragment := range []string{
		"  removed: read-missing.txt",
		"  removed: walk-missing.txt",
	} {
		if !strings.Contains(diff, fragment) {
			t.Fatalf("formatCommandTreeDiff() = %q, want fragment %q", diff, fragment)
		}
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
