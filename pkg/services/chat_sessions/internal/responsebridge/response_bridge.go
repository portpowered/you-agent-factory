// Package responsebridge sequences Factory Session response events onto a
// Chat Session's canonical aggregate stream.
package responsebridge

import (
	"context"
	"errors"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

const (
	responseBridgeSourceType events.SourceType = "factory_response_event"
	responseBridgeSchemaID   events.SchemaID   = "factory.response_event.v1"
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
	sequencer Sequencer
}

// New constructs an inert response-event bridge.
func New(sequencer Sequencer) *Service {
	return &Service{sequencer: sequencer}
}

// Run sequences one Factory Session's response stream while invoke and the
// transport-owned live drain run. It preserves invoke's result and error;
// delivery is additive and its failures never rewrite the invocation outcome.
func (s *Service) Run(
	ctx context.Context,
	subscriber factorysessions.TargetExecutionService,
	chatSessionID string,
	sessionVersion uint64,
	factorySessionID string,
	liveDrain func(context.Context),
	invoke func(context.Context) (factorysessions.InvocationResult, error),
) (factorysessions.InvocationResult, error) {
	bridgeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	bridgeDone := make(chan struct{})
	go func() {
		defer close(bridgeDone)
		_ = s.drain(bridgeCtx, subscriber, chatSessionID, sessionVersion, factorySessionID)
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

func (s *Service) drain(
	ctx context.Context,
	subscriber factorysessions.TargetExecutionService,
	chatSessionID string,
	sessionVersion uint64,
	factorySessionID string,
) error {
	if s == nil || s.sequencer == nil || subscriber == nil {
		return nil
	}
	cursor, err := subscriber.SubscribeFactoryResponseEvents(ctx, factorysessions.ResponseEventSubscriptionRequest{SessionID: factorySessionID})
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
			currentVersion, err = s.sequence(ctx, chatSessionID, currentVersion, chatItemIDByFactoryItemID, event)
			if err != nil {
				return err
			}
		}
	}
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
