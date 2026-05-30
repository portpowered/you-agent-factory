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
	if rec.Code != http.StatusBadRequest || response.Code != factoryapi.INVALIDFACTORY {
		t.Fatalf("response = %#v status=%d", response, rec.Code)
	}
	if response.Targets == nil || len(*response.Targets) != 1 {
		t.Fatalf("targets = %#v, want one canonical target", response.Targets)
	}
	got := (*response.Targets)[0]
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
