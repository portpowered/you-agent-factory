package stdio

// prompt_stream.go implements the ACP-L1-V2-T03 consumer side of
// chat-session streaming: attaching to a Chat Session, draining its
// aggregate topic in order, projecting each record, and delivering it before
// the V1 fallback (deliverPromptUpdates). It intentionally does not
// implement the Factory-response producer side itself -- the Chat
// Sessions-owned responsebridge.Service.Run
// (invoked through the injected acp.ResponseBridge collaborator; see
// response_bridge.go and dispatchFactoryTurn's two Factory dispatch branches
// in session_prompt.go) owns calling chatsessions.Service.Sequence and
// chatsessions.Service.AdvanceStreamHead to put a Factory response
// workers.Draft onto chat-session/<id>/events in the first place, since that
// is Factory Sessions event logic this package's own governing PRD bars it
// from adding. ACP prompt admission owns the distinct user-authored record
// before it dispatches a Factory turn. This file provides two consumers of
// that topic:
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
	"sync"
	"time"

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

const workerChildProjectionSkipOperation = "ProjectWorkerChild"

// liveStreamHeadPollInterval bounds how long a live subscriber waits between
// observing a sequenced event and observing the producer's accompanying
// StreamHead advancement. Sequence commits the event before AdvanceStreamHead
// makes it deliverable, so the tiny wait is necessary to avoid notifying a
// record whose attachment cursor cannot yet be persisted.
const liveStreamHeadPollInterval = time.Millisecond

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
// synchronization so the connection's asynchronous prompt execution cannot
// race a live drain or another session's prompt while reusing this cache.
type attachmentCache struct {
	mu                sync.Mutex
	bySession         map[string]chatsessions.Attachment
	resumeIDBySession map[string]string
	loadedSessions    map[string]bool
}

// attachmentCacheContextKey is the unexported context key attachmentCache is
// carried under, so no other package can inject or observe it.
type attachmentCacheContextKey struct{}

// historyReplayContextKey marks the one session/load replay path. Live prompt
// streams intentionally suppress user-authored records because the connected
// client supplied them; a later load must include those same retained records
// to rebuild the complete conversation.
type historyReplayContextKey struct{}

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

func contextWithHistoryReplay(ctx context.Context) context.Context {
	return context.WithValue(ctx, historyReplayContextKey{}, true)
}

func isHistoryReplay(ctx context.Context) bool {
	replay, _ := ctx.Value(historyReplayContextKey{}).(bool)
	return replay
}

