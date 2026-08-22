package script_pollers_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	scriptpollerswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers/wire"
)

func TestDurableCursorRecorder_RestartRecoversExactOpaqueFacts(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	ctx := context.Background()
	const (
		automationID = "automation durable / exact"
		instanceID   = "script-poller-instance:durable-exact"
		cursor       = "opaque cursor / page=7?token=keep"
		checkpoint   = `{"last":"issue-7","updated":"2026-08-21T12:34:56Z"}`
	)

	first, err := scriptpollerswire.NewDurableCursorRecorder(baseDir, osFileSystem{})
	if err != nil {
		t.Fatalf("NewDurableCursorRecorder(first): %v", err)
	}
	if err := first.CommitCursor(ctx, scriptpollers.CommitCursorRequest{
		AutomationID: automationID,
		InstanceID:   instanceID,
		Cursor:       cursor,
		Checkpoint:   checkpoint,
	}); err != nil {
		t.Fatalf("first CommitCursor(): %v", err)
	}

	second, err := scriptpollerswire.NewDurableCursorRecorder(baseDir, osFileSystem{})
	if err != nil {
		t.Fatalf("NewDurableCursorRecorder(second): %v", err)
	}
	got, err := second.GetCursor(ctx, automations.GetCursorRequest{
		InstanceID:     instanceID,
		ExpectedCursor: cursor,
	})
	if err != nil {
		t.Fatalf("second GetCursor(): %v", err)
	}
	if got.AutomationID != automationID || got.InstanceID != instanceID ||
		got.Cursor != cursor || got.Checkpoint != checkpoint {
		t.Fatalf("second GetCursor() = %+v, want exact durable facts", got)
	}
}

func TestDurableCursorRecorder_PreservesOptimisticSemantics(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	recorder, err := scriptpollerswire.NewDurableCursorRecorder(baseDir, osFileSystem{})
	if err != nil {
		t.Fatalf("NewDurableCursorRecorder(): %v", err)
	}
	ctx := context.Background()
	const instanceID = "script-poller-instance:durable-conflict"
	if err := recorder.CommitCursor(ctx, scriptpollers.CommitCursorRequest{
		AutomationID: "automation-conflict",
		InstanceID:   instanceID,
		Cursor:       "cursor-current",
	}); err != nil {
		t.Fatalf("initial CommitCursor(): %v", err)
	}

	_, err = recorder.GetCursor(ctx, automations.GetCursorRequest{
		InstanceID:     instanceID,
		ExpectedCursor: "cursor-stale",
	})
	assertAutomationsError(t, err, scriptpollers.GetCursorOperation, automations.ErrorCodeConflict, automations.ErrConflict)

	err = recorder.CommitCursor(ctx, scriptpollers.CommitCursorRequest{
		AutomationID:   "automation-conflict",
		InstanceID:     instanceID,
		ExpectedCursor: "cursor-stale",
		Cursor:         "cursor-replacement",
	})
	assertAutomationsError(t, err, scriptpollers.CommitCursorOperation, automations.ErrorCodeConflict, automations.ErrConflict)

	got, err := recorder.GetCursor(ctx, automations.GetCursorRequest{InstanceID: instanceID})
	if err != nil {
		t.Fatalf("GetCursor() after stale replacement: %v", err)
	}
	if got.Cursor != "cursor-current" {
		t.Fatalf("cursor after stale replacement = %q, want cursor-current", got.Cursor)
	}
}

func TestDurableCursorRecorder_FailedReplacementLeavesLastCommitReadable(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	ctx := context.Background()
	const instanceID = "script-poller-instance:durable-failure"
	first, err := scriptpollerswire.NewDurableCursorRecorder(baseDir, osFileSystem{})
	if err != nil {
		t.Fatalf("NewDurableCursorRecorder(first): %v", err)
	}
	if err := first.CommitCursor(ctx, scriptpollers.CommitCursorRequest{
		AutomationID: "automation-failure",
		InstanceID:   instanceID,
		Cursor:       "cursor-committed",
		Checkpoint:   "checkpoint-committed",
	}); err != nil {
		t.Fatalf("initial CommitCursor(): %v", err)
	}

	persistErr := errors.New("rename destination is unavailable")
	failing, err := scriptpollerswire.NewDurableCursorRecorder(baseDir, osFileSystem{renameErr: persistErr})
	if err != nil {
		t.Fatalf("NewDurableCursorRecorder(failing): %v", err)
	}
	err = failing.CommitCursor(ctx, scriptpollers.CommitCursorRequest{
		AutomationID:   "automation-failure",
		InstanceID:     instanceID,
		ExpectedCursor: "cursor-committed",
		Cursor:         "cursor-not-committed",
		Checkpoint:     "checkpoint-not-committed",
	})
	if err == nil || !strings.Contains(err.Error(), persistErr.Error()) {
		t.Fatalf("failed CommitCursor() error = %v, want actionable persistence failure", err)
	}

	second, err := scriptpollerswire.NewDurableCursorRecorder(baseDir, osFileSystem{})
	if err != nil {
		t.Fatalf("NewDurableCursorRecorder(second): %v", err)
	}
	got, err := second.GetCursor(ctx, automations.GetCursorRequest{InstanceID: instanceID})
	if err != nil {
		t.Fatalf("GetCursor() after failed replacement: %v", err)
	}
	if got.Cursor != "cursor-committed" || got.Checkpoint != "checkpoint-committed" {
		t.Fatalf("cursor after failed replacement = %+v, want last committed values", got)
	}
}

func TestDurableCursorRecorder_ClassifiesCorruptState(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	recorder, err := scriptpollerswire.NewDurableCursorRecorder(baseDir, osFileSystem{})
	if err != nil {
		t.Fatalf("NewDurableCursorRecorder(): %v", err)
	}
	const instanceID = "script-poller-instance:durable-corrupt"
	if err := recorder.CommitCursor(context.Background(), scriptpollers.CommitCursorRequest{
		AutomationID: "automation-corrupt",
		InstanceID:   instanceID,
		Cursor:       "cursor-valid",
	}); err != nil {
		t.Fatalf("CommitCursor(): %v", err)
	}
	paths, err := filepath.Glob(filepath.Join(baseDir, ".infinite-you", "poller-cursors", "*.json"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("cursor state paths = %#v, glob error = %v, want one state file", paths, err)
	}
	if err := os.WriteFile(paths[0], []byte("not-json"), 0o600); err != nil {
		t.Fatalf("corrupt cursor state: %v", err)
	}

	_, err = recorder.GetCursor(context.Background(), automations.GetCursorRequest{InstanceID: instanceID})
	if err == nil {
		t.Fatal("GetCursor() on corrupt state = nil, want classified failure")
	}
	typed, ok := err.(*automations.Error)
	if !ok || typed.Op != scriptpollers.GetCursorOperation || typed.Code != automations.ErrorCodeFailed {
		t.Fatalf("corrupt state error = %#v, want typed failed cursor read error", err)
	}
	if !strings.Contains(err.Error(), "decode script poller cursor") {
		t.Fatalf("corrupt state error = %v, want decode context", err)
	}
}

type osFileSystem struct {
	renameErr error
}

func (osFileSystem) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (osFileSystem) MkdirAll(path string, mode os.FileMode) error {
	return os.MkdirAll(path, mode)
}

func (osFileSystem) WriteFile(path string, payload []byte, mode os.FileMode) error {
	return os.WriteFile(path, payload, mode)
}

func (f osFileSystem) Rename(oldPath, newPath string) error {
	if f.renameErr != nil {
		return f.renameErr
	}
	return os.Rename(oldPath, newPath)
}
