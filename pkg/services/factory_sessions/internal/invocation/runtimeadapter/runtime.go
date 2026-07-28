// Package runtimeadapter binds the Factory Session invocation owner to live
// Factory Runtime state without adding runtime construction dependencies to
// the transport-facing invocation contract package.
package runtimeadapter

import (
	"context"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

// Observe derives one invocation wait observation from canonical runtime state
// and event history.
func Observe(
	ctx context.Context,
	state *sessionruntime.Service,
	sessionID string,
	input sessioninvocation.SessionInvocationWaitInput,
	worldStateProjector factory.WorldStateProjector,
) (sessioninvocation.SessionInvocationObservation, error) {
	activeFactory, err := runtimebinding.FactoryForSession(state, sessionID)
	if err != nil {
		return sessioninvocation.SessionInvocationObservation{}, err
	}
	eventSource, err := runtimebinding.LegacyEventSourceForService(activeFactory)
	if err != nil {
		return sessioninvocation.SessionInvocationObservation{}, err
	}
	observeResult, err := activeFactory.Observe(ctx, factory.ObserveRequest{
		Scope: factory.ObservationScopeFull,
	})
	if err != nil {
		return sessioninvocation.SessionInvocationObservation{}, err
	}
	observation := observeResult.Observation
	events, err := eventSource.GetFactoryEvents(ctx)
	if err != nil {
		return sessioninvocation.SessionInvocationObservation{}, err
	}
	if worldStateProjector == nil {
		return sessioninvocation.SessionInvocationObservation{}, fmt.Errorf("Recordings world-state projector is required")
	}
	worldState, err := worldStateProjector(events, observation.Progress.TickCount)
	if err != nil {
		return sessioninvocation.SessionInvocationObservation{}, err
	}

	var missingPrimary *work.PrimaryResultError
	if strings.TrimSpace(input.RequestID) != "" {
		legacyObservation, legacyErr := runtimebinding.LegacyObservationForService(activeFactory)
		if legacyErr != nil {
			return sessioninvocation.SessionInvocationObservation{}, legacyErr
		}
		snapshot, snapshotErr := legacyObservation.GetEngineStateSnapshot(ctx)
		if snapshotErr != nil {
			return sessioninvocation.SessionInvocationObservation{}, snapshotErr
		}
		missingPrimary = sessioninvocation.ClassifyMissingPrimaryResultFromSnapshot(sessionID, snapshot, input)
	}

	return sessioninvocation.SessionInvocationObservation{
		WorldState:           worldState,
		FactoryState:         observation.Health.FactoryState,
		ActiveWork:           factory.ObservationHasActiveWork(observation),
		MissingPrimaryResult: missingPrimary,
	}, nil
}

// WriteLogRecord adapts a canonical safe invocation log record to zap.
func WriteLogRecord(logger *zap.Logger, record sessioninvocation.SessionInvocationLogRecord) {
	fields := make([]zap.Field, 0, len(record.Fields)+1)
	for key, value := range record.Fields {
		fields = append(fields, zap.Any(key, value))
	}
	if record.Error != nil {
		fields = append(fields, zap.Error(record.Error))
	}
	if record.Level == "warn" {
		logger.Warn(record.Message, fields...)
		return
	}
	logger.Info(record.Message, fields...)
}
