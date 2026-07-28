package script_pollers

import (
	"context"
	"fmt"
	"strings"
	"sync"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
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
}

// CursorRecorder persists opaque script-poller recovery facts without exposing
// private storage types to peers.
type CursorRecorder interface {
	GetCursor(context.Context, automations.GetCursorRequest) (automations.GetCursorResult, error)
	CommitCursor(context.Context, CommitCursorRequest) error
}

// CommitCursorRequest records one advanced opaque cursor/checkpoint fact.
type CommitCursorRequest struct {
	AutomationID   string
	InstanceID     string
	ExpectedCursor automations.Cursor
	Cursor         automations.Cursor
	Checkpoint     string
}

// ResumeCursor is the opaque recovery position supplied to a resumed command.
type ResumeCursor struct {
	Cursor     automations.Cursor
	Checkpoint string
}

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

type memoryCursorRecorder struct {
	mu      sync.RWMutex
	records map[string]cursorRecord
}

type cursorRecord struct {
	automationID string
	cursor       automations.Cursor
	checkpoint   string
}

// NewMemoryCursorRecorder constructs an inert in-memory cursor recorder.
func NewMemoryCursorRecorder() CursorRecorder {
	return &memoryCursorRecorder{
		records: make(map[string]cursorRecord),
	}
}

func (r *memoryCursorRecorder) GetCursor(
	_ context.Context,
	request automations.GetCursorRequest,
) (automations.GetCursorResult, error) {
	instanceID := strings.TrimSpace(request.InstanceID)
	if instanceID == "" || instanceID != request.InstanceID {
		return automations.GetCursorResult{}, invalidCursorOperationError(GetCursorOperation, "malformed instance identity")
	}

	r.mu.RLock()
	record, ok := r.records[instanceID]
	r.mu.RUnlock()
	if !ok {
		return automations.GetCursorResult{}, &automations.Error{
			Op:   GetCursorOperation,
			Code: automations.ErrorCodeNotFound,
			Err:  automations.ErrNotFound,
		}
	}
	if request.ExpectedCursor != "" && request.ExpectedCursor != record.cursor {
		return automations.GetCursorResult{}, CursorConflictError(GetCursorOperation)
	}
	return automations.GetCursorResult{
		AutomationID: record.automationID,
		InstanceID:   instanceID,
		Cursor:       record.cursor,
		Checkpoint:   record.checkpoint,
	}, nil
}

func (r *memoryCursorRecorder) CommitCursor(
	_ context.Context,
	request CommitCursorRequest,
) error {
	instanceID := strings.TrimSpace(request.InstanceID)
	if instanceID == "" || instanceID != request.InstanceID {
		return invalidCursorOperationError(CommitCursorOperation, "malformed instance identity")
	}
	if strings.TrimSpace(string(request.Cursor)) == "" {
		return invalidCursorOperationError(CommitCursorOperation, "cursor must be non-empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	current, exists := r.records[instanceID]
	if request.ExpectedCursor != "" {
		if !exists || current.cursor != request.ExpectedCursor {
			return CursorConflictError(CommitCursorOperation)
		}
	} else if exists && current.cursor != "" {
		return CursorConflictError(CommitCursorOperation)
	}

	r.records[instanceID] = cursorRecord{
		automationID: strings.TrimSpace(request.AutomationID),
		cursor:       request.Cursor,
		checkpoint:   request.Checkpoint,
	}
	return nil
}

func invalidCursorOperationError(op, message string) error {
	return &automations.Error{
		Op:   op,
		Code: automations.ErrorCodeInvalid,
		Err:  fmt.Errorf("%w: %s", automations.ErrInvalidRequest, message),
	}
}
