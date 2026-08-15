// Package responsebridge sequences Factory Session response events onto a
// Chat Session's canonical aggregate stream.
package responsebridge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

const (
	responseBridgeSourceType events.SourceType = "factory_response_event"
	responseBridgeSchemaID   events.SchemaID   = "factory.response_event.v1"
	responseBridgeOperation                    = "BridgeFactoryResponseEvents"
)

var errResponseEventAwaitingWorkerAssociation = errors.New("response event is awaiting worker association")

// Sequencer is the narrow Chat Sessions capability the bridge uses to commit
// source-native Factory response events and advance the aggregate head.
type Sequencer interface {
	Sequence(context.Context, chatsessions.SequenceRequest) (chatsessions.SequenceResult, error)
	AdvanceStreamHead(context.Context, chatsessions.AdvanceStreamHeadRequest) (chatsessions.AdvanceStreamHeadResult, error)
}

// Service owns the response-event draining lifecycle for the Chat Sessions
// consumer. Its collaborator is injected once at construction.
type Service struct {
	sequencer     Sequencer
	factoryTarget factorysessions.TargetExecutionService
	workerEvents  events.Service
	logger        logging.Logger
}

// New constructs an inert response-event bridge.
func New(
	sequencer Sequencer,
	factoryTarget factorysessions.TargetExecutionService,
	workerEvents events.Service,
	logger logging.Logger,
) *Service {
	return &Service{
		sequencer:     sequencer,
		factoryTarget: factoryTarget,
		workerEvents:  workerEvents,
		logger:        logging.EnsureLogger(logger),
	}
}

// Run sequences one Factory Session's response stream while invoke and the
// transport-owned live drain run. Once invoke returns, it stops the live
// cursor consumer, drains every response event retained through that terminal
// boundary with a non-cancelled context, and returns any bridge failure when
// invoke itself succeeded. A prompt therefore never reports success while a
// committed response-event tail was silently lost.
func (s *Service) Run(
	ctx context.Context,
	chatSessionID string,
	sessionVersion uint64,
	factorySessionID string,
	liveDrain func(context.Context),
	invoke func(context.Context) (factorysessions.InvocationResult, error),
) (result factorysessions.InvocationResult, err error) {
	if s == nil || s.sequencer == nil || s.factoryTarget == nil {
		return invoke(ctx)
	}
	s.logStart(chatSessionID, factorySessionID)
	failureClass := ""
	defer func() {
		s.logOutcome(chatSessionID, factorySessionID, result.Status, failureClass)
	}()

	cursor, subscribeErr := s.factoryTarget.SubscribeFactoryResponseEvents(ctx, factorysessions.ResponseEventSubscriptionRequest{SessionID: factorySessionID})
	if subscribeErr != nil {
		result, invokeErr := invoke(ctx)
		if invokeErr != nil {
			failureClass = "factory_invocation"
			return result, invokeErr
		}
		failureClass = "response_event_subscription"
		return result, fmt.Errorf("subscribe factory response events: %w", subscribeErr)
	}
	defer cursor.Detach()

	liveCtx, cancelLive := context.WithCancel(ctx)
	defer cancelLive()
	deliveryCtx := context.WithoutCancel(ctx)
	state := drainState{
		currentVersion:            sessionVersion,
		chatItemIDByFactoryItemID: make(map[string]string),
		childrenByWorkerSessionID: make(map[string]*workerChild),
		workerSessionIDByDispatch: make(map[string]string),
	}
	children, childSubscribeErr := s.startWorkerChildren(liveCtx, deliveryCtx, chatSessionID, factorySessionID, &state)
	if childSubscribeErr != nil {
		result, invokeErr := invoke(ctx)
		if invokeErr != nil {
			failureClass = "factory_invocation"
			return result, invokeErr
		}
		failureClass = "worker_event_subscription"
		return result, fmt.Errorf("subscribe factory worker events: %w", childSubscribeErr)
	}
	bridgeDone := make(chan struct{})
	var liveBridgeErr error
	go func() {
		defer close(bridgeDone)
		liveBridgeErr = s.drainLive(liveCtx, deliveryCtx, cursor, chatSessionID, &state)
	}()

	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		if liveDrain != nil {
			liveDrain(liveCtx)
		}
	}()

	result, invokeErr := invoke(ctx)
	cancelLive()
	<-bridgeDone
	<-drainDone
	childErr := children.finish(deliveryCtx)
	bridgeErr := liveBridgeErr
	if bridgeErr == nil {
		bridgeErr = s.drainTail(deliveryCtx, cursor, chatSessionID, &state)
	}
	if invokeErr != nil {
		failureClass = "factory_invocation"
		return result, invokeErr
	}
	if bridgeErr != nil {
		failureClass = "response_event_bridge"
		return result, fmt.Errorf("sequence factory response events: %w", bridgeErr)
	}
	if childErr != nil {
		failureClass = "worker_event_bridge"
		return result, fmt.Errorf("sequence worker session events: %w", childErr)
	}
	return result, nil
}

