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
	opening       roles.StdioExecutionOpening
	buildFixture  roles.FixtureStdioApplicationBuilder
	buildRuntime  roles.RuntimeStdioApplicationBuilder
	presentations factorysessions.OpeningPresentationOwner
}

func NewStdioOpeningService(
	opening roles.StdioExecutionOpening,
	buildFixture roles.FixtureStdioApplicationBuilder,
	buildRuntime roles.RuntimeStdioApplicationBuilder,
	presentations ...factorysessions.OpeningPresentationOwner,
) (*StdioOpeningService, error) {
	if opening == nil {
		return nil, fmt.Errorf("session execution opening factory is required")
	}
	if buildFixture == nil || buildRuntime == nil {
		return nil, fmt.Errorf("stdio application builders are required")
	}
	var presentationOwner factorysessions.OpeningPresentationOwner
	if len(presentations) > 0 {
		presentationOwner = presentations[0]
	}
	return &StdioOpeningService{
		opening: opening, buildFixture: buildFixture, buildRuntime: buildRuntime,
		presentations: presentationOwner,
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
	presentation, err := service.presentationScope(request.ScopeID)
	if err != nil {
		return nil, err
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
		application, err := service.buildRuntime(ctx, opened, presentation.Input, presentation.Output)
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
	application, err := service.buildFixture(ctx, execution, presentation.Input, presentation.Output)
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

func (service *StdioOpeningService) presentationScope(id factorysessions.OpeningScopeID) (factorysessions.StdioOpeningScope, error) {
	if id == "" || service.presentations == nil {
		return factorysessions.StdioOpeningScope{}, fmt.Errorf("stdio opening presentation scope %q is unavailable", id)
	}
	scope, ok := service.presentations.Stdio(id)
	if !ok {
		return factorysessions.StdioOpeningScope{}, fmt.Errorf("stdio opening presentation scope %q is unavailable", id)
	}
	return scope, nil
}
