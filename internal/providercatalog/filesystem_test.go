package providercatalog

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestGenerateInstallsValidatedArtifacts(t *testing.T) {
	root := t.TempDir()
	source := repositoryFixture(t)
	copyFixtureToDisk(t, root, source)
	expected, err := Build(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := Generate(root); err != nil {
		t.Fatal(err)
	}
	for name, payload := range expected.Files {
		actual, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(actual, payload) {
			t.Fatalf("generated %s differs from validated plan", name)
		}
	}
}

func copyFixtureToDisk(t *testing.T, root string, fixture fstest.MapFS) {
	t.Helper()
	for name, file := range fixture {
		target := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(target, file.Data, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}
