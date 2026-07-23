package factorysession

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// InvocationAPI maps generated invocation requests onto the canonical Factory
// Session invocation owner.
type InvocationAPI struct {
	owner SessionInvoker
}

// SessionInvoker is the exact consumer-owned invocation role.
type SessionInvoker interface {
	InvokeFactorySession(context.Context, string, factorysessions.InvocationRequest) (factorydefinitions.FactoryInvocationResult, error)
}

var _ apisurface.InvocationAPI = (*InvocationAPI)(nil)

// NewInvocationAPI constructs the transport adapter without a runtime-host facade.
func NewInvocationAPI(owner SessionInvoker) *InvocationAPI {
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
	owner SessionInvoker,
	sessionID string,
	request factoryapi.InvocationRequest,
) (apisurface.FactoryInvocationResult, error) {
	if owner == nil {
		return apisurface.FactoryInvocationResult{}, fmt.Errorf("Factory Session invocation service is required")
	}
	return owner.InvokeFactorySession(ctx, sessionID, InvocationRequestFromAPI(request))
}
