package factorysession_test

import (
	"errors"
	"net/http"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

func TestLifecycleControlErrorResponse_MapsControlConflictAndNotFound(t *testing.T) {
	status, response, ok := factorysession.LifecycleControlErrorResponse(
		"dur-sess-js-success-002",
		&factorysessionexecution.ControlError{
			Operation: factorysessionexecution.LifecycleControlCancel,
			Outcome:   factorysessionexecution.LifecycleControlOutcomeTerminalSession,
			Status:    factorysessionexecution.LifecycleStatusSucceeded,
			Message:   "terminal",
		},
	)
	if !ok {
		t.Fatal("LifecycleControlErrorResponse = false, want true")
	}
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	mapped, ok := response.(factoryapi.FactorySessionLifecycleControlResponse)
	if !ok {
		t.Fatalf("response = %T, want FactorySessionLifecycleControlResponse", response)
	}
	if mapped.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("outcome = %q, want TERMINAL_SESSION", mapped.Outcome)
	}

	status, response, ok = factorysession.LifecycleControlErrorResponse(
		"dur-sess-missing-999",
		factorysessionexecution.ErrSessionNotFound,
	)
	if !ok {
		t.Fatal("LifecycleControlErrorResponse = false, want true for not found")
	}
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	errResp, ok := response.(factoryapi.ErrorResponse)
	if !ok || errResp.Code != factoryapi.NOTFOUND {
		t.Fatalf("response = %#v, want NOT_FOUND ErrorResponse", response)
	}
}

func TestLifecycleControlSuccessStatus_MapsAcceptedCancelAndTerminate(t *testing.T) {
	cancelStatus := factorysession.LifecycleControlSuccessStatus(factorysessionexecution.LifecycleControlResult{
		Outcome: factorysessionexecution.LifecycleControlOutcomeAccepted,
		Status:  factorysessionexecution.LifecycleStatusCanceling,
	})
	if cancelStatus != http.StatusAccepted {
		t.Fatalf("cancel status = %d, want 202", cancelStatus)
	}

	terminateStatus := factorysession.LifecycleControlSuccessStatus(factorysessionexecution.LifecycleControlResult{
		Outcome: factorysessionexecution.LifecycleControlOutcomeAccepted,
		Status:  factorysessionexecution.LifecycleStatusTerminated,
	})
	if terminateStatus != http.StatusOK {
		t.Fatalf("terminate status = %d, want 200", terminateStatus)
	}
}

func TestLifecycleControlErrorResponse_ReturnsFalseForUnknownErrors(t *testing.T) {
	if _, _, ok := factorysession.LifecycleControlErrorResponse("dur-sess-001", errors.New("other")); ok {
		t.Fatal("LifecycleControlErrorResponse = true, want false")
	}
}

func TestLifecycleControlErrorResponse_MapsValidationAndDispatchNotFound(t *testing.T) {
	status, response, ok := factorysession.LifecycleControlErrorResponse(
		"dur-sess-js-run-n-001",
		factorysessionexecution.NewValidationError("dispatchId", "dispatchId is required"),
	)
	if !ok {
		t.Fatal("LifecycleControlErrorResponse = false, want true for validation")
	}
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	errResp, ok := response.(factoryapi.ErrorResponse)
	if !ok || errResp.Code != factoryapi.BADREQUEST {
		t.Fatalf("response = %#v, want BAD_REQUEST ErrorResponse", response)
	}

	status, response, ok = factorysession.LifecycleControlErrorResponse(
		"dur-sess-js-run-n-001",
		factorysessionexecution.ErrDispatchNotFound,
	)
	if !ok {
		t.Fatal("LifecycleControlErrorResponse = false, want true for dispatch not found")
	}
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	errResp, ok = response.(factoryapi.ErrorResponse)
	if !ok || errResp.Code != factoryapi.NOTFOUND {
		t.Fatalf("response = %#v, want NOT_FOUND ErrorResponse", response)
	}
}
