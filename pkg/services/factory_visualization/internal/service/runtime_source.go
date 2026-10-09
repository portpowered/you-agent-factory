package service

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
)

type runtimeSource struct {
	reader RuntimeReader
}

// NewRuntimeSourceOpening retains one observation owner. Opening a source only
// retains its addressed session identity; it never changes Current Factory.
func NewRuntimeSourceOpening(reader RuntimeReader) func(string) Source {
	owner := &runtimeSource{reader: reader}
	return func(sessionID string) Source {
		return selectedRuntimeSource{owner: owner, sessionID: sessionID}
	}
}

type selectedRuntimeSource struct {
	owner     *runtimeSource
	sessionID string
}

func (source selectedRuntimeSource) SubscribeFactoryEvents(ctx context.Context,
	reconnect *factorydefinitions.FactoryEventReconnectCursor,
	scope factorydefinitions.FactoryEventReconnectScope,
) (*factorydefinitions.FactoryEventStream, error) {
	return source.owner.subscribeFactoryEvents(ctx, reconnect, scope, source.sessionID)
}

func (source selectedRuntimeSource) GetRuntimeSnapshotFacts(ctx context.Context) (*liveviewprojection.RuntimeSnapshotFacts, error) {
	return source.owner.getRuntimeSnapshotFacts(ctx, source.sessionID)
}

func (s *runtimeSource) withRuntimeRead(sessionID string, read func(*factorysessions.LiveRuntime) error) error {
	if s == nil || s.reader == nil {
		return factorysessions.ErrRuntimeNotAvailable
	}
	return s.reader.WithRuntimeReadForSession(sessionID, read)
}

func (s *runtimeSource) subscribeFactoryEvents(ctx context.Context,
	reconnect *factorydefinitions.FactoryEventReconnectCursor,
	scope factorydefinitions.FactoryEventReconnectScope, sessionID string,
) (stream *factorydefinitions.FactoryEventStream, err error) {
	err = s.withRuntimeRead(sessionID, func(runtime *factorysessions.LiveRuntime) error {
		if runtime == nil || runtime.Factory == nil {
			return factorysessions.ErrRuntimeNotAvailable
		}
		// TODO(P5B): subscribe from Recordings so Visualization no longer needs
		// this source-native compatibility capability. Until then Visualization
		// reads the event boundary Factory Sessions declares on the live
		// runtime instead of recovering one from the runtime value.
		events := runtime.WorkAndEventIngress
		if events == nil {
			return fmt.Errorf("Factory Runtime event subscription is required until Recordings migration")
		}
		var subscribeErr error
		stream, subscribeErr = events.SubscribeFactoryEvents(ctx, reconnect, scope)
		return subscribeErr
	})
	return stream, err
}

func (s *runtimeSource) getRuntimeSnapshotFacts(ctx context.Context, sessionID string) (facts *liveviewprojection.RuntimeSnapshotFacts, err error) {
	err = s.withRuntimeRead(sessionID, func(runtime *factorysessions.LiveRuntime) error {
		if runtime == nil || runtime.Factory == nil {
			return factorysessions.ErrRuntimeNotAvailable
		}
		observeResult, observeErr := runtime.Factory.Observe(ctx, factoryruntime.ObserveRequest{
			Scope: factoryruntime.ObservationScopeFull,
		})
		if observeErr != nil {
			return observeErr
		}
		facts = runtimeSnapshotFactsFromObservation(observeResult.Observation)
		return nil
	})
	return facts, err
}

func runtimeSnapshotFactsFromObservation(
	observation factoryruntime.Observation,
) *liveviewprojection.RuntimeSnapshotFacts {
	return &liveviewprojection.RuntimeSnapshotFacts{
		RuntimeObservation: liveviewprojection.RuntimeObservation{
			TickCount:     observation.Progress.TickCount,
			FactoryState:  observation.Health.FactoryState,
			RuntimeStatus: factorydefinitions.RuntimeStatus(observation.Status),
			Uptime:        observation.Health.Uptime,
		},
		ActiveThrottlePauses: append(
			[]factorydefinitions.ActiveThrottlePause(nil),
			observation.Health.ActiveThrottlePauses...,
		),
	}
}
