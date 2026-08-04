package stdio

// prompt_stream.go implements the ACP-L1-V2-T03 consumer side of
// chat-session streaming: attaching to a Chat Session, draining its
// aggregate topic in order, projecting each record, and delivering it
// before the V1 fallback (deliverPromptUpdates). It intentionally does not
// implement the producer side -- nothing in this repository yet calls
// chatsessions.Service.Sequence (and its required follow-up
// chatsessions.Service.AdvanceStreamHead) to put a Factory response
// workers.Draft onto chat-session/<id>/events in the first place. That
// production bridge is out of this transport-owned story's scope (the
// governing PRD's own top-level acceptance criteria bar this package from
// adding "Factory Sessions event logic"), so until a future story delivers
// it, streamTurnUpdates always observes an empty topic in production and
// every real turn keeps falling back to the unchanged V1 synchronous final
// text below. Test fixtures exercise the streaming path directly by calling
// Sequence/AdvanceStreamHead themselves, standing in for that future bridge.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/mapping"
)

// retainedReadBatchLimit bounds one events.Service.Read call this transport
// issues while draining a Chat Session's aggregate stream. It is a plain
// paging size, not a policy on how much history a turn may ever observe:
// streamTurnUpdates loops until Read reports ReadOutcomeAtHead.
const retainedReadBatchLimit = 64

// attachmentCache remembers one connection's already-registered
// chatsessions.Attachment per Chat Session for the lifetime of one
// serveConnection invocation, so a later turn on the same session reuses the
// same attachment (and its already-advanced delivery cursor) instead of
// registering a fresh one -- Attach itself is not idempotent by
// ConnectionID/SessionID (see chatsessions.Service.Attach), so this
// transport is the one that must not call it more than once per session per
// connection. It is carried request-scoped through context.Context (see
// contextWithAttachmentCache), mirroring the existing promptNotifier
// pattern in session_prompt.go, rather than as an explicit parameter every
// handler and existing test would otherwise have to accept. It needs no
// synchronization of its own: serveConnection dispatches every request on
// one connection strictly sequentially (see its own doc comment), so no two
// goroutines ever observe the same attachmentCache instance concurrently.
type attachmentCache struct {
	bySession map[string]chatsessions.Attachment
}

// attachmentCacheContextKey is the unexported context key attachmentCache is
// carried under, so no other package can inject or observe it.
type attachmentCacheContextKey struct{}

// contextWithAttachmentCache attaches cache to ctx for the duration of one
// connection.
func contextWithAttachmentCache(ctx context.Context, cache *attachmentCache) context.Context {
	return context.WithValue(ctx, attachmentCacheContextKey{}, cache)
}

// attachmentCacheFromContext retrieves the cache attached by
// contextWithAttachmentCache, or nil when ctx carries none -- for example
// every existing unit test in this package that calls admitPromptTurn or
// dispatchFactoryTurn directly with context.Background().
func attachmentCacheFromContext(ctx context.Context) *attachmentCache {
	cache, _ := ctx.Value(attachmentCacheContextKey{}).(*attachmentCache)
	return cache
}

// get reports the cached Attachment for sessionID, or ok=false when this
// connection has not attached to sessionID yet (including when c is nil, the
// no-cache-attached case).
func (c *attachmentCache) get(sessionID string) (attachment chatsessions.Attachment, ok bool) {
	if c == nil {
		return chatsessions.Attachment{}, false
	}
	attachment, ok = c.bySession[sessionID]
	return attachment, ok
}

// set records a's identity as this connection's attachment for sessionID. A
// nil c is a no-op, matching a nil promptNotifier's no-op convention
// elsewhere in this package.
func (c *attachmentCache) set(sessionID string, a chatsessions.Attachment) {
	if c == nil {
		return
	}
	if c.bySession == nil {
		c.bySession = make(map[string]chatsessions.Attachment)
	}
	c.bySession[sessionID] = a
}

