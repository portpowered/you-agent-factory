package filesystem

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestLocalRemoveTreeRemovesOnlyNamedTargetAndIsIdempotent(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	if !secureTreeRemovalSupported {
		assertUnsupportedTreeRemoval(t, parent, "model")
		return
	}
	target := filepath.Join(parent, "model")
	if err := os.MkdirAll(filepath.Join(target, "revision", "nested"), 0o755); err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "revision", "nested", "weights.bin"), []byte("asset"), 0o644); err != nil {
		t.Fatalf("write target asset: %v", err)
	}
	sibling := filepath.Join(parent, "sibling")
	if err := os.WriteFile(sibling, []byte("preserve"), 0o644); err != nil {
		t.Fatalf("write sibling: %v", err)
	}

	changed, err := (Local{}).RemoveTree(context.Background(), parent, "model")
	if err != nil || !changed {
		t.Fatalf("RemoveTree = changed %t, err %v; want changed", changed, err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target = %v, want absent", err)
	}
	if _, err := os.Stat(parent); err != nil {
		t.Fatalf("parent = %v, want preserved", err)
	}
	if body, err := os.ReadFile(sibling); err != nil || string(body) != "preserve" {
		t.Fatalf("sibling = %q, %v; want preserved", body, err)
	}

	changed, err = (Local{}).RemoveTree(context.Background(), parent, "model")
	if err != nil || changed {
		t.Fatalf("repeated RemoveTree = changed %t, err %v; want unchanged", changed, err)
	}
}

func TestLocalRemoveTreeCancellationWinsOverParentAndTargetAbsence(t *testing.T) {
	t.Parallel()

	t.Run("parent absence", func(t *testing.T) {
		ctx := &absenceCancellingContext{cancelOn: 2}
		parent := filepath.Join(t.TempDir(), "not-created", "cache")
		changed, err := (Local{}).RemoveTree(ctx, parent, "model")
		if changed || !errors.Is(err, context.Canceled) {
			t.Fatalf("RemoveTree = changed %t, err %v; want cancellation before absent result", changed, err)
		}
	})

	t.Run("target absence", func(t *testing.T) {
		ctx := &absenceCancellingContext{cancelOn: 2}
		changed, err := (Local{}).RemoveTree(ctx, t.TempDir(), "model")
		if changed || !errors.Is(err, context.Canceled) {
			t.Fatalf("RemoveTree = changed %t, err %v; want cancellation before absent result", changed, err)
		}
	})
}

func TestLocalRemoveTreeFailsClosedOnTargetDirectoryLink(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	outside := t.TempDir()
	marker := filepath.Join(outside, "outside-marker")
	if err := os.WriteFile(marker, []byte("preserve"), 0o644); err != nil {
		t.Fatalf("write outside marker: %v", err)
	}
	link := filepath.Join(parent, "model")
	createDirectoryLink(t, link, outside)

	changed, err := (Local{}).RemoveTree(context.Background(), parent, "model")
	if err == nil || changed {
		t.Fatalf("RemoveTree = changed %t, err %v; want fail-closed unchanged", changed, err)
	}
	assertFileBody(t, marker, "preserve")
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("target link = %v, want untouched", err)
	}
}

func TestLocalRemoveTreeFailsClosedOnTargetSelfLink(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	link := filepath.Join(parent, "model")
	if err := os.Symlink(".", link); err != nil {
		if runtime.GOOS != "windows" {
			t.Fatalf("create self-directory link: %v", err)
		}
		junctionErr := exec.Command("cmd.exe", "/c", "mklink", "/J", link, parent).Run()
		if junctionErr != nil {
			t.Skipf("self-directory symlink/reparse capability unavailable on %s: symlink=%v; junction=%v", runtime.GOOS, err, junctionErr)
		}
	}
	t.Cleanup(func() { _ = os.Remove(link) })

	changed, err := (Local{}).RemoveTree(context.Background(), parent, "model")
	if err == nil || changed {
		t.Fatalf("RemoveTree = changed %t, err %v; want self-link fail closed", changed, err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("self-link = %v, want untouched", err)
	}
}

func TestLocalRemoveTreeRejectsTargetRegularFile(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	target := filepath.Join(parent, "model")
	if err := os.WriteFile(target, []byte("preserve"), 0o644); err != nil {
		t.Fatalf("write regular target: %v", err)
	}

	changed, err := (Local{}).RemoveTree(context.Background(), parent, "model")
	if err == nil || changed {
		t.Fatalf("RemoveTree = changed %t, err %v; want regular target rejection", changed, err)
	}
	assertFileBody(t, target, "preserve")
}

func TestLocalRemoveTreeFailsClosedOnCacheParentLink(t *testing.T) {
	t.Parallel()

	linkParent := t.TempDir()
	outside := t.TempDir()
	model := filepath.Join(outside, "model")
	if err := os.MkdirAll(model, 0o755); err != nil {
		t.Fatalf("create outside model: %v", err)
	}
	marker := filepath.Join(model, "outside-marker")
	if err := os.WriteFile(marker, []byte("preserve"), 0o644); err != nil {
		t.Fatalf("write outside marker: %v", err)
	}
	parentLink := filepath.Join(linkParent, "cache")
	createDirectoryLink(t, parentLink, outside)

	changed, err := (Local{}).RemoveTree(context.Background(), parentLink, "model")
	if err == nil || changed {
		t.Fatalf("RemoveTree = changed %t, err %v; want fail-closed unchanged", changed, err)
	}
	assertFileBody(t, marker, "preserve")
	if _, err := os.Lstat(parentLink); err != nil {
		t.Fatalf("cache parent link = %v, want untouched", err)
	}
}

func TestLocalRemoveTreeUnlinksChildLinkWithoutFollowingOutside(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	if !secureTreeRemovalSupported {
		assertUnsupportedTreeRemoval(t, parent, "model")
		return
	}
	target := filepath.Join(parent, "model")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("create target: %v", err)
	}
	outside := t.TempDir()
	marker := filepath.Join(outside, "outside-marker")
	if err := os.WriteFile(marker, []byte("preserve"), 0o644); err != nil {
		t.Fatalf("write outside marker: %v", err)
	}
	createDirectoryLink(t, filepath.Join(target, "redirect"), outside)

	changed, err := (Local{}).RemoveTree(context.Background(), parent, "model")
	if err != nil || !changed {
		t.Fatalf("RemoveTree = changed %t, err %v; want successful removal", changed, err)
	}
	assertFileBody(t, marker, "preserve")
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target = %v, want absent", err)
	}
}

