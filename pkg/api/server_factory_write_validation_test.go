package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestSaveCurrentFactory_ReturnsMultipleTopologyValidationTargets(t *testing.T) {
	targets := []factoryapi.FactoryValidationTarget{
		{
			Code:     factoryvalidation.CodeDuplicateIdentifier,
			Severity: factoryapi.FactoryValidationSeverityError,
			Message:  `duplicate worker identifier "worker-a".`,
			Subject: factoryapi.FactoryValidationSubject{
				Type:     factoryapi.FactoryValidationSubjectTypeWorker,
				Id:       "worker-a",
				Location: factoryapi.FactoryValidationSubjectLocationDefinition,
			},
		},
		{
			Code:     factoryvalidation.CodeDanglingWorkerReference,
			Severity: factoryapi.FactoryValidationSeverityError,
			Message:  `workstation "process" references unknown worker "missing-worker".`,
			Subject: factoryapi.FactoryValidationSubject{
				Type:     factoryapi.FactoryValidationSubjectTypeWorkstation,
				Id:       "process",
				Location: factoryapi.FactoryValidationSubjectLocationDefinition,
			},
		},
		{
			Code:     factoryvalidation.CodeDanglingPlaceReference,
			Severity: factoryapi.FactoryValidationSeverityError,
			Message:  `workstation "process" routes to unknown place.`,
			Subject: factoryapi.FactoryValidationSubject{
				Type:     factoryapi.FactoryValidationSubjectTypeWorkstation,
				Id:       "process",
				Location: factoryapi.FactoryValidationSubjectLocationOutputs,
			},
		},
	}
	srv := newTestServer(&testutil.MockFactory{
		SaveCurrentFactoryErr: apisurface.NewTopologyValidationError(
			"Factory topology contains invalid graph references.",
			targets,
		),
	})

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(`{"name":"beta"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	assertErrorResponsePreservesLegacyFields(
		t,
		response,
		factoryapi.INVALIDFACTORY,
		factoryapi.ErrorFamilyBadRequest,
		"Factory payload is not a valid Agent Factory definition.",
	)
	if response.Targets == nil || len(*response.Targets) < 3 {
		t.Fatalf("targets = %#v, want multiple blocking validation targets", response.Targets)
	}
	assertHasValidationTargetCode(t, *response.Targets, factoryvalidation.CodeDuplicateIdentifier)
	assertHasValidationTargetCode(t, *response.Targets, factoryvalidation.CodeDanglingWorkerReference)
	assertHasValidationTargetCode(t, *response.Targets, factoryvalidation.CodeDanglingPlaceReference)
	assertBlockingValidationTarget(t, (*response.Targets)[0])
}

func TestCreateFactory_ReturnsTopologyValidationTargets(t *testing.T) {
	target := factoryapi.FactoryValidationTarget{
		Code:     factoryvalidation.CodeDanglingPlaceReference,
		Severity: factoryapi.FactoryValidationSeverityError,
		Message:  "workstation process routes to unknown place.",
		Subject: factoryapi.FactoryValidationSubject{
			Type:     factoryapi.FactoryValidationSubjectTypeWorkstation,
			Id:       "process",
			Location: factoryapi.FactoryValidationSubjectLocationOutputs,
		},
	}
	srv := newTestServer(&testutil.MockFactory{
		CreateNamedFactoryErr: apisurface.NewTopologyValidationError(
			"Factory topology contains invalid graph references.",
			[]factoryapi.FactoryValidationTarget{target},
		),
	})

	req := httptest.NewRequest(http.MethodPost, "/factories", bytes.NewBufferString(validNamedFactoryBody("beta", "beta-task")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	assertErrorResponsePreservesLegacyFields(
		t,
		response,
		factoryapi.INVALIDFACTORY,
		factoryapi.ErrorFamilyBadRequest,
		"Factory payload is not a valid Agent Factory definition.",
	)
	if response.Targets == nil || len(*response.Targets) != 1 {
		t.Fatalf("targets = %#v, want one canonical target", response.Targets)
	}
	gotTarget := (*response.Targets)[0]
	assertBlockingValidationTarget(t, gotTarget)
	if gotTarget.Code != factoryvalidation.CodeDanglingPlaceReference ||
		gotTarget.Subject.Type != factoryapi.FactoryValidationSubjectTypeWorkstation ||
		gotTarget.Subject.Id != "process" ||
		gotTarget.Subject.Location != factoryapi.FactoryValidationSubjectLocationOutputs {
		t.Fatalf("error target = %#v, want dangling output workstation target", gotTarget)
	}
}

func TestCreateFactory_RejectsInvalidFactoryPayloadWithTargets(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{CreateNamedFactoryErr: apisurface.ErrInvalidNamedFactory})
	body := `{"name":"beta","workTypes":[{"name":"beta-task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}],"workers":[{"name":"planner","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514"}],"workstations":[{"name":"plan-task","behavior":"STANDARD","type":"MODEL_WORKSTATION","worker":"missing-worker","inputs":[{"workType":"beta-task","state":"init"}],"outputs":[{"workType":"beta-task","state":"done"}]}]}`
	req := httptest.NewRequest(http.MethodPost, "/factories", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	assertErrorResponsePreservesLegacyFields(
		t,
		response,
		factoryapi.INVALIDFACTORY,
		factoryapi.ErrorFamilyBadRequest,
		"Factory payload is not a valid Agent Factory definition.",
	)
	if response.Targets == nil || len(*response.Targets) != 1 || (*response.Targets)[0].Code != factoryvalidation.CodeFactoryPayloadInvalid {
		t.Fatalf("error targets = %#v, want canonical invalid factory payload target", response.Targets)
	}
	assertBlockingValidationTarget(t, (*response.Targets)[0])
}

func TestSaveCurrentFactory_ReturnsBobWorkstationOnFailureTarget(t *testing.T) {
	target := factoryapi.FactoryValidationTarget{
		Code:     factoryvalidation.CodeWorkstationMissingFailureRoute,
		Severity: factoryapi.FactoryValidationSeverityError,
		Message:  `workstation "bob" must define a failure route.`,
		Subject: factoryapi.FactoryValidationSubject{
			Type:     factoryapi.FactoryValidationSubjectTypeWorkstation,
			Id:       "bob",
			Location: factoryapi.FactoryValidationSubjectLocationOnFailure,
		},
	}
	srv := newTestServer(&testutil.MockFactory{
		SaveCurrentFactoryErr: apisurface.NewTopologyValidationError(
			"Factory topology contains invalid graph references.",
			[]factoryapi.FactoryValidationTarget{target},
		),
	})

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(`{"name":"beta"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("response = %#v status=%d", response, rec.Code)
	}
	assertErrorResponsePreservesLegacyFields(
		t,
		response,
		factoryapi.INVALIDFACTORY,
		factoryapi.ErrorFamilyBadRequest,
		"Factory payload is not a valid Agent Factory definition.",
	)
	if response.Targets == nil || len(*response.Targets) != 1 {
		t.Fatalf("targets = %#v, want one canonical target", response.Targets)
	}
	got := (*response.Targets)[0]
	assertBlockingValidationTarget(t, got)
	assertHasValidationTarget(
		t,
		[]factoryapi.FactoryValidationTarget{got},
		factoryvalidation.CodeWorkstationMissingFailureRoute,
		factoryapi.FactoryValidationSubjectTypeWorkstation,
		"bob",
		factoryapi.FactoryValidationSubjectLocationOnFailure,
		"bob ON_FAILURE target",
	)
}

func TestCreateFactory_ReturnsBobWorkstationOnFailureTarget(t *testing.T) {
	target := factoryapi.FactoryValidationTarget{
		Code:     factoryvalidation.CodeWorkstationMissingFailureRoute,
		Severity: factoryapi.FactoryValidationSeverityError,
		Message:  `workstation "bob" must define a failure route.`,
		Subject: factoryapi.FactoryValidationSubject{
			Type:     factoryapi.FactoryValidationSubjectTypeWorkstation,
			Id:       "bob",
			Location: factoryapi.FactoryValidationSubjectLocationOnFailure,
		},
	}
	srv := newTestServer(&testutil.MockFactory{
		CreateNamedFactoryErr: apisurface.NewTopologyValidationError(
			"Factory topology contains invalid graph references.",
			[]factoryapi.FactoryValidationTarget{target},
		),
	})

	req := httptest.NewRequest(http.MethodPost, "/factories", bytes.NewBufferString(validNamedFactoryBody("beta", "beta-task")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("response = %#v status=%d", response, rec.Code)
	}
	assertErrorResponsePreservesLegacyFields(
		t,
		response,
		factoryapi.INVALIDFACTORY,
		factoryapi.ErrorFamilyBadRequest,
		"Factory payload is not a valid Agent Factory definition.",
	)
	if response.Targets == nil || len(*response.Targets) != 1 {
		t.Fatalf("targets = %#v, want one canonical target", response.Targets)
	}
	got := (*response.Targets)[0]
	assertBlockingValidationTarget(t, got)
	assertHasValidationTarget(
		t,
		[]factoryapi.FactoryValidationTarget{got},
		factoryvalidation.CodeWorkstationMissingFailureRoute,
		factoryapi.FactoryValidationSubjectTypeWorkstation,
		"bob",
		factoryapi.FactoryValidationSubjectLocationOnFailure,
		"bob ON_FAILURE target",
	)
}

func assertErrorResponsePreservesLegacyFields(
	t *testing.T,
	response factoryapi.ErrorResponse,
	wantCode factoryapi.ErrorResponseCode,
	wantFamily factoryapi.ErrorFamily,
	wantMessage string,
) {
	t.Helper()
	if response.Code != wantCode {
		t.Fatalf("error code = %q, want %q", response.Code, wantCode)
	}
	if response.Family != wantFamily {
		t.Fatalf("error family = %q, want %q", response.Family, wantFamily)
	}
	if response.Message != wantMessage {
		t.Fatalf("error message = %q, want %q", response.Message, wantMessage)
	}
}

func assertBlockingValidationTarget(t *testing.T, target factoryapi.FactoryValidationTarget) {
	t.Helper()
	if target.Severity != factoryapi.FactoryValidationSeverityError {
		t.Fatalf("target severity = %q, want error", target.Severity)
	}
	if target.Code == "" {
		t.Fatal("target code is empty, want stable validation code")
	}
	if target.Message == "" {
		t.Fatal("target message is empty, want human-readable validation message")
	}
	if target.Subject.Type == "" || target.Subject.Location == "" {
		t.Fatalf("target subject = %#v, want structured type and location", target.Subject)
	}
	if target.Subject.Type != factoryapi.FactoryValidationSubjectTypeFactory && target.Subject.Id == "" {
		t.Fatalf("target subject = %#v, want non-empty id for component-scoped targets", target.Subject)
	}
}
