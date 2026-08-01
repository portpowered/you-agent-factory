package service

import (
	"context"
	"fmt"
	"strings"
	"sync"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
)

const (
	getCursorOperation    = "script_poller.get_cursor"
	commitCursorOperation = "script_poller.commit_cursor"

	scriptPollerCursorEnvVar     = "INFINITE_YOU_SCRIPT_POLLER_CURSOR"
	scriptPollerCheckpointEnvVar = "INFINITE_YOU_SCRIPT_POLLER_CHECKPOINT"
)

type scriptPollerSupervision struct {
	automationID   string
	sourceID       string
	instanceID     string
	expectedCursor automations.Cursor
}

type cursorRecorder interface {
	GetCursor(context.Context, automations.GetCursorRequest) (automations.GetCursorResult, error)
	CommitCursor(context.Context, commitCursorRequest) error
}

type commitCursorRequest struct {
	automationID   string
	instanceID     string
	expectedCursor automations.Cursor
	cursor         automations.Cursor
	checkpoint     string
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

func newMemoryCursorRecorder() cursorRecorder {
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
		return automations.GetCursorResult{}, invalidCursorOperationError(getCursorOperation, "malformed instance identity")
	}

	r.mu.RLock()
	record, ok := r.records[instanceID]
	r.mu.RUnlock()
	if !ok {
		return automations.GetCursorResult{}, &automations.Error{
			Op:   getCursorOperation,
			Code: automations.ErrorCodeNotFound,
			Err:  automations.ErrNotFound,
		}
	}
	if request.ExpectedCursor != "" && request.ExpectedCursor != record.cursor {
		return automations.GetCursorResult{}, cursorConflictError(getCursorOperation)
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
	request commitCursorRequest,
) error {
	instanceID := strings.TrimSpace(request.instanceID)
	if instanceID == "" || instanceID != request.instanceID {
		return invalidCursorOperationError(commitCursorOperation, "malformed instance identity")
	}
	if strings.TrimSpace(string(request.cursor)) == "" {
		return invalidCursorOperationError(commitCursorOperation, "cursor must be non-empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	current, exists := r.records[instanceID]
	if request.expectedCursor != "" {
		if !exists || current.cursor != request.expectedCursor {
			return cursorConflictError(commitCursorOperation)
		}
	} else if exists && current.cursor != "" {
		return cursorConflictError(commitCursorOperation)
	}

	r.records[instanceID] = cursorRecord{
		automationID: strings.TrimSpace(request.automationID),
		cursor:       request.cursor,
		checkpoint:   request.checkpoint,
	}
	return nil
}

func cursorConflictError(op string) error {
	return &automations.Error{
		Op:   op,
		Code: automations.ErrorCodeConflict,
		Err:  automations.ErrConflict,
	}
}

func cursorPersistError(err error) error {
	if err == nil {
		return nil
	}
	return &automations.Error{
		Op:   commitCursorOperation,
		Code: automations.ErrorCodeFailed,
		Err:  fmt.Errorf("script poller cursor persistence failed: %w", err),
	}
}

func invalidCursorOperationError(op, message string) error {
	return &automations.Error{
		Op:   op,
		Code: automations.ErrorCodeInvalid,
		Err:  fmt.Errorf("%w: %s", automations.ErrInvalidRequest, message),
	}
}
