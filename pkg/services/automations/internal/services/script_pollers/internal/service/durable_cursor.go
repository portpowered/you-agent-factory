package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
)

const (
	durableCursorSchemaVersion = "automations-script-poller-cursor/v1"
	durableCursorStateDir      = ".infinite-you/poller-cursors"
)

// CursorPersistenceFileSystem is the exact filesystem effect needed by the
// durable script-poller cursor recorder. Path and replacement policy remain
// owned by Automations.
type CursorPersistenceFileSystem interface {
	ReadFile(string) ([]byte, error)
	MkdirAll(string, fs.FileMode) error
	WriteFile(string, []byte, fs.FileMode) error
	Rename(string, string) error
}

type durableCursorRecorder struct {
	dir   string
	files CursorPersistenceFileSystem
	mu    sync.RWMutex
}

var _ scriptpollers.CursorRecorder = (*durableCursorRecorder)(nil)

type persistedCursor struct {
	SchemaVersion string             `json:"schemaVersion"`
	AutomationID  string             `json:"automationId"`
	InstanceID    string             `json:"instanceId"`
	Cursor        automations.Cursor `json:"cursor"`
	Checkpoint    string             `json:"checkpoint,omitempty"`
}

// NewDurableCursorRecorder constructs an Automations-owned cursor recorder.
// baseDir is the runtime or factory directory beneath which the stable
// .infinite-you state convention is applied. Construction performs no IO.
func NewDurableCursorRecorder(
	baseDir string,
	files CursorPersistenceFileSystem,
) (scriptpollers.CursorRecorder, error) {
	if files == nil {
		return nil, errors.New("script poller cursor filesystem is required")
	}
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return nil, errors.New("script poller cursor base directory is required")
	}
	return &durableCursorRecorder{
		dir:   cursorStateDir(baseDir),
		files: files,
	}, nil
}

func cursorStateDir(baseDir string) string {
	return filepath.Join(strings.TrimSpace(baseDir), filepath.FromSlash(durableCursorStateDir))
}

