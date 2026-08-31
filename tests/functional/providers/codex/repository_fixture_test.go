package codex

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
)

func TestMaterializeTrustedCodexRepositoryWritesDeterministicMetadata(t *testing.T) {
	dir := t.TempDir()
	if err := materializeTrustedCodexRepository(platformfilesystem.Local{}, dir); err != nil {
		t.Fatalf("materializeTrustedCodexRepository() error = %v", err)
	}

	refsHeadsDir := filepath.Join(dir, ".git", "refs", "heads")
	if info, err := os.Stat(refsHeadsDir); err != nil || !info.IsDir() {
		t.Fatalf("trusted repository refs/heads = (%v, %v), want directory", info, err)
	}
	for _, test := range []struct {
		name string
		path string
		want string
	}{
		{name: "HEAD", path: filepath.Join(dir, ".git", "HEAD"), want: codexTrustedRepositoryHEAD},
		{name: "config", path: filepath.Join(dir, ".git", "config"), want: codexTrustedRepositoryConfig},
	} {
		contents, err := os.ReadFile(test.path)
		if err != nil {
			t.Fatalf("read trusted repository %s: %v", test.name, err)
		}
		if string(contents) != test.want {
			t.Errorf("trusted repository %s = %q, want %q", test.name, contents, test.want)
		}
	}
}

func TestMaterializeTrustedCodexRepositoryReportsFilesystemActionAndPath(t *testing.T) {
	for _, test := range []struct {
		name       string
		failAction string
		wantAction string
		wantPath   string
		wantMkdirs int
		wantWrites int
	}{
		{name: "create refs/heads", failAction: "mkdir", wantAction: "create trusted Codex repository refs/heads directory", wantPath: filepath.Join(".git", "refs", "heads"), wantMkdirs: 1},
		{name: "write HEAD", failAction: "HEAD", wantAction: "write trusted Codex repository HEAD", wantPath: filepath.Join(".git", "HEAD"), wantMkdirs: 1, wantWrites: 1},
		{name: "write config", failAction: "config", wantAction: "write trusted Codex repository config", wantPath: filepath.Join(".git", "config"), wantMkdirs: 1, wantWrites: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			fileSystem := &failingCodexRepositoryFileSystem{failAction: test.failAction}
			err := materializeTrustedCodexRepository(fileSystem, filepath.FromSlash("fixture"))
			if err == nil {
				t.Fatal("materializeTrustedCodexRepository() error = nil, want filesystem failure")
			}
			wantPath := fmt.Sprintf("%q", filepath.Join(filepath.FromSlash("fixture"), test.wantPath))
			if !strings.Contains(err.Error(), test.wantAction) || !strings.Contains(err.Error(), wantPath) {
				t.Fatalf("materializeTrustedCodexRepository() error = %q, want action %q and path %q", err, test.wantAction, wantPath)
			}
			if fileSystem.mkdirCalls != test.wantMkdirs || fileSystem.writeCalls != test.wantWrites {
				t.Fatalf("filesystem operations = mkdir:%d write:%d, want mkdir:%d write:%d before failure", fileSystem.mkdirCalls, fileSystem.writeCalls, test.wantMkdirs, test.wantWrites)
			}
		})
	}
}

type failingCodexRepositoryFileSystem struct {
	failAction string
	mkdirCalls int
	writeCalls int
}

func (fileSystem *failingCodexRepositoryFileSystem) MkdirAll(path string, _ fs.FileMode) error {
	fileSystem.mkdirCalls++
	if fileSystem.failAction == "mkdir" {
		return fmt.Errorf("injected mkdir failure")
	}
	return nil
}

func (fileSystem *failingCodexRepositoryFileSystem) WriteFile(path string, _ []byte, _ fs.FileMode) error {
	fileSystem.writeCalls++
	if strings.HasSuffix(path, filepath.Join(".git", fileSystem.failAction)) {
		return fmt.Errorf("injected write failure")
	}
	return nil
}
