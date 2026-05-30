package apiserver_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestFactoryValidation_EquivalentCanonicalTargetsAcrossPackageConfigAndAPIPaths(t *testing.T) {
	t.Parallel()

	factory, err := factoryvalidation.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}
	cfg, err := factoryconfig.FactoryConfigFromOpenAPI(factory)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}

	explicit := factoryvalidation.Validate(&cfg)
	packageSignatures := factoryvalidation.CanonicalTargetSignatures(explicit.Targets)

	configFindings := factoryconfig.CanonicalStructuralFindings(&cfg)
	if len(configFindings) != len(explicit.Targets) {
		t.Fatalf("config findings = %d, package targets = %d, want equivalent coverage",
			len(configFindings), len(explicit.Targets))
	}
	for index, target := range explicit.Targets {
		if configFindings[index].Rule != target.Code {
			t.Fatalf("config finding[%d].Rule = %q, want package target code %q",
				index, configFindings[index].Rule, target.Code)
		}
	}

	srv := newAPITestServer(&testutil.MockFactory{})
	req := httptest.NewRequest(
		http.MethodPost,
		"/factory-validations",
		bytes.NewBufferString(factoryvalidation.CrossPathInvalidFactoryJSON),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /factory-validations status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	result := decodeJSONResponse[factoryapi.FactoryValidationResult](t, rec)
	apiSignatures := factoryvalidation.CanonicalAPITargetSignatures(result.Targets)
	if !factoryvalidation.EquivalentCanonicalTargetSignatures(packageSignatures, apiSignatures) {
		t.Fatalf("package signatures = %#v, api signatures = %#v, want equivalent canonical targets",
			packageSignatures, apiSignatures)
	}
}

func TestValidateFactory_OperationalCronRulesRemainConfigScoped(t *testing.T) {
	srv := newAPITestServer(&testutil.MockFactory{})

	body := `{
		"name":"alpha",
		"workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}],
		"workers":[{"name":"w1"}],
		"workstations":[{
			"name":"daily-refresh",
			"behavior":"CRON",
			"worker":"w1",
			"outputs":[{"workType":"task","state":"init"}]
		}]
	}`

	req := httptest.NewRequest(http.MethodPost, "/factory-validations", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	result := decodeJSONResponse[factoryapi.FactoryValidationResult](t, rec)
	for _, target := range result.Targets {
		if target.Code == "cron-config" || target.Code == "cron-schedule" {
			t.Fatalf("validation-only API targets = %#v, want operational cron rules to stay config-scoped", result.Targets)
		}
	}

	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "w1"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "daily-refresh",
			Kind:           interfaces.WorkstationKindCron,
			WorkerTypeName: "w1",
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		}},
	}
	configFindings := factoryconfig.NewConfigValidator().Validate(cfg).Findings
	assertConfigFindingRule(t, configFindings, "cron-config")
}

