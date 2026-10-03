package script_pollers

import (
	"fmt"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	cursorscopes "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cursorscopes"
)

const (
	GetCursorOperation    = "script_poller.get_cursor"
	CommitCursorOperation = "script_poller.commit_cursor"

	// ScriptPollerCursorEnvVar supplies the committed opaque cursor to a resumed
	// script-poller command.
	ScriptPollerCursorEnvVar = "INFINITE_YOU_SCRIPT_POLLER_CURSOR"
	// ScriptPollerCheckpointEnvVar supplies the committed opaque checkpoint to a
	// resumed script-poller command.
	ScriptPollerCheckpointEnvVar = "INFINITE_YOU_SCRIPT_POLLER_CHECKPOINT"
)

// ScriptPollerSupervision carries Automations-owned resume and concurrency facts
// for one supervised script-poller instance.
type ScriptPollerSupervision struct {
	AutomationID   string
	SourceID       string
	InstanceID     string
	ExpectedCursor automations.Cursor
	CursorScope    CursorScope
}

// Domain value aliases preserve script supervision's recovery values.
type CursorScope = cursorscopes.CursorScope
type CommitCursorRequest = cursorscopes.CommitCursorRequest
type ResumeCursor = cursorscopes.ResumeCursor

// CursorConflictError reports a stale or mismatched expected cursor.
func CursorConflictError(op string) error {
	return &automations.Error{
		Op:   op,
		Code: automations.ErrorCodeConflict,
		Err:  automations.ErrConflict,
	}
}

// CursorPersistError reports that cursor persistence failed after a successful
// poll cycle.
func CursorPersistError(err error) error {
	if err == nil {
		return nil
	}
	return &automations.Error{
		Op:   CommitCursorOperation,
		Code: automations.ErrorCodeFailed,
		Err:  fmt.Errorf("script poller cursor persistence failed: %w", err),
	}
}
