//go:build linux

package filesystem

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxRemoveTreeUsesNativeHandleRelativeRemoval(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	target := filepath.Join(parent, "model")
	if err := os.MkdirAll(filepath.Join(target, "revision", "nested"), 0o755); err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "revision", "nested", "asset"), []byte("remove"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	sibling := filepath.Join(parent, "sibling")
	if err := os.WriteFile(sibling, []byte("preserve"), 0o644); err != nil {
		t.Fatalf("write sibling: %v", err)
	}

	result, err := (Local{}).RemoveTree(context.Background(), parent, "model")
	if err != nil || result.State != RemoveTreeRemoved {
		t.Fatalf("native RemoveTree = %#v, err %v; want removed", result, err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target = %v, want absent", err)
	}
	if body, err := os.ReadFile(sibling); err != nil || string(body) != "preserve" {
		t.Fatalf("sibling = %q, %v; want preserved", body, err)
	}
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
		marker := filepath.Join(model, "marker")
		if err := os.WriteFile(marker, []byte("preserve"), 0o644); err != nil {
			t.Fatalf("write outside marker: %v", err)
		}
		link := filepath.Join(linkParent, "cache")
		if err := os.Symlink(outside, link); err != nil {
			t.Fatalf("create parent link: %v", err)
		}

		result, err := (Local{}).RemoveTree(context.Background(), link, "model")
		if result.State != RemoveTreeNotAttempted || err == nil {
			t.Fatalf("RemoveTree = %#v, err %v; want fail-closed parent link", result, err)
		}
		assertLinuxPathPresent(t, marker)
		assertLinuxPathPresent(t, link)
	})

	t.Run("target link", func(t *testing.T) {
		parent := t.TempDir()
		outside := t.TempDir()
		marker := filepath.Join(outside, "marker")
		if err := os.WriteFile(marker, []byte("preserve"), 0o644); err != nil {
			t.Fatalf("write outside marker: %v", err)
		}
		if err := os.Symlink(outside, filepath.Join(parent, "model")); err != nil {
			t.Fatalf("create target link: %v", err)
		}

		result, err := (Local{}).RemoveTree(context.Background(), parent, "model")
		if result.State != RemoveTreeNotAttempted || err == nil {
			t.Fatalf("target link RemoveTree = %#v, err %v; want fail-closed target link", result, err)
		}
		assertLinuxPathPresent(t, marker)
		assertLinuxPathPresent(t, filepath.Join(parent, "model"))
	})
}

func TestLinuxRemoveTreeStopsAtBoundedDepth(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "model")
	deep := target
	for index := 0; index < unixMaxRemovalDepth+2; index++ {
		deep = filepath.Join(deep, "level")
	}
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("create deep tree: %v", err)
	}

	result, err := (Local{}).RemoveTree(context.Background(), parent, "model")
	if err == nil || result.State != RemoveTreeRemaining {
		t.Fatalf("bounded-depth RemoveTree = %#v, err %v; want remaining error", result, err)
	}
	assertLinuxPathPresent(t, target)
}

func assertLinuxPathPresent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("path %q = %v, want preserved", path, err)
	}
}
