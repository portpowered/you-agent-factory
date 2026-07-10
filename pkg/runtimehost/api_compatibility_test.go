package runtimehost

import (
	"context"
	"errors"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
)

type compatibilitySessionGateway struct {
	SessionGateway
	getFactorySession func(context.Context, string) (factoryapi.FactorySession, error)
}

func (f compatibilitySessionGateway) GetFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySession, error) {
	return f.getFactorySession(ctx, sessionID)
}

type compatibilityModelAPI struct {
	apisurface.ModelAPI
	getModel func(context.Context, string) (factoryapi.ModelDetail, error)
}

func (f compatibilityModelAPI) GetModel(ctx context.Context, modelName string) (factoryapi.ModelDetail, error) {
	return f.getModel(ctx, modelName)
}

type compatibilityFactorySave struct {
	save func(context.Context, string, factoryapi.FactorySaveMode, factoryapi.Factory) (factoryapi.Factory, error)
}

func (f compatibilityFactorySave) Save(ctx context.Context, sessionID string, mode factoryapi.FactorySaveMode, request factoryapi.Factory) (factoryapi.Factory, error) {
	return f.save(ctx, sessionID, mode, request)
}

type compatibilityInvocationAPI struct {
	invoke func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error)
}

func (f compatibilityInvocationAPI) InvokeFactorySession(ctx context.Context, sessionID string, request factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
	return f.invoke(ctx, sessionID, request)
}

type compatibilityDurableExecutionAPI struct {
	apisurface.DurableSessionExecutionAPI
	startAsync func(context.Context, factoryapi.FactorySessionExecutionRequest) (factoryapi.FactorySessionExecutionResponse, error)
}

func (f compatibilityDurableExecutionAPI) StartDurableFactorySessionAsync(ctx context.Context, request factoryapi.FactorySessionExecutionRequest) (factoryapi.FactorySessionExecutionResponse, error) {
	return f.startAsync(ctx, request)
}

func TestHostCompatibilityFacadeForwardsToCanonicalCollaborators(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), struct{}{}, "compatibility-context")
	sentinel := errors.New("typed collaborator outcome")
	requestFactory := factoryapi.Factory{Name: "submitted"}
	invocationRequest := factoryapi.InvocationRequest{}
	durableRequest := factoryapi.FactorySessionExecutionRequest{}
	calls := map[string]int{}

	host := &Host{}
	host.sessionGateway = compatibilitySessionGateway{getFactorySession: func(gotCtx context.Context, sessionID string) (factoryapi.FactorySession, error) {
		calls["session"]++
		if gotCtx != ctx || sessionID != "missing-session" {
			t.Fatalf("session args = (%v, %q)", gotCtx, sessionID)
		}
		return factoryapi.FactorySession{}, sentinel
	}}
	host.modelService = compatibilityModelAPI{getModel: func(gotCtx context.Context, modelName string) (factoryapi.ModelDetail, error) {
		calls["model"]++
		if gotCtx != ctx || modelName != "missing-model" {
			t.Fatalf("model args = (%v, %q)", gotCtx, modelName)
		}
		return factoryapi.ModelDetail{}, sentinel
	}}
	host.factorySave = compatibilityFactorySave{save: func(gotCtx context.Context, sessionID string, mode factoryapi.FactorySaveMode, request factoryapi.Factory) (factoryapi.Factory, error) {
		calls["factory-definition"]++
		if gotCtx != ctx || sessionID != "session-1" || mode != factoryapi.FactorySaveModeReplaceCurrent || request.Name != requestFactory.Name {
			t.Fatalf("factory-definition args = (%v, %q, %q, %#v)", gotCtx, sessionID, mode, request)
		}
		return factoryapi.Factory{}, sentinel
	}}
	host.invocationAPI = compatibilityInvocationAPI{invoke: func(gotCtx context.Context, sessionID string, request factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
		calls["invocation"]++
		if gotCtx != ctx || sessionID != "session-1" {
			t.Fatalf("invocation args = (%v, %q, %#v)", gotCtx, sessionID, request)
		}
		return apisurface.FactoryInvocationResult{}, sentinel
	}}
	host.durableExecutionAPI = compatibilityDurableExecutionAPI{startAsync: func(gotCtx context.Context, request factoryapi.FactorySessionExecutionRequest) (factoryapi.FactorySessionExecutionResponse, error) {
		calls["durable-execution"]++
		if gotCtx != ctx {
			t.Fatalf("durable context was not preserved")
		}
		return factoryapi.FactorySessionExecutionResponse{}, sentinel
	}}

	_, sessionErr := host.GetFactorySession(ctx, "missing-session")
	_, modelErr := host.GetModel(ctx, "missing-model")
	_, definitionErr := host.SaveFactoryForSession(ctx, "session-1", factoryapi.FactorySaveModeReplaceCurrent, requestFactory)
	_, invocationErr := host.InvokeFactorySession(ctx, "session-1", invocationRequest)
	_, durableErr := host.StartDurableFactorySessionAsync(ctx, durableRequest)
	for role, err := range map[string]error{"session": sessionErr, "model": modelErr, "factory-definition": definitionErr, "invocation": invocationErr, "durable-execution": durableErr} {
		if !errors.Is(err, sentinel) || calls[role] != 1 {
			t.Errorf("%s result = (%v, %d calls), want unchanged error and one call", role, err, calls[role])
		}
	}
}

var _ apisurface.APISurface = (*Host)(nil)
var _ apisurface.SessionAPISurface = (*Host)(nil)
