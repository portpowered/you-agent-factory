// Package wire constructs the private Models Inference subservice.
package wire

import (
	"fmt"
	"reflect"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	modelcatalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	inferenceartifacts "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference/internal/artifacts"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference/internal/service"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

// NewInvocationArtifactExporter keeps artifact materialization behind the
// Inference owner. Models wire composes this provider without importing the
// private artifact implementation directly.
func NewInvocationArtifactExporter(
	fileSystem modelseffects.InvocationArtifactFileSystem,
) (modelseffects.InvocationArtifactExporter, error) {
	return inferenceartifacts.NewExporter(fileSystem)
}

// InvocationArtifactRegistrar is the completed artifact registration resource.
type InvocationArtifactRegistrar = inferenceartifacts.Registrar

// NewInvocationArtifactRegistrar constructs the completed artifact resource.
func NewInvocationArtifactRegistrar(fileSystem modelseffects.InvocationArtifactFileSystem) (*InvocationArtifactRegistrar, error) {
	return inferenceartifacts.NewRegistrar(fileSystem)
}

// NewExecutionDeadline selects the existing execution policy at composition.
func NewExecutionDeadline() func() time.Duration {
	return func() time.Duration { return 30 * time.Minute }
}

// NewService constructs an inert Inference owner over completed collaborators.
func NewService(
	scopes runtimescopes.Service,
	assets scopedassets.Service,
	catalog modelcatalog.Service,
	runtimeHost runtimehost.Service,
	invocationRuntime internalservice.InvocationRuntime,
	artifactRegistrar *InvocationArtifactRegistrar,
	clock func() time.Time,
	executionDeadline func() time.Duration,
) (inference.Service, error) {
	if scopes == nil {
		return nil, fmt.Errorf(
			"%w: Models Runtime Scopes service is required",
			models.ErrInvalidInferenceDependencies,
		)
	}
	if isNilDependency(assets) {
		return nil, fmt.Errorf(
			"%w: Models Assets service is required",
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
	if artifactRegistrar == nil {
		return nil, fmt.Errorf("%w: Models Inference artifact registrar is required", models.ErrInvalidInferenceDependencies)
	}
	if executionDeadline == nil {
		return nil, fmt.Errorf("%w: Models Inference execution deadline is required", models.ErrInvalidInferenceDependencies)
	}
	return internalservice.New(
		scopes,
		assets,
		catalog,
		runtimeHost,
		invocationRuntime,
		artifactRegistrar,
		clock,
		executionDeadline,
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
