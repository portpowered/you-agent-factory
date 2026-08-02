//go:build windows

package filesystem

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWindowsRemoveTreeDeniesRenameAfterOpen(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	target := filepath.Join(parent, "model")
	if err := os.MkdirAll(filepath.Join(target, "revision"), 0o755); err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "revision", "asset"), []byte("asset"), 0o644); err != nil {
		t.Fatalf("write target asset: %v", err)
	}
	moved := filepath.Join(t.TempDir(), "moved-model")
	var renameErr error
	ctx := &windowsRemovalContext{onCall: func(call int) {
		if call == 3 {
			renameErr = os.Rename(target, moved)
		}
	}}

	result, err := (Local{}).RemoveTree(ctx, parent, "model")
	if renameErr == nil {
		t.Fatal("rename-after-open succeeded; target handle did not hold the canonical name")
	}
	if err != nil || result.State != RemoveTreeRemoved {
		t.Fatalf("RemoveTree = result %#v, err %v; want deletion while rename is denied", result, err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canonical target = %v, want absent", err)
	}
	if _, err := os.Lstat(moved); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("moved target = %v, want no moved replacement", err)
	}
}

func TestWindowsRemoveTreePartialDeletionRetriesFromCanonicalHandle(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	target := filepath.Join(parent, "model")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("create target: %v", err)
	}
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(target, name), []byte(name), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	ctx := &windowsCancelAfterContext{cancelOn: 7}
	result, err := (Local{}).RemoveTree(ctx, parent, "model")
	if result.State != RemoveTreeRemaining || !errors.Is(err, context.Canceled) {
		t.Fatalf("partial RemoveTree = result %#v, err %v; want cancellation with remaining tree", result, err)
	}
	if _, err := os.Stat(filepath.Join(target, "a")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("first asset = %v, want removed before cancellation", err)
	}
	if _, err := os.Stat(filepath.Join(target, "b")); err != nil {
		t.Fatalf("second asset = %v, want retry state", err)
	}

	result, err = (Local{}).RemoveTree(context.Background(), parent, "model")
	if err != nil || result.State != RemoveTreeRemoved {
		t.Fatalf("retry RemoveTree = result %#v, err %v; want completion", result, err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target after retry = %v, want absent", err)
	}
	if _, err := os.Stat(parent); err != nil {
		t.Fatalf("parent after retry = %v, want preserved", err)
	}
}

func TestWindowsRemoveTreeRejectsParentAndTargetReplacementRaces(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	target := filepath.Join(parent, "model")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "asset"), []byte("remove"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	movedParent := filepath.Join(t.TempDir(), "moved-cache")
	replacement := filepath.Join(parent, "replacement")
	var parentRenameErr, targetRenameErr error
	ctx := &windowsRemovalContext{onCall: func(call int) {
		switch call {
		case 2:
			parentRenameErr = os.Rename(parent, movedParent)
		case 3:
			targetRenameErr = os.Rename(target, replacement)
		}
	}}

	result, err := (Local{}).RemoveTree(ctx, parent, "model")
	if parentRenameErr == nil || targetRenameErr == nil {
		t.Fatalf("replacement races succeeded: parent=%v target=%v", parentRenameErr, targetRenameErr)
	}
	if err != nil || result.State != RemoveTreeRemoved {
		t.Fatalf("RemoveTree = result %#v, err %v; want protected removal", result, err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target = %v, want absent", err)
	}
	if _, err := os.Lstat(replacement); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement = %v, want absent because rename was denied", err)
	}
}

func TestWindowsRemoveTreeRejectsConcurrentTargetNameMutation(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	target := filepath.Join(parent, "model")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "asset"), []byte("remove"), 0o644); err != nil {
		t.Fatalf("write target asset: %v", err)
	}
	moved := filepath.Join(t.TempDir(), "moved-model")
	var attempts atomic.Int32
	var successes atomic.Int32
	attemptStarted := make(chan struct{})
	mutatorDone := make(chan struct{})
	mutatorStarted := false
	ctx := &windowsRemovalContext{onCall: func(call int) {
		if call != 3 || mutatorStarted {
			return
		}
		mutatorStarted = true
		go func() {
			defer close(mutatorDone)
			for index := 0; index < 256; index++ {
				attempts.Add(1)
				if index == 0 {
					close(attemptStarted)
				}
				if err := os.Rename(target, moved); err == nil {
					successes.Add(1)
					return
				}
			}
		}()
		<-attemptStarted
	}}

	result, err := (Local{}).RemoveTree(ctx, parent, "model")
	<-mutatorDone
	if err != nil || result.State != RemoveTreeRemoved {
		t.Fatalf("RemoveTree = result %#v, err %v; want successful removal", result, err)
	}
	if attempts.Load() == 0 {
		t.Fatal("concurrent mutator made no target-name mutation attempt")
	}
	if successes.Load() != 0 {
		t.Fatalf("concurrent target rename succeeded %d times", successes.Load())
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canonical target = %v, want absent", err)
	}
	if _, err := os.Lstat(moved); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("moved target = %v, want absent", err)
	}
}

