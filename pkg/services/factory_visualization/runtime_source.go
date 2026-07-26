package factory_visualization

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
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

func (s *currentRuntimeSource) GetEngineStateSnapshot(
	ctx context.Context,
) (snapshot *factoryruntime.StateSnapshot, err error) {
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
		var snapshotErr error
		snapshot, snapshotErr = legacyObservation.GetEngineStateSnapshot(ctx)
		return snapshotErr
	})
	return snapshot, err
}
