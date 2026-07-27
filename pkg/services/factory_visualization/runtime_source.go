package factory_visualization

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
	visualizationcontracts "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/contracts"
)

type currentRuntimeSource struct {
	reader RuntimeReader
}

// RuntimeReader is the exact consumer-owned live-runtime observation role.
type RuntimeReader = visualizationcontracts.RuntimeReader

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
		var subscribeErr error
		legacyRuntime, ok := runtime.Factory.(factoryruntime.APIFactory)
		if !ok {
			return fmt.Errorf("legacy Factory Runtime event subscription is required")
		}
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
		legacyObservation, ok := runtime.Factory.(factoryruntime.APIFactory)
		if !ok {
			return factorysessions.ErrRuntimeNotAvailable
		}
		snapshot, snapshotErr := legacyObservation.GetEngineStateSnapshot(ctx)
		if snapshotErr != nil {
			return snapshotErr
		}
		facts = sanitizeRuntimeSnapshotFacts(snapshot)
		return nil
	})
	return facts, err
}

func sanitizeRuntimeSnapshotFacts(
	snapshot *factoryruntime.StateSnapshot,
) *liveviewprojection.RuntimeSnapshotFacts {
	if snapshot == nil {
		return nil
	}
	return &liveviewprojection.RuntimeSnapshotFacts{
		RuntimeObservation: liveviewprojection.RuntimeObservation{
			TickCount:     snapshot.TickCount,
			FactoryState:  snapshot.FactoryState,
			RuntimeStatus: snapshot.RuntimeStatus,
			Uptime:        snapshot.Uptime,
		},
		ActiveThrottlePauses: append(
			[]factorydefinitions.ActiveThrottlePause(nil),
			snapshot.ActiveThrottlePauses...,
		),
	}
}