func TestWindowsRemoveTreeRejectsMalformedParentPrefixes(t *testing.T) {
	t.Parallel()

	for _, parent := range []string{
		`relative-cache`, `C:relative-cache`, `C:\cache\..\outside`,
		`\\server\share\cache`, `\\?\C:\cache`, `\\.\C:\cache`,
	} {
		parent := parent
		t.Run(parent, func(t *testing.T) {
			result, err := (Local{}).RemoveTree(context.Background(), parent, "model")
			if result.State != RemoveTreeNotAttempted || err == nil {
				t.Fatalf("RemoveTree(%q) = result %#v, err %v; want malformed-parent rejection", parent, result, err)
			}
		})
	}
}

func TestWindowsRemoveTreeRejectsOverlargeParentComponents(t *testing.T) {
	t.Parallel()

	deepComponents := make([]string, windowsMaxParentDepth+1)
	for index := range deepComponents {
		deepComponents[index] = "cache"
	}
	longDepthParent := `C:\` + strings.Join(deepComponents, `\`)
	longComponentParent := `C:\` + strings.Repeat("x", windowsMaxParentNameBytes+1)
	longPathParent := `C:\` + strings.Repeat("cache\\", windowsMaxParentPathBytes)

	for _, parent := range []string{longDepthParent, longComponentParent, longPathParent} {
		result, err := (Local{}).RemoveTree(context.Background(), parent, "model")
		if result.State != RemoveTreeNotAttempted || err == nil {
			t.Fatalf("RemoveTree(%q) = result %#v, err %v; want bounded-parent rejection", parent, result, err)
		}
	}
}

func TestWindowsRemoveTreeUsesCaseSensitiveDirectoryLookup(t *testing.T) {
	parent := t.TempDir()
	if err := exec.Command("fsutil.exe", "file", "SetCaseSensitiveInfo", parent, "enable").Run(); err != nil {
		t.Skipf("case-sensitive directory support unavailable: %v", err)
	}
	lower := filepath.Join(parent, "model")
	upper := filepath.Join(parent, "MODEL")
	for _, root := range []string{lower, upper} {
		if err := os.MkdirAll(filepath.Join(root, "revision"), 0o755); err != nil {
			t.Fatalf("create %s: %v", root, err)
		}
	}
	if err := os.WriteFile(filepath.Join(lower, "revision", "remove"), []byte("remove"), 0o644); err != nil {
		t.Fatalf("write lower target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(upper, "revision", "preserve"), []byte("preserve"), 0o644); err != nil {
		t.Fatalf("write upper target: %v", err)
	}

	result, err := (Local{}).RemoveTree(context.Background(), parent, "model")
	if err != nil || result.State != RemoveTreeRemoved {
		t.Fatalf("case-sensitive RemoveTree = %#v, err %v; want removed lower target", result, err)
	}
	if _, err := os.Lstat(lower); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lower target = %v, want absent", err)
	}
	if _, err := os.Stat(filepath.Join(upper, "revision", "preserve")); err != nil {
		t.Fatalf("upper target = %v, want preserved", err)
	}
}

func TestWindowsRemoveTreeAcceptsUniqueCaseInsensitiveTargetName(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "Model")
	if err := os.MkdirAll(filepath.Join(target, "revision"), 0o755); err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "revision", "asset"), []byte("remove"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	result, err := (Local{}).RemoveTree(context.Background(), parent, "model")
	if err != nil || result.State != RemoveTreeRemoved {
		t.Fatalf("case-insensitive RemoveTree = %#v, err %v; want removed unique target", result, err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target = %v, want absent", err)
	}
}

func TestWindowsRemoveTreeStopsAtBoundedDepth(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "model")
	deep := target
	for index := 0; index < windowsMaxRemovalDepth+2; index++ {
		deep = filepath.Join(deep, "level")
	}
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("create deep tree: %v", err)
	}

	result, err := (Local{}).RemoveTree(context.Background(), parent, "model")
	if err == nil || result.State != RemoveTreeRemaining {
		t.Fatalf("bounded-depth RemoveTree = %#v, err %v; want remaining error", result, err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("target after depth bound = %v, want retryable tree", err)
	}
}

type windowsRemovalContext struct {
	calls  int
	onCall func(int)
}

type windowsCancelAfterContext struct {
	calls    int
	cancelOn int
}

func (ctx *windowsCancelAfterContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (ctx *windowsCancelAfterContext) Done() <-chan struct{}       { return nil }
func (ctx *windowsCancelAfterContext) Value(any) any               { return nil }
func (ctx *windowsCancelAfterContext) Err() error {
	ctx.calls++
	if ctx.calls >= ctx.cancelOn {
		return context.Canceled
	}
	return nil
}

func (ctx *windowsRemovalContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (ctx *windowsRemovalContext) Done() <-chan struct{}       { return nil }
func (ctx *windowsRemovalContext) Value(any) any               { return nil }
func (ctx *windowsRemovalContext) Err() error {
	ctx.calls++
	if ctx.onCall != nil {
		ctx.onCall(ctx.calls)
	}
	return nil
}
