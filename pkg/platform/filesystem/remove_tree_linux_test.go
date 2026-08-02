//go:build linux

package filesystem

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxRemoveTreeFailsClosedBeforeAnyFilesystemMutation(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	target := filepath.Join(parent, "model")
	if err := os.MkdirAll(filepath.Join(target, "revision"), 0o755); err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "revision", "asset"), []byte("preserve"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(parent, "sibling"), []byte("preserve"), 0o644); err != nil {
		t.Fatalf("write sibling: %v", err)
	}

	changed, err := (Local{}).RemoveTree(context.Background(), parent, "model")
	if changed || !errors.Is(err, errSecureTreeRemovalUnsupported) {
		t.Fatalf("RemoveTree = changed %t, err %v; want unsupported and unchanged", changed, err)
	}
	assertLinuxPathPresent(t, target)
	assertLinuxPathPresent(t, filepath.Join(target, "revision", "asset"))
	assertLinuxPathPresent(t, filepath.Join(parent, "sibling"))
}

func TestLinuxRemoveTreeFailsClosedForLinkBoundaries(t *testing.T) {
	t.Parallel()

	t.Run("parent link", func(t *testing.T) {
		linkParent := t.TempDir()
		outside := t.TempDir()
		model := filepath.Join(outside, "model")
		if err := os.MkdirAll(model, 0o755); err != nil {
			t.Fatalf("create outside model: %v", err)
		}
		if err := os.WriteFile(filepath.Join(model, "marker"), []byte("preserve"), 0o644); err != nil {
			t.Fatalf("write outside marker: %v", err)
		}
		link := filepath.Join(linkParent, "cache")
		if err := os.Symlink(outside, link); err != nil {
			t.Fatalf("create parent link: %v", err)
		}

		changed, err := (Local{}).RemoveTree(context.Background(), link, "model")
		if changed || !errors.Is(err, errSecureTreeRemovalUnsupported) {
			t.Fatalf("RemoveTree = changed %t, err %v; want unsupported and unchanged", changed, err)
		}
		assertLinuxPathPresent(t, filepath.Join(model, "marker"))
		assertLinuxPathPresent(t, link)
	})

	t.Run("target link and self link", func(t *testing.T) {
		parent := t.TempDir()
		outside := t.TempDir()
		if err := os.WriteFile(filepath.Join(outside, "marker"), []byte("preserve"), 0o644); err != nil {
			t.Fatalf("write outside marker: %v", err)
		}
		if err := os.Symlink(outside, filepath.Join(parent, "model")); err != nil {
			t.Fatalf("create target link: %v", err)
		}

		changed, err := (Local{}).RemoveTree(context.Background(), parent, "model")
		if changed || !errors.Is(err, errSecureTreeRemovalUnsupported) {
			t.Fatalf("target link RemoveTree = changed %t, err %v; want unsupported and unchanged", changed, err)
		}
		assertLinuxPathPresent(t, filepath.Join(outside, "marker"))
		assertLinuxPathPresent(t, filepath.Join(parent, "model"))

		selfParent := t.TempDir()
		if err := os.Symlink(".", filepath.Join(selfParent, "model")); err != nil {
			t.Fatalf("create self link: %v", err)
		}
		changed, err = (Local{}).RemoveTree(context.Background(), selfParent, "model")
		if changed || !errors.Is(err, errSecureTreeRemovalUnsupported) {
			t.Fatalf("self link RemoveTree = changed %t, err %v; want unsupported and unchanged", changed, err)
		}
		assertLinuxPathPresent(t, filepath.Join(selfParent, "model"))
	})
}

func assertLinuxPathPresent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("path %q = %v, want preserved", path, err)
	}
}
