package apiserver_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestValidateFactory_RejectsRetiredFactoryRunnerWithMigrationGuidance(t *testing.T) {
	t.Parallel()

	srv := newAPITestServer(&testutil.MockFactory{})
	body := strings.Replace(
		validNamedFactoryBody("beta", "beta-task"),
		`"name":"beta"`,
		`"name":"beta","runner":"codex"`,
		1,
	)

	req := httptest.NewRequest(http.MethodPost, "/factory-validations", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertFactoryRetiredRunnerMigrationError(t, rec, "factory.runner is retired; use factory.modelProvider")
}

func TestSaveCurrentFactory_RejectsRetiredWorkstationRunnerWithMigrationGuidance(t *testing.T) {
	t.Parallel()

	srv := newAPITestServer(&testutil.MockFactory{})
	factoryBody := strings.Replace(
		validNamedFactoryBody("beta", "beta-task"),
		`"name":"plan-task"`,
		`"name":"plan-task","runner":"codex"`,
		1,
	)
	body := saveFactoryForSessionRequestBody(factoryBody)

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertFactoryRetiredRunnerMigrationError(
		t,
		rec,
		"workstations[0].runner is retired; use workstations[0].modelProvider",
	)
}

func assertFactoryRetiredRunnerMigrationError(t *testing.T, rec *httptest.ResponseRecorder, wantMessage string) {
	t.Helper()

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}

	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	assertErrorResponsePreservesLegacyFields(
		t,
		response,
		factoryapi.BADREQUEST,
		factoryapi.ErrorFamilyBadRequest,
		wantMessage,
	)
}

func TestValidateFactory_RetiredRunnerErrorMatchesCLIValidationShape(t *testing.T) {
	t.Parallel()

	body := fmt.Sprintf(`{
		"name":"runner-migration",
		"runner":"gemini",
		"workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}],
		"workers":[{"name":"w1","type":"MODEL_WORKER","modelProvider":"CODEX","executorProvider":"SCRIPT_WRAP"}],
		"workstations":[{"name":"step","worker":"w1","type":"MODEL_WORKSTATION","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"done"}]}]
	}`)

	srv := newAPITestServer(&testutil.MockFactory{})
	req := httptest.NewRequest(http.MethodPost, "/factory-validations", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertFactoryRetiredRunnerMigrationError(t, rec, "factory.runner is retired; use factory.modelProvider")
}
