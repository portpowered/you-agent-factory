package runtimehost

import (
	"context"
	"errors"
	"fmt"
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
	getErr := fmt.Errorf("requested model: %w", apisurface.ErrModelNotFound)
	pullErr := &apisurface.ManagedRuntimePullError{
		Result: apisurface.ModelPullResult{ModelName: "pull-result", ManagedPullOutcome: "TIMED_OUT"},
		Cause:  errors.New("pull sentinel"),
	}
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

	if !reflect.DeepEqual(listed, stub.listResult) || gotListErr != listErr {
		t.Fatalf("ListModels = (%#v, %v), want exact result and sentinel error", listed, gotListErr)
	}
	if detail.Name != "detail-result" || gotGetErr != getErr {
		t.Fatalf("GetModel = (%#v, %v), want exact result and sentinel error", detail, gotGetErr)
	}
	if !reflect.DeepEqual(pulled, stub.pullResult) || gotPullErr != pullErr {
		t.Fatalf("PullModel = (%#v, %v), want exact result and sentinel error", pulled, gotPullErr)
	}
	if !errors.Is(gotGetErr, apisurface.ErrModelNotFound) || !apisurface.IsManagedRuntimePullError(gotPullErr) {
		t.Fatalf("typed errors = (%v, %v), want model-not-found and managed-runtime-pull errors", gotGetErr, gotPullErr)
	}
	assertCatalogCallsForwardedOnce(t, stub, ctx)
}

func TestHostInvokeModelForwardsContextRequestResultAndErrorUnchanged(t *testing.T) {
	t.Parallel()

	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("request"), "invoke-request")
	invokeErr := &apisurface.ManagedRuntimeInvocationError{
		Identity:       "invoke-model",
		ReadinessState: factoryapi.ManagedRuntimeReadinessStateMISSING,
		Cause:          apisurface.ErrManagedRuntimeMissing,
	}
	request := factoryapi.ModelInvocationRequest{Operation: "TTS"}
	stub := &catalogModelServiceStub{
		invokeResult: apisurface.ModelInvocationResult{ModelName: "invoke-result", Operation: "TTS"},
		invokeErr:    invokeErr,
	}

	result, err := (&Host{modelService: stub}).InvokeModel(ctx, "invoke-model", request)
	if !reflect.DeepEqual(result, stub.invokeResult) || err != invokeErr {
		t.Fatalf("InvokeModel = (%#v, %v), want exact result and sentinel error", result, err)
	}
	if !apisurface.IsManagedRuntimeMissing(err) {
		t.Fatalf("InvokeModel error = %v, want typed unavailable-runtime error", err)
	}
	if len(stub.contexts) != 1 || stub.contexts[0] != ctx || len(stub.modelNames) != 1 || stub.modelNames[0] != "invoke-model" {
		t.Fatalf("forwarded context/model = (%#v, %#v), want original context and invoke-model", stub.contexts, stub.modelNames)
	}
	if len(stub.requests) != 1 || !reflect.DeepEqual(stub.requests[0], request) {
		t.Fatalf("invoke requests = %#v, want exact TTS request", stub.requests)
	}
	if !reflect.DeepEqual(stub.calls, []string{"invoke"}) {
		t.Fatalf("model calls = %#v, want invoke exactly once", stub.calls)
	}
}

func assertCatalogCallsForwardedOnce(t *testing.T, stub *catalogModelServiceStub, ctx context.Context) {
	t.Helper()
	if len(stub.contexts) != 3 || stub.contexts[0] != ctx || stub.contexts[1] != ctx || stub.contexts[2] != ctx {
		t.Fatalf("model contexts = %#v, want original context three times", stub.contexts)
	}
	if len(stub.modelNames) != 2 || stub.modelNames[0] != "requested-model" || stub.modelNames[1] != "pull-model" {
		t.Fatalf("model names = %#v, want requested-model then pull-model", stub.modelNames)
	}
	if !reflect.DeepEqual(stub.calls, []string{"list", "get", "pull"}) {
		t.Fatalf("model calls = %#v, want each operation exactly once", stub.calls)
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
	calls        []string
}

func (s *catalogModelServiceStub) ListModels(ctx context.Context) (factoryapi.ListModelsResponse, error) {
	s.calls = append(s.calls, "list")
	s.contexts = append(s.contexts, ctx)
	return s.listResult, s.listErr
}

func (s *catalogModelServiceStub) GetModel(ctx context.Context, modelName string) (factoryapi.ModelDetail, error) {
	s.calls = append(s.calls, "get")
	s.contexts = append(s.contexts, ctx)
	s.modelNames = append(s.modelNames, modelName)
	return s.getResult, s.getErr
}

func (s *catalogModelServiceStub) PullModel(ctx context.Context, modelName string) (apisurface.ModelPullResult, error) {
	s.calls = append(s.calls, "pull")
	s.contexts = append(s.contexts, ctx)
	s.modelNames = append(s.modelNames, modelName)
	return s.pullResult, s.pullErr
}

func (s *catalogModelServiceStub) InvokeModel(ctx context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
	s.calls = append(s.calls, "invoke")
	s.contexts = append(s.contexts, ctx)
	s.modelNames = append(s.modelNames, modelName)
	s.requests = append(s.requests, request)
	return s.invokeResult, s.invokeErr
}
