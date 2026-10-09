package service

import (
	"context"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

// Invoke passes the selected canonical session to the fixed invocation owner.
func (a *Assembly) Invoke(ctx context.Context, request factorysessions.SessionInvokeRequest) (factorysessions.InvocationResult, error) {
	if err := validateCanonicalInvokeRequest(request); err != nil {
		return factorysessions.InvocationResult{}, err
	}
	sessionID := strings.TrimSpace(request.SessionID)
	session := a.Resolve(sessionID)
	if session == nil {
		return factorysessions.InvocationResult{}, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, sessionID)
	}
	bound := runtimebinding.SessionStateFrom(session)
	if bound == nil {
		return factorysessions.InvocationResult{}, fmt.Errorf("%w: session invocation owner is unavailable", factorysessions.ErrRuntimeNotAvailable)
	}
	return invokeCanonicalSession(ctx, a.invoker, request)
}

// ResolveInvocationInput applies the selected signature through the fixed
// invocation owner, which does not belong to a runtime generation.
func (a *Assembly) ResolveInvocationInput(config *factorydefinitions.FactoryConfig, request factorysessions.InvocationRequest) (factorysessions.ResolvedInvocationInput, error) {
	return a.invoker.ResolveInvocationInput(config, request)
}
