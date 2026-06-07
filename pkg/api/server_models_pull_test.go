package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestPullModel_ReturnsManagedRuntimeSourceFetchFailureOutcome(t *testing.T) {
	mf := &testutil.MockFactory{
		PullModelErr: &apisurface.ManagedRuntimePullError{
			Result: apisurface.ModelPullResult{
				ModelName:          "OMNIVOICE_Q4_K_M",
				ProviderLocality:   interfaces.ModelLocalityLocal,
				ManagedPullOutcome: "SOURCE_FETCH_FAILED",
				ReadinessState:     "FAILED",
			},
			Cause: apisurface.ErrManagedRuntimeSourceFetchFailed,
		},
	}
	srv := newTestServer(mf)

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
				ProviderLocality:   interfaces.ModelLocalityLocal,
				ManagedPullOutcome: "TIMED_OUT",
				ReadinessState:     "FAILED",
			},
			Cause: context.DeadlineExceeded,
		},
	}
	srv := newTestServer(mf)

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
