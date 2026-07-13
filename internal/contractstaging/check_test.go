package contractstaging_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractjoiner"
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
	if diagnostics := contractjoiner.Generate(contractstaging.JoinInput(root)); len(diagnostics) != 0 {
		t.Fatalf("Generate() diagnostics = %#v", diagnostics)
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
