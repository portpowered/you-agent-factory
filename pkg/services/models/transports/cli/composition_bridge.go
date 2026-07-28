package cli

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
)

type compositionCollaboratorInvocation struct {
	InvocationOperation
	collaborator factorysessions.ModelsCLIPresentationCollaborator
}

func (inv compositionCollaboratorInvocation) CompositionModelsRoot() modelinference.Service {
	return inv.collaborator.ModelsPresentationRoot()
}

func (inv compositionCollaboratorInvocation) CompositionOpenCatalogScope(
	ctx context.Context,
) (InvokeRuntimeScope, error) {
	opened, err := inv.collaborator.OpenModelsCatalogScope(ctx)
	if err != nil {
		return InvokeRuntimeScope{}, err
	}
	return InvokeRuntimeScope{Scope: opened.Scope, Close: opened.Close}, nil
}

func (inv compositionCollaboratorInvocation) CompositionOpenInvokeScope(
	ctx context.Context,
	cfg InvokeConfig,
) (InvokeRuntimeScope, error) {
	opened, err := inv.collaborator.OpenModelsPresentationScope(ctx, presentationScopeRequestFromInvoke(cfg))
	if err != nil {
		return InvokeRuntimeScope{}, err
	}
	return InvokeRuntimeScope{Scope: opened.Scope, Close: opened.Close}, nil
}

func adaptCompositionInvocation(invocation InvocationOperation) InvocationOperation {
	if invocation == nil {
		return nil
	}
	if _, ok := invocation.(CompositionModelsRoot); ok {
		return invocation
	}
	if collaborator, ok := invocation.(factorysessions.ModelsCLIPresentationCollaborator); ok {
		return compositionCollaboratorInvocation{
			InvocationOperation: invocation,
			collaborator:        collaborator,
		}
	}
	return invocation
}

// AdaptCompositionInvocationForTest exposes composition adaptation for adapter tests.
func AdaptCompositionInvocationForTest(invocation InvocationOperation) InvocationOperation {
	return adaptCompositionInvocation(invocation)
}

func presentationScopeRequestFromInvoke(cfg InvokeConfig) factorysessions.ModelsPresentationScopeRequest {
	return factorysessions.ModelsPresentationScopeRequest{
		FactoryDir:       cfg.FactoryDir,
		HomeDir:          cfg.HomeDir,
		OperatorDefaults: cfg.OperatorDefaults,
		Logger:           cfg.Logger,
		Verbose:          cfg.Verbose,
	}
}
