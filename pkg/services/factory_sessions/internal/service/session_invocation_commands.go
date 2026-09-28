package service

import (
	"context"
	"fmt"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/models"
)

// InvokeModelForSession uses the model invoker retained on the canonical live
// session. No invocation-specific runtime service or registry is allocated.
func (r *Root) InvokeModelForSession(ctx context.Context, sessionID, modelName string, request models.Request) (models.Result, error) {
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		return models.Result{}, err
	}
	if bound.ModelInvoker == nil {
		return models.Result{}, fmt.Errorf("%w: model invoker for session %q", factorysessions.ErrRuntimeNotAvailable, sessionID)
	}
	return bound.ModelInvoker.InvokeModel(ctx, modelName, request)
}

// ResolveInvocationInputForSession applies the selected Factory signature
// through the input resolver attached to that same live session.
func (r *Root) ResolveInvocationInputForSession(_ context.Context, sessionID string, request factorysessions.InvocationRequest) (factorysessions.ResolvedInvocationInput, error) {
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		return factorysessions.ResolvedInvocationInput{}, err
	}
	if bound.InputResolver == nil {
		return factorysessions.ResolvedInvocationInput{}, fmt.Errorf("%w: input resolver for session %q", factorysessions.ErrRuntimeNotAvailable, sessionID)
	}
	config, err := runtimebinding.RuntimeConfigForSession(r, sessionID)
	if err != nil {
		return factorysessions.ResolvedInvocationInput{}, err
	}
	return bound.InputResolver.ResolveInvocationInput(config.FactoryConfig(), request)
}

func (r *Root) ModelsScopeForSession(_ context.Context, sessionID string) (models.RuntimeScopeRef, error) {
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		return models.RuntimeScopeRef{}, err
	}
	return bound.ModelsScope, nil
}
