package main

import (
	"bytes"
	"crypto/sha256"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractstaging"
)

func TestRunGeneratesApprovedArtifactsReproduciblyWithoutChangingOtherFiles(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "contracts/common/documentation.schema.json", `{"$id":"https://schemas.portpowered.com/you/contracts/common/documentation.schema.json","$defs":{"itemId":{"type":"string"}}}`)
	writeFixture(t, root, "contracts/common/deprecations.schema.json", `{"$id":"https://schemas.portpowered.com/you/contracts/common/deprecations.schema.json","properties":{"itemId":{"$ref":"https://schemas.portpowered.com/you/contracts/common/documentation.schema.json#/$defs/itemId"}}}`)
	writeFixture(t, root, "contracts/manifest.schema.json", `{"$id":"https://schemas.portpowered.com/you/contracts/manifest.schema.json","properties":{"packageId":{"$ref":"https://schemas.portpowered.com/you/contracts/common/documentation.schema.json#/$defs/itemId"}}}`)
	writeRawArtifactSources(t, root)
	initGitRepo(t, root)
	protected := []string{
		"contracts/testdata/fixture.json",
		"pkg/generatedclient/client.gen.go",
		"ui/src/api/generated/openapi.ts",
		"bin/you.exe",
		".cache/contracts/data",
		"unrelated.txt",
	}
	for _, path := range protected {
		writeFixture(t, root, path, "protected:"+path)
	}
	before := fileDigests(t, root, protected)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if status := run(root, stdout, stderr); status != 0 || stderr.Len() != 0 {
		t.Fatalf("first run() = %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	first := fileDigests(t, root, contractstaging.AllowedArtifacts())
	stdout.Reset()
	if status := run(root, stdout, stderr); status != 0 || stderr.Len() != 0 {
		t.Fatalf("second run() = %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	second := fileDigests(t, root, contractstaging.AllowedArtifacts())

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated generation changed artifact digests:\nfirst=%x\nsecond=%x", first, second)
	}
	if after := fileDigests(t, root, protected); !reflect.DeepEqual(before, after) {
		t.Fatalf("generation changed protected files:\nbefore=%x\nafter=%x", before, after)
	}
	assertOnlyAllowedArtifacts(t, root)
}

func TestRunPropagatesAComponentChangeToEveryConsumer(t *testing.T) {
	root := t.TempDir()
	documentation := "contracts/common/documentation.schema.json"
	writeFixture(t, root, documentation, `{"$id":"https://schemas.portpowered.com/you/contracts/common/documentation.schema.json","$defs":{"itemId":{"type":"string"}}}`)
	writeFixture(t, root, "contracts/common/deprecations.schema.json", `{"properties":{"itemId":{"$ref":"https://schemas.portpowered.com/you/contracts/common/documentation.schema.json#/$defs/itemId"}}}`)
	writeFixture(t, root, "contracts/manifest.schema.json", `{"properties":{"packageId":{"$ref":"https://schemas.portpowered.com/you/contracts/common/documentation.schema.json#/$defs/itemId"}}}`)
	writeRawArtifactSources(t, root)
	initGitRepo(t, root)
	if status := run(root, &bytes.Buffer{}, &bytes.Buffer{}); status != 0 {
		t.Fatalf("initial run() = %d", status)
	}
	before := fileDigests(t, root, contractstaging.AllowedArtifacts())

	writeFixture(t, root, documentation, `{"$id":"https://schemas.portpowered.com/you/contracts/common/documentation.schema.json","$defs":{"itemId":{"type":"integer"}}}`)
	if status := run(root, &bytes.Buffer{}, &bytes.Buffer{}); status != 0 {
		t.Fatalf("changed run() = %d", status)
	}
	after := fileDigests(t, root, contractstaging.AllowedArtifacts())

	for _, path := range contractstaging.AllowedArtifacts() {
		if !bytes.Contains([]byte(path), []byte("/joined/")) || bytes.Contains([]byte(path), []byte("documentation.schema.json")) {
			continue
		}
		if before[path] == after[path] {
			t.Errorf("component change did not change joined consumer %s", path)
		}
	}
}

func assertOnlyAllowedArtifacts(t *testing.T, root string) {
	t.Helper()
	want := contractstaging.AllowedArtifacts()
	var got []string
	base := filepath.Join(root, "packages", "api", "generated")
	if err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		got = append(got, filepath.ToSlash(relative))
		return nil
	}); err != nil {
		t.Fatalf("walk generated artifacts: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("generated paths = %q, want allowlist %q", got, want)
	}
}

func writeRawArtifactSources(t *testing.T, root string) {
	t.Helper()
	for _, artifact := range contractstaging.RawArtifacts() {
		contents := "canonical:" + artifact.Source
		if artifact.Source == "api/openapi.yaml" {
			contents = "components:\n  schemas:\n    Factory:\n      type: object\n      properties:\n        child:\n          $ref: '#/components/schemas/Child'\n    Child:\n      type: string\n    FactoryEvent:\n      type: object\n      required: [schemaVersion, id, type, context, payload]\n      properties:\n        schemaVersion:\n          type: string\n        id:\n          type: string\n        type:\n          type: string\n        context:\n          type: object\n        payload:\n          $ref: '#/components/schemas/Child'\n      discriminator:\n        propertyName: type\n        mapping:\n          TEST: '#/components/schemas/Child'\n    FactoryRecording:\n      type: object\n      required: [schemaVersion, sessionId, events]\n      properties:\n        schemaVersion:\n          type: string\n        sessionId:\n          type: string\n        events:\n          type: array\n          items:\n            $ref: '#/components/schemas/FactoryEvent'\n"
		}
		writeFixture(t, root, artifact.Source, contents)
	}
}

func fileDigests(t *testing.T, root string, paths []string) map[string][sha256.Size]byte {
	t.Helper()
	digests := make(map[string][sha256.Size]byte, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		digests[path] = sha256.Sum256(content)
	}
	return digests
}

func writeFixture(t *testing.T, root, path, contents string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(target, []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func initGitRepo(t *testing.T, root string) {
	t.Helper()
	commands := [][]string{
		{"git", "-C", root, "init"},
		{"git", "-C", root, "config", "user.email", "contracts-generate@test"},
		{"git", "-C", root, "config", "user.name", "contracts-generate"},
		{"git", "-C", root, "add", "-A"},
		{"git", "-C", root, "commit", "-m", "contract generation fixture"},
	}
	for _, command := range commands {
		if output, err := exec.Command(command[0], command[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", command, output)
		}
	}
}
