package apiserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelcontract "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type modelPullService struct {
	modelcontract.Service
	pull func(context.Context, string) (modelcontract.PullResult, error)
}

func (modelPullService) ListModels(context.Context) (modelcontract.List, error) {
	panic("unexpected models.Service.ListModels call")
}

func (modelPullService) GetModel(context.Context, string) (modelcontract.Detail, error) {
	panic("unexpected models.Service.GetModel call")
}

func (api modelPullService) PullModel(ctx context.Context, modelName string) (modelcontract.PullResult, error) {
	if api.pull == nil {
		panic("unexpected models.Service.PullModel call")
	}
	return api.pull(ctx, modelName)
}

func TestPullModel_ReturnsManagedRuntimeSourceFetchFailureOutcome(t *testing.T) {
	models := modelPullService{pull: func(_ context.Context, modelName string) (modelcontract.PullResult, error) {
		if modelName != "OMNIVOICE_Q4_K_M" {
			t.Fatalf("model name = %q, want OMNIVOICE_Q4_K_M", modelName)
		}
		return modelcontract.PullResult{}, &modelcontract.PullError{
			Result: modelcontract.PullResult{
				ModelName:          "OMNIVOICE_Q4_K_M",
				ProviderLocality:   workerconfig.ModelLocalityLocal,
				ManagedPullOutcome: "SOURCE_FETCH_FAILED",
				ReadinessState:     "FAILED",
			},
			Cause: modelcontract.ErrSourceFetchFailed,
		}
	}}
	srv := newAPITestServer(models)

	req := httptest.NewRequest(http.MethodPost, "/models/OMNIVOICE_Q4_K_M/pull", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
	response := decodeJSONResponse[factoryapi.ModelPullResponse](t, rec)
	if response.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeSOURCEFETCHFAILED {
		t.Fatalf("pull outcome = %s, want SOURCE_FETCH_FAILED", response.ManagedRuntimePull.PullOutcome)
	}
	if response.ManagedRuntimePull.ReadinessState != factoryapi.ManagedRuntimeReadinessStateFAILED {
		t.Fatalf("readiness = %s, want FAILED", response.ManagedRuntimePull.ReadinessState)
	}
}

func TestPullModel_ReturnsManagedRuntimeTimeoutOutcome(t *testing.T) {
	models := modelPullService{pull: func(_ context.Context, modelName string) (modelcontract.PullResult, error) {
		if modelName != "OMNIVOICE_Q4_K_M" {
			t.Fatalf("model name = %q, want OMNIVOICE_Q4_K_M", modelName)
		}
		return modelcontract.PullResult{}, &modelcontract.PullError{
			Result: modelcontract.PullResult{
				ModelName:          "OMNIVOICE_Q4_K_M",
				ProviderLocality:   workerconfig.ModelLocalityLocal,
				ManagedPullOutcome: "TIMED_OUT",
				ReadinessState:     "FAILED",
			},
			Cause: context.DeadlineExceeded,
		}
	}}
	srv := newAPITestServer(models)

	req := httptest.NewRequest(http.MethodPost, "/models/OMNIVOICE_Q4_K_M/pull", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d: %s", rec.Code, rec.Body.String())
	}
	response := decodeJSONResponse[factoryapi.ModelPullResponse](t, rec)
	if response.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeTIMEDOUT {
		t.Fatalf("pull outcome = %s, want TIMED_OUT", response.ManagedRuntimePull.PullOutcome)
	}
}
