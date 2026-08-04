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
	case errors.Is(err, events.ErrOperationFailed):
		return "closed"
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

// logSubscribeRejected records that a Subscribe call was rejected after its
// starting cursor was evaluated against the topic (an unresolvable cursor
// beyond the live head): safe topic/from/limit context plus the error
// classification, never payload content.
func (st *Store) logSubscribeRejected(req events.SubscribeRequest, err error) {
	st.logger.Info("events subscribe outcome",
		"topic", string(req.Topic),
		"from", uint64(req.From.Position),
		"limit", req.Limit,
		"outcome", "rejected",
		"error_class", classifySubscribeError(err),
	)
}

// logSubscribeAccepted records that a Subscribe call was accepted: safe
// topic/from/limit context only.
func (st *Store) logSubscribeAccepted(req events.SubscribeRequest) {
	st.logger.Info("events subscribe outcome",
		"topic", string(req.Topic),
		"from", uint64(req.From.Position),
		"limit", req.Limit,
		"outcome", "accepted",
	)
}

// logSubscribeGap records that a new subscription's starting cursor named an
// evicted position: safe topic/requested/earliest-retained/head facts, never
// payload content.
func (st *Store) logSubscribeGap(topic events.Topic, gap *events.GapFacts) {
	st.logger.Info("events subscribe gap",
		"topic", string(topic),
		"requested", uint64(gap.Requested),
		"earliest_retained", uint64(gap.EarliestRetained),
		"head", uint64(gap.Head),
	)
}

// logSubscribeBackpressure records that a live subscriber's bounded buffer
// filled and the subscriber was terminated with DeliveryBackpressure: safe
// topic context only, logged exactly once at the point of detection.
func (st *Store) logSubscribeBackpressure(topic events.Topic) {
	st.logger.Info("events subscribe backpressure", "topic", string(topic))
}

// logSubscribeCanceled records that a Subscription observation ended because
// its context was canceled: safe topic context only.
func (st *Store) logSubscribeCanceled(topic events.Topic) {
	st.logger.Info("events subscribe canceled", "topic", string(topic))
}

// logSubscribeTopicClosed records that a topic's live subscribers were
// terminated with DeliveryClosed: safe topic context and the number of
// subscribers closed, logged exactly once per topic closure.
func (st *Store) logSubscribeTopicClosed(topic events.Topic, subscriberCount int) {
	st.logger.Info("events subscribe topic closed", "topic", string(topic), "subscriber_count", subscriberCount)
}

// logStoreClosed records the Store's own terminal shutdown outcome.
func (st *Store) logStoreClosed() {
	st.logger.Info("events store closed")
}

// classifySubscribeError maps a rejected Subscribe's error to a stable,
// boundary-safe classification string, never err.Error() text.
func classifySubscribeError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, events.ErrUnresolvableCursor):
		return "unresolvable_cursor"
	default:
		return "validation"
	}
}

// logAttachIntent records that req passed request-shape validation and
// AttachSource is about to evaluate it against source topic state. It fires
// only once ctx.Err() and req.Validate() have both succeeded, mirroring
// logAppendIntent's timing; a request rejected before that point (malformed,
// self-attachment, incompatible cursor, canceled context) never produces
// this log.
func (st *Store) logAttachIntent(req events.AttachSourceRequest) {
	st.logger.Debug("events attach intent",
		"destination", string(req.Destination),
		"source", string(req.Source),
		"mode", attachModeLabel(req.Mode),
	)
}

// logAttachOutcome records the terminal outcome of one AttachSource call:
// safe destination/source/mode context plus either a rejected classification
// or the accepted/already-attached outcome and resolved starting position.
// It never logs a forwarded record's payload.
func (st *Store) logAttachOutcome(req events.AttachSourceRequest, result events.AttachSourceResult, err error) {
	fields := []any{
		"destination", string(req.Destination),
		"source", string(req.Source),
		"mode", attachModeLabel(req.Mode),
	}
	if err != nil {
		fields = append(fields, "outcome", "rejected", "error_class", classifyAttachError(err))
		st.logger.Info("events attach outcome", fields...)
		return
	}

	outcome := "accepted"
	if result.Outcome == events.AttachOutcomeAlreadyAttached {
		outcome = "already_attached"
	}
	fields = append(fields, "outcome", outcome, "start_at", uint64(result.StartAt.Position))
	st.logger.Info("events attach outcome", fields...)
}

// logAttachTopicClosed records that a topic's outgoing attachment
// registrations were torn down by Store.Close(): safe topic context and the
// number of attachments removed, logged exactly once per topic closure.
func (st *Store) logAttachTopicClosed(topic events.Topic, attachmentCount int) {
	st.logger.Info("events attach topic closed", "topic", string(topic), "attachment_count", attachmentCount)
}

// classifyAttachError maps a rejected AttachSource's error to a stable,
// boundary-safe classification string, never err.Error() text.
func classifyAttachError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, events.ErrSelfAttachment):
		return "self_attachment"
	case errors.Is(err, events.ErrUnsupportedAttachMode):
		return "unsupported_mode"
	case errors.Is(err, events.ErrIncompatibleAttachmentCursor):
		return "incompatible_cursor"
	case errors.Is(err, events.ErrUnresolvableCursor):
		return "unresolvable_cursor"
	case errors.Is(err, events.ErrOperationFailed):
		return "closed"
	default:
		return "validation"
	}
}

// attachModeLabel maps mode to a stable log field value.
func attachModeLabel(mode events.AttachMode) string {
	switch mode {
	case events.AttachModeRetainedThenLive:
		return "retained_then_live"
	case events.AttachModeLiveOnly:
		return "live_only"
	default:
		return "unspecified"
	}
}
