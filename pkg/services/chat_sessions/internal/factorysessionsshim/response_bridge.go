package factorysessionsshim

import (
	"context"
	"errors"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// responseBridgeSourceType and responseBridgeSchemaID identify every record
// BridgeFactoryResponseEvents commits onto a Chat Session's aggregate
// stream, distinguishing this producer from any other source (for example
// "worker") that may someday share the same topic.
const (
	responseBridgeSourceType events.SourceType = "factory_response_event"
	responseBridgeSchemaID   events.SchemaID   = "factory.response_event.v1"
)

// Sequencer is the narrow Chat Sessions capability BridgeFactoryResponseEvents
// commits every eligible Factory response event through: append one
// source-native record onto a Chat Session's aggregate stream, and durably
// advance that session's StreamHead past it. chatsessions.Service satisfies
// this structurally; a caller supplies it explicitly (rather than this shim
// depending on chat_sessions.Service at construction time) so this package
// stays free of a fixed chatsessions.Service field the way its own doc
// comment already requires of FactoryTargetService.
type Sequencer interface {
	Sequence(context.Context, chatsessions.SequenceRequest) (chatsessions.SequenceResult, error)
	AdvanceStreamHead(context.Context, chatsessions.AdvanceStreamHeadRequest) (chatsessions.AdvanceStreamHeadResult, error)
}

// ResponseEventSubscriber is the narrow Factory Sessions capability
// BridgeFactoryResponseEvents subscribes through. FactoryTargetService
// already satisfies this structurally via its own
// SubscribeFactoryResponseEvents method.
type ResponseEventSubscriber interface {
	SubscribeFactoryResponseEvents(
		context.Context,
		factorysessions.ResponseEventSubscriptionRequest,
	) (*factorysessions.ResponseEventCursor, error)
}

// BridgeFactoryResponseEvents subscribes to factorySessionID's Factory
// response-event stream from the beginning and sequences every event it
// observes onto chatSessionID's chat-session aggregate stream through
// sequencer, in order. Factory response events and chatsessions.SequenceRequest
// share the identical workers.Kind/workers.Phase/Payload vocabulary
// (responseevents.Kind and responseevents.Phase are literal aliases of
// workers.Kind/workers.Phase -- see responseevents.types.go), so this bridge
// is a lossless, unfiltered copy: it commits every event this attachment
// observes and leaves the decision of what is customer-facing to the
// existing mapping.Project dispatcher a later reader (for example this
// transport's own streamTurnUpdates) already applies, never duplicating that
// policy here.
//
// A Factory response event's own ItemID/ParentItemID are scoped to the
// Factory Session's response-event stream, not the Chat Session's own
// sequencer identity space (Sequence mints its own ItemID independent of
// what the Factory response-event stream assigned -- see
// chatsessions.Service.Sequence's own doc comment): this function keeps a
// local, call-scoped map from a Factory-response-event ItemID to the ItemID
// Sequence assigned it, so a later child event's ParentItemID resolves to
// the correct already-sequenced Chat Session identity instead of a foreign
// one Sequence would reject.
//
// It returns once the subscription itself reports
// factorysessions.ErrResponseEventSubscriptionClosed -- the response-event
// store's own graceful signal that no further events will ever be published
// for this Factory Session, reached once the run this bridge accompanies
// terminalizes -- which is treated as a normal, non-error stop, not once a
// specific Kind/Phase value is observed in the event stream itself. It also
// returns when ctx is done, or when a Sequence or AdvanceStreamHead call
// itself fails.
// RunWithResponseBridge starts BridgeFactoryResponseEvents concurrently with
// invoke, and also starts liveDrain (the ACP transport's own genuine
// mid-generation consumer loop -- see acp.ResponseBridge's own doc comment;
// a nil liveDrain is a no-op, matching a construction that never wired one)
// concurrently with the same invoke call, returning invoke's own result and
// error unchanged once invoke itself returns. This is the one place that
// actually owns both goroutines, their join channels, and their shared
// derived cancellation: callers (in particular the ACP transport, which only
// ever holds plain function values of this exact shape) never need their own
// concurrency primitives to run producer and consumer streaming alongside a
// synchronous Factory dispatch call.
//
// Once invoke returns, RunWithResponseBridge stops both background
// operations and waits for them to actually exit before returning: in the
// ordinary case (a real Factory response-event subscription that already
// closed on its own once the run it accompanies terminalized -- see
// BridgeFactoryResponseEvents' own doc comment -- and a liveDrain that has
// already drained everything committed so far) this cancellation is a no-op
// that simply confirms both already stopped, but it guarantees neither
// goroutine can ever outlive this call. A failure in either background
// operation (including this stop-triggered cancellation) is never propagated
// as invoke's own result or error: both are additive, best-effort streaming
// plumbing layered onto whatever invoke itself already does, not a new
// failure mode for invoke's caller. liveDrain's own doc comment documents why
// this is safe: whatever it does not manage to deliver before cancellation is
// still delivered afterward by the transport's own post-invocation retained
// catch-up sweep, resuming from the same shared attachment cursor.
func RunWithResponseBridge(
	ctx context.Context,
	sequencer Sequencer,
	subscriber ResponseEventSubscriber,
	chatSessionID string,
	sessionVersion uint64,
	factorySessionID string,
	liveDrain func(context.Context),
	invoke func(context.Context) (factorysessions.InvocationResult, error),
) (factorysessions.InvocationResult, error) {
	bridgeCtx, cancel := context.WithCancel(ctx)

	bridgeDone := make(chan struct{})
	go func() {
		defer close(bridgeDone)
		_ = BridgeFactoryResponseEvents(bridgeCtx, sequencer, subscriber, chatSessionID, sessionVersion, factorySessionID)
	}()

	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		if liveDrain != nil {
			liveDrain(bridgeCtx)
		}
	}()

	result, err := invoke(ctx)

	cancel()
	<-bridgeDone
	<-drainDone

	return result, err
}

