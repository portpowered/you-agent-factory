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

type pointerFileSystem struct {
	read     func(string) ([]byte, error)
	stat     func(string) (fs.FileInfo, error)
	mkdirErr error
	writeErr error
	writes   int
}

func (f *pointerFileSystem) ReadFile(path string) ([]byte, error)  { return f.read(path) }
func (f *pointerFileSystem) Stat(path string) (fs.FileInfo, error) { return f.stat(path) }
func (f *pointerFileSystem) MkdirAll(string, fs.FileMode) error    { return f.mkdirErr }
func (f *pointerFileSystem) WriteFile(string, []byte, fs.FileMode) error {
	f.writes++
	return f.writeErr
}

func TestCurrentPointerFailureDoesNotFallBackToDirectDefinition(t *testing.T) {
	t.Parallel()
	cause := errors.New("pointer read denied")
	for _, tc := range []struct {
		name, payload, message string
		readErr                error
	}{
		{name: "unreadable", readErr: cause, message: cause.Error()},
		{name: "empty", payload: " \n", message: "factory name is required"},
		{name: "unsafe", payload: "../escape", message: "invalid named factory name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fileSystem := &pointerFileSystem{
				read: func(string) ([]byte, error) { return []byte(tc.payload), tc.readErr },
				stat: func(string) (fs.FileInfo, error) {
					t.Fatal("invalid pointer must not inspect a fallback")
					return nil, nil
				},
			}
			resolver, err := New(fileSystem)
			if err != nil {
				t.Fatal(err)
			}
			got, err := resolver.ResolveCurrentDir("root")
			if got != "" || err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("ResolveCurrentDir = %q, %v; want %s", got, err, tc.message)
			}
			if tc.readErr != nil && !errors.Is(err, tc.readErr) {
				t.Fatalf("lost read cause: %v", err)
			}
		})
	}
}

func TestCurrentPointerWriteRetainsFilesystemFailures(t *testing.T) {
	t.Parallel()
	// Only the resolver is under test; Stat supplies a regular definition.
	definition := filepath.Join(t.TempDir(), "factory.json")
	if err := os.WriteFile(definition, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(definition)
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("filesystem denied")
	for _, stage := range []string{"stat", "mkdir", "write"} {
		t.Run(stage, func(t *testing.T) {
			fileSystem := &pointerFileSystem{stat: func(string) (fs.FileInfo, error) { return info, nil }}
			switch stage {
			case "stat":
				fileSystem.stat = func(string) (fs.FileInfo, error) { return nil, cause }
			case "mkdir":
				fileSystem.mkdirErr = cause
			case "write":
				fileSystem.writeErr = cause
			}
			resolver, err := New(fileSystem)
			if err != nil {
				t.Fatal(err)
			}
			if err := resolver.WriteCurrentPointer("root", "alpha"); !errors.Is(err, cause) {
				t.Fatalf("write error = %v", err)
			}
			wantWrites := 0
			if stage == "write" {
				wantWrites = 1
			}
			if fileSystem.writes != wantWrites {
				t.Fatalf("writes = %d, want %d", fileSystem.writes, wantWrites)
			}
		})
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

func TestRequireDefinitionDirRejectsAbsentOrNonRegularDefinition(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := testNamedPaths.RequireDefinitionDir(root); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing definition = %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, factoryConfigFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := testNamedPaths.RequireDefinitionDir(root); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("directory definition = %v", err)
	}
	if err := testNamedPaths.RequireDefinitionDir(" "); err == nil || err.Error() != "factory directory is required" {
		t.Fatalf("empty directory = %v", err)
	}
}
