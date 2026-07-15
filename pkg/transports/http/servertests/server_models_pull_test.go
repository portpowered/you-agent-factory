package apiserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
)

func TestPullModel_ReturnsManagedRuntimeSourceFetchFailureOutcome(t *testing.T) {
	mf := &testutil.MockFactory{
		PullModelErr: &apisurface.ManagedRuntimePullError{
			Result: apisurface.ModelPullResult{
				ModelName:          "OMNIVOICE_Q4_K_M",
				ProviderLocality:   workerconfig.ModelLocalityLocal,
				ManagedPullOutcome: "SOURCE_FETCH_FAILED",
				ReadinessState:     "FAILED",
			},
			Cause: apisurface.ErrManagedRuntimeSourceFetchFailed,
		},
	}
	srv := newAPITestServer(mf)

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
	mf := &testutil.MockFactory{
		PullModelErr: &apisurface.ManagedRuntimePullError{
			Result: apisurface.ModelPullResult{
				ModelName:          "OMNIVOICE_Q4_K_M",
				ProviderLocality:   workerconfig.ModelLocalityLocal,
				ManagedPullOutcome: "TIMED_OUT",
				ReadinessState:     "FAILED",
			},
			Cause: context.DeadlineExceeded,
		},
	}
	srv := newAPITestServer(mf)

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