// BridgeFactoryResponseEvents subscribes to factorySessionID's Factory
// response-event stream from the beginning and sequences every event it
// observes onto chatSessionID's chat-session aggregate stream through
// sequencer, in order. It is the pure drain loop RunWithResponseBridge runs
// concurrently with a synchronous Factory dispatch call; most callers should
// use RunWithResponseBridge instead of calling this directly.
func BridgeFactoryResponseEvents(
	ctx context.Context,
	sequencer Sequencer,
	subscriber ResponseEventSubscriber,
	chatSessionID string,
	sessionVersion uint64,
	factorySessionID string,
) error {
	cursor, err := subscriber.SubscribeFactoryResponseEvents(ctx, factorysessions.ResponseEventSubscriptionRequest{
		SessionID: factorySessionID,
	})
	if err != nil {
		return err
	}
	defer cursor.Detach()

	currentVersion := sessionVersion
	chatItemIDByFactoryItemID := make(map[string]string)
	for {
		batch, nextErr := cursor.Next(ctx)
		if nextErr != nil {
			if errors.Is(nextErr, factorysessions.ErrResponseEventSubscriptionClosed) {
				return nil
			}
			return nextErr
		}
		for _, event := range batch {
			currentVersion, err = sequenceResponseEvent(ctx, sequencer, chatSessionID, currentVersion, chatItemIDByFactoryItemID, event)
			if err != nil {
				return err
			}
		}
	}
}

// sequenceResponseEvent commits one Factory response event onto
// chatSessionID's aggregate stream and advances StreamHead past it,
// returning the session's version after that advancement so the caller's
// next call uses the current, not stale, ExpectedVersion.
func sequenceResponseEvent(
	ctx context.Context,
	sequencer Sequencer,
	chatSessionID string,
	sessionVersion uint64,
	chatItemIDByFactoryItemID map[string]string,
	event factorysessions.FactoryResponseEvent,
) (uint64, error) {
	parentItemID := ""
	if event.ParentItemID != "" {
		parentItemID = chatItemIDByFactoryItemID[event.ParentItemID]
	}

	seqResult, err := sequencer.Sequence(ctx, chatsessions.SequenceRequest{
		SessionID:      chatSessionID,
		SourceType:     responseBridgeSourceType,
		SourceID:       events.SourceID(event.FactorySessionID),
		SourceSequence: events.SourceSequence(event.Sequence),
		SourceEventID:  events.SourceEventID(event.EventID),
		SchemaID:       responseBridgeSchemaID,
		Kind:           event.Kind,
		Phase:          event.Phase,
		ParentItemID:   parentItemID,
		Payload:        event.Payload,
	})
	if err != nil {
		return sessionVersion, err
	}
	if event.ItemID != "" {
		chatItemIDByFactoryItemID[event.ItemID] = seqResult.ItemID
	}

	advanceResult, err := sequencer.AdvanceStreamHead(ctx, chatsessions.AdvanceStreamHeadRequest{
		SessionID:         chatSessionID,
		ExpectedVersion:   sessionVersion,
		AggregateSequence: seqResult.AggregateSequence,
		SourceType:        responseBridgeSourceType,
		SourceID:          events.SourceID(event.FactorySessionID),
		SourceSequence:    events.SourceSequence(event.Sequence),
		SourceEventID:     events.SourceEventID(event.EventID),
	})
	if err != nil {
		return sessionVersion, err
	}
	return advanceResult.Session.Version, nil
}
