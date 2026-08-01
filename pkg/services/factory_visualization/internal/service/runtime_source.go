package service

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
)

type currentRuntimeSource struct {
	reader RuntimeReader
}

// NewCurrentRuntimeSource adapts the currently selected Factory Session to the
// exact observation capability consumed by Factory Visualization.
func NewCurrentRuntimeSource(reader RuntimeReader) Source {
	return &currentRuntimeSource{reader: reader}
}

func (s *currentRuntimeSource) SubscribeFactoryEvents(
	ctx context.Context,
	reconnect *factorydefinitions.FactoryEventReconnectCursor,
	scope factorydefinitions.FactoryEventReconnectScope,
) (stream *factorydefinitions.FactoryEventStream, err error) {
	if s == nil || s.reader == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	err = s.reader.WithRuntimeRead(func(runtime *factorysessions.LiveRuntime) error {
		if runtime == nil || runtime.Factory == nil {
			return factorysessions.ErrRuntimeNotAvailable
		}
		legacyRuntime, ok := runtime.Factory.(factoryruntime.APIFactory)
		if !ok {
			return fmt.Errorf("legacy Factory Runtime event subscription is required")
		}
		var subscribeErr error
		stream, subscribeErr = legacyRuntime.SubscribeFactoryEvents(ctx, reconnect, scope)
		return subscribeErr
	})
	return stream, err
}

func (s *currentRuntimeSource) GetRuntimeSnapshotFacts(
	ctx context.Context,
) (facts *liveviewprojection.RuntimeSnapshotFacts, err error) {
	if s == nil || s.reader == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	err = s.reader.WithRuntimeRead(func(runtime *factorysessions.LiveRuntime) error {
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
