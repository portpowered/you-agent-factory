package service

import (
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/events"
)

// classifyAppendError maps a rejected Append's error to a stable,
// boundary-safe classification string for structured logs. The
// classification never includes err.Error() text: a caller-supplied field
// such as topic or source identity is already logged explicitly alongside
// it, so free-text error interpolation would only risk leaking payload
// content through a validation message.
func classifyAppendError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, events.ErrEmptyPayload), errors.Is(err, events.ErrMalformedPayloadJSON):
		return "invalid_payload"
	default:
		return "validation"
	}
}

// logAppendIntent records that req passed validation and is about to be
// committed. It fires only after Append's validation-before-effects checks
// succeed, so a rejected request never produces this log.
func (st *Store) logAppendIntent(req events.AppendRequest) {
	st.logger.Debug("events append intent",
		"topic", string(req.Topic),
		"source_type", string(req.SourceType),
		"source_id", string(req.SourceID),
		"source_sequence", uint64(req.SourceSequence),
		"source_event_id", string(req.SourceEventID),
	)
}

// logAppendOutcome records the terminal outcome of one Append call: safe,
// stable identity fields plus either a rejected classification or the
// accepted/duplicate outcome and assigned position. It never logs
// req.Payload or any other payload/source-content bytes.
func (st *Store) logAppendOutcome(req events.AppendRequest, result events.AppendResult, err error) {
	fields := []any{
		"topic", string(req.Topic),
		"source_type", string(req.SourceType),
		"source_id", string(req.SourceID),
		"source_sequence", uint64(req.SourceSequence),
		"source_event_id", string(req.SourceEventID),
	}
	if err != nil {
		fields = append(fields, "outcome", "rejected", "error_class", classifyAppendError(err))
		st.logger.Info("events append outcome", fields...)
		return
	}

	outcome := "accepted"
	if result.Outcome == events.AppendOutcomeDuplicate {
		outcome = "duplicate"
	}
	fields = append(fields, "outcome", outcome, "position", uint64(result.Record.ID.Position))
	st.logger.Info("events append outcome", fields...)
}

// logReadOutcome records the terminal outcome of one Read call: safe,
// stable topic/cursor context plus the explicit outcome Read observed. It
// fires only once a well-formed request has been evaluated (Read rejects a
// malformed request or canceled context before this call), and it never
// logs a Record's payload or other source content.
func (st *Store) logReadOutcome(req events.ReadRequest, result events.ReadResult) {
	fields := []any{
		"topic", string(req.Topic),
		"from", uint64(req.From.Position),
		"limit", req.Limit,
	}
	switch result.Outcome {
	case events.ReadOutcomeProgress:
		fields = append(fields, "outcome", "progress", "count", len(result.Records), "next", uint64(result.Next.Position))
	case events.ReadOutcomeAtHead:
		fields = append(fields, "outcome", "at_head", "head", uint64(result.Retained.Head))
	case events.ReadOutcomeInvalidCursor:
		fields = append(fields, "outcome", "invalid_cursor")
	case events.ReadOutcomeGap:
		fields = append(fields, "outcome", "gap", "earliest_retained", uint64(result.Gap.EarliestRetained), "head", uint64(result.Gap.Head))
	}
	st.logger.Info("events read outcome", fields...)
}