// logStart records the bridge's stable identifiers without source response
// payloads, prompt text, provider commands, or paths.
func (s *Service) logStart(chatSessionID, factorySessionID string) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Debug(
		"chat_sessions response bridge start",
		"op", responseBridgeOperation,
		"chat_session_id", chatSessionID,
		"factory_session_id", factorySessionID,
	)
}

// logOutcome records the terminal bridge outcome through stable identifiers
// and a bounded classification only. It intentionally excludes the response
// payload and the underlying error string, either of which can carry unsafe
// provider-originated data.
func (s *Service) logOutcome(
	chatSessionID, factorySessionID string,
	status factorysessions.InvocationTerminalStatus,
	failureClass string,
) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Info(
		"chat_sessions response bridge outcome",
		"op", responseBridgeOperation,
		"chat_session_id", chatSessionID,
		"factory_session_id", factorySessionID,
		"terminal_status", string(status),
		"error_class", failureClass,
	)
}

type drainState struct {
	mu                        sync.Mutex
	currentVersion            uint64
	chatItemIDByFactoryItemID map[string]string
	childrenByWorkerSessionID map[string]*workerChild
	workerSessionIDByDispatch map[string]string
	// A response event and its Factory dispatch-to-Worker association travel
	// on independent streams. Keep an event whose dispatch has not been
	// associated yet out of the top-level Chat projection until the terminal
	// association sweep can classify it.
	pendingResponseEvents []factorysessions.FactoryResponseEvent
}

func (s *Service) drainLive(
	liveCtx, deliveryCtx context.Context,
	cursor *factorysessions.ResponseEventCursor,
	chatSessionID string,
	state *drainState,
) error {
	for {
		batch, nextErr := cursor.Next(liveCtx)
		if nextErr != nil {
			if errors.Is(nextErr, factorysessions.ErrResponseEventSubscriptionClosed) ||
				errors.Is(nextErr, context.Canceled) || errors.Is(nextErr, context.DeadlineExceeded) {
				return nil
			}
			return nextErr
		}
		if err := s.sequenceBatch(deliveryCtx, chatSessionID, state, batch, true); err != nil {
			if errors.Is(err, errResponseEventAwaitingWorkerAssociation) {
				return nil
			}
			return err
		}
	}
}

func (s *Service) drainTail(
	deliveryCtx context.Context,
	cursor *factorysessions.ResponseEventCursor,
	chatSessionID string,
	state *drainState,
) error {
	batch, err := cursor.Drain()
	if errors.Is(err, factorysessions.ErrResponseEventSubscriptionClosed) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := s.sequencePending(deliveryCtx, chatSessionID, state); err != nil {
		return err
	}
	return s.sequenceBatch(deliveryCtx, chatSessionID, state, batch, false)
}

func (s *Service) sequencePending(
	ctx context.Context,
	chatSessionID string,
	state *drainState,
) error {
	state.mu.Lock()
	pending := append([]factorysessions.FactoryResponseEvent(nil), state.pendingResponseEvents...)
	state.pendingResponseEvents = nil
	state.mu.Unlock()
	if len(pending) == 0 {
		return nil
	}
	return s.sequenceBatch(ctx, chatSessionID, state, pending, false)
}