// get reports the cached Attachment for sessionID, or ok=false when this
// connection has not attached to sessionID yet (including when c is nil, the
// no-cache-attached case).
func (c *attachmentCache) get(sessionID string) (attachment chatsessions.Attachment, ok bool) {
	if c == nil {
		return chatsessions.Attachment{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
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
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bySession == nil {
		c.bySession = make(map[string]chatsessions.Attachment)
	}
	c.bySession[sessionID] = a
}

// markLoaded records that this connection has already completed a
// session/load for sessionID. It distinguishes a closed session's stale
// pre-close attachment (which must be replaced so load can replay retained
// history) from the attachment that this same load request installed (which
// a retry must reuse to avoid duplicate delivery).
func (c *attachmentCache) markLoaded(sessionID string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loadedSessions == nil {
		c.loadedSessions = make(map[string]bool)
	}
	c.loadedSessions[sessionID] = true
}

func (c *attachmentCache) wasLoaded(sessionID string) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loadedSessions[sessionID]
}

// setResumeAttachmentID remembers the opaque identity a cooperating ACP
// client supplied for a future Attach on this connection. It is only held for
// this connection's lifetime and is passed straight back to the Chat Sessions
// authority; this transport neither derives nor persists cursor identity.
func (c *attachmentCache) setResumeAttachmentID(sessionID, attachmentID string) {
	if c == nil || attachmentID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.resumeIDBySession == nil {
		c.resumeIDBySession = make(map[string]string)
	}
	c.resumeIDBySession[sessionID] = attachmentID
}

// resumeAttachmentID returns the optional exact attachment identity this
// connection supplied for sessionID. No identity deliberately means a new
// consumer: guessing a sole detached attachment would become unsafe as soon
// as two independent consumers have disconnected.
func (c *attachmentCache) resumeAttachmentID(sessionID string) string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.resumeIDBySession[sessionID]
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
	cache.mu.Lock()
	attachments := make(map[string]chatsessions.Attachment, len(cache.bySession))
	for sessionID, attachment := range cache.bySession {
		attachments[sessionID] = attachment
	}
	cache.mu.Unlock()
	for sessionID, attachment := range attachments {
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
// A connection resumes only when the cooperating client supplied its own
// opaque attachment identity in the vendor-namespaced ACP _meta extension.
// Without that identity this method creates a fresh consumer instead of
// guessing at a detached attachment: two clients can legitimately have the
// same ACP session ID but different delivery cursors, and choosing a detached
// candidate would leak or replay another client's output. See session_load.go
// for the customer-facing session/load and session/resume path that records
// the identity before calling this method.
func (s *Server) ensureAttachment(ctx context.Context, connectionID, sessionID string) (attachment chatsessions.Attachment, ok bool, err error) {
	cache := attachmentCacheFromContext(ctx)
	if a, cached := cache.get(sessionID); cached {
		return a, true, nil
	}
	if connectionID == "" {
		return chatsessions.Attachment{}, false, nil
	}
	resumeAttachmentID := cache.resumeAttachmentID(sessionID)

	result, err := s.chatSessions.Attach(ctx, chatsessions.AttachRequest{
		SessionID:          sessionID,
		ConnectionID:       connectionID,
		Interactive:        true,
		Resume:             resumeAttachmentID != "",
		ResumeAttachmentID: resumeAttachmentID,
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
// acp.ResponseBridge and responsebridge.Service.Run's own doc
// comments). ctx is the bridge-derived live-consumer context the response
// bridge cancels once invoke returns, so Subscription.Next(ctx) unblocks and
// this method returns as soon as the turn's dispatch itself completes.
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
// (the response bridge, which does not itself use its return value). s.events
// == nil, no attachment (blank
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
			version, ready := s.waitForLiveStreamHead(ctx, sessionID, delivery.Record.ID.Position)
			if !ready {
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

// waitForLiveStreamHead returns the latest session version only after the
// sequenced record at position is covered by Session.StreamHead. Events are
// visible immediately after Sequence, while AdvanceStreamHead is a following
// optimistic operation, so a subscription can otherwise receive a record in
// the short interval where acknowledging it is impossible. Crucially, this
// waits before notifying the client: if the turn finishes before the head
// advances, the retained post-invocation drain delivers the record once
// instead of redelivering an already-notified record.
func (s *Server) waitForLiveStreamHead(
	ctx context.Context,
	sessionID string,
	position events.AggregateSequence,
) (uint64, bool) {
	for {
		current, err := s.chatSessions.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: sessionID})
		if err != nil {
			return 0, false
		}
		if current.Session.StreamHead >= uint64(position) {
			return current.Session.Version, true
		}

		timer := time.NewTimer(liveStreamHeadPollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return 0, false
		case <-timer.C:
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

		update, projErr := projectSequencedItem(ctx, item)
		if projErr != nil {
			if item.WorkerSessionAssociation == nil {
				return false, deliveredMessage, projErr
			}
			s.logWorkerChildProjectionSkipped(item)
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
		// responsebridge.Service.Run cancels the instant the wrapped Factory
		// invocation returns, which can race a still-in-flight
		// AcknowledgeAttachment call for a record notify already delivered).
		// Without this, that race leaves the cursor unadvanced past an
		// already-delivered record, and the guaranteed-correct
		// streamTurnUpdates catch-up sweep that follows redelivers it a second
		// time -- directly violating this transport's "at most once per
		// attachment" delivery guarantee. Matches detachAttachments' identical
		// "must finish despite ctx already being canceled" idiom.
		ackResult, ackErr := s.acknowledgeDeliveredPosition(ctx, sessionID, attachment.ID, sessionVersion, rec.ID.Position)
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

// projectSequencedItem selects the child lifecycle mapper only when the
// aggregate envelope carries the canonical Factory dispatch-to-Worker-Session
// association. Unassociated records continue through the established generic
// Project path unchanged, preserving T03 delivery behavior.
func projectSequencedItem(ctx context.Context, item chatsessions.SequencedItem) (*acpsdk.SessionUpdate, error) {
	if item.WorkerSessionAssociation != nil {
		return mapping.ProjectWorkerChild(item)
	}
	draft := mapping.DraftFromSequencedItem(item)
	if isHistoryReplay(ctx) {
		return mapping.ProjectRetained(draft)
	}
	return mapping.Project(draft)
}

// logWorkerChildProjectionSkipped records a typed child mapping failure
// without surfacing source payload contents or terminating delivery for a
// healthy sibling. The pure mapper still returns its typed error to direct
// callers; this retained/live consumer acknowledges the malformed child item
// and continues the canonical aggregate order.
func (s *Server) logWorkerChildProjectionSkipped(item chatsessions.SequencedItem) {
	if s == nil || s.logger == nil || item.WorkerSessionAssociation == nil {
		return
	}
	s.logger.Warn(
		"acp skipped malformed worker child projection",
		"op", workerChildProjectionSkipOperation,
		"dispatch_id", item.WorkerSessionAssociation.DispatchID,
		"worker_session_id", item.WorkerSessionAssociation.WorkerSessionID,
		"item_id", item.ItemID,
		"parent_item_id", item.ParentItemID,
		"kind", string(item.Kind),
		"phase", string(item.Phase),
	)
}

// acknowledgeDeliveredPosition persists a position already handed to the
// client. The response bridge can advance StreamHead again between the live
// drain's head observation and this acknowledgement, making only the
// optimistic ExpectedVersion stale. Refresh and retry that benign conflict so
// a delivered record is not replayed by the retained catch-up drain. Other
// failures retain drainRecords' existing behavior.
func (s *Server) acknowledgeDeliveredPosition(
	ctx context.Context,
	sessionID, attachmentID string,
	expectedVersion uint64,
	position events.AggregateSequence,
) (chatsessions.AcknowledgeAttachmentResult, error) {
	ackCtx := context.WithoutCancel(ctx)
	for {
		result, err := s.chatSessions.AcknowledgeAttachment(ackCtx, chatsessions.AcknowledgeAttachmentRequest{
			SessionID:       sessionID,
			AttachmentID:    attachmentID,
			ExpectedVersion: expectedVersion,
			AfterSequence:   position,
		})
		if err == nil {
			return result, nil
		}

		var conflict *chatsessions.ConflictError
		if !errors.As(err, &conflict) {
			return chatsessions.AcknowledgeAttachmentResult{}, err
		}
		current, getErr := s.chatSessions.GetSession(ackCtx, chatsessions.GetSessionRequest{SessionID: sessionID})
		if getErr != nil {
			return chatsessions.AcknowledgeAttachmentResult{}, getErr
		}
		expectedVersion = current.Session.Version
	}
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
	// acknowledgeDeliveredPosition keeps the same cancellation guarantee as
	// drainRecords and refreshes a stale optimistic version. A response bridge
	// can advance StreamHead after this gap notice reaches the client but before
	// its cursor acknowledgement, so returning that benign conflict would leave
	// the already-delivered gap retryable and duplicate it during retained
	// catch-up.
	ackResult, ackErr := s.acknowledgeDeliveredPosition(ctx, sessionID, attachment.ID, sessionVersion, resumeAt)
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
