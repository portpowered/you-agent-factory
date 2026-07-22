package factorysessions

import (
	"context"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// InvocationMetric records one emitted runtime counter together with its
// low-cardinality dimensions.
type InvocationMetric struct {
	Name   string
	Labels map[string]string
}

// InvocationMetricsRecorder receives invocation counter emissions.
type InvocationMetricsRecorder interface {
	RecordInvocationMetric(InvocationMetric)
}

// FactoryScaffoldInitializer initializes a newly selected Factory directory.
type FactoryScaffoldInitializer func(string) error

// EditableFactoryValidator validates a detached Factory definition before
// persistence.
type EditableFactoryValidator func(
	context.Context,
	*interfaces.FactorySnapshot,
	interfaces.WorkstationLoader,
) error

// ReconnectCursorValidator validates an acknowledged cursor against a
// detached canonical event snapshot. Recordings supplies the implementation.
type ReconnectCursorValidator func(
	[]interfaces.FactoryEvent,
	interfaces.FactoryEventReconnectCursor,
	interfaces.FactoryEventReconnectScope,
) error

// RuntimeResolver resolves one live Factory Session and its attached runtime.
type RuntimeResolver interface {
	Resolve(sessionID string) *LiveSession
}

// CurrentRuntimeResolver exposes the currently selected live runtime without
// exposing the Factory Sessions implementation.
type CurrentRuntimeResolver interface {
	CurrentRuntime() *LiveRuntime
}

// RuntimeReader executes a read against the currently selected live runtime
// without exposing the Factory Sessions registry implementation.
type RuntimeReader interface {
	WithRuntimeRead(func(*LiveRuntime) error) error
}