func (s *Service) sequenceBatch(
	ctx context.Context,
	chatSessionID string,
	state *drainState,
	batch []factorysessions.FactoryResponseEvent,
	deferUnknownWorkerDispatches bool,
) error {
	// During live delivery, an unassociated dispatch is not yet safe to
	// project as a top-level assistant item: it may be a Worker copy whose
	// canonical parent is still arriving on the Factory Events stream. The
	// caller stops live draining after this batch and the terminal sweep
	// retries it after worker associations have been presented.
	for index, event := range batch {
		state.mu.Lock()
		if carriedByWorkerSession(state, event) {
			state.mu.Unlock()
			continue
		}
		if deferUnknownWorkerDispatches && s.workerEvents != nil && hasUnknownDispatch(state, event) {
			state.pendingResponseEvents = append(state.pendingResponseEvents, batch[index:]...)
			state.mu.Unlock()
			return errResponseEventAwaitingWorkerAssociation
		}
		version, err := s.sequence(ctx, chatSessionID, state.currentVersion, state.chatItemIDByFactoryItemID, event)
		if err == nil {
			state.currentVersion = version
		}
		state.mu.Unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

func hasUnknownDispatch(state *drainState, event factorysessions.FactoryResponseEvent) bool {
	dispatchID := strings.TrimSpace(event.DispatchID)
	return dispatchID != "" && state.workerSessionIDByDispatch[dispatchID] == ""
}

// carriedByWorkerSession reports whether this Factory response event describes
// work already reaching the Chat Session through its Worker's own topic.
//
// A Worker is a tool call, so its output belongs inside that tool call, and the
// Worker topic is what delivers it there with the association and parent
// lineage the child projection needs. The same observation is also published to
// the Factory Session response-event stream, which the CLI, dashboard, and HTTP
// SSE feed read -- so it arrives here a second time, carrying no association
// and no parent. Sequencing that copy would give the Worker a second, top-level
// voice alongside its tool call, which is exactly what a Worker must not have.
//
// The dispatch association is the discriminator rather than the record's Kind:
// membership is a canonical fact this bridge already tracks, whereas a
// Kind-based rule would have to guess, and would silently start dropping
// Factory-authored records the day a Factory emits one of the same kinds.
func carriedByWorkerSession(state *drainState, event factorysessions.FactoryResponseEvent) bool {
	dispatchID := strings.TrimSpace(event.DispatchID)
	if dispatchID == "" {
		return false
	}
	return state.workerSessionIDByDispatch[dispatchID] != ""
}

func (s *Service) sequence(
	ctx context.Context,
	chatSessionID string,
	sessionVersion uint64,
	chatItemIDByFactoryItemID map[string]string,
	event factorysessions.FactoryResponseEvent,
) (uint64, error) {
	parentItemID := ""
	if event.ParentItemID != "" {
		parentItemID = chatItemIDByFactoryItemID[event.ParentItemID]
	}
	seqResult, err := s.sequencer.Sequence(ctx, chatsessions.SequenceRequest{
		SessionID: chatSessionID, SourceType: responseBridgeSourceType,
		SourceID: events.SourceID(event.FactorySessionID), SourceSequence: events.SourceSequence(event.Sequence),
		SourceEventID: events.SourceEventID(event.EventID), SchemaID: responseBridgeSchemaID,
		Kind: event.Kind, Phase: event.Phase, ParentItemID: parentItemID, Payload: event.Payload,
	})
	if err != nil {
		return sessionVersion, err
	}
	if event.ItemID != "" {
		chatItemIDByFactoryItemID[event.ItemID] = seqResult.ItemID
	}
	advanceResult, err := s.sequencer.AdvanceStreamHead(ctx, chatsessions.AdvanceStreamHeadRequest{
		SessionID: chatSessionID, ExpectedVersion: sessionVersion, AggregateSequence: seqResult.AggregateSequence,
		SourceType: responseBridgeSourceType, SourceID: events.SourceID(event.FactorySessionID),
		SourceSequence: events.SourceSequence(event.Sequence), SourceEventID: events.SourceEventID(event.EventID),
	})
	if err != nil {
		return sessionVersion, err
	}
	return advanceResult.Session.Version, nil
}
