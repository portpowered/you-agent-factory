//go:build windows

package filesystem

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

	changed, err := (Local{}).RemoveTree(ctx, parent, "model")
	if renameErr == nil {
		t.Fatal("rename-after-open succeeded; target handle did not hold the canonical name")
	}
	if err != nil || !changed {
		t.Fatalf("RemoveTree = changed %t, err %v; want deletion while rename is denied", changed, err)
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
	changed, err := (Local{}).RemoveTree(ctx, parent, "model")
	if !changed || !errors.Is(err, context.Canceled) {
		t.Fatalf("partial RemoveTree = changed %t, err %v; want cancellation after one handle deletion", changed, err)
	}
	if _, err := os.Stat(filepath.Join(target, "a")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("first asset = %v, want removed before cancellation", err)
	}
	if _, err := os.Stat(filepath.Join(target, "b")); err != nil {
		t.Fatalf("second asset = %v, want retry state", err)
	}

	changed, err = (Local{}).RemoveTree(context.Background(), parent, "model")
	if err != nil || !changed {
		t.Fatalf("retry RemoveTree = changed %t, err %v; want completion", changed, err)
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

	changed, err := (Local{}).RemoveTree(ctx, parent, "model")
	if parentRenameErr == nil || targetRenameErr == nil {
		t.Fatalf("replacement races succeeded: parent=%v target=%v", parentRenameErr, targetRenameErr)
	}
	if err != nil || !changed {
		t.Fatalf("RemoveTree = changed %t, err %v; want protected removal", changed, err)
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

	changed, err := (Local{}).RemoveTree(ctx, parent, "model")
	<-mutatorDone
	if err != nil || !changed {
		t.Fatalf("RemoveTree = changed %t, err %v; want successful removal", changed, err)
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
			changed, err := (Local{}).RemoveTree(context.Background(), parent, "model")
			if changed || err == nil {
				t.Fatalf("RemoveTree(%q) = changed %t, err %v; want malformed-parent rejection", parent, changed, err)
			}
		})
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
