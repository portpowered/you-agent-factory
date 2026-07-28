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
	o.presentationMu.Lock()
	defer o.presentationMu.Unlock()
	if o.presentationRoot != nil {
		return o.presentationRoot
	}
	root := o.openRuntime.ModelsRoot()
	if root == nil {
		return nil
	}
	bound, err := root.ForRuntime(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig {
			cfg := models.RuntimeConfig{}
			return &cfg
		},
	})
	if err != nil {
		return root
	}
	o.presentationRoot = bound
	return bound
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
