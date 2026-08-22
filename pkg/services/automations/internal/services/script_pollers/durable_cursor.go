package script_pollers

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

var _ CursorRecorder = (*durableCursorRecorder)(nil)

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
) (CursorRecorder, error) {
	if files == nil {
		return nil, errors.New("script poller cursor filesystem is required")
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
		return automations.GetCursorResult{}, invalidCursorOperationError(GetCursorOperation, "malformed instance identity")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	persisted, err := r.load(instanceID)
	if errors.Is(err, fs.ErrNotExist) {
		return automations.GetCursorResult{}, &automations.Error{
			Op:   GetCursorOperation,
			Code: automations.ErrorCodeNotFound,
			Err:  automations.ErrNotFound,
		}
	}
	if err != nil {
		return automations.GetCursorResult{}, durableCursorReadError(err)
	}
	if request.ExpectedCursor != "" && request.ExpectedCursor != persisted.Cursor {
		return automations.GetCursorResult{}, CursorConflictError(GetCursorOperation)
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
	request CommitCursorRequest,
) error {
	if err := cursorContextError(ctx); err != nil {
		return err
	}
	instanceID := strings.TrimSpace(request.InstanceID)
	if instanceID == "" || instanceID != request.InstanceID {
		return invalidCursorOperationError(CommitCursorOperation, "malformed instance identity")
	}
	if strings.TrimSpace(string(request.Cursor)) == "" {
		return invalidCursorOperationError(CommitCursorOperation, "cursor must be non-empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	current, err := r.load(instanceID)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read script poller cursor before commit: %w", err)
	}
	if request.ExpectedCursor != "" {
		if !exists || current.Cursor != request.ExpectedCursor {
			return CursorConflictError(CommitCursorOperation)
		}
	} else if exists && current.Cursor != "" {
		return CursorConflictError(CommitCursorOperation)
	}

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
		Op:   GetCursorOperation,
		Code: automations.ErrorCodeFailed,
		Err:  fmt.Errorf("read script poller cursor persistence failed: %w", err),
	}
}

func cursorContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
