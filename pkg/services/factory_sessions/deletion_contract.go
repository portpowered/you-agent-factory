package factorysessions

import (
	"context"
	"errors"
	"strings"
)

// ErrSessionDeletionConflict identifies a deletion request that is unsafe for
// the selected Factory Session's current identity or runtime lifecycle.
var ErrSessionDeletionConflict = errors.New("factory session deletion conflicts with the current session state")

// SessionDeletionReason identifies the policy branch that refused deletion.
type SessionDeletionReason string

const (
	SessionDeletionReasonDefault       SessionDeletionReason = "DEFAULT_SESSION"
	SessionDeletionReasonRuntimeActive SessionDeletionReason = "RUNTIME_ACTIVE"
)

// SessionDeletionError is the typed Factory Sessions outcome for a deletion
// request that must not remove or stop the selected session.
type SessionDeletionError struct {
	SessionID string
	Reason    SessionDeletionReason
	Status    LifecycleStatus
	Message   string
}

func (err *SessionDeletionError) Error() string {
	if err == nil {
		return ""
	}
	if message := strings.TrimSpace(err.Message); message != "" {
		return message
	}
	if reason := strings.TrimSpace(string(err.Reason)); reason != "" {
		return reason
	}
	return ErrSessionDeletionConflict.Error()
}

func (err *SessionDeletionError) Unwrap() error { return ErrSessionDeletionConflict }

// LiveDeletionService is the owner-published capability used by the DELETE
// Factory Session route. It deliberately remains separate from
// LiveControlService.CloseFactorySession, whose destructive close operation is
// retained for internal process lifecycle teardown.
type LiveDeletionService interface {
	DeleteFactorySession(context.Context, string) error
}
