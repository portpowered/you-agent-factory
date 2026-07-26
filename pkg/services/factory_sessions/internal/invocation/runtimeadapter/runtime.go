// Package runtimeadapter binds the Factory Session invocation owner to live
// Factory Runtime state without adding runtime construction dependencies to
// the transport-facing invocation contract package.
package runtimeadapter

import (
	"context"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"go.uber.org/zap"
)

// FactoryConfig resolves the canonical Factory configuration for one
// registered live session.
func FactoryConfig(state *sessionruntime.Service, sessionID string) (*interfaces.FactoryConfig, error) {
	runtimeConfig, err := runtimebinding.RuntimeConfigForSession(state, sessionID)
	if err != nil || runtimeConfig == nil {
		return nil, err
	}
	return runtimeConfig.FactoryConfig(), nil
}

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
	legacyObservation, err := runtimebinding.LegacyObservationForService(activeFactory)
	if err != nil {
		return sessioninvocation.SessionInvocationObservation{}, err
	}
	snapshot, err := legacyObservation.GetEngineStateSnapshot(ctx)
	if err != nil {
		return sessioninvocation.SessionInvocationObservation{}, err
	}
	events, err := activeFactory.GetFactoryEvents(ctx)
	if err != nil {
		return sessioninvocation.SessionInvocationObservation{}, err
	}
	if worldStateProjector == nil {
		return sessioninvocation.SessionInvocationObservation{}, fmt.Errorf("Recordings world-state projector is required")
	}
	worldState, err := worldStateProjector(events, snapshot.TickCount)
	if err != nil {
		return sessioninvocation.SessionInvocationObservation{}, err
	}
	return sessioninvocation.SessionInvocationObservation{
		WorldState: worldState, FactoryState: snapshot.FactoryState,
		ActiveWork:           factory.SnapshotHasActiveWork(snapshot),
		MissingPrimaryResult: sessioninvocation.ClassifyMissingPrimaryResultFromSnapshot(sessionID, snapshot, input),
	}, nil
}

// WriteLogRecord adapts a canonical safe invocation log record to zap.
func WriteLogRecord(logger *zap.Logger, record sessioninvocation.SessionInvocationLogRecord) {
	if logger == nil {
		return
	}
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
