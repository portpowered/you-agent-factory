package contractjoiner_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractjoiner"
)

const joinedDirectory = "packages/api/generated/joined"

func TestGeneratePublishesCompleteDeterministicSetInsideJoinedBoundary(t *testing.T) {
	root := t.TempDir()
	writeGenerationFixture(t, root, "contracts/components/shared.json", `{"$id":"shared","type":"string"}`)
	writeGenerationFixture(t, root, "contracts/roots/z.json", `{"$id":"z","properties":{"value":{"$ref":"../components/shared.json"}}}`)
	writeGenerationFixture(t, root, "contracts/roots/a.json", `{"$id":"a","type":"object"}`)
	input := contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          []string{"contracts/roots/z.json", "contracts/roots/a.json"},
		Components:     []string{"contracts/components/shared.json"},
	}

	before := authoredGenerationBytes(t, root)
	if diagnostics := contractjoiner.Generate(input); len(diagnostics) != 0 {
		t.Fatalf("Generate() diagnostics = %#v, want none", diagnostics)
	}
	first := generatedTree(t, root)
	if diagnostics := contractjoiner.Generate(contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          []string{"contracts/roots/a.json", "contracts/roots/z.json"},
		Components:     []string{"contracts/components/shared.json"},
	}); len(diagnostics) != 0 {
		t.Fatalf("Generate(shuffled) diagnostics = %#v, want none", diagnostics)
	}
	second := generatedTree(t, root)

	if !equalByteTrees(first, second) {
		t.Fatalf("repeated generated tree changed:\nfirst=%q\nsecond=%q", first, second)
	}
	if len(second) != 2 {
		t.Fatalf("generated file count = %d, want 2", len(second))
	}
	joinedZ := filepath.ToSlash(filepath.Join(joinedDirectory, "contracts/roots/z.json"))
	if !bytes.Contains(second[joinedZ], []byte(`"$id": "shared"`)) {
		t.Fatalf("generated %s does not contain joined component: %s", joinedZ, second[joinedZ])
	}
	if after := authoredGenerationBytes(t, root); !equalByteTrees(before, after) {
		t.Fatalf("authored inputs changed:\nbefore=%q\nafter=%q", before, after)
	}
	if _, err := os.Stat(filepath.Join(root, "contracts", "roots", "joined")); !os.IsNotExist(err) {
		t.Fatalf("unexpected write outside generated boundary: %v", err)
	}
}

func TestGenerateFailurePreservesPreviousCompleteSet(t *testing.T) {
	root := t.TempDir()
	writeGenerationFixture(t, root, "contracts/root.json", `{"$id":"root","type":"object"}`)
	input := contractjoiner.Input{RepositoryRoot: root, Roots: []string{"contracts/root.json"}}
	if diagnostics := contractjoiner.Generate(input); len(diagnostics) != 0 {
		t.Fatalf("initial Generate() diagnostics = %#v, want none", diagnostics)
	}
	before := generatedTree(t, root)

	writeGenerationFixture(t, root, "contracts/broken.json", `{"$ref":"missing.json"}`)
	diagnostics := contractjoiner.Generate(contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          []string{"contracts/root.json", "contracts/broken.json"},
		Components:     []string{"contracts/missing.json"},
	})
	if len(diagnostics) != 1 || diagnostics[0].Code != "reference.missing" {
		t.Fatalf("Generate(failure) diagnostics = %#v, want stable missing-reference diagnostic", diagnostics)
	}
	if after := generatedTree(t, root); !equalByteTrees(before, after) {
		t.Fatalf("failed generation changed prior output:\nbefore=%q\nafter=%q", before, after)
	}
}

func TestGenerateUnsupportedReferenceKeywordPreservesPreviousCompleteSet(t *testing.T) {
	root := t.TempDir()
	writeGenerationFixture(t, root, "contracts/root.json", `{"$id":"root","type":"object"}`)
	input := contractjoiner.Input{RepositoryRoot: root, Roots: []string{"contracts/root.json"}}
	if diagnostics := contractjoiner.Generate(input); len(diagnostics) != 0 {
		t.Fatalf("initial Generate() diagnostics = %#v, want none", diagnostics)
	}
	before := generatedTree(t, root)

	writeGenerationFixture(t, root, "contracts/dynamic.json", `{"nested":{"$dynamicRef":"https://example.test/external.json#node"}}`)
	diagnostics := contractjoiner.Generate(contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          []string{"contracts/root.json", "contracts/dynamic.json"},
	})
	if len(diagnostics) != 1 || diagnostics[0].Code != "reference.unsupported" || diagnostics[0].Path != "/nested/$dynamicRef" {
		t.Fatalf("Generate(dynamic reference) diagnostics = %#v, want stable unsupported-reference diagnostic", diagnostics)
	}
	if after := generatedTree(t, root); !equalByteTrees(before, after) {
		t.Fatalf("failed generation changed prior output:\nbefore=%q\nafter=%q", before, after)
	}
}

func TestGenerateRequiresAtLeastOneRootWithoutChangingPreviousOutput(t *testing.T) {
	root := t.TempDir()
	writeGenerationFixture(t, root, "packages/api/generated/joined/existing.json", `{"existing":true}`)
	before := generatedTree(t, root)

	diagnostics := contractjoiner.Generate(contractjoiner.Input{RepositoryRoot: root})
	if len(diagnostics) != 1 || diagnostics[0].Code != "generation.input" {
		t.Fatalf("Generate(no roots) diagnostics = %#v, want generation.input", diagnostics)
	}
	if after := generatedTree(t, root); !equalByteTrees(before, after) {
		t.Fatalf("empty generation changed prior output:\nbefore=%q\nafter=%q", before, after)
	}
}

func TestGenerateRejectsSymlinkedOutputBoundaryOutsideRepository(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	writeGenerationFixture(t, root, "contracts/root.json", `{"type":"object"}`)
	packages := filepath.Join(root, "packages")
	if err := os.Symlink(external, packages); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	diagnostics := contractjoiner.Generate(contractjoiner.Input{
		RepositoryRoot: root,
		Roots:          []string{"contracts/root.json"},
	})
	if len(diagnostics) != 1 || diagnostics[0].Code != "generation.boundary" {
		t.Fatalf("Generate(external output) diagnostics = %#v, want generation.boundary", diagnostics)
	}
	entries, err := os.ReadDir(external)
	if err != nil {
		t.Fatalf("read external directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("generation wrote outside repository: %v", entries)
	}
}

func generatedTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	result := map[string][]byte{}
	base := filepath.Join(root, filepath.FromSlash(joinedDirectory))
	err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(relative)] = content
		return nil
	})
	if err != nil {
		t.Fatalf("walk generated tree: %v", err)
	}
	return result
}

func authoredGenerationBytes(t *testing.T, root string) map[string][]byte {
	t.Helper()
	result := map[string][]byte{}
	err := filepath.WalkDir(filepath.Join(root, "contracts"), func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(relative)] = content
		return nil
	})
	if err != nil {
		t.Fatalf("read authored inputs: %v", err)
	}
	return result
}

func equalByteTrees(left, right map[string][]byte) bool {
	if len(left) != len(right) {
		return false
	}
	for path, leftContent := range left {
		if !bytes.Equal(leftContent, right[path]) {
			return false
		}
	}
	return true
}

func writeGenerationFixture(t *testing.T, root, path, contents string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(target, []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
