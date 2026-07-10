package runtimehost

import (
	"context"
	"errors"
	"reflect"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
)

func TestHostModelMethodsForwardContextResultsAndErrorsUnchanged(t *testing.T) {
	t.Parallel()

	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("request"), "catalog-request")
	listErr := errors.New("list sentinel")
	getErr := errors.New("get sentinel")
	pullErr := errors.New("pull sentinel")
	stub := &catalogModelServiceStub{
		listResult: factoryapi.ListModelsResponse{Results: []factoryapi.ModelSummary{{Name: "list-result"}}},
		listErr:    listErr,
		getResult:  factoryapi.ModelDetail{Name: "detail-result"},
		getErr:     getErr,
		pullResult: apisurface.ModelPullResult{ModelName: "pull-result", ManagedPullOutcome: "TIMED_OUT"},
		pullErr:    pullErr,
	}
	host := &Host{modelService: stub}

	listed, gotListErr := host.ListModels(ctx)
	detail, gotGetErr := host.GetModel(ctx, "requested-model")
	pulled, gotPullErr := host.PullModel(ctx, "pull-model")

	if listed.Results[0].Name != "list-result" || !errors.Is(gotListErr, listErr) {
		t.Fatalf("ListModels = (%#v, %v), want exact result and sentinel error", listed, gotListErr)
	}
	if detail.Name != "detail-result" || !errors.Is(gotGetErr, getErr) {
		t.Fatalf("GetModel = (%#v, %v), want exact result and sentinel error", detail, gotGetErr)
	}
	if !reflect.DeepEqual(pulled, stub.pullResult) || !errors.Is(gotPullErr, pullErr) {
		t.Fatalf("PullModel = (%#v, %v), want exact result and sentinel error", pulled, gotPullErr)
	}
	if len(stub.contexts) != 3 || stub.contexts[0] != ctx || stub.contexts[1] != ctx || stub.contexts[2] != ctx {
		t.Fatalf("model contexts = %#v, want original context three times", stub.contexts)
	}
	if len(stub.modelNames) != 2 || stub.modelNames[0] != "requested-model" || stub.modelNames[1] != "pull-model" {
		t.Fatalf("model names = %#v, want requested-model then pull-model", stub.modelNames)
	}
}

func TestHostInvokeModelForwardsContextRequestResultAndErrorUnchanged(t *testing.T) {
	t.Parallel()

	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("request"), "invoke-request")
	invokeErr := errors.New("invoke sentinel")
	request := factoryapi.ModelInvocationRequest{Operation: "TTS"}
	stub := &catalogModelServiceStub{
		invokeResult: apisurface.ModelInvocationResult{ModelName: "invoke-result", Operation: "TTS"},
		invokeErr:    invokeErr,
	}

	result, err := (&Host{modelService: stub}).InvokeModel(ctx, "invoke-model", request)
	if !reflect.DeepEqual(result, stub.invokeResult) || !errors.Is(err, invokeErr) {
		t.Fatalf("InvokeModel = (%#v, %v), want exact result and sentinel error", result, err)
	}
	if len(stub.contexts) != 1 || stub.contexts[0] != ctx || len(stub.modelNames) != 1 || stub.modelNames[0] != "invoke-model" {
		t.Fatalf("forwarded context/model = (%#v, %#v), want original context and invoke-model", stub.contexts, stub.modelNames)
	}
	if len(stub.requests) != 1 || !reflect.DeepEqual(stub.requests[0], request) {
		t.Fatalf("invoke requests = %#v, want exact TTS request", stub.requests)
	}
}

type catalogModelServiceStub struct {
	listResult   factoryapi.ListModelsResponse
	listErr      error
	getResult    factoryapi.ModelDetail
	getErr       error
	pullResult   apisurface.ModelPullResult
	pullErr      error
	invokeResult apisurface.ModelInvocationResult
	invokeErr    error
	contexts     []context.Context
	modelNames   []string
	requests     []factoryapi.ModelInvocationRequest
}

func (s *catalogModelServiceStub) ListModels(ctx context.Context) (factoryapi.ListModelsResponse, error) {
	s.contexts = append(s.contexts, ctx)
	return s.listResult, s.listErr
}

func (s *catalogModelServiceStub) GetModel(ctx context.Context, modelName string) (factoryapi.ModelDetail, error) {
	s.contexts = append(s.contexts, ctx)
	s.modelNames = append(s.modelNames, modelName)
	return s.getResult, s.getErr
}

func (s *catalogModelServiceStub) PullModel(ctx context.Context, modelName string) (apisurface.ModelPullResult, error) {
	s.contexts = append(s.contexts, ctx)
	s.modelNames = append(s.modelNames, modelName)
	return s.pullResult, s.pullErr
}

func (s *catalogModelServiceStub) InvokeModel(ctx context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
	s.contexts = append(s.contexts, ctx)
	s.modelNames = append(s.modelNames, modelName)
	s.requests = append(s.requests, request)
	return s.invokeResult, s.invokeErr
}

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
	host.sessionInvoker = compatibilityInvocationAPI{invoke: func(gotCtx context.Context, sessionID string, request factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
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
	_, invocationErr := host.InvokeFactorySession(ctx, "session-1", factoryapi.InvocationRequest{})
	_, durableErr := host.StartDurableFactorySessionAsync(ctx, factoryapi.FactorySessionExecutionRequest{})
	for role, err := range map[string]error{"session": sessionErr, "model": modelErr, "factory-definition": definitionErr, "invocation": invocationErr, "durable-execution": durableErr} {
		if !errors.Is(err, sentinel) || calls[role] != 1 {
			t.Errorf("%s result = (%v, %d calls), want unchanged error and one call", role, err, calls[role])
		}
	}
}

var _ apisurface.APISurface = (*Host)(nil)
var _ apisurface.SessionAPISurface = (*Host)(nil)
