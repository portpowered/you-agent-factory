package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/work"
)

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
