// Package wire constructs the private Models Runtime Host subservice.
package wire

import (
	"fmt"
	"reflect"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/service"
	hostleases "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases"
	leaseswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases/wire"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

// NewService constructs an inert Runtime Host over accepted runtime-scope and
// assets contracts. Construction validates injected supervision effects and
// allocates host state only; it does not launch subprocesses.
func NewService(
	scopes runtimescopes.Service,
	assets scopedassets.Service,
	leases HostLeases,
	state *SlotState,
	processLauncher modelseffects.HostProcessLauncher,
	hostHTTP modelseffects.HostHTTPDoer,
	hostClock modelseffects.HostClock,
	hostLogger modelseffects.HostDiagnosticLogger,
	hostMetrics modelseffects.HostMetricsRecorder,
	platform models.AssetHostPlatform,
	protocol modelseffects.HostProtocolNegotiator,
	compatibility modelseffects.HostCompatibilityChecker,
	resolveSymlinks modelseffects.HostResolveSymlinks,
	evidence modelseffects.RuntimeEvidenceRecorder,
	idleUnloadAfter time.Duration,
	maxLoadedRuntimes int,
) (runtimehost.Service, error) {
	if scopes == nil {
		return nil, fmt.Errorf("%w: Models Runtime Scopes service is required", models.ErrInvalidHostDependencies)
	}
	if isNilDependency(assets) {
		return nil, fmt.Errorf("%w: Models Assets service is required", models.ErrInvalidHostDependencies)
	}
	if isNilDependency(processLauncher) {
		return nil, fmt.Errorf("%w: model host process launcher is required", models.ErrInvalidHostDependencies)
	}
	if isNilDependency(hostHTTP) {
		return nil, fmt.Errorf("%w: model host HTTP client is required", models.ErrInvalidHostDependencies)
	}
	if isNilDependency(hostClock) {
		return nil, fmt.Errorf("%w: model host clock is required", models.ErrInvalidHostDependencies)
	}
	return internalservice.New(
		scopes,
		assets, leases, state,
		processLauncher,
		hostHTTP,
		hostClock,
		hostLogger,
		hostMetrics,
		platform, protocol, compatibility, resolveSymlinks, evidence, idleUnloadAfter, maxLoadedRuntimes,
	), nil
}

func isNilDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// SlotState and HostLeases expose construction types through the enclosing owner.
type SlotState = internalservice.SlotState
type HostLeases = hostleases.Service

func NewSlotState() *SlotState { return internalservice.NewSlotState() }
func NewSlotFacts(scopes runtimescopes.Service, assets scopedassets.Service, state *SlotState) modelseffects.SlotFactsProvider {
	return internalservice.NewSlotFacts(scopes, assets, state)
}
func NewSlotCoordinator(state *SlotState, scopes runtimescopes.Service, clock modelseffects.HostClock, logger modelseffects.HostDiagnosticLogger, metrics modelseffects.HostMetricsRecorder, idleUnloadAfter time.Duration) modelseffects.SlotCapacityCoordinator {
	return internalservice.NewSlotCoordinator(state, scopes, clock, logger, metrics, idleUnloadAfter)
}
func NewLeases(clock modelseffects.HostClock, facts modelseffects.SlotFactsProvider, coordinator modelseffects.SlotCapacityCoordinator) (HostLeases, error) {
	return leaseswire.NewService(clock, facts, coordinator)
}
