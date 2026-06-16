package modeltests

import (
	"context"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestFactoryService_ListModels_SourcesManagedRuntimeFromModelHost(t *testing.T) {
	svc := buildModelCatalogService(t, modelCatalogConfig(true))

	models, err := svc.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models.Results) != 1 {
		t.Fatalf("models count = %d, want 1", len(models.Results))
	}
	if models.Results[0].ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateMISSING {
		t.Fatalf("managed readiness = %s, want MISSING from model host projection", models.Results[0].ManagedRuntime.ReadinessState)
	}
}

func TestFactoryService_InvokeModel_BlocksMissingManagedRuntimeFromModelHost(t *testing.T) {
	svc := buildModelCatalogService(t, modelCatalogConfig(true))

	_, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Content: &factoryapi.WorkContent{
			mustGeneratedServiceTextPart(t, "hello world"),
		},
	})
	if err == nil {
		t.Fatal("InvokeModel: nil error, want managed runtime missing")
	}
}
