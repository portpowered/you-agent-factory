package executionopening

import (
	"context"
	"errors"
	"fmt"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
)

// StdioOpeningService owns the Factory Session selection and opening policy
// for one MCP stdio invocation. Its builder ports adapt already-injected
// transport and lifecycle machinery without exposing those implementations.
type StdioOpeningService struct {
	opening      roles.StdioExecutionOpening
	buildFixture roles.FixtureStdioApplicationBuilder
	buildRuntime roles.RuntimeStdioApplicationBuilder
}

func NewStdioOpeningService(
	opening roles.StdioExecutionOpening,
	buildFixture roles.FixtureStdioApplicationBuilder,
	buildRuntime roles.RuntimeStdioApplicationBuilder,
) (*StdioOpeningService, error) {
	if opening == nil {
		return nil, fmt.Errorf("session execution opening factory is required")
	}
	if buildFixture == nil || buildRuntime == nil {
		return nil, fmt.Errorf("stdio application builders are required")
	}
	return &StdioOpeningService{
		opening: opening, buildFixture: buildFixture, buildRuntime: buildRuntime,
	}, nil
}

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func (service *StdioOpeningService) OpenStdio(
	ctx context.Context,
	request factorysessions.StdioOpeningRequest,
) (roles.StdioApplication, error) {
	if service == nil || service.opening == nil {
		return nil, fmt.Errorf("session execution opening factory is required")
	}
	if ctx == nil {
		return nil, fmt.Errorf("context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request.Input == nil || request.Output == nil {
		return nil, fmt.Errorf("stdio streams are required")
	}
	if request.RuntimeBacked {
		root, err := service.opening.ResolveProjectRoot(request.ProjectRoot)
		if err != nil {
			return nil, err
		}
		opened, err := service.opening.OpenExecutionRuntime(ctx, factorysessions.ExecutionRuntimeOpeningRequest{
			ProjectRoot: root, SystemConfigHome: request.SystemConfigHome,
		})
		if err != nil {
			return nil, err
		}
		application, err := service.buildRuntime(ctx, opened, request.Input, request.Output)
		if err != nil {
			return nil, err
		}
		if application == nil {
			return nil, fmt.Errorf("runtime stdio application is required")
		}
		return application, nil
	}

	execution, err := service.opening.Build(ctx, "", "", request.FixtureCatalogPath, "")
	if err != nil {
		return nil, err
	}
	application, err := service.buildFixture(ctx, execution, request.Input, request.Output)
	if err == nil && application != nil {
		return application, nil
	}
	if err == nil {
		err = fmt.Errorf("fixture stdio application is required")
	}
	if closeErr := execution.Close(); closeErr != nil {
		err = errors.Join(err, fmt.Errorf("close session execution: %w", closeErr))
	}
	return nil, err
}
