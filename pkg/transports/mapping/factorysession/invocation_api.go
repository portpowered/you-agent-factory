package factorysession

import (
	"context"
	"fmt"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// InvocationAPI maps generated invocation requests onto the canonical Factory
// Session invocation owner.
type InvocationAPI struct {
	owner factorysessions.SessionInvoker
}

var _ apisurface.InvocationAPI = (*InvocationAPI)(nil)

// NewInvocationAPI constructs the transport adapter without a runtime-host facade.
func NewInvocationAPI(owner factorysessions.SessionInvoker) *InvocationAPI {
	return &InvocationAPI{owner: owner}
}

func (a *InvocationAPI) InvokeFactorySession(ctx context.Context, sessionID string, request factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
	if a == nil || a.owner == nil {
		return apisurface.FactoryInvocationResult{}, fmt.Errorf("Factory Session invocation service is required")
	}
	return InvokeFactorySession(ctx, a.owner, sessionID, request)
}

// InvokeFactorySession maps and invokes one session request without
// constructing a stateful transport adapter.
func InvokeFactorySession(
	ctx context.Context,
	owner factorysessions.SessionInvoker,
	sessionID string,
	request factoryapi.InvocationRequest,
) (apisurface.FactoryInvocationResult, error) {
	if owner == nil {
		return apisurface.FactoryInvocationResult{}, fmt.Errorf("Factory Session invocation service is required")
	}
	return owner.InvokeFactorySession(ctx, sessionID, InvocationRequestFromAPI(request))
}