// detachAttachments releases every attachment this connection registered
// through ensureAttachment, best-effort: it is deferred once, when
// serveConnection's one connection-scoped invocation ends (see server.go),
// so a disconnected connection never leaves an attachment permanently
// registered against a session it will never resume. It runs against
// context.WithoutCancel(ctx) so detachment still completes when ctx already
// carries the cancellation or error that ended the connection -- disconnect
// must always release the delivery consumer regardless of why the
// connection ended. It calls only chatsessions.Service.Detach, never
// RequestControl, AdvanceTurn, or any other turn-mutating operation: Detach
// removes just the named delivery consumer (see its own contract on
// chatsessions.Service), so any turn still active on the session keeps
// running unaffected, for any other attachment still observing it. A
// per-session detach failure is logged as a bounded, payload-free
// diagnostic (matching Serve's own outcome logging) and does not stop the
// remaining sessions in cache from being released.
func (s *Server) detachAttachments(ctx context.Context, cache *attachmentCache) {
	if s == nil || s.chatSessions == nil || cache == nil {
		return
	}
	detachCtx := context.WithoutCancel(ctx)
	logger := logging.EnsureLogger(s.logger)
	for sessionID, attachment := range cache.bySession {
		if _, err := s.chatSessions.Detach(detachCtx, chatsessions.DetachRequest{
			SessionID:    sessionID,
			AttachmentID: attachment.ID,
		}); err != nil {
			logger.Warn("acp stdio attachment detach failed", "sessionId", sessionID, "outcome", terminalOutcomeLabel(err))
		}
	}
}

// errStreamGapEncountered marks a cursor events.Read could not resolve at
// all (events.ReadOutcomeInvalidCursor) -- a foreign or otherwise
// unresolvable position, distinct from an ordinary retention gap
// (events.ReadOutcomeGap), which streamTurnUpdates instead recovers from by
// delivering an explicit gap notice and resuming retained catch-up (see
// deliverReadTimeGap). It never reaches a client verbatim --
// classifyDependencyFailure maps it to a bounded internal-error response the
// same way it maps every other dependency failure.
var errStreamGapEncountered = errors.New("acp: chat session stream cursor could not be resolved")

// errMalformedSequencedEnvelope marks a committed Events record on a Chat
// Session's aggregate topic whose payload does not decode as the
// chatsessions.SequencedItem envelope Sequence always commits. It never
// reaches a client verbatim.
var errMalformedSequencedEnvelope = errors.New("acp: chat session aggregate record is not a well-formed sequenced item envelope")

// ensureAttachment returns this connection's already-registered Attachment
// for sessionID, reusing attachmentCacheFromContext(ctx) when a prior turn
// on this same connection already attached, or registers a new one via
// chatSessions.Attach otherwise. connectionID identifies the caller for a
// freshly registered Attachment; a blank connectionID (a request identity
// with no connection pairing, such as a transport-minted RequestIdentity)
// never attaches and reports ok=false so a caller can safely skip streaming
// for this turn rather than fail it outright.
func (s *Server) ensureAttachment(ctx context.Context, connectionID, sessionID string) (attachment chatsessions.Attachment, ok bool, err error) {
	cache := attachmentCacheFromContext(ctx)
	if a, cached := cache.get(sessionID); cached {
		return a, true, nil
	}
	if connectionID == "" {
		return chatsessions.Attachment{}, false, nil
	}

	result, err := s.chatSessions.Attach(ctx, chatsessions.AttachRequest{
		SessionID:    sessionID,
		ConnectionID: connectionID,
		Interactive:  true,
	})
	if err != nil {
		return chatsessions.Attachment{}, false, err
	}
	cache.set(sessionID, result.Attachment)
	return result.Attachment, true, nil
}

