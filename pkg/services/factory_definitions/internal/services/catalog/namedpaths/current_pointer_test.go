package namedpaths

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadCurrentPointerAndResolveCurrentDir(t *testing.T) {
	rootDir := t.TempDir()
	factoryDir, err := MapDir(rootDir, "@you/example")
	if err != nil {
		t.Fatalf("MapDir: %v", err)
	}
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(factoryDir, factoryConfigFile),
		[]byte(`{"name":"@you/example"}`),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
	if err := testNamedPaths.WriteCurrentPointer(rootDir, "@you/example"); err != nil {
		t.Fatalf("WriteCurrentPointer: %v", err)
	}

	name, err := testNamedPaths.ReadCurrentPointer(rootDir)
	if err != nil {
		t.Fatalf("ReadCurrentPointer: %v", err)
	}
	if name != "@you/example" {
		t.Fatalf("name = %q, want @you/example", name)
	}
	resolved, err := testNamedPaths.ResolveCurrentDir(rootDir)
	if err != nil {
		t.Fatalf("ResolveCurrentDir: %v", err)
	}
	if resolved != factoryDir {
		t.Fatalf("resolved = %q, want %q", resolved, factoryDir)
	}
}

func TestResolveCurrentDirFallsBackToDirectFactory(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(rootDir, factoryConfigFile),
		[]byte(`{"name":"direct"}`),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
	resolved, err := testNamedPaths.ResolveCurrentDir(rootDir)
	if err != nil {
		t.Fatalf("ResolveCurrentDir: %v", err)
	}
	if resolved != rootDir {
		t.Fatalf("resolved = %q, want %q", resolved, rootDir)
	}
}

func TestResolveCurrentDirClassifiesMissingLayout(t *testing.T) {
	_, err := testNamedPaths.ResolveCurrentDir(t.TempDir())
	if !errors.Is(err, ErrLayoutNotFound) {
		t.Fatalf("error = %v, want ErrLayoutNotFound", err)
	}
}
