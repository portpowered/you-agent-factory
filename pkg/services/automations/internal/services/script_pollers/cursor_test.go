package script_pollers_test

import (
	"context"
	"errors"
	"testing"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
)

func TestMemoryCursorRecorder_CommitsAndReadsOpaqueFacts(t *testing.T) {
	t.Parallel()

	recorder := scriptpollers.NewMemoryCursorRecorder()
	ctx := context.Background()
	const (
		automationID = "workflow-cursor"
		instanceID   = "instance-cursor-1"
	)

	if err := recorder.CommitCursor(ctx, scriptpollers.CommitCursorRequest{
		AutomationID: automationID,
		InstanceID:   instanceID,
		Cursor:       "opaque-cursor-1",
		Checkpoint:   "checkpoint-1",
	}); err != nil {
		t.Fatalf("CommitCursor() error = %v", err)
	}

	cursor, err := recorder.GetCursor(ctx, automations.GetCursorRequest{
		InstanceID:     instanceID,
		ExpectedCursor: "opaque-cursor-1",
	})
	if err != nil {
		t.Fatalf("GetCursor() error = %v", err)
	}
	if cursor.AutomationID != automationID ||
		cursor.InstanceID != instanceID ||
		cursor.Cursor != "opaque-cursor-1" ||
		cursor.Checkpoint != "checkpoint-1" {
		t.Fatalf("GetCursor() = %+v, want automation/instance/cursor/checkpoint %q/%q/%q/%q",
			cursor, automationID, instanceID, "opaque-cursor-1", "checkpoint-1")
	}
}

func TestMemoryCursorRecorder_RejectsStaleExpectedCursor(t *testing.T) {
	t.Parallel()

	recorder := scriptpollers.NewMemoryCursorRecorder()
	ctx := context.Background()
	const instanceID = "instance-stale"

	if err := recorder.CommitCursor(ctx, scriptpollers.CommitCursorRequest{
		AutomationID: "workflow-stale",
		InstanceID:   instanceID,
		Cursor:       "cursor-current",
	}); err != nil {
		t.Fatalf("CommitCursor() error = %v", err)
	}

	_, err := recorder.GetCursor(ctx, automations.GetCursorRequest{
		InstanceID:     instanceID,
		ExpectedCursor: "cursor-stale",
	})
	assertAutomationsError(t, err, "script_poller.get_cursor", automations.ErrorCodeConflict, automations.ErrConflict)

	err = recorder.CommitCursor(ctx, scriptpollers.CommitCursorRequest{
		AutomationID:   "workflow-stale",
		InstanceID:     instanceID,
		ExpectedCursor: "cursor-stale",
		Cursor:         "cursor-next",
	})
	assertAutomationsError(t, err, "script_poller.commit_cursor", automations.ErrorCodeConflict, automations.ErrConflict)

	cursor, err := recorder.GetCursor(ctx, automations.GetCursorRequest{InstanceID: instanceID})
	if err != nil {
		t.Fatalf("GetCursor() after stale commit = %v", err)
	}
	if cursor.Cursor != "cursor-current" {
		t.Fatalf("cursor after stale commit = %q, want authoritative cursor-current", cursor.Cursor)
	}
}

func assertAutomationsError(
	t *testing.T,
	err error,
	op string,
	code automations.ErrorCode,
	target error,
) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s error = nil, want typed %s", op, code)
	}
	typed, ok := err.(*automations.Error)
	if !ok {
		t.Fatalf("%s error type = %T, want *automations.Error", op, err)
	}
	if typed.Op != op || typed.Code != code {
		t.Fatalf("%s error = %+v, want op=%q code=%q", op, typed, op, code)
	}
	if !errors.Is(err, target) {
		t.Fatalf("%s error = %v, want errors.Is %v", op, err, target)
	}
}
