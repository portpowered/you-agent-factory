package portablefiles

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
)

func TestResolveTargetStripsDomainPrefixAndContainsPath(t *testing.T) {
	root, err := PrepareValidationRoot(platformfilesystem.Local{}, t.TempDir())
	if err != nil {
		t.Fatalf("prepare root: %v", err)
	}
	target, err := ResolveTarget(root, "factory/scripts/run.sh", "factory")
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if got, want := target.Path(), filepath.Join(root.TargetDir(), "scripts", "run.sh"); got != want {
		t.Fatalf("target path = %q, want %q", got, want)
	}
}

func TestResolveTargetRejectsTraversal(t *testing.T) {
	root, err := PrepareValidationRoot(platformfilesystem.Local{}, t.TempDir())
	if err != nil {
		t.Fatalf("prepare root: %v", err)
	}
	if _, err := ResolveTarget(root, "../outside", "factory"); err == nil {
		t.Fatal("expected traversal to be rejected")
	}
}

func TestValidateFilesystemPathRejectsEscapingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks may require Windows developer mode")
	}
	rootDir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(rootDir, "scripts")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	root, err := PrepareValidationRoot(platformfilesystem.Local{}, rootDir)
	if err != nil {
		t.Fatalf("prepare root: %v", err)
	}
	target, err := ResolveTarget(root, "factory/scripts/run.sh", "factory")
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if err := ValidateFilesystemPath(platformfilesystem.Local{}, root, "factory/scripts/run.sh", target); err == nil {
		t.Fatal("expected escaping symlink to be rejected")
	}
}
