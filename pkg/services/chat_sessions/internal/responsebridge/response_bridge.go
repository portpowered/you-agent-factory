// Package responsebridge sequences Factory Session response events onto a
// Chat Session's canonical aggregate stream.
package responsebridge

import (
	"context"
	"errors"
	"fmt"

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
	logger        logging.Logger
}

// New constructs an inert response-event bridge.
func New(
	sequencer Sequencer,
	factoryTarget factorysessions.TargetExecutionService,
	logger logging.Logger,
) *Service {
	return &Service{
		sequencer:     sequencer,
		factoryTarget: factoryTarget,
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
	state := drainState{currentVersion: sessionVersion, chatItemIDByFactoryItemID: make(map[string]string)}
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
	currentVersion            uint64
	chatItemIDByFactoryItemID map[string]string
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
		if err := s.sequenceBatch(deliveryCtx, chatSessionID, state, batch); err != nil {
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
	return s.sequenceBatch(deliveryCtx, chatSessionID, state, batch)
}

func (s *Service) sequenceBatch(
	ctx context.Context,
	chatSessionID string,
	state *drainState,
	batch []factorysessions.FactoryResponseEvent,
) error {
	for _, event := range batch {
		version, err := s.sequence(ctx, chatSessionID, state.currentVersion, state.chatItemIDByFactoryItemID, event)
		if err != nil {
			return err
		}
		state.currentVersion = version
	}
	return nil
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