func TestValidateFactory_PreservesCanonicalStructuralCodesFromCrossPathFixture(t *testing.T) {
	srv := newAPITestServer(&testutil.MockFactory{})

	req := httptest.NewRequest(
		http.MethodPost,
		"/factory-validations",
		bytes.NewBufferString(factoryvalidation.CrossPathInvalidFactoryJSON),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	result := decodeJSONResponse[factoryapi.FactoryValidationResult](t, rec)
	wantCodes := []string{
		factoryvalidation.CodeDuplicateIdentifier,
		factoryvalidation.CodeDanglingWorkerReference,
		factoryvalidation.CodeDanglingPlaceReference,
		factoryvalidation.CodeWorkstationMissingFailureRoute,
		factoryvalidation.CodeWorkTypeMissingCompletionState,
		factoryvalidation.CodeWorkTypeMissingFailureState,
		factoryvalidation.CodeWorkStateMissingTerminalPath,
	}
	for _, code := range wantCodes {
		assertHasValidationTargetCode(t, result.Targets, code)
	}
}

func assertConfigFindingRule(t *testing.T, findings []factoryconfig.Finding, rule string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Rule == rule {
			return
		}
	}
	t.Fatalf("expected config finding rule %q, got %#v", rule, findings)
}

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
	srv := newAPITestServer(&testutil.MockFactory{
		SaveFactoryForSessionErr: apisurface.NewTopologyValidationError(
			"Factory topology contains invalid graph references.",
			targets,
		),
	})

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(saveFactoryForSessionBody(`{"name":"beta"}`)))
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
	srv := newAPITestServer(&testutil.MockFactory{})
	req := httptest.NewRequest(http.MethodPost, "/factories", bytes.NewBufferString(validNamedFactoryBody("beta", "beta-task")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertPostFactoriesRouteRemoved(t, rec)
}

func TestCreateFactory_RejectsInvalidFactoryPayloadWithTargets(t *testing.T) {
	srv := newAPITestServer(&testutil.MockFactory{})
	body := `{"name":"beta","workTypes":[{"name":"beta-task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}],"workers":[{"name":"planner","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514"}],"workstations":[{"name":"plan-task","behavior":"STANDARD","type":"MODEL_WORKSTATION","worker":"missing-worker","inputs":[{"workType":"beta-task","state":"init"}],"outputs":[{"workType":"beta-task","state":"done"}]}]}`
	req := httptest.NewRequest(http.MethodPost, "/factories", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertPostFactoriesRouteRemoved(t, rec)
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
	srv := newAPITestServer(&testutil.MockFactory{
		SaveFactoryForSessionErr: apisurface.NewTopologyValidationError(
			"Factory topology contains invalid graph references.",
			[]factoryapi.FactoryValidationTarget{target},
		),
	})

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(saveFactoryForSessionBody(`{"name":"beta"}`)))
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
	srv := newAPITestServer(&testutil.MockFactory{})
	req := httptest.NewRequest(http.MethodPost, "/factories", bytes.NewBufferString(validNamedFactoryBody("beta", "beta-task")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertPostFactoriesRouteRemoved(t, rec)
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

func decodeJSONResponse[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()

	var out T
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode JSON response: %v", err)
	}
	return out
}

func assertHasValidationTarget(
	t *testing.T,
	targets []factoryapi.FactoryValidationTarget,
	code string,
	subjectType factoryapi.FactoryValidationSubjectType,
	subjectID string,
	location factoryapi.FactoryValidationSubjectLocation,
	want string,
) {
	t.Helper()
	for _, target := range targets {
		if target.Code != code {
			continue
		}
		if target.Subject.Type != subjectType || target.Subject.Id != subjectID || target.Subject.Location != location {
			continue
		}
		return
	}
	t.Fatalf("validation targets = %#v, want %s", targets, want)
}

func assertHasValidationTargetCode(t *testing.T, targets []factoryapi.FactoryValidationTarget, code string) {
	t.Helper()
	for _, target := range targets {
		if target.Code == code {
			return
		}
	}
	t.Fatalf("targets = %#v, want code %q", targets, code)
}

func validNamedFactoryBody(name, workType string) string {
	return fmt.Sprintf(`{"name":%q,%s`, name, strings.TrimPrefix(namedFactoryPayloadJSON(name, workType), "{"))
}

func saveFactoryForSessionBody(factoryJSON string) string {
	return fmt.Sprintf(`{"factory":%s}`, factoryJSON)
}

func assertPostFactoriesRouteRemoved(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /factories status = %d, want 404 (route removed from published API): %s", rec.Code, rec.Body.String())
	}
}

func namedFactoryPayloadJSON(project, workType string) string {
	return fmt.Sprintf(`{
		"name": %q,
		"id": %q,
		"workTypes": [{
			"name": %q,
			"states": [
				{"name":"init","type":"INITIAL"},
				{"name":"done","type":"TERMINAL"},
				{"name":"failed","type":"FAILED"}
			]
		}],
		"workers": [{
			"name":"planner",
			"type":"MODEL_WORKER",
			"modelProvider":"CLAUDE",
			"executorProvider":"SCRIPT_WRAP",
			"model":"claude-sonnet-4-20250514"
		}],
		"workstations": [{
			"name":"plan-task",
			"behavior":"STANDARD",
			"type":"MODEL_WORKSTATION",
			"worker":"planner",
			"inputs":[{"workType":%q,"state":"init"}],
			"outputs":[{"workType":%q,"state":"done"}]
		}]
	}`, project, project, workType, workType, workType)
}
