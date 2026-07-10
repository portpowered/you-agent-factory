package runtimehost

import (
	"context"
	"errors"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
)

func TestHostCatalogMethodsForwardContextResultsAndErrorsUnchanged(t *testing.T) {
	t.Parallel()

	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("request"), "catalog-request")
	listErr := errors.New("list sentinel")
	getErr := errors.New("get sentinel")
	stub := &catalogModelServiceStub{
		listResult: factoryapi.ListModelsResponse{Results: []factoryapi.ModelSummary{{Name: "list-result"}}},
		listErr:    listErr,
		getResult:  factoryapi.ModelDetail{Name: "detail-result"},
		getErr:     getErr,
	}
	host := &Host{modelService: stub}

	listed, gotListErr := host.ListModels(ctx)
	detail, gotGetErr := host.GetModel(ctx, "requested-model")

	if listed.Results[0].Name != "list-result" || !errors.Is(gotListErr, listErr) {
		t.Fatalf("ListModels = (%#v, %v), want exact result and sentinel error", listed, gotListErr)
	}
	if detail.Name != "detail-result" || !errors.Is(gotGetErr, getErr) {
		t.Fatalf("GetModel = (%#v, %v), want exact result and sentinel error", detail, gotGetErr)
	}
	if len(stub.contexts) != 2 || stub.contexts[0] != ctx || stub.contexts[1] != ctx {
		t.Fatalf("catalog contexts = %#v, want original context twice", stub.contexts)
	}
	if stub.modelName != "requested-model" {
		t.Fatalf("catalog model name = %q, want requested-model", stub.modelName)
	}
}

type catalogModelServiceStub struct {
	listResult factoryapi.ListModelsResponse
	listErr    error
	getResult  factoryapi.ModelDetail
	getErr     error
	contexts   []context.Context
	modelName  string
}

func (s *catalogModelServiceStub) ListModels(ctx context.Context) (factoryapi.ListModelsResponse, error) {
	s.contexts = append(s.contexts, ctx)
	return s.listResult, s.listErr
}

func (s *catalogModelServiceStub) GetModel(ctx context.Context, modelName string) (factoryapi.ModelDetail, error) {
	s.contexts = append(s.contexts, ctx)
	s.modelName = modelName
	return s.getResult, s.getErr
}

func (*catalogModelServiceStub) PullModel(context.Context, string) (apisurface.ModelPullResult, error) {
	return apisurface.ModelPullResult{}, nil
}

func (*catalogModelServiceStub) InvokeModel(context.Context, string, factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
	return apisurface.ModelInvocationResult{}, nil
}
