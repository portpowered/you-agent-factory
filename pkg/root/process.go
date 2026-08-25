package root

import (
	"context"
	"fmt"

	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/wire"
)

// BuildProcess constructs the reusable application process. Production passes
// an empty edge set; functional tests replace only their external boundaries.
func BuildProcess(
	ctx context.Context,
	edges serviceedges.Edges,
) (*initializerapplication.Process, error) {
	if ctx == nil {
		return nil, fmt.Errorf("build application process: context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("build application process: %w", err)
	}
	applicationProcess, err := wire.InjectBundle(ctx, edges)
	if err != nil {
		return nil, fmt.Errorf("build application process: %w", err)
	}
	return applicationProcess, nil
}

// DetachedOperationsFromProcess returns the Factory Sessions detached
// operation view carried by the canonical process composition. The process is
// accepted as an opaque value so narrow functional harness interfaces can pass
// their underlying application process without exposing the application graph.
func DetachedOperationsFromProcess(process any) factorysessions.DetachedService {
	applicationProcess, ok := process.(*initializerapplication.Process)
	if !ok || applicationProcess == nil {
		return nil
	}
	capability := applicationProcess.DetachedOperations()
	if capability == nil {
		return nil
	}
	operations, _ := capability.DetachedOperations().(factorysessions.DetachedService)
	return operations
}

// RuntimeMetricsQueryFromProcess returns the Factory Visualization metrics
// query carried by the canonical process composition. The process is accepted
// as an opaque value so callers can use the same narrow boundary in functional
// harnesses without exposing the application graph.
func RuntimeMetricsQueryFromProcess(process any) factoryvisualization.RuntimeMetricsQuery {
	applicationProcess, ok := process.(*initializerapplication.Process)
	if !ok || applicationProcess == nil {
		return nil
	}
	capability := applicationProcess.RuntimeMetricsQuery()
	if capability == nil {
		return nil
	}
	query, _ := capability.RuntimeMetricsQuery().(factoryvisualization.RuntimeMetricsQuery)
	return query
}

// CostsQueryFromProcess returns the stateless Costs operation carried by the
// canonical process composition.
func CostsQueryFromProcess(process any) costs.CostsQuery {
	applicationProcess, ok := process.(*initializerapplication.Process)
	if !ok || applicationProcess == nil {
		return nil
	}
	capability := applicationProcess.RuntimeCostsQuery()
	if capability == nil {
		return nil
	}
	query, _ := capability.RuntimeCostsQuery().(costs.CostsQuery)
	return query
}

// RecordingsProjection is the narrow Recordings read capability exposed by a
// canonical process for the generated HTTP projection boundary.
type RecordingsProjection interface {
	ReconstructFactoryWorldState([]recordings.FactoryEvent, int) (recordings.FactoryWorldState, error)
	ProjectWorkstationRequests(recordings.FactoryWorldState) recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice
}

// RecordingsProjectionFromProcess returns the already-composed Recordings
// projection capability carried by a canonical process.
func RecordingsProjectionFromProcess(process any) RecordingsProjection {
	applicationProcess, ok := process.(*initializerapplication.Process)
	if !ok || applicationProcess == nil {
		return nil
	}
	capability := applicationProcess.RecordingsProjection()
	if capability == nil {
		return nil
	}
	projection, _ := capability.RecordingsProjection().(RecordingsProjection)
	return projection
}

// OperatorSettingsFromProcess returns the already-composed Operator Settings
// root carried by a canonical process. No service is constructed at this
// boundary.
func OperatorSettingsFromProcess(process any) operatorsettings.Service {
	applicationProcess, ok := process.(*initializerapplication.Process)
	if !ok || applicationProcess == nil {
		return nil
	}
	capability := applicationProcess.OperatorSettings()
	if capability == nil {
		return nil
	}
	settings, _ := capability.OperatorSettings().(operatorsettings.Service)
	return settings
}

// ExecutionRuntimeOpening is the root-owned, narrow durable-execution opening
// view carried by a canonical application process.
type ExecutionRuntimeOpening interface {
	OpenExecutionRuntime(context.Context, factorysessions.ExecutionRuntimeOpeningRequest) (factorysessions.OpenedExecutionRuntime, error)
}

type executionRuntimeOpeningAdapter struct {
	opening factorysessions.ExecutionRuntimeOpeningFunc
}

func (adapter executionRuntimeOpeningAdapter) OpenExecutionRuntime(
	ctx context.Context,
	request factorysessions.ExecutionRuntimeOpeningRequest,
) (factorysessions.OpenedExecutionRuntime, error) {
	return adapter.opening(ctx, request)
}

// ExecutionRuntimeOpeningFromProcess returns the canonical Factory Sessions
// durable-execution opening composed for a root process. The process remains
// opaque to callers; only the narrow service-owned opening contract crosses
// this root boundary.
func ExecutionRuntimeOpeningFromProcess(process any) ExecutionRuntimeOpening {
	applicationProcess, ok := process.(*initializerapplication.Process)
	if !ok || applicationProcess == nil {
		return nil
	}
	capability := applicationProcess.ExecutionRuntimeOpening()
	if capability == nil {
		return nil
	}
	opening, _ := capability.ExecutionRuntimeOpening().(factorysessions.ExecutionRuntimeOpeningFunc)
	if opening == nil {
		return nil
	}
	return executionRuntimeOpeningAdapter{opening: opening}
}

// BuildStatelessWorkers constructs the standalone Workers Execute root. It
// intentionally does not construct or open a Factory Runtime or Factory
// Session, so callers can submit one detached attempt directly.
func BuildStatelessWorkers(
	ctx context.Context,
	edges serviceedges.Edges,
) (workers.Service, error) {
	if ctx == nil {
		return nil, fmt.Errorf("build stateless Workers: context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("build stateless Workers: %w", err)
	}
	service, err := wire.BuildStatelessWorkers(ctx, edges)
	if err != nil {
		return nil, fmt.Errorf("build stateless Workers: %w", err)
	}
	return service, nil
}

// BuildMockStatelessWorkers constructs the explicit mock-feature Workers root.
// The mock registry is selected only by this opt-in root boundary; ordinary
// BuildProcess and BuildStatelessWorkers composition remain production-only.
func BuildMockStatelessWorkers(
	ctx context.Context,
	edges serviceedges.Edges,
	mockWorkers *workers.MockWorkersConfig,
) (workers.Service, error) {
	if ctx == nil {
		return nil, fmt.Errorf("build mock stateless Workers: context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("build mock stateless Workers: %w", err)
	}
	service, err := wire.BuildMockStatelessWorkers(ctx, edges, mockWorkers)
	if err != nil {
		return nil, fmt.Errorf("build mock stateless Workers: %w", err)
	}
	return service, nil
}
