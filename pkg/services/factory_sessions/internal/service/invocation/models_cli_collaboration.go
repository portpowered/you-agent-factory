package invocation

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func (o *operation) OpenModelsCatalogScope(
	ctx context.Context,
) (models.PresentationScope, error) {
	if o == nil || o.openRuntime == nil {
		return models.PresentationScope{}, errors.New("invocation operation is required")
	}
	root := o.modelsRoot
	if root == nil {
		return models.PresentationScope{}, errors.New("models presentation root is unavailable")
	}
	opened, err := root.OpenRuntimeScope(ctx, models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{Runtime: models.RuntimeConfig{}},
	})
	if err != nil {
		return models.PresentationScope{}, err
	}
	scope := opened.Scope
	closeFn := func(closeCtx context.Context) error {
		closed, closeErr := root.CloseRuntimeScope(closeCtx, models.CloseRuntimeScopeRequest{Scope: scope})
		if closeErr != nil {
			return closeErr
		}
		if !closed.Closed {
			return errors.New("close Models catalog scope: scope was not closed")
		}
		return nil
	}
	return models.PresentationScope{Scope: scope, Close: closeFn}, nil
}

func (o *operation) OpenModelsPresentationScope(
	ctx context.Context,
	request models.PresentationScopeRequest,
) (models.PresentationScope, error) {
	if o == nil {
		return models.PresentationScope{}, errors.New("invocation operation is required")
	}
	factoryDir, err := o.resolveModelInvocationFactoryDir(request.FactoryDir, request.WorkingDirectory)
	if err != nil {
		return models.PresentationScope{}, err
	}
	target := roles.InvocationTarget{
		FactoryDir:       factoryDir,
		HomeDir:          request.HomeDir,
		OperatorDefaults: resolvedOperatorDefaultsFromPresentation(request.OperatorDefaults),
		Verbose:          request.Verbose,
		ModelCacheDir:    request.ModelCacheDir,
	}
	opened, lifecycle, err := o.open(ctx, target)
	if err != nil {
		return models.PresentationScope{}, err
	}
	if opened.ModelsScope.IsZero() {
		closeErr := lifecycle.close(ctx, opened)
		return models.PresentationScope{}, errors.Join(
			errors.New("models presentation scope is unavailable"),
			closeErr,
		)
	}
	return models.PresentationScope{
		Scope: opened.ModelsScope,
		Close: func(closeCtx context.Context) error {
			return lifecycle.close(closeCtx, opened)
		},
	}, nil
}

func resolvedOperatorDefaultsFromPresentation(
	defaults models.PresentationOperatorDefaults,
) operatorconfig.ResolvedDefaults {
	return operatorconfig.ResolvedDefaults{
		WorkerModelProvider: defaults.WorkerModelProvider,
		WorkerModel:         defaults.WorkerModel,
	}
}

// The two scope methods are consumed only by the application Wire adapter.
// They are deliberately absent from roles.InvocationOperation and from the
// Factory Sessions service root; Models transport composition receives them as
// an explicit Wire-owned port.