// streamTurnUpdates attaches (or reuses this connection's existing
// attachment) to sessionID's chat-session events topic and drains every
// currently retained record after the attachment's own delivery cursor:
// each record is decoded as a chatsessions.SequencedItem, reconstructed into
// the workers.Draft mapping.Project expects (mapping.DraftFromSequencedItem),
// projected, and -- for a declared projectable outcome -- delivered through
// notify in strictly increasing aggregate order before the attachment's
// cursor advances past it. It returns whether at least one
// agent_message_chunk was delivered, the signal deliverPromptUpdates uses to
// suppress the V1 synchronous final-text fallback.
//
// A record whose projection or notification fails stops the drain
// immediately without acknowledging that record, so a later attempt can
// retry it; the returned error classifies as a bounded dependency failure
// through the same path every other "session/prompt" dependency failure
// uses. A record that projects and notifies successfully but that
// AcknowledgeAttachment rejects with *chatsessions.AttachmentPositionError
// is not itself a failure: it means the session's StreamHead (the position
// the Chat Session aggregate sequencer -- not this transport -- durably
// commits to) has not yet advanced past this record. Advancing StreamHead is
// producer-side bookkeeping this L1 V2 transport slice does not own (see
// chatsessions.Service.AdvanceStreamHead's own contract, and
// docs/internal/projects/acp-client/final-proposal.md §5.2's "explicit
// optimistic Chat Session operations advance StreamHead and attachment
// AfterSequence independently"), so streamTurnUpdates simply stops for this
// call, having already delivered what it safely could; a later turn's drain
// resumes from the same unacknowledged cursor position once StreamHead
// catches up, so no record is ever lost or silently skipped.
//
// s.events == nil (a Server constructed without the Events collaborator --
// for example a narrower slice test that never exercises streaming, or a
// deployment that has not yet wired an Events collaborator) is a no-op
// success: it reports no delivered message so deliverPromptUpdates falls
// back to the existing V1 synchronous final text unchanged.
func (s *Server) streamTurnUpdates(
	ctx context.Context,
	connectionID, sessionID string,
	sessionVersion uint64,
	notify promptNotifier,
) (deliveredMessage bool, err error) {
	if s.events == nil {
		return false, nil
	}

	attachment, ok, err := s.ensureAttachment(ctx, connectionID, sessionID)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	topic := chatsessions.EventsTopic(sessionID)
	cursor := events.Cursor{Topic: topic, Position: events.AggregateSequence(attachment.AfterSequence)}

	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return deliveredMessage, ctxErr
		}

		read, readErr := s.events.Read(ctx, events.ReadRequest{Topic: topic, From: cursor, Limit: retainedReadBatchLimit})
		if readErr != nil {
			return deliveredMessage, readErr
		}

		switch read.Outcome {
		case events.ReadOutcomeAtHead:
			return deliveredMessage, nil
		case events.ReadOutcomeInvalidCursor:
			return deliveredMessage, errStreamGapEncountered
		case events.ReadOutcomeGap:
			stop, gapErr := s.deliverReadTimeGap(ctx, sessionID, sessionVersion, &attachment, read.Gap, notify)
			if gapErr != nil {
				return deliveredMessage, gapErr
			}
			if stop {
				return deliveredMessage, nil
			}
			cursor = events.Cursor{Topic: topic, Position: events.AggregateSequence(read.Gap.EarliestRetained) - 1}
		case events.ReadOutcomeProgress:
			stop, delivered, drainErr := s.drainRecords(ctx, sessionID, sessionVersion, &attachment, read.Records, notify)
			deliveredMessage = deliveredMessage || delivered
			if drainErr != nil {
				return deliveredMessage, drainErr
			}
			if stop {
				return deliveredMessage, nil
			}
			cursor = read.Next
		default:
			return deliveredMessage, fmt.Errorf("acp: events read returned unexpected outcome %d", read.Outcome)
		}
	}
}

// deliverPromptUpdates delivers startResult's turn output to the connection
// that admitted it: it first drains any canonical chat-session records
// available for startResult.Session.ID through streamTurnUpdates, and only
// when that drain delivered no canonical agent_message_chunk falls back to
// the V1 synchronous final text built from fallbackText (dispatched.outcome.Text
// in dispatchFactoryTurn) via deliverPromptText -- so a turn whose canonical
// message output was already streamed never also emits a duplicate final-only
// notification, while a turn with no canonical message output (including
// every turn today, since no production caller sequences Factory Draft
// records onto the chat-session topic yet -- see prompt_stream.go's package
// doc) keeps its existing V1 behavior unchanged.
func (s *Server) deliverPromptUpdates(
	ctx context.Context,
	startResult chatsessions.StartTurnResult,
	sessionVersion uint64,
	reqIdentity chatsessions.RequestIdentity,
	fallbackText []string,
) error {
	notify := promptNotifierFromContext(ctx)

	deliveredMessage, err := s.streamTurnUpdates(ctx, reqIdentity.ConnectionID, startResult.Session.ID, sessionVersion, notify)
	if err != nil {
		return err
	}
	if deliveredMessage {
		return nil
	}
	return deliverPromptText(notify, startResult.Session.ID, fallbackText)
}

