// Package wire constructs the private Models Inference subservice.
package wire

import (
	"fmt"
	"reflect"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelcatalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference/internal/service"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

// NewService constructs an inert Inference owner over accepted runtime-scope,
// catalog, and runtime-host contracts. Construction validates injected effects
// and allocates inference state only; it does not launch subprocesses.
func NewService(
	scopes runtimescopes.Service,
	catalog modelcatalog.Service,
	runtimeHost runtimehost.Service,
	invocationRuntime inference.InvocationRuntime,
	clock func() time.Time,
) (inference.Service, error) {
	if scopes == nil {
		return nil, fmt.Errorf(
			"%w: Models Runtime Scopes service is required",
			models.ErrInvalidInferenceDependencies,
		)
	}
	if isNilDependency(catalog) {
		return nil, fmt.Errorf(
			"%w: Models Catalog service is required",
			models.ErrInvalidInferenceDependencies,
		)
	}
	if isNilDependency(runtimeHost) {
		return nil, fmt.Errorf(
			"%w: Models Runtime Host service is required",
			models.ErrInvalidInferenceDependencies,
		)
	}
	if isNilDependency(invocationRuntime) {
		return nil, fmt.Errorf(
			"%w: Models Inference invocation runtime is required",
			models.ErrInvalidInferenceDependencies,
		)
	}
	if clock == nil {
		return nil, fmt.Errorf(
			"%w: Models Inference clock is required",
			models.ErrInvalidInferenceDependencies,
		)
	}
	return internalservice.New(scopes, catalog, runtimeHost, invocationRuntime, clock), nil
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