func (r *durableCursorRecorder) GetCursor(
	ctx context.Context,
	request automations.GetCursorRequest,
) (automations.GetCursorResult, error) {
	if err := cursorContextError(ctx); err != nil {
		return automations.GetCursorResult{}, err
	}
	instanceID := strings.TrimSpace(request.InstanceID)
	if instanceID == "" || instanceID != request.InstanceID {
		return automations.GetCursorResult{}, invalidCursorOperationError(scriptpollers.GetCursorOperation, "malformed instance identity")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	persisted, err := r.load(instanceID)
	if errors.Is(err, fs.ErrNotExist) {
		return automations.GetCursorResult{}, &automations.Error{
			Op:   scriptpollers.GetCursorOperation,
			Code: automations.ErrorCodeNotFound,
			Err:  automations.ErrNotFound,
		}
	}
	if err != nil {
		return automations.GetCursorResult{}, durableCursorReadError(err)
	}
	if request.ExpectedCursor != "" && request.ExpectedCursor != persisted.Cursor {
		return automations.GetCursorResult{}, scriptpollers.CursorConflictError(scriptpollers.GetCursorOperation)
	}
	return automations.GetCursorResult{
		AutomationID: persisted.AutomationID,
		InstanceID:   persisted.InstanceID,
		Cursor:       persisted.Cursor,
		Checkpoint:   persisted.Checkpoint,
	}, nil
}

func (r *durableCursorRecorder) CommitCursor(
	ctx context.Context,
	request scriptpollers.CommitCursorRequest,
) error {
	if err := cursorContextError(ctx); err != nil {
		return err
	}
	instanceID, err := commitCursorInstanceID(request)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	current, exists, err := r.loadForCommit(instanceID)
	if err != nil {
		return err
	}
	if err := validateExpectedCursor(request.ExpectedCursor, current, exists); err != nil {
		return err
	}
	return r.persist(ctx, instanceID, request)
}

func commitCursorInstanceID(request scriptpollers.CommitCursorRequest) (string, error) {
	instanceID := strings.TrimSpace(request.InstanceID)
	if instanceID == "" || instanceID != request.InstanceID {
		return "", invalidCursorOperationError(scriptpollers.CommitCursorOperation, "malformed instance identity")
	}
	if strings.TrimSpace(string(request.Cursor)) == "" {
		return "", invalidCursorOperationError(scriptpollers.CommitCursorOperation, "cursor must be non-empty")
	}
	return instanceID, nil
}

func (r *durableCursorRecorder) loadForCommit(instanceID string) (persistedCursor, bool, error) {
	current, err := r.load(instanceID)
	if errors.Is(err, fs.ErrNotExist) {
		return persistedCursor{}, false, nil
	}
	if err != nil {
		return persistedCursor{}, false, fmt.Errorf("read script poller cursor before commit: %w", err)
	}
	return current, true, nil
}

func validateExpectedCursor(expected automations.Cursor, current persistedCursor, exists bool) error {
	if expected != "" && (!exists || current.Cursor != expected) {
		return scriptpollers.CursorConflictError(scriptpollers.CommitCursorOperation)
	}
	if expected == "" && exists && current.Cursor != "" {
		return scriptpollers.CursorConflictError(scriptpollers.CommitCursorOperation)
	}
	return nil
}

func (r *durableCursorRecorder) persist(
	ctx context.Context,
	instanceID string,
	request scriptpollers.CommitCursorRequest,
) error {
	persisted := persistedCursor{
		SchemaVersion: durableCursorSchemaVersion,
		AutomationID:  strings.TrimSpace(request.AutomationID),
		InstanceID:    instanceID,
		Cursor:        request.Cursor,
		Checkpoint:    request.Checkpoint,
	}
	payload, err := json.Marshal(persisted)
	if err != nil {
		return fmt.Errorf("encode script poller cursor: %w", err)
	}
	if err := r.files.MkdirAll(r.dir, 0o700); err != nil {
		return fmt.Errorf("create script poller cursor directory %q: %w", r.dir, err)
	}
	if err := cursorContextError(ctx); err != nil {
		return err
	}
	target := r.cursorPath(instanceID)
	temporary := target + ".tmp"
	if err := r.files.WriteFile(temporary, payload, 0o600); err != nil {
		return fmt.Errorf("write temporary script poller cursor %q: %w", temporary, err)
	}
	if err := cursorContextError(ctx); err != nil {
		return err
	}
	if err := r.files.Rename(temporary, target); err != nil {
		return fmt.Errorf("commit script poller cursor %q: %w", target, err)
	}
	return nil
}

func (r *durableCursorRecorder) load(instanceID string) (persistedCursor, error) {
	encoded, err := r.files.ReadFile(r.cursorPath(instanceID))
	if err != nil {
		return persistedCursor{}, err
	}
	var persisted persistedCursor
	if err := json.Unmarshal(encoded, &persisted); err != nil {
		return persistedCursor{}, fmt.Errorf("decode script poller cursor: %w", err)
	}
	if persisted.SchemaVersion != durableCursorSchemaVersion {
		return persistedCursor{}, fmt.Errorf(
			"unsupported script poller cursor schema version %q",
			persisted.SchemaVersion,
		)
	}
	if persisted.InstanceID != instanceID {
		return persistedCursor{}, fmt.Errorf(
			"script poller cursor instance identity mismatch: got %q, want %q",
			persisted.InstanceID,
			instanceID,
		)
	}
	if strings.TrimSpace(string(persisted.Cursor)) == "" {
		return persistedCursor{}, errors.New("script poller cursor is empty")
	}
	return persisted, nil
}

func (r *durableCursorRecorder) cursorPath(instanceID string) string {
	digest := sha256.Sum256([]byte(instanceID))
	return filepath.Join(r.dir, hex.EncodeToString(digest[:])+".json")
}

func durableCursorReadError(err error) error {
	return &automations.Error{
		Op:   scriptpollers.GetCursorOperation,
		Code: automations.ErrorCodeFailed,
		Err:  fmt.Errorf("read script poller cursor persistence failed: %w", err),
	}
}

func invalidCursorOperationError(op, message string) error {
	return &automations.Error{
		Op:   op,
		Code: automations.ErrorCodeInvalid,
		Err:  fmt.Errorf("%w: %s", automations.ErrInvalidRequest, message),
	}
}

func cursorContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
