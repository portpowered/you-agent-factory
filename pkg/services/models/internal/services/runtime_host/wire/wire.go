// Package wire constructs the private Models Runtime Host subservice.
package wire

import (
	"fmt"
	"reflect"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/service"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

// NewService constructs an inert Runtime Host over accepted runtime-scope and
// assets contracts. Construction validates injected supervision effects and
// allocates host state only; it does not launch subprocesses.
func NewService(
	scopes runtimescopes.Service,
	assets scopedassets.Service,
	processLauncher modelseffects.HostProcessLauncher,
	hostHTTP modelseffects.HostHTTPDoer,
	hostClock modelseffects.HostClock,
	hostLogger modelseffects.HostDiagnosticLogger,
	hostMetrics modelseffects.HostMetricsRecorder,
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
	return internalservice.NewWired(
		scopes,
		assets,
		processLauncher,
		hostHTTP,
		hostClock,
		hostLogger,
		hostMetrics,
	)
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
