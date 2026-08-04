package stdio

// prompt_stream.go implements the ACP-L1-V2-T03 consumer side of
// chat-session streaming: attaching to a Chat Session, draining its
// aggregate topic in order, projecting each record, and delivering it before
// the V1 fallback (deliverPromptUpdates). It intentionally does not
// implement the producer side itself -- the Chat Sessions-owned
// factorysessionsshim.BridgeFactoryResponseEvents/RunWithResponseBridge
// (invoked through the injected acp.ResponseBridge collaborator; see
// response_bridge.go and dispatchFactoryTurn's two Factory dispatch branches
// in session_prompt.go) owns calling chatsessions.Service.Sequence and
// chatsessions.Service.AdvanceStreamHead to put a Factory response
// workers.Draft onto chat-session/<id>/events in the first place, since that
// is Factory Sessions event logic this package's own governing PRD bars it
// from adding. This file provides two consumers of that topic:
// liveDrainTurnUpdates (Subscribe-based, run concurrently with the in-flight
// Factory invocation via the same injected collaborator, for genuine
// mid-generation delivery) and streamTurnUpdates (Read-based, run strictly
// after the invocation and its response-event bridge have both fully
// returned, the guaranteed-correct retained catch-up and duplicate-suppression
// backstop for whatever the live drain did not manage to observe). Both
// share the exact same attachment cursor and AcknowledgeAttachment
// mechanism, so neither ever skips or duplicates a record relative to the
// other regardless of how far the live drain gets before it is stopped.

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
// on this same connection already attached, or registers one via
// chatSessions.Attach otherwise. connectionID identifies the caller for a
// freshly registered Attachment; a blank connectionID (a request identity
// with no connection pairing, such as a transport-minted RequestIdentity)
// never attaches and reports ok=false so a caller can safely skip streaming
// for this turn rather than fail it outright.
//
// Every Attach call here requests Resume: true, so a connection that is the
// first to touch sessionID since an earlier connection's Detach (see
// detachAttachments) reactivates that earlier connection's own interactive
// attachment -- with its original ID and already-advanced AfterSequence
// delivery cursor -- instead of starting over at position zero; a session
// with no detached interactive attachment (its first-ever attach, or one
// whose interactive attachment is still actively connected elsewhere) is
// unaffected, since AttachRequest.Resume only ever reactivates a match and
// otherwise creates an ordinary fresh attachment. This is what lets a
// reconnecting client -- whether it reaches this transport again through
// "session/load"/"session/resume" (see handleSessionLoad/handleSessionResume
// in session_load.go, which call this same method eagerly) or simply issues
// its next "session/prompt" on the same known session id -- resume
// delivery from where its previous connection left off.
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
		Resume:       true,
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

// liveDrainTurnUpdates subscribes to sessionID's chat-session events topic
// (through events.Service.Subscribe, not the Read-based polling
// streamTurnUpdates uses) from this connection's attachment cursor, and
// delivers each record through notify as soon as the subscription observes
// it -- genuine mid-generation delivery, running concurrently with the
// in-flight Factory invocation dispatchFactoryInvocation wraps (see
// acp.ResponseBridge and factorysessionsshim.RunWithResponseBridge's own doc
// comments). ctx is the bridge-derived context RunWithResponseBridge cancels
// once invoke returns, so Subscription.Next(ctx) unblocks and this method
// returns as soon as the turn's dispatch itself completes.
//
// This is the live counterpart to streamTurnUpdates' post-invocation retained
// catch-up, not a replacement for it: both share the exact same attachment
// cursor and AcknowledgeAttachment mechanism (ensureAttachment, drainRecords,
// deliverReadTimeGap), and both persist the advanced cursor into this
// connection's attachmentCache immediately after each record. Whatever this
// call does not manage to observe and acknowledge before ctx is canceled --
// the ordinary case once invoke returns and the surrounding bridge stops this
// drain, and also any record dropped by a version conflict racing the
// concurrent Factory response-event bridge's own AdvanceStreamHead calls
// (see currentSessionVersion's own doc comment) -- is still delivered
// afterward by streamTurnUpdates' own call, resuming from wherever this call's
// last successful AcknowledgeAttachment left the cached position. No record
// is ever skipped or duplicated regardless of how far this live drain gets.
//
// Like the Factory response-event bridge it runs alongside, a failure here is
// never propagated as the turn's own failure: it is additive, best-effort
// streaming layered onto the guaranteed-correct post-invocation sweep, so
// this method simply stops rather than surfacing an error to its caller
// (RunWithResponseBridge, which does not itself check a return value -- see
// that function's own signature). s.events == nil, no attachment (blank
// connectionID or an Attach failure), or a Subscribe failure are all silent
// no-ops, matching streamTurnUpdates' own s.events == nil convention.
//
// It returns whether at least one agent_message_chunk was delivered here --
// dispatchFactoryInvocation threads this back out (see its own liveDelivered
// return value) so deliverPromptUpdates' V1 final-text suppression decision
// accounts for a message this live drain already delivered, not only what
// the post-invocation streamTurnUpdates sweep itself observes. Without this,
// a message delivered live but not re-observed by the post-invocation sweep
// (the ordinary case, since both share one cursor -- see this method's own
// doc comment above) would incorrectly still fall back to a duplicate V1
// final-text notification.
func (s *Server) liveDrainTurnUpdates(ctx context.Context, connectionID, sessionID string, sessionVersion uint64, notify promptNotifier) (deliveredMessage bool) {
	if s.events == nil {
		return false
	}

	attachment, ok, err := s.ensureAttachment(ctx, connectionID, sessionID)
	if err != nil || !ok {
		return false
	}

	topic := chatsessions.EventsTopic(sessionID)
	subscription, err := s.events.Subscribe(ctx, events.SubscribeRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic, Position: events.AggregateSequence(attachment.AfterSequence)},
		Limit: retainedReadBatchLimit,
	})
	if err != nil {
		return false
	}

	for {
		delivery := subscription.Next(ctx)
		switch delivery.Kind {
		case events.DeliveryRecord:
			version, versionErr := s.currentSessionVersion(ctx, sessionID, sessionVersion)
			if versionErr != nil {
				return deliveredMessage
			}
			stop, delivered, drainErr := s.drainRecords(ctx, sessionID, version, &attachment, []events.Record{delivery.Record}, notify)
			deliveredMessage = deliveredMessage || delivered
			if drainErr != nil || stop {
				return deliveredMessage
			}
		case events.DeliveryGap:
			version, versionErr := s.currentSessionVersion(ctx, sessionID, sessionVersion)
			if versionErr != nil {
				return deliveredMessage
			}
			stop, gapErr := s.deliverReadTimeGap(ctx, sessionID, version, &attachment, delivery.Gap, notify)
			if gapErr != nil || stop {
				return deliveredMessage
			}
		default:
			// events.DeliveryClosed, events.DeliveryCanceled,
			// events.DeliveryBackpressure, and any unrecognized kind all end
			// this best-effort drain the same way: streamTurnUpdates' own
			// post-invocation sweep is the guaranteed-correct backstop.
			return deliveredMessage
		}
	}
}

