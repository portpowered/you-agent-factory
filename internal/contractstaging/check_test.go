package contractstaging_test

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractstaging"
)

func TestCheckPassesCleanStagingAndDoesNotChangeRepositoryBytes(t *testing.T) {
	defer contractstaging.LockRepositoryStagingForTest()()

	root := checkFixture(t)
	before := checkTree(t, root)

	drift, err := contractstaging.Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("Check() drift = %#v, want none", drift)
	}
	afterCheck := checkTree(t, root)
	if !reflect.DeepEqual(before, afterCheck) {
		t.Fatalf("Check() changed repository bytes on clean fixture: %v", changedCheckTreePaths(before, afterCheck))
	}

	if err := contractstaging.Generate(root); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	afterFirstGenerate := checkTree(t, root)
	if !reflect.DeepEqual(before, afterFirstGenerate) {
		t.Fatalf("Check() and first Generate() changed repository bytes on clean fixture: %v", changedCheckTreePaths(before, afterFirstGenerate))
	}

	if err := contractstaging.Generate(root); err != nil {
		t.Fatalf("second Generate() error = %v", err)
	}
	afterSecondGenerate := checkTree(t, root)
	if !reflect.DeepEqual(before, afterSecondGenerate) {
		t.Fatalf("second Generate() changed repository bytes on clean fixture: %v", changedCheckTreePaths(before, afterSecondGenerate))
	}
}

func changedCheckTreePaths(before, after map[string]string) []string {
	paths := make(map[string]struct{}, len(before)+len(after))
	for path := range before {
		paths[path] = struct{}{}
	}
	for path := range after {
		paths[path] = struct{}{}
	}

	changed := make([]string, 0)
	for path := range paths {
		if before[path] != after[path] {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	return changed
}

func TestCheckRejectsUnexpectedNonPackageArtifactPathsWhenOrderedAndSorted(t *testing.T) {
	root := checkFixture(t)
	contractstagingPath := filepath.Join(root, "packages", "api", "generated")
	path := filepath.Join(contractstagingPath, "unexpected.txt")
	if err := os.WriteFile(path, []byte("bad"), 0o644); err != nil {
		t.Fatalf("write unexpected fixture: %v", err)
	}
	writeCheckFixture(t, root, "packages/api/generated/ui/openapi.ts", "ui")

	drift, err := contractstaging.Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(drift.Unexpected) == 0 || !strings.Contains(drift.Unexpected[0], "packages/api/generated/") {
		t.Fatalf("Check() drift = %#v, want unexpected package artifact", drift)
	}
}

func checkFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	// The development manifest is commit-independent. Keep this a working-tree
	// fixture so Git's mutable metadata cannot enter the byte-stability proof.
	writeCheckFixture(t, root, "contracts/common/documentation.schema.json", `{"$id":"https://schemas.portpowered.com/you/contracts/common/documentation.schema.json","$defs":{"itemId":{"type":"string"}}}`)
	writeCheckFixture(t, root, "contracts/common/deprecations.schema.json", `{"$id":"https://schemas.portpowered.com/you/contracts/common/deprecations.schema.json","properties":{"itemId":{"$ref":"https://schemas.portpowered.com/you/contracts/common/documentation.schema.json#/$defs/itemId"}}}`)
	writeCheckFixture(t, root, "contracts/manifest.schema.json", `{"$id":"https://schemas.portpowered.com/you/contracts/manifest.schema.json","properties":{"packageId":{"$ref":"https://schemas.portpowered.com/you/contracts/common/documentation.schema.json#/$defs/itemId"}}}`)
	for _, artifact := range contractstaging.RawArtifacts() {
		writeCheckFixture(t, root, artifact.Source, canonicalFixture(artifact.Source))
	}
	if err := contractstaging.Generate(root); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	return root
}

func checkTree(t *testing.T, root string) map[string]string {
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

func writeCheckFixture(t *testing.T, root, path, contents string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(target, []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func canonicalFixture(path string) string {
	if path == "api/openapi.yaml" {
		return `components:
  schemas:
    Factory:
      type: object
      properties:
        child:
          $ref: '#/components/schemas/Child'
    Child:
      type: string
    FactoryEvent:
      type: object
      required: [type, payload]
      properties:
        type:
          type: string
          enum: [READY]
        payload:
          $ref: '#/components/schemas/ReadyPayload'
      discriminator:
        propertyName: type
        mapping:
          READY: '#/components/schemas/ReadyPayload'
    FactoryRecording:
      type: object
      properties:
        events:
          type: array
          items:
            $ref: '#/components/schemas/FactoryEvent'
    ReadyPayload:
      type: object
`
	}
	if path == "contracts/javascript/runtime-api.json" {
		return `{"sharedSchemas":{"javascript.schema.agent_run_spec":{"schema":{"type":"object","additionalProperties":false,"properties":{"prompt":{"type":"string"}},"required":["prompt"]}}}}`
	}
	return "canonical:" + path
}
