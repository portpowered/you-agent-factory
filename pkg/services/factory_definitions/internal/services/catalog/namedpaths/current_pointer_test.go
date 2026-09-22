package namedpaths

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

func TestRemoveCurrentPointerRemovesSelectorAndIsIdempotent(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	factoryDir := filepath.Join(rootDir, "alpha")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(factory): %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, factoryConfigFile), []byte(`{"name":"alpha"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
	if err := testNamedPaths.WriteCurrentPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentPointer: %v", err)
	}

	if err := testNamedPaths.RemoveCurrentPointer(rootDir); err != nil {
		t.Fatalf("RemoveCurrentPointer: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, currentFactoryPointerFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pointer stat error = %v, want not-exist", err)
	}
	if err := testNamedPaths.RemoveCurrentPointer(rootDir); err != nil {
		t.Fatalf("RemoveCurrentPointer(missing): %v", err)
	}
}

func TestRemoveCurrentPointerRejectsInvalidRootAndUnsupportedFilesystem(t *testing.T) {
	t.Parallel()

	if err := testNamedPaths.RemoveCurrentPointer(" "); err == nil || !strings.Contains(err.Error(), "factory root is required") {
		t.Fatalf("RemoveCurrentPointer(empty root) = %v, want required-root error", err)
	}
	resolver, err := New(noRemoveCurrentPointerFileSystem{})
	if err != nil {
		t.Fatalf("New(no-removal filesystem): %v", err)
	}
	if err := resolver.RemoveCurrentPointer(t.TempDir()); err == nil || !strings.Contains(err.Error(), "does not support removal") {
		t.Fatalf("RemoveCurrentPointer(unsupported filesystem) = %v, want unsupported-removal error", err)
	}
}

func TestRemoveCurrentPointerRejectsNilResolver(t *testing.T) {
	t.Parallel()

	var resolver *Resolver
	if err := resolver.RemoveCurrentPointer(t.TempDir()); err == nil {
		t.Fatal("RemoveCurrentPointer(nil) error = nil, want required filesystem error")
	}
}

func TestWriteCurrentPointerReportsMissingDefinitionAndWriteFailure(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	if err := testNamedPaths.WriteCurrentPointer(rootDir, "missing"); err == nil || !strings.Contains(err.Error(), "find factory config") {
		t.Fatalf("WriteCurrentPointer(missing) = %v, want missing-config error", err)
	}

	factoryDir := filepath.Join(rootDir, "alpha")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(alpha): %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, factoryConfigFile), []byte(`{"name":"alpha"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(alpha factory): %v", err)
	}
	failingResolver, err := New(writeCurrentPointerErrorFileSystem{err: errors.New("pointer write unavailable")})
	if err != nil {
		t.Fatalf("New(failing filesystem): %v", err)
	}
	if err := failingResolver.WriteCurrentPointer(rootDir, "alpha"); err == nil || !strings.Contains(err.Error(), "write current factory pointer") {
		t.Fatalf("WriteCurrentPointer(write failure) = %v, want wrapped write error", err)
	}
}

func TestResolveExistingDirReportsMissingAndNonRegularDefinitions(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	missingDir := filepath.Join(rootDir, "missing")
	if err := os.MkdirAll(missingDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(missing): %v", err)
	}
	if _, err := testNamedPaths.ResolveExistingDir(rootDir, "missing"); err == nil || !strings.Contains(err.Error(), "existing target could not be loaded") {
		t.Fatalf("ResolveExistingDir(missing config) = %v, want existing-target diagnostic", err)
	}

	directoryConfig := filepath.Join(rootDir, "directory", factoryConfigFile)
	if err := os.MkdirAll(directoryConfig, 0o755); err != nil {
		t.Fatalf("MkdirAll(directory config): %v", err)
	}
	if _, err := testNamedPaths.ResolveExistingDir(rootDir, "directory"); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("ResolveExistingDir(directory config) = %v, want regular-file error", err)
	}
	if _, err := testNamedPaths.ResolveExistingDir(rootDir, "absent"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ResolveExistingDir(absent) = %v, want ErrNotFound", err)
	}
	if _, err := testNamedPaths.ResolveExistingDir(" ", "alpha"); err == nil || !strings.Contains(err.Error(), "factory root is required") {
		t.Fatalf("ResolveExistingDir(empty root) = %v, want required-root error", err)
	}
}

func TestRequireDefinitionDirCoversValidMissingAndNonRegularLayouts(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	validDir := filepath.Join(rootDir, "valid")
	if err := os.MkdirAll(validDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(valid): %v", err)
	}
	if err := os.WriteFile(filepath.Join(validDir, factoryConfigFile), []byte(`{"name":"valid"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(valid factory): %v", err)
	}
	if err := testNamedPaths.RequireDefinitionDir(validDir); err != nil {
		t.Fatalf("RequireDefinitionDir(valid): %v", err)
	}
	if err := testNamedPaths.RequireDefinitionDir(filepath.Join(rootDir, "missing")); err == nil || !strings.Contains(err.Error(), "find factory config") {
		t.Fatalf("RequireDefinitionDir(missing) = %v, want missing-config error", err)
	}
	directoryDir := filepath.Join(rootDir, "directory")
	if err := os.MkdirAll(filepath.Join(directoryDir, factoryConfigFile), 0o755); err != nil {
		t.Fatalf("MkdirAll(directory factory): %v", err)
	}
	if err := testNamedPaths.RequireDefinitionDir(directoryDir); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("RequireDefinitionDir(directory) = %v, want regular-file error", err)
	}
	if err := testNamedPaths.RequireDefinitionDir(" "); err == nil || !strings.Contains(err.Error(), "factory directory is required") {
		t.Fatalf("RequireDefinitionDir(empty) = %v, want required-directory error", err)
	}
}

type noRemoveCurrentPointerFileSystem struct{}

func (noRemoveCurrentPointerFileSystem) ReadFile(string) ([]byte, error)             { return nil, fs.ErrNotExist }
func (noRemoveCurrentPointerFileSystem) Stat(string) (fs.FileInfo, error)            { return nil, fs.ErrNotExist }
func (noRemoveCurrentPointerFileSystem) MkdirAll(string, fs.FileMode) error          { return nil }
func (noRemoveCurrentPointerFileSystem) WriteFile(string, []byte, fs.FileMode) error { return nil }

type writeCurrentPointerErrorFileSystem struct {
	noRemoveCurrentPointerFileSystem
	err error
}

func (f writeCurrentPointerErrorFileSystem) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

func (f writeCurrentPointerErrorFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}

func (f writeCurrentPointerErrorFileSystem) WriteFile(string, []byte, fs.FileMode) error {
	return f.err
}