// deliverPromptUpdates delivers startResult's turn output to the connection
// that admitted it: it first drains any canonical chat-session records
// available for startResult.Session.ID through streamTurnUpdates, and only
// when neither that drain nor the live drain dispatchFactoryInvocation already
// ran concurrently with the Factory invocation (liveDelivered) delivered a
// canonical agent_message_chunk falls back to the V1 synchronous final text
// built from fallbackText (dispatched.outcome.Text in dispatchFactoryTurn) via
// deliverPromptText -- so a turn whose canonical message output was already
// streamed, live or in retained catch-up, never also emits a duplicate
// final-only notification, while a turn with no canonical message output
// keeps its existing V1 behavior unchanged. liveDelivered must reflect
// dispatchOutcome.liveDelivered, not be recomputed here: the live drain's own
// delivery already happened inside dispatchFactoryInvocation, entirely
// before this method is ever called (see dispatchFactoryTurn).
func (s *Server) deliverPromptUpdates(
	ctx context.Context,
	startResult chatsessions.StartTurnResult,
	sessionVersion uint64,
	reqIdentity chatsessions.RequestIdentity,
	liveDelivered bool,
	fallbackText []string,
) error {
	notify := promptNotifierFromContext(ctx)

	deliveredMessage, err := s.streamTurnUpdates(ctx, reqIdentity.ConnectionID, startResult.Session.ID, sessionVersion, notify)
	if err != nil {
		return err
	}
	if deliveredMessage || liveDelivered {
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

		// context.WithoutCancel: this record was already handed to notify above
		// -- the client has already received it -- so the acknowledgement that
		// persists its cursor position must still complete even if ctx is
		// canceled for a reason unrelated to this specific record (concretely:
		// liveDrainTurnUpdates runs against a bridge-derived ctx that
		// RunWithResponseBridge cancels the instant the wrapped Factory
		// invocation returns, which can race a still-in-flight
		// AcknowledgeAttachment call for a record notify already delivered).
		// Without this, that race leaves the cursor unadvanced past an
		// already-delivered record, and the guaranteed-correct
		// streamTurnUpdates catch-up sweep that follows redelivers it a second
		// time -- directly violating this transport's "at most once per
		// attachment" delivery guarantee. Matches detachAttachments' identical
		// "must finish despite ctx already being canceled" idiom.
		ackResult, ackErr := s.chatSessions.AcknowledgeAttachment(context.WithoutCancel(ctx), chatsessions.AcknowledgeAttachmentRequest{
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
	// context.WithoutCancel: see drainRecords' identical comment -- the gap
	// notice was already handed to notify above, so this acknowledgement must
	// still complete despite ctx being canceled for a reason unrelated to
	// this specific record.
	ackResult, ackErr := s.chatSessions.AcknowledgeAttachment(context.WithoutCancel(ctx), chatsessions.AcknowledgeAttachmentRequest{
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
