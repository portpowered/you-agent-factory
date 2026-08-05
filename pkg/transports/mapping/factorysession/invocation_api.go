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
	owner factorysessions.InvocationService
}

var _ apisurface.InvocationAPI = (*InvocationAPI)(nil)

// NewInvocationAPI constructs the transport adapter without a runtime-host facade.
func NewInvocationAPI(owner factorysessions.InvocationService) *InvocationAPI {
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
	owner factorysessions.InvocationService,
	sessionID string,
	request factoryapi.InvocationRequest,
) (apisurface.FactoryInvocationResult, error) {
	if owner == nil {
		return apisurface.FactoryInvocationResult{}, fmt.Errorf("Factory Session invocation service is required")
	}
	result, err := owner.InvokeFactorySession(ctx, sessionID, InvocationRequestFromAPI(request))
	if err != nil {
		return apisurface.FactoryInvocationResult{}, err
	}
	return apisurface.FactoryInvocationResult{
		RequestID: result.RequestID, TraceID: result.TraceID,
		Status:        factorydefinitions.InvocationTerminalStatus(result.Status),
		PrimaryResult: result.PrimaryResult, ErrorCode: result.ErrorCode,
		Message: result.Message, SessionID: result.SessionID, WorkID: result.WorkID,
		WorkName: result.WorkName, WorkState: result.WorkState,
	}, nil
}
