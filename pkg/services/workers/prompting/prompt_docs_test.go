package prompting

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestNewFactoryDocsLoader_RequiresInjectedFileSystem(t *testing.T) {
	if _, err := NewFactoryDocsLoader(nil); err == nil {
		t.Fatal("NewFactoryDocsLoader succeeded without filesystem")
	}
}

func TestLoadBundledDocContentsFromFactoryDir_PropagatesInjectedFileSystemError(t *testing.T) {
	want := errors.New("walk failed")
	fileSystem := factoryDocsFileSystemFake{
		stat:    func(string) (fs.FileInfo, error) { return factoryDocsFileInfo{name: "docs", directory: true}, nil },
		walkDir: func(string, fs.WalkDirFunc) error { return want },
	}
	_, err := loadBundledDocContentsFromFactoryDir("factory", fileSystem)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want injected filesystem error", err)
	}
}

func TestLoadBundledDocContentsFromFactoryDir_UsesOnlyInjectedFileSystem(t *testing.T) {
	var statPath, walkRoot, readPath string
	fileSystem := factoryDocsFileSystemFake{
		stat: func(path string) (fs.FileInfo, error) {
			statPath = path
			return factoryDocsFileInfo{name: "docs", directory: true}, nil
		},
		walkDir: func(root string, visit fs.WalkDirFunc) error {
			walkRoot = root
			return visit(filepath.Join(root, "overview.md"), fs.FileInfoToDirEntry(factoryDocsFileInfo{name: "overview.md"}), nil)
		},
		readFile: func(path string) ([]byte, error) {
			readPath = path
			return []byte("injected-content"), nil
		},
	}

	contents, err := loadBundledDocContentsFromFactoryDir("factory-root", fileSystem)
	if err != nil {
		t.Fatalf("load bundled docs: %v", err)
	}
	wantDocsDir := filepath.Join("factory-root", "docs")
	if statPath != wantDocsDir || walkRoot != wantDocsDir || readPath != filepath.Join(wantDocsDir, "overview.md") {
		t.Fatalf("injected calls = stat %q, walk %q, read %q; want docs-root paths", statPath, walkRoot, readPath)
	}
	if got := contents["factory/docs/overview.md"]; got != "injected-content" {
		t.Fatalf("content = %q, want injected-content", got)
	}
}

func TestNormalizeFactoryBundledDocTargetPaths_FiltersAndSorts(t *testing.T) {
	got := NormalizeFactoryBundledDocTargetPaths([]string{
		"factory/docs/guide.md",
		"factory/scripts/setup.py",
		"factory/docs/overview.md",
		"factory/docs/guide.md",
	})

	want := []string{"factory/docs/guide.md", "factory/docs/overview.md"}
	if len(got) != len(want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
	for index, targetPath := range want {
		if got[index] != targetPath {
			t.Fatalf("paths[%d] = %q, want %q", index, got[index], targetPath)
		}
	}
}

func TestLoadBundledDocContentsFromFactoryDir_LoadsDocs(t *testing.T) {
	factoryDir := t.TempDir()
	docsDir := filepath.Join(factoryDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "overview.md"), []byte("overview"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	contents, err := loadBundledDocContentsFromFactoryDir(factoryDir, platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("loadBundledDocContentsFromFactoryDir: %v", err)
	}
	if contents["factory/docs/overview.md"] != "overview" {
		t.Fatalf("contents = %#v, want overview doc content", contents)
	}
}

func mustFactoryDocsLoader(t *testing.T, fileSystem platformfilesystem.ReadFileTree) workers.FactoryDocsLoader {
	t.Helper()
	loader, err := NewFactoryDocsLoader(fileSystem)
	if err != nil {
		t.Fatalf("NewFactoryDocsLoader: %v", err)
	}
	return loader
}

type factoryDocsFileSystemFake struct {
	stat     func(string) (fs.FileInfo, error)
	walkDir  func(string, fs.WalkDirFunc) error
	readFile func(string) ([]byte, error)
}

func (fake factoryDocsFileSystemFake) Stat(path string) (fs.FileInfo, error) {
	return fake.stat(path)
}

func (fake factoryDocsFileSystemFake) WalkDir(root string, visit fs.WalkDirFunc) error {
	return fake.walkDir(root, visit)
}

func (fake factoryDocsFileSystemFake) ReadFile(path string) ([]byte, error) {
	return fake.readFile(path)
}

type factoryDocsFileInfo struct {
	name      string
	directory bool
}

func (info factoryDocsFileInfo) Name() string { return info.name }
func (factoryDocsFileInfo) Size() int64       { return 0 }
func (info factoryDocsFileInfo) Mode() fs.FileMode {
	if info.directory {
		return fs.ModeDir | 0o755
	}
	return 0o644
}
func (factoryDocsFileInfo) ModTime() time.Time { return time.Time{} }
func (info factoryDocsFileInfo) IsDir() bool   { return info.directory }
func (factoryDocsFileInfo) Sys() any           { return nil }