func TestLocalRemoveTreeDoesNotFollowReplacedDirectoryLink(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	if !secureTreeRemovalSupported {
		assertUnsupportedTreeRemoval(t, parent, "model")
		return
	}
	target := filepath.Join(parent, "model")
	child := filepath.Join(target, "revision")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("create child: %v", err)
	}
	outside := t.TempDir()
	marker := filepath.Join(outside, "outside-marker")
	if err := os.WriteFile(marker, []byte("preserve"), 0o644); err != nil {
		t.Fatalf("write outside marker: %v", err)
	}
	original := filepath.Join(target, "revision-original")
	if err := os.Rename(child, original); err != nil {
		t.Fatalf("replace child before removal: %v", err)
	}
	createDirectoryLink(t, child, outside)

	changed, err := (Local{}).RemoveTree(context.Background(), parent, "model")
	if err != nil || !changed {
		t.Fatalf("RemoveTree = changed %t, err %v; want successful removal", changed, err)
	}
	assertFileBody(t, marker, "preserve")
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target = %v, want absent", err)
	}
}

func TestLocalRemoveTreeRejectsMalformedTargetNames(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"", ".", "..", "\x00", "model/child", `model\child`, "C:model", `C:\model`,
		`\\server\share`, `\\?\C:\model`, `\\.\C:\model`, " model ",
	} {
		name := name
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			marker := filepath.Join(parent, "sibling")
			if err := os.WriteFile(marker, []byte("preserve"), 0o644); err != nil {
				t.Fatalf("write sibling: %v", err)
			}
			changed, err := (Local{}).RemoveTree(context.Background(), parent, name)
			if err == nil || changed {
				t.Fatalf("RemoveTree(%q) = changed %t, err %v; want rejection", name, changed, err)
			}
			assertFileBody(t, marker, "preserve")
		})
	}
}

func createDirectoryLink(t *testing.T, link, target string) {
	t.Helper()
	if err := os.Symlink(target, link); err == nil {
		t.Cleanup(func() { _ = os.Remove(link) })
		return
	} else if runtime.GOOS != "windows" {
		t.Skipf("directory symlink capability unavailable on %s: %v", runtime.GOOS, err)
	} else {
		symlinkErr := err
		junctionErr := exec.Command("cmd.exe", "/c", "mklink", "/J", link, target).Run()
		if junctionErr == nil {
			t.Cleanup(func() { _ = os.Remove(link) })
			return
		}
		t.Skipf("directory symlink/reparse capability unavailable on %s: symlink=%v; junction=%v", runtime.GOOS, symlinkErr, junctionErr)
	}
}

func assertFileBody(t *testing.T, path, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	if string(body) != want {
		t.Fatalf("%q = %q, want %q", path, body, want)
	}
}

func assertUnsupportedTreeRemoval(t *testing.T, parent, target string) {
	t.Helper()
	changed, err := (Local{}).RemoveTree(context.Background(), parent, target)
	if changed || !errors.Is(err, errSecureTreeRemovalUnsupported) {
		t.Fatalf("RemoveTree = changed %t, err %v; want unsupported and unchanged", changed, err)
	}
}

type absenceCancellingContext struct {
	calls    int
	cancelOn int
}

func (ctx *absenceCancellingContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (ctx *absenceCancellingContext) Done() <-chan struct{}       { return nil }
func (ctx *absenceCancellingContext) Value(any) any               { return nil }
func (ctx *absenceCancellingContext) Err() error {
	ctx.calls++
	if ctx.calls >= ctx.cancelOn {
		return context.Canceled
	}
	return nil
}
