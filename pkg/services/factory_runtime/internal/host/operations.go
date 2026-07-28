package host

import (
	"context"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// ReplacementFactoryChangePayload extracts the canonical definition payload
// emitted by a newly built replacement runtime.
func ReplacementFactoryChangePayload(events []interfaces.FactoryEvent) (interfaces.FactoryChangeEventPayload, bool) {
	for _, event := range events {
		if event.Type != interfaces.FactoryEventTypeInitialStructureRequest {
			continue
		}
		var payload interfaces.InitialStructureRequestEventPayload
		if err := event.DecodePayload(&payload); err != nil {
			return interfaces.FactoryChangeEventPayload{}, false
		}
		return interfaces.FactoryChangeEventPayload{
			Factory: payload.Factory, Metadata: payload.Metadata, SourceDirectory: payload.SourceDirectory,
		}, true
	}
	return interfaces.FactoryChangeEventPayload{}, false
}

// PublishFactoryChange records the replacement definition on both the new and
// previous runtime ledgers. Failure to read the previous tick is returned so
// the caller can report it without failing an otherwise successful replacement.
func PublishFactoryChange(
	ctx context.Context,
	current *Handle,
	replacement *Bundle,
	clock factory.Clock,
) error {
	if clock == nil {
		return fmt.Errorf("publish Factory Runtime change: clock is required")
	}
	if replacement == nil || replacement.EventHistory == nil {
		return nil
	}
	payload, ok := ReplacementFactoryChangePayload(replacement.EventHistory.CanonicalEvents())
	if !ok {
		return nil
	}
	eventTime := clock.Now()
	replacement.EventHistory.RecordFactoryChange(1, payload, eventTime)
	if current == nil || current.Bundle == nil || current.Bundle.EventHistory == nil {
		return nil
	}
	snapshot, err := current.Bundle.Factory.GetEngineStateSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("read current runtime tick for factory-change event: %w", err)
	}
	current.Bundle.EventHistory.RecordFactoryChange(snapshot.TickCount+1, payload, eventTime)
	return nil
}

// ErrRuntimeNotAvailable reports that no hosted runtime bundle is available for an operation.
var ErrRuntimeNotAvailable = fmt.Errorf("factory service runtime is not available")

func hostedFactory(bundle *Bundle) (factory.Factory, error) {
	if bundle == nil || bundle.Factory == nil {
		return nil, ErrRuntimeNotAvailable
	}
	return bundle.Factory, nil
}

// SubmitWorkRequest submits a canonical work request batch to the hosted runtime.
func SubmitWorkRequest(ctx context.Context, bundle *Bundle, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	activeFactory, err := hostedFactory(bundle)
	if err != nil {
		return work.WorkRequestSubmitResult{}, err
	}
	return activeFactory.SubmitWorkRequest(ctx, request)
}

// MoveWork applies a synchronous operator relocation on the hosted runtime.
func MoveWork(
	ctx context.Context,
	bundle *Bundle,
	workID, stateName string,
	source work.WorkStateChangeSource,
	requestID string,
) (work.OperatorMoveResult, error) {
	activeFactory, err := hostedFactory(bundle)
	if err != nil {
		return work.OperatorMoveResult{}, err
	}
	return activeFactory.MoveWork(ctx, workID, stateName, source, requestID)
}

// SubscribeFactoryEvents returns canonical factory event history followed by live events
// from the hosted runtime.
func SubscribeFactoryEvents(
	ctx context.Context,
	bundle *Bundle,
	reconnect *interfaces.FactoryEventReconnectCursor,
	scope interfaces.FactoryEventReconnectScope,
) (*interfaces.FactoryEventStream, error) {
	activeFactory, err := hostedFactory(bundle)
	if err != nil {
		return nil, err
	}
	stream, err := activeFactory.SubscribeFactoryEvents(ctx, reconnect, scope)
	if err != nil {
		return nil, fmt.Errorf("subscribe factory events: %w", err)
	}
	return stream, nil
}

// SubscribeFactoryEventsForSession scopes event subscription to one factory session.
func SubscribeFactoryEventsForSession(
	ctx context.Context,
	bundle *Bundle,
	sessionID string,
	reconnect *interfaces.FactoryEventReconnectCursor,
) (*interfaces.FactoryEventStream, error) {
	stream, err := SubscribeFactoryEvents(ctx, bundle, reconnect, interfaces.FactoryEventReconnectScope{SessionID: sessionID})
	if err != nil {
		return nil, err
	}
	if stream != nil && bundle != nil {
		stream.BackendScopeID = strings.TrimSpace(bundle.BackendScopeID)
	}
	return stream, nil
}

// GetEngineStateSnapshot returns the hosted runtime's aggregate observability snapshot.
func GetEngineStateSnapshot(ctx context.Context, bundle *Bundle) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	activeFactory, err := hostedFactory(bundle)
	if err != nil {
		return nil, err
	}
	snap, err := activeFactory.GetEngineStateSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("get engine state snapshot: %w", err)
	}
	return snap, nil
}

// WaitToComplete returns a channel that closes when the hosted runtime reaches a terminal
// completion state. When no runtime is available, the returned channel is already closed.
func WaitToComplete(bundle *Bundle) <-chan struct{} {
	activeFactory, err := hostedFactory(bundle)
	if err != nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return activeFactory.WaitToComplete()
}