// drainRecords projects and delivers each of records in strictly increasing
// order, advancing *attachment's acknowledged cursor after each one this
// call has safely observed and persisting that advance into this
// connection's attachmentCache immediately -- not only once the whole drain
// finishes -- so a notifier failure part-way through (see
// TestStreamTurnUpdatesNotifierFailureLeavesRecordRetryable) still keeps
// every record already acknowledged before the failure out of a later
// retry's redelivery, and a later turn on the same connection
// (ensureAttachment's cache.get) always resumes from this attachment's
// latest known position rather than the position it had when this drain
// started. stop reports that the drain should end without an error -- the
// StreamHead-lag case documented on streamTurnUpdates.
func (s *Server) drainRecords(
	ctx context.Context,
	sessionID string,
	sessionVersion uint64,
	attachment *chatsessions.Attachment,
	records []events.Record,
	notify promptNotifier,
) (stop bool, deliveredMessage bool, err error) {
	cache := attachmentCacheFromContext(ctx)
	for _, rec := range records {
		var item chatsessions.SequencedItem
		if unmarshalErr := json.Unmarshal(rec.Payload, &item); unmarshalErr != nil {
			return false, deliveredMessage, fmt.Errorf("%w: %v", errMalformedSequencedEnvelope, unmarshalErr)
		}

		update, projErr := mapping.Project(mapping.DraftFromSequencedItem(item))
		if projErr != nil {
			return false, deliveredMessage, projErr
		}

		if update != nil {
			if notify != nil {
				if notifyErr := notify(acpsdk.SessionNotification{
					SessionId: acpsdk.SessionId(sessionID),
					Update:    *update,
				}); notifyErr != nil {
					return false, deliveredMessage, notifyErr
				}
			}
			if update.AgentMessageChunk != nil {
				deliveredMessage = true
			}
		}

		ackResult, ackErr := s.chatSessions.AcknowledgeAttachment(ctx, chatsessions.AcknowledgeAttachmentRequest{
			SessionID:       sessionID,
			AttachmentID:    attachment.ID,
			ExpectedVersion: sessionVersion,
			AfterSequence:   rec.ID.Position,
		})
		if ackErr != nil {
			var posErr *chatsessions.AttachmentPositionError
			if errors.As(ackErr, &posErr) {
				return true, deliveredMessage, nil
			}
			return false, deliveredMessage, ackErr
		}
		*attachment = ackResult.Attachment
		cache.set(sessionID, *attachment)
	}
	return false, deliveredMessage, nil
}

// deliverReadTimeGap projects and delivers one read-time retention gap that
// events.Read itself detected (events.ReadOutcomeGap) -- distinct from a
// producer-committed workers.KindStreamGap record, which drainRecords
// already handles like any other sequenced item -- then advances the
// attachment's cursor past the evicted range so the caller's next Read call
// resumes retained catch-up starting at gap.EarliestRetained. The evicted
// range is exactly (gap.Requested, gap.EarliestRetained): read.go's own
// "from+1 == earliest is not a gap" invariant guarantees
// gap.EarliestRetained-1 is always >= gap.Requested+1 >= 1 whenever this
// outcome occurs, so it is always a valid already-assigned
// AcknowledgeAttachment position. stop reports the same StreamHead-lag case
// drainRecords documents: AcknowledgeAttachment rejected the advance because
// the session's StreamHead has not yet caught up, so the caller should stop
// this call's drain without treating it as a failure.
func (s *Server) deliverReadTimeGap(
	ctx context.Context,
	sessionID string,
	sessionVersion uint64,
	attachment *chatsessions.Attachment,
	gap *events.GapFacts,
	notify promptNotifier,
) (stop bool, err error) {
	payload, marshalErr := json.Marshal(workers.StreamGapPayload{
		FromSequence:           int64(gap.Requested) + 1,
		ToSequence:             int64(gap.EarliestRetained) - 1,
		FirstAvailableSequence: int64(gap.EarliestRetained),
		Reason:                 "retention eviction",
	})
	if marshalErr != nil {
		return false, marshalErr
	}

	update, projErr := mapping.ProjectStreamGap(workers.Draft{
		Kind:    workers.KindStreamGap,
		Phase:   workers.PhaseUpdated,
		Payload: payload,
	})
	if projErr != nil {
		return false, projErr
	}
	if update != nil && notify != nil {
		if notifyErr := notify(acpsdk.SessionNotification{
			SessionId: acpsdk.SessionId(sessionID),
			Update:    *update,
		}); notifyErr != nil {
			return false, notifyErr
		}
	}

	resumeAt := events.AggregateSequence(gap.EarliestRetained) - 1
	ackResult, ackErr := s.chatSessions.AcknowledgeAttachment(ctx, chatsessions.AcknowledgeAttachmentRequest{
		SessionID:       sessionID,
		AttachmentID:    attachment.ID,
		ExpectedVersion: sessionVersion,
		AfterSequence:   resumeAt,
	})
	if ackErr != nil {
		var posErr *chatsessions.AttachmentPositionError
		if errors.As(ackErr, &posErr) {
			return true, nil
		}
		return false, ackErr
	}
	*attachment = ackResult.Attachment
	attachmentCacheFromContext(ctx).set(sessionID, *attachment)
	return false, nil
}
