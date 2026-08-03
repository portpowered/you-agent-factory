package service

import (
	"errors"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

// classifyError maps a returned chat_sessions error to a stable,
// boundary-safe classification string for structured logs. The
// classification never includes err.Error() text, since a wrapped
// *ValidationError/*ConflictError/*BusyError can carry a caller-supplied
// entity ID that a log field should still surface explicitly and
// intentionally, not via free-text error interpolation.
func classifyError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, chatsessions.ErrStaleVersion):
		return "conflict"
	case errors.Is(err, chatsessions.ErrBusy):
		return "busy"
	case errors.Is(err, chatsessions.ErrNotFound):
		return "not_found"
	case errors.Is(err, chatsessions.ErrInvalidTransition):
		return "invalid_transition"
	case errors.Is(err, chatsessions.ErrTargetEpisodeNotClosed),
		errors.Is(err, chatsessions.ErrTargetEpisodeNumberExhausted):
		return "invariant_violation"
	default:
		return "validation"
	}
}

// logStart records that op began for sessionID. Callers of the two logging
// helpers in this file never pass a working root, prompt content, raw JSON-RPC
// identity, credential, provider command, or filesystem path as a field --
// only operation names, entity identifiers, versions, and error
// classifications cross this boundary.
func (s *Store) logStart(op, sessionID string) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Debug("chat_sessions operation start", "op", op, "session_id", sessionID)
}

// logOutcome records the accepted or terminal result of op. On failure, err
// is classified and no other fields are logged since a failure path's
// zero-value result carries no accepted fact worth recording. On success,
// extra carries only safe entity identifiers and versions the caller
// supplies explicitly.
func (s *Store) logOutcome(op, sessionID string, err error, extra ...any) {
	if s == nil || s.logger == nil {
		return
	}
	if err != nil {
		s.logger.Info("chat_sessions operation outcome", "op", op, "session_id", sessionID, "error_class", classifyError(err))
		return
	}
	fields := append([]any{"op", op, "session_id", sessionID, "error_class", ""}, extra...)
	s.logger.Info("chat_sessions operation outcome", fields...)
}
