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

func (api modelPullService) PullModelForScope(ctx context.Context, request modelcontract.PullModelRequest) (modelcontract.PullResult, error) {
	if api.pull == nil {
		panic("unexpected models.Service.PullModelForScope call")
	}
	return api.pull(ctx, request.Name)
}

func TestPullModel_ReturnsTypedSourceFetchFailure(t *testing.T) {
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
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if response.Code != factoryapi.ErrorResponseCodeINTERNALERROR || response.Message == "" {
		t.Fatalf("error response = %#v, want non-empty INTERNAL_ERROR", response)
	}
}

func TestPullModel_ReturnsTypedTimeoutFailure(t *testing.T) {
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
	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if response.Code != factoryapi.ErrorResponseCodeINTERNALERROR || response.Message != "models request timed out" {
		t.Fatalf("error response = %#v, want typed timeout error", response)
	}
}
