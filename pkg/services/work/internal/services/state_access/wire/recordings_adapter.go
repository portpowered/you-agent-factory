package wire

import (
	"context"
	"errors"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	stateaccess "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access"
)

type recordingsAdapter struct {
	root recordings.Service
}

// NewRecordingsAdapter constructs the private Work state_access Recordings port
// from the published Recordings service root contract.
func NewRecordingsAdapter(root recordings.Service) stateaccess.RecordingsAdapter {
	if root == nil {
		return nil
	}
	return recordingsAdapter{root: root}
}

func (a recordingsAdapter) ReadWorkSnapshot(
	ctx context.Context,
	sessionID string,
) (work.ReadSnapshot, error) {
	if a.root == nil {
		return work.ReadSnapshot{}, errors.New("Recordings service is required")
	}
	if err := requireContext(ctx); err != nil {
		return work.ReadSnapshot{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return work.ReadSnapshot{}, recordings.ErrInvalidProjectionScope
	}
	scope := recordings.CanonicalEventScope{FactorySessionID: sessionID}
	events, err := a.canonicalEventsForScope(ctx, scope)
	if err != nil {
		return work.ReadSnapshot{}, err
	}
	reconstructed, err := a.root.ReconstructWorldState(
		reconstructWorldStateRequest(scope, events),
	)
	if err != nil {
		return work.ReadSnapshot{}, err
	}
	return readSnapshotFromWorldState(reconstructed.WorldState)
}

func (a recordingsAdapter) canonicalEventsForScope(
	ctx context.Context,
	scope recordings.CanonicalEventScope,
) ([]recordings.CanonicalEvent, error) {
	subscribed, err := a.root.SubscribeFrom(ctx, recordings.SubscribeRequest{Scope: scope})
	if err != nil {
		return nil, fmt.Errorf("subscribe Recordings canonical facts: %w", err)
	}
	return canonicalEventsFromSubscription(ctx, subscribed.Subscription)
}

func reconstructWorldStateRequest(
	scope recordings.CanonicalEventScope,
	events []recordings.CanonicalEvent,
) recordings.ReconstructWorldStateRequest {
	selectedTick := 0
	for _, event := range events {
		if tick := event.FactoryTick; tick > selectedTick {
			selectedTick = tick
		}
	}
	return recordings.ReconstructWorldStateRequest{
		Scope:        scope,
		Events:       append([]recordings.CanonicalEvent(nil), events...),
		SelectedTick: selectedTick,
	}
}

func canonicalEventsFromSubscription(
	ctx context.Context,
	subscription recordings.EventSubscription,
) ([]recordings.CanonicalEvent, error) {
	if subscription == nil {
		return nil, nil
	}
	events := make([]recordings.CanonicalEvent, 0)
	for {
		outcome := subscription.Next(ctx)
		switch outcome.Kind {
		case recordings.SubscriptionEvent:
			events = append(events, outcome.Event)
		case recordings.SubscriptionClosed:
			return events, nil
		case recordings.SubscriptionGap:
			if outcome.Gap == nil {
				return nil, errors.New("recordings subscription gap")
			}
			return nil, fmt.Errorf(
				"recordings subscription gap at sequence %d",
				outcome.Gap.ExpectedSequence,
			)
		default:
			return events, nil
		}
	}
}

func requireContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("Work state access context is required")
	}
	return ctx.Err()
}

func isPublicWorkItem(item work.FactoryWorkItem) bool {
	return !interfaces.IsSystemTimeWorkType(item.WorkTypeID)
}
