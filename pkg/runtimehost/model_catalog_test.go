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

type catalogModelServiceStub struct {
	listResult factoryapi.ListModelsResponse
	listErr    error
	getResult  factoryapi.ModelDetail
	getErr     error
	pullResult apisurface.ModelPullResult
	pullErr    error
	contexts   []context.Context
	modelNames []string
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

func (*catalogModelServiceStub) InvokeModel(context.Context, string, factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
	return apisurface.ModelInvocationResult{}, nil
}
