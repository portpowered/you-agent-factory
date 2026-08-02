package invocation

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func (o *operation) ModelsPresentationRoot() models.Service {
	if o == nil || o.openRuntime == nil {
		return nil
	}
	return o.openRuntime.ModelsRoot()
}

func (o *operation) OpenModelsCatalogScope(
	ctx context.Context,
) (models.PresentationScope, error) {
	if o == nil || o.openRuntime == nil {
		return models.PresentationScope{}, errors.New("invocation operation is required")
	}
	o.catalogScopeMu.Lock()
	defer o.catalogScopeMu.Unlock()
	if !o.catalogScope.IsZero() {
		return models.PresentationScope{
			Scope: o.catalogScope,
			Close: o.catalogScopeClose,
		}, nil
	}
	root := o.openRuntime.ModelsRoot()
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
		o.catalogScopeMu.Lock()
		defer o.catalogScopeMu.Unlock()
		if o.catalogScope == scope {
			o.catalogScope = models.RuntimeScopeRef{}
			o.catalogScopeClose = nil
		}
		return nil
	}
	o.catalogScope = scope
	o.catalogScopeClose = closeFn
	return models.PresentationScope{Scope: scope, Close: closeFn}, nil
}

func (o *operation) OpenModelsPresentationScope(
	ctx context.Context,
	request models.PresentationScopeRequest,
) (models.PresentationScope, error) {
	if o == nil {
		return models.PresentationScope{}, errors.New("invocation operation is required")
	}
	factoryDir, err := o.ResolveModelInvocationFactoryDir(request.FactoryDir)
	if err != nil {
		return models.PresentationScope{}, err
	}
	target := roles.InvocationTarget{
		FactoryDir:       factoryDir,
		HomeDir:          request.HomeDir,
		OperatorDefaults: operatorconfig.ResolvedDefaults{},
		Logger:           request.Logger,
		Verbose:          request.Verbose,
		ModelCacheDir:    request.ModelCacheDir,
	}
	if defaults, ok := request.OperatorDefaults.(operatorconfig.ResolvedDefaults); ok {
		target.OperatorDefaults = defaults
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

var _ interface {
	ModelsPresentationRoot() models.Service
	OpenModelsCatalogScope(context.Context) (models.PresentationScope, error)
	OpenModelsPresentationScope(context.Context, models.PresentationScopeRequest) (models.PresentationScope, error)
} = (*operation)(nil)
