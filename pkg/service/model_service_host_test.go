package service

import (
	"context"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
)

func TestFactoryService_ModelMethodsDelegateToInjectedModelAPI(t *testing.T) {
	t.Parallel()

	stub := &stubModelService{
		listResult:   factoryapi.ListModelsResponse{Results: []factoryapi.ModelSummary{{Name: "catalog-model"}}},
		gotModel:     factoryapi.ModelDetail{Name: "detail-model"},
		pullResult:   apisurface.ModelPullResult{ModelName: "pull-model"},
		invokeResult: apisurface.ModelInvocationResult{ModelName: "invoke-model"},
	}
	svc := &FactoryService{modelService: stub}

	if _, ok := svc.requireModelService().(*stubModelService); !ok {
		t.Fatalf("requireModelService type = %T, want injected stub", svc.requireModelService())
	}

	listed, err := svc.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(listed.Results) != 1 || listed.Results[0].Name != "catalog-model" {
		t.Fatalf("ListModels result = %#v, want delegated catalog-model", listed)
	}
	if _, err := svc.GetModel(context.Background(), "detail-model"); err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	if _, err := svc.PullModel(context.Background(), "pull-model"); err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	if _, err := svc.InvokeModel(context.Background(), "invoke-model", factoryapi.ModelInvocationRequest{Operation: "TTS"}); err != nil {
		t.Fatalf("InvokeModel: %v", err)
	}
	if strings.Join(stub.calls, ",") != "list,get,pull,invoke" {
		t.Fatalf("model service calls = %#v, want list,get,pull,invoke", stub.calls)
	}
}

func TestWireModelServiceCollaborator_UsesModelsServiceByDefault(t *testing.T) {
	t.Parallel()

	api := wireModelServiceCollaborator(nil, nil)
	if _, ok := api.(*modelsservice.Service); !ok {
		t.Fatalf("wireModelServiceCollaborator(nil) type = %T, want *modelsservice.Service", api)
	}
}
