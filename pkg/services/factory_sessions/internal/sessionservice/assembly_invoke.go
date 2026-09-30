package service

import (
	"context"
	"fmt"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

// Invoke resolves the invocation owner from the canonical live session record.
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
	if bound == nil || bound.Invoker == nil {
		return factorysessions.InvocationResult{}, fmt.Errorf("%w: session invocation owner is unavailable", factorysessions.ErrRuntimeNotAvailable)
	}
	return invokeCanonicalSession(ctx, bound.Invoker, request)
}
