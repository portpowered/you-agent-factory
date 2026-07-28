package invocation

import (
	"context"
	"errors"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/models"
)

func (o *operation) ModelsPresentationRoot() models.Service {
	if o == nil || o.openRuntime == nil {
		return nil
	}
	return o.openRuntime.ModelsRoot()
}

func (o *operation) OpenModelsCatalogScope(
	ctx context.Context,
) (factorysessions.ModelsPresentationScope, error) {
	if o == nil || o.openRuntime == nil {
		return factorysessions.ModelsPresentationScope{}, errors.New("invocation operation is required")
	}
	o.catalogScopeMu.Lock()
	defer o.catalogScopeMu.Unlock()
	if !o.catalogScope.IsZero() {
		return factorysessions.ModelsPresentationScope{
			Scope: o.catalogScope,
			Close: o.catalogScopeClose,
		}, nil
	}
	root := o.openRuntime.ModelsRoot()
	if root == nil {
		return factorysessions.ModelsPresentationScope{}, errors.New("models presentation root is unavailable")
	}
	opened, err := root.OpenRuntimeScope(ctx, models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{Runtime: models.RuntimeConfig{}},
	})
	if err != nil {
		return factorysessions.ModelsPresentationScope{}, err
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
	return factorysessions.ModelsPresentationScope{Scope: scope, Close: closeFn}, nil
}

func (o *operation) OpenModelsPresentationScope(
	ctx context.Context,
	request factorysessions.ModelsPresentationScopeRequest,
) (factorysessions.ModelsPresentationScope, error) {
	if o == nil {
		return factorysessions.ModelsPresentationScope{}, errors.New("invocation operation is required")
	}
	factoryDir, err := o.ResolveModelInvocationFactoryDir(request.FactoryDir)
	if err != nil {
		return factorysessions.ModelsPresentationScope{}, err
	}
	target := roles.InvocationTarget{
		FactoryDir:       factoryDir,
		HomeDir:          request.HomeDir,
		OperatorDefaults: request.OperatorDefaults,
		Logger:           request.Logger,
		Verbose:          request.Verbose,
		ModelCacheDir:    request.ModelCacheDir,
	}
	opened, lifecycle, err := o.open(ctx, target)
	if err != nil {
		return factorysessions.ModelsPresentationScope{}, err
	}
	if opened.ModelsScope.IsZero() {
		closeErr := lifecycle.close(ctx, opened)
		return factorysessions.ModelsPresentationScope{}, errors.Join(
			errors.New("models presentation scope is unavailable"),
			closeErr,
		)
	}
	return factorysessions.ModelsPresentationScope{
		Scope: opened.ModelsScope,
		Close: func(closeCtx context.Context) error {
			return lifecycle.close(closeCtx, opened)
		},
	}, nil
}

var _ factorysessions.ModelsCLIPresentationCollaborator = (*operation)(nil)
