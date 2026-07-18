package contractstaging_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractstaging"
)

func TestCheckReportsEveryDriftCategoryInDeterministicPathOrder(t *testing.T) {
	root := checkFixture(t)
	allowed := contractstaging.AllowedArtifacts()
	writeCheckFixture(t, root, allowed[0], "stale-z")
	writeCheckFixture(t, root, allowed[1], "stale-a")
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(allowed[2]))); err != nil {
		t.Fatalf("remove expected artifact: %v", err)
	}
	unexpected := []string{
		"packages/api/generated/ui/openapi.ts",
		"packages/api/generated/client.gen.go",
		"packages/api/generated/bin/you.exe",
		"packages/api/generated/.cache/contracts/data",
		"packages/api/generated/unrelated.txt",
	}
	for _, path := range unexpected {
		writeCheckFixture(t, root, path, path)
	}

	want := contractstaging.Drift{
		Stale:      []string{allowed[0], allowed[1]},
		Missing:    []string{allowed[2]},
		Unexpected: []string{unexpected[3], unexpected[2], unexpected[1], unexpected[0], unexpected[4]},
	}
	first, err := contractstaging.Check(root)
	if err != nil {
		t.Fatalf("first Check() error = %v", err)
	}
	second, err := contractstaging.Check(root)
	if err != nil {
		t.Fatalf("second Check() error = %v", err)
	}
	if !reflect.DeepEqual(first, want) || !reflect.DeepEqual(second, want) {
		t.Fatalf("Check() drift = %#v then %#v, want %#v", first, second, want)
	}
}

func TestCheckDistinguishesIndividualStaleMissingAndUnexpectedCases(t *testing.T) {
	allowed := contractstaging.AllowedArtifacts()
	tests := []struct {
		name   string
		mutate func(t *testing.T, root string)
		want   contractstaging.Drift
	}{
		{
			name: "stale",
			mutate: func(t *testing.T, root string) {
				writeCheckFixture(t, root, allowed[0], "stale")
			},
			want: contractstaging.Drift{Stale: []string{allowed[0]}},
		},
		{
			name: "missing",
			mutate: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, filepath.FromSlash(allowed[1]))); err != nil {
					t.Fatalf("remove expected artifact: %v", err)
				}
			},
			want: contractstaging.Drift{Missing: []string{allowed[1]}},
		},
		{
			name: "unexpected",
			mutate: func(t *testing.T, root string) {
				writeCheckFixture(t, root, "packages/api/generated/forbidden.txt", "forbidden")
			},
			want: contractstaging.Drift{Unexpected: []string{"packages/api/generated/forbidden.txt"}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := checkFixture(t)
			test.mutate(t, root)
			got, err := contractstaging.Check(root)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Check() drift = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestCheckPassesCleanStagingAndDoesNotChangeRepositoryBytes(t *testing.T) {
	root := checkFixture(t)
	writeCheckFixture(t, root, "contracts/testdata/fixture.json", "fixture")
	writeCheckFixture(t, root, "unrelated.txt", "unrelated")
	before := checkTree(t, root)

	drift, err := contractstaging.Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("Check() drift = %#v, want none", drift)
	}
	if after := checkTree(t, root); !reflect.DeepEqual(after, before) {
		t.Fatal("Check() changed repository bytes on success")
	}
}

func checkFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeCheckFixture(t, root, "contracts/common/documentation.schema.json", `{"$id":"https://schemas.portpowered.com/you/contracts/common/documentation.schema.json","$defs":{"itemId":{"type":"string"}}}`)
	writeCheckFixture(t, root, "contracts/common/deprecations.schema.json", `{"$id":"https://schemas.portpowered.com/you/contracts/common/deprecations.schema.json","properties":{"itemId":{"$ref":"https://schemas.portpowered.com/you/contracts/common/documentation.schema.json#/$defs/itemId"}}}`)
	writeCheckFixture(t, root, "contracts/manifest.schema.json", `{"$id":"https://schemas.portpowered.com/you/contracts/manifest.schema.json","properties":{"packageId":{"$ref":"https://schemas.portpowered.com/you/contracts/common/documentation.schema.json#/$defs/itemId"}}}`)
	for _, artifact := range contractstaging.RawArtifacts() {
		writeCheckFixture(t, root, artifact.Source, canonicalFixture(artifact.Source))
	}
	initCheckGitRepo(t, root)
	if err := contractstaging.Generate(root); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	return root
}

func canonicalFixture(path string) string {
	if path == "api/openapi.yaml" {
		return standaloneSchemaFixture()
	}
	return "canonical:" + path
}

func standaloneSchemaFixture() string {
	return "components:\n  schemas:\n    Factory:\n      type: object\n      properties:\n        child:\n          $ref: '#/components/schemas/Child'\n    Child:\n      type: string\n    FactoryEvent:\n      type: object\n      required: [schemaVersion, id, type, context, payload]\n      properties:\n        schemaVersion:\n          type: string\n          enum: [agent-factory.event.v1]\n        id:\n          type: string\n        type:\n          type: string\n          enum: [INITIAL_STRUCTURE_REQUEST]\n        context:\n          type: object\n        payload:\n          $ref: '#/components/schemas/Child'\n      discriminator:\n        propertyName: type\n        mapping:\n          INITIAL_STRUCTURE_REQUEST: '#/components/schemas/Child'\n    FactoryRecording:\n      type: object\n      required: [schemaVersion, sessionId, events]\n      properties:\n        schemaVersion:\n          type: string\n          enum: [agent-factory.recording.v1]\n        sessionId:\n          type: string\n        events:\n          type: array\n          items:\n            $ref: '#/components/schemas/FactoryEvent'\n"
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

const checkFixtureDefaultBranch = "main"

func initCheckGitRepo(t *testing.T, root string) {
	t.Helper()
	commands := [][]string{
		{"git", "-C", root, "init", "--initial-branch", checkFixtureDefaultBranch},
		{"git", "-C", root, "config", "user.email", "contractstaging-check@test"},
		{"git", "-C", root, "config", "user.name", "contractstaging-check"},
		{"git", "-C", root, "add", "-A"},
		{"git", "-C", root, "commit", "-m", "contract staging check fixture"},
	}
	for _, command := range commands {
		if output, err := exec.Command(command[0], command[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", command, output)
		}
	}
}

func localGitCloneURI(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve fixture path: %v", err)
	}
	slash := filepath.ToSlash(abs)
	if len(slash) >= 2 && slash[1] == ':' {
		return "file:///" + slash
	}
	return "file://" + slash
}
