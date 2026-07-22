package apiserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	factoryconfig "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/diagnostics"
	"go.uber.org/zap"
)

const (
	testValidationCodeDuplicateIdentifier                 = "factory.duplicateIdentifier"
	testValidationCodeDanglingWorkerReference             = "factory.worker.danglingReference"
	testValidationCodeDanglingPlaceReference              = "factory.route.danglingPlaceReference"
	testValidationCodeWorkstationMissingOutputRoutes      = "factory.workstation.missingOutputRoutes"
	testValidationCodeWorkstationMissingFailureRoute      = "factory.workstation.missingFailureRoute"
	testValidationCodeWorkTypeMissingCompletionState      = "factory.workType.missingCompletionState"
	testValidationCodeWorkTypeMissingFailureState         = "factory.workType.missingFailureState"
	testValidationCodeWorkStateMissingTerminalPath        = "factory.workState.missingTerminalCompletionPath"
	testValidationCodeWorkTypeHandlingUniqueDefault       = "work-type-handling-behavior-unique-default"
	testValidationCodeInvocationUnknownOutputPath         = "factory.invocationSignature.unknownOutputPathParameter"
	testValidationCodeInvocationInvalidInterpolation      = "factory.invocationSignature.invalidInterpolationReference"
	testValidationCodeInvocationIncompatibleInterpolation = "factory.invocationSignature.incompatibleInterpolationReference"
)

func TestFactoryValidation_EquivalentCanonicalTargetsAcrossPackageConfigAndAPIPaths(t *testing.T) {
	t.Parallel()

	targets := crossPathValidationTargets()
	configFindings := factoryconfig.FactoryDefinitionFindings(targets)
	if len(configFindings) != len(targets) {
		t.Fatalf("config findings = %d, package targets = %d, want equivalent coverage",
			len(configFindings), len(targets))
	}
	for index, target := range targets {
		if configFindings[index].Rule != target.Code {
			t.Fatalf("config finding[%d].Rule = %q, want package target code %q",
				index, configFindings[index].Rule, target.Code)
		}
	}

	srv := newAPITestServerWithValidator(nil, programmableFactoryValidator{
		result: interfaces.ValidationResult{Targets: targets},
	})
	req := httptest.NewRequest(
		http.MethodPost,
		"/factory-validations",
		bytes.NewBufferString(factoryfixtures.CrossPathInvalidFactoryJSON),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /factory-validations status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	result := decodeJSONResponse[factoryapi.FactoryValidationResult](t, rec)
	if len(result.Targets) != len(targets) {
		t.Fatalf("API targets = %d, root targets = %d", len(result.Targets), len(targets))
	}
	for index, target := range targets {
		if result.Targets[index].Code != target.Code {
			t.Fatalf("API target[%d] code = %q, want %q", index, result.Targets[index].Code, target.Code)
		}
	}
}

func TestValidateFactory_OperationalCronRulesRemainConfigScoped(t *testing.T) {
	validator := programmableFactoryValidator{
		topology: interfaces.TopologyValidationResult{Findings: []interfaces.TopologyFinding{{
			Severity: interfaces.ValidationSeverityError,
			Rule:     "cron-config",
			Message:  "cron configuration is required",
		}}},
	}
	srv := newAPITestServerWithValidator(nil, validator)

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
		Workers: []interfaces.FactoryWorkerConfig{{Name: "w1"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "daily-refresh",
			Kind:           interfaces.WorkstationKindCron,
			WorkerTypeName: "w1",
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		}},
	}
	configFindings := factoryconfig.TopologyFindings(
		validator.ValidateTopology(t.Context(), cfg, nil).Findings,
	)
	assertConfigFindingRule(t, configFindings, "cron-config")
}

func TestValidateFactory_RejectsDuplicateDefaultHandlingWorkTypes(t *testing.T) {
	t.Parallel()

	srv := newAPITestServerWithValidator(nil, programmableFactoryValidator{
		result: interfaces.ValidationResult{Targets: []interfaces.ValidationTarget{
			validationTarget(
				testValidationCodeWorkTypeHandlingUniqueDefault,
				"handlingBehavior DEFAULT may only be assigned once",
				interfaces.ValidationSubjectTypeWorkType,
				"story",
				interfaces.ValidationSubjectLocationDefinition,
			),
		}},
	})
	body := `{
		"name":"alpha",
		"workTypes":[
			{"name":"story","handlingBehavior":["DEFAULT"],"states":[
				{"name":"queued","type":"INITIAL"},
				{"name":"done","type":"TERMINAL"}
			]},
			{"name":"task","handlingBehavior":["DEFAULT"],"states":[
				{"name":"queued","type":"INITIAL"},
				{"name":"done","type":"TERMINAL"}
			]}
		],
		"workers":[{"name":"worker-a"}],
		"workstations":[{
			"name":"process",
			"worker":"worker-a",
			"inputs":[{"workType":"story","state":"queued"}],
			"outputs":[{"workType":"story","state":"done"}]
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
	assertHasValidationTargetCode(t, result.Targets, testValidationCodeWorkTypeHandlingUniqueDefault)
	for _, target := range result.Targets {
		if target.Code == testValidationCodeWorkTypeHandlingUniqueDefault &&
			!strings.Contains(target.Message, "handlingBehavior DEFAULT") {
			t.Fatalf("target message = %q, want duplicate default handling explanation", target.Message)
		}
	}
}

func TestValidateFactory_RejectsResourceSlotAuthoredAsWorkstationRoute(t *testing.T) {
	t.Parallel()

	srv := newAPITestServerWithValidator(nil, programmableFactoryValidator{
		result: interfaces.ValidationResult{Targets: []interfaces.ValidationTarget{
			validationTarget(
				testValidationCodeDanglingPlaceReference,
				"resource slots are not Work routes",
				interfaces.ValidationSubjectTypeRoute,
				"process->executor-slot:available",
				interfaces.ValidationSubjectLocationInputs,
			),
		}},
	})
	body := `{
		"name":"alpha",
		"resources":[{"name":"executor-slot","capacity":10}],
		"workTypes":[{"name":"task","states":[
			{"name":"init","type":"INITIAL"},
			{"name":"done","type":"TERMINAL"}
		]}],
		"workers":[{"name":"processor"}],
		"workstations":[{
			"name":"process",
			"worker":"processor",
			"inputs":[
				{"workType":"task","state":"init"},
				{"workType":"executor-slot","state":"available"}
			],
			"outputs":[{"workType":"task","state":"done"}]
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
	assertHasValidationTargetCode(t, result.Targets, testValidationCodeDanglingPlaceReference)
	assertHasValidationTarget(
		t,
		result.Targets,
		testValidationCodeDanglingPlaceReference,
		factoryapi.FactoryValidationSubjectTypeRoute,
		"process->executor-slot:available",
		factoryapi.FactoryValidationSubjectLocationInputs,
		`resource slot route target`,
	)
}

func TestValidateFactory_PreservesCanonicalStructuralCodesFromCrossPathFixture(t *testing.T) {
	srv := newAPITestServerWithValidator(nil, programmableFactoryValidator{
		result: interfaces.ValidationResult{Targets: crossPathValidationTargets()},
	})

	req := httptest.NewRequest(
		http.MethodPost,
		"/factory-validations",
		bytes.NewBufferString(factoryfixtures.CrossPathInvalidFactoryJSON),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	result := decodeJSONResponse[factoryapi.FactoryValidationResult](t, rec)
	wantCodes := []string{
		testValidationCodeDuplicateIdentifier,
		testValidationCodeDanglingWorkerReference,
		testValidationCodeDanglingPlaceReference,
		testValidationCodeWorkstationMissingFailureRoute,
		testValidationCodeWorkTypeMissingCompletionState,
		testValidationCodeWorkTypeMissingFailureState,
		testValidationCodeWorkStateMissingTerminalPath,
	}
	for _, code := range wantCodes {
		assertHasValidationTargetCode(t, result.Targets, code)
	}
}

func TestValidateFactory_RejectsInvalidInvocationSignature(t *testing.T) {
	t.Parallel()

	srv := newAPITestServerWithValidator(nil, programmableFactoryValidator{
		result: interfaces.ValidationResult{Targets: []interfaces.ValidationTarget{
			validationTarget(testValidationCodeInvocationUnknownOutputPath, "unknown output path parameter", interfaces.ValidationSubjectTypeFactory, "invocationSignature", interfaces.ValidationSubjectLocationDefinition),
			validationTarget(testValidationCodeInvocationInvalidInterpolation, "invalid interpolation reference", interfaces.ValidationSubjectTypeWorker, "worker-a", interfaces.ValidationSubjectLocationDefinition),
			validationTarget(testValidationCodeInvocationIncompatibleInterpolation, "incompatible interpolation reference", interfaces.ValidationSubjectTypeWorkstation, "process", interfaces.ValidationSubjectLocationDefinition),
		}},
	})
	body := `{
		"name":"signature-invalid",
		"invocationSignature":{
			"unknownNamedArgumentPolicy":"COLLECT",
			"parameters":[
				{
					"name":"items",
					"externalName":"items",
					"valueMode":"REPEATED",
					"bindings":[{"kind":"NAMED"}]
				}
			],
			"outputContract":{
				"mode":"FILE",
				"pathParameter":"missing-output"
			}
		},
		"workTypes":[{"name":"task","states":[
			{"name":"queued","type":"INITIAL"},
			{"name":"done","type":"TERMINAL"},
			{"name":"failed","type":"FAILED"}
		]}],
		"workers":[{"name":"worker-a","type":"INFERENCE_WORKER","model":"${missing}"}],
		"workstations":[{
			"name":"process",
			"worker":"worker-a",
			"inputs":[{"workType":"task","state":"queued"}],
			"outputs":[{"workType":"task","state":"done"}],
			"onFailure":[{"workType":"task","state":"failed"}],
			"body":"Use ${items}"
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
	assertHasValidationTargetCode(t, result.Targets, testValidationCodeInvocationUnknownOutputPath)
	assertHasValidationTargetCode(t, result.Targets, testValidationCodeInvocationInvalidInterpolation)
	assertHasValidationTargetCode(t, result.Targets, testValidationCodeInvocationIncompatibleInterpolation)
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
			Code:     testValidationCodeDuplicateIdentifier,
			Severity: factoryapi.FactoryValidationSeverityError,
			Message:  `duplicate worker identifier "worker-a".`,
			Subject: factoryapi.FactoryValidationSubject{
				Type:     factoryapi.FactoryValidationSubjectTypeWorker,
				Id:       "worker-a",
				Location: factoryapi.FactoryValidationSubjectLocationDefinition,
			},
		},
		{
			Code:     testValidationCodeDanglingWorkerReference,
			Severity: factoryapi.FactoryValidationSeverityError,
			Message:  `workstation "process" references unknown worker "missing-worker".`,
			Subject: factoryapi.FactoryValidationSubject{
				Type:     factoryapi.FactoryValidationSubjectTypeWorkstation,
				Id:       "process",
				Location: factoryapi.FactoryValidationSubjectLocationDefinition,
			},
		},
		{
			Code:     testValidationCodeDanglingPlaceReference,
			Severity: factoryapi.FactoryValidationSeverityError,
			Message:  `workstation "process" routes to unknown place.`,
			Subject: factoryapi.FactoryValidationSubject{
				Type:     factoryapi.FactoryValidationSubjectTypeWorkstation,
				Id:       "process",
				Location: factoryapi.FactoryValidationSubjectLocationOutputs,
			},
		},
	}
	srv := newAPITestServerWithValidator(apiFactorySaveScript{
		save: func(context.Context, string, factoryapi.FactorySaveMode, factoryapi.Factory) (factoryapi.Factory, error) {
			return factoryapi.Factory{}, apisurface.NewTopologyValidationError(
				"Factory topology contains invalid graph references.",
				targets,
			)
		},
	}, programmableFactoryValidator{})

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(saveFactoryForSessionRequestBody(`{"name":"beta"}`)))
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
		factoryapi.ErrorResponseCodeINVALIDFACTORY,
		factoryapi.ErrorFamilyBadRequest,
		"Factory payload is not a valid Agent Factory definition.",
	)
	if response.Targets == nil || len(*response.Targets) < 3 {
		t.Fatalf("targets = %#v, want multiple blocking validation targets", response.Targets)
	}
	assertHasValidationTargetCode(t, *response.Targets, testValidationCodeDuplicateIdentifier)
	assertHasValidationTargetCode(t, *response.Targets, testValidationCodeDanglingWorkerReference)
	assertHasValidationTargetCode(t, *response.Targets, testValidationCodeDanglingPlaceReference)
	assertBlockingValidationTarget(t, (*response.Targets)[0])
}

func TestUpsertNamedFactory_ReturnsTopologyValidationTargets(t *testing.T) {
	target := factoryapi.FactoryValidationTarget{
		Code:     testValidationCodeDanglingPlaceReference,
		Severity: factoryapi.FactoryValidationSeverityError,
		Message:  "workstation process routes to unknown place.",
		Subject: factoryapi.FactoryValidationSubject{
			Type:     factoryapi.FactoryValidationSubjectTypeWorkstation,
			Id:       "process",
			Location: factoryapi.FactoryValidationSubjectLocationOutputs,
		},
	}
	srv := newAPITestServerWithValidator(apiFactorySaveScript{
		save: func(context.Context, string, factoryapi.FactorySaveMode, factoryapi.Factory) (factoryapi.Factory, error) {
			return factoryapi.Factory{}, apisurface.NewTopologyValidationError(
				"Factory topology contains invalid graph references.",
				[]factoryapi.FactoryValidationTarget{target},
			)
		},
	}, programmableFactoryValidator{})

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(upsertNamedFactoryRequestBody(validNamedFactoryBody("beta", "beta-task"))))
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
		factoryapi.ErrorResponseCodeINVALIDFACTORY,
		factoryapi.ErrorFamilyBadRequest,
		"Factory payload is not a valid Agent Factory definition.",
	)
	if response.Targets == nil || len(*response.Targets) != 1 {
		t.Fatalf("targets = %#v, want one canonical target", response.Targets)
	}
	gotTarget := (*response.Targets)[0]
	assertBlockingValidationTarget(t, gotTarget)
	if gotTarget.Code != testValidationCodeDanglingPlaceReference ||
		gotTarget.Subject.Type != factoryapi.FactoryValidationSubjectTypeWorkstation ||
		gotTarget.Subject.Id != "process" ||
		gotTarget.Subject.Location != factoryapi.FactoryValidationSubjectLocationOutputs {
		t.Fatalf("error target = %#v, want dangling output workstation target", gotTarget)
	}
}

func TestUpsertNamedFactory_RejectsInvalidFactoryPayloadWithTargets(t *testing.T) {
	srv := newAPITestServerWithValidator(
		apiFactorySaveScript{save: func(context.Context, string, factoryapi.FactorySaveMode, factoryapi.Factory) (factoryapi.Factory, error) {
			return factoryapi.Factory{}, apisurface.ErrInvalidNamedFactory
		}},
		programmableFactoryValidator{},
	)
	body := `{"name":"beta","workTypes":[{"name":"beta-task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}],"workers":[{"name":"planner","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514"}],"workstations":[{"name":"plan-task","behavior":"STANDARD","type":"MODEL_WORKSTATION","worker":"missing-worker","inputs":[{"workType":"beta-task","state":"init"}],"outputs":[{"workType":"beta-task","state":"done"}]}]}`
	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(upsertNamedFactoryRequestBody(body)))
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
		factoryapi.ErrorResponseCodeINVALIDFACTORY,
		factoryapi.ErrorFamilyBadRequest,
		"Factory payload is not a valid Agent Factory definition.",
	)
	if response.Targets == nil || len(*response.Targets) != 1 || (*response.Targets)[0].Code != interfaces.ValidationCodeFactoryPayloadInvalid {
		t.Fatalf("error targets = %#v, want canonical invalid factory payload target", response.Targets)
	}
	assertBlockingValidationTarget(t, (*response.Targets)[0])
}

func TestSaveCurrentFactory_ReturnsBobWorkstationOnFailureTarget(t *testing.T) {
	target := factoryapi.FactoryValidationTarget{
		Code:     testValidationCodeWorkstationMissingFailureRoute,
		Severity: factoryapi.FactoryValidationSeverityError,
		Message:  `workstation "bob" must define a failure route.`,
		Subject: factoryapi.FactoryValidationSubject{
			Type:     factoryapi.FactoryValidationSubjectTypeWorkstation,
			Id:       "bob",
			Location: factoryapi.FactoryValidationSubjectLocationOnFailure,
		},
	}
	srv := newAPITestServerWithValidator(apiFactorySaveScript{
		save: func(context.Context, string, factoryapi.FactorySaveMode, factoryapi.Factory) (factoryapi.Factory, error) {
			return factoryapi.Factory{}, apisurface.NewTopologyValidationError(
				"Factory topology contains invalid graph references.",
				[]factoryapi.FactoryValidationTarget{target},
			)
		},
	}, programmableFactoryValidator{})

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(saveFactoryForSessionRequestBody(`{"name":"beta"}`)))
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
		factoryapi.ErrorResponseCodeINVALIDFACTORY,
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
		testValidationCodeWorkstationMissingFailureRoute,
		factoryapi.FactoryValidationSubjectTypeWorkstation,
		"bob",
		factoryapi.FactoryValidationSubjectLocationOnFailure,
		"bob ON_FAILURE target",
	)
}

func TestUpsertNamedFactory_ReturnsBobWorkstationOnFailureTarget(t *testing.T) {
	target := factoryapi.FactoryValidationTarget{
		Code:     testValidationCodeWorkstationMissingFailureRoute,
		Severity: factoryapi.FactoryValidationSeverityError,
		Message:  `workstation "bob" must define a failure route.`,
		Subject: factoryapi.FactoryValidationSubject{
			Type:     factoryapi.FactoryValidationSubjectTypeWorkstation,
			Id:       "bob",
			Location: factoryapi.FactoryValidationSubjectLocationOnFailure,
		},
	}
	srv := newAPITestServerWithValidator(apiFactorySaveScript{
		save: func(context.Context, string, factoryapi.FactorySaveMode, factoryapi.Factory) (factoryapi.Factory, error) {
			return factoryapi.Factory{}, apisurface.NewTopologyValidationError(
				"Factory topology contains invalid graph references.",
				[]factoryapi.FactoryValidationTarget{target},
			)
		},
	}, programmableFactoryValidator{})

	req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(upsertNamedFactoryRequestBody(validNamedFactoryBody("beta", "beta-task"))))
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
		factoryapi.ErrorResponseCodeINVALIDFACTORY,
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
		testValidationCodeWorkstationMissingFailureRoute,
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

func saveFactoryForSessionRequestBody(factoryJSON string) string {
	return fmt.Sprintf(`{"factory":%s}`, factoryJSON)
}

func upsertNamedFactoryRequestBody(factoryJSON string) string {
	return fmt.Sprintf(`{"mode":"UPSERT_NAMED_AND_ACTIVATE","factory":%s}`, factoryJSON)
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

type programmableFactoryValidator struct {
	result   interfaces.ValidationResult
	topology interfaces.TopologyValidationResult
}

// apiFactorySaveScript is the exact public Factory Definitions edge exercised
// by validation protocol tests. Every unprogrammed operation is a test defect.
type apiFactorySaveScript struct {
	get         func(context.Context, string) (factoryapi.Factory, error)
	save        func(context.Context, string, factoryapi.FactorySaveMode, factoryapi.Factory) (factoryapi.Factory, error)
	saveCurrent func(context.Context, string, factoryapi.Factory) (factoryapi.Factory, error)
}

func (script apiFactorySaveScript) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	if script.get == nil {
		panic("unexpected FactorySaveAPI.GetCurrentFactoryForSession call")
	}
	return script.get(ctx, sessionID)
}

func (script apiFactorySaveScript) SaveFactoryForSession(ctx context.Context, sessionID string, mode factoryapi.FactorySaveMode, request factoryapi.Factory) (factoryapi.Factory, error) {
	if script.save == nil {
		panic("unexpected FactorySaveAPI.SaveFactoryForSession call")
	}
	return script.save(ctx, sessionID, mode, request)
}

func (script apiFactorySaveScript) SaveCurrentFactoryForSession(ctx context.Context, sessionID string, request factoryapi.Factory) (factoryapi.Factory, error) {
	if script.saveCurrent == nil {
		panic("unexpected FactorySaveAPI.SaveCurrentFactoryForSession call")
	}
	return script.saveCurrent(ctx, sessionID, request)
}

func (validator programmableFactoryValidator) ValidateSubmittedDefinition(
	ctx context.Context,
	request interfaces.SubmittedDefinitionValidationRequest,
) (interfaces.ValidationResult, error) {
	result := validator.Validate(ctx, request.Config, request.WorkflowSourceReader)
	return result, nil
}

func (validator programmableFactoryValidator) Validate(
	context.Context,
	*interfaces.FactoryConfig,
	interfaces.WorkflowSourceReader,
) interfaces.ValidationResult {
	return validator.result
}

func (validator programmableFactoryValidator) ValidateBlockingLoad(
	context.Context,
	*interfaces.FactoryConfig,
) interfaces.ValidationResult {
	return validator.result
}

func (validator programmableFactoryValidator) ValidateTopology(
	context.Context,
	*interfaces.FactoryConfig,
	interfaces.RequiredToolChecker,
) interfaces.TopologyValidationResult {
	return validator.topology
}

func (programmableFactoryValidator) WorkerWorkstationBehaviorCompatibility(
	context.Context,
	*interfaces.FactoryConfig,
) []interfaces.ValidationTarget {
	return nil
}

func (programmableFactoryValidator) WorkTypeHandlingBehavior(
	context.Context,
	*interfaces.FactoryConfig,
	bool,
) []interfaces.ValidationTarget {
	return nil
}

func (validator programmableFactoryValidator) PruneLayout(
	context.Context,
	*interfaces.FactoryConfig,
	interfaces.PendingFactoryGraphTopology,
) interfaces.ValidationResult {
	return validator.result
}

func newAPITestServerWithValidator(
	definitions apisurface.FactorySaveAPI,
	validator interfaces.SubmittedDefinitionValidationOperation,
) *api.Server {
	return newAPIServerFromRoles(
		nil, nil, nil, nil, nil, nil, nil, definitions,
		validator, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zap.NewNop(),
	)
}

func validationTarget(
	code string,
	message string,
	subjectType interfaces.ValidationSubjectType,
	subjectID string,
	location interfaces.ValidationSubjectLocation,
) interfaces.ValidationTarget {
	return interfaces.ValidationTarget{
		Code:     code,
		Severity: interfaces.ValidationSeverityError,
		Message:  message,
		Subject: interfaces.ValidationSubject{
			Type: subjectType, ID: subjectID, Location: location,
		},
	}
}

func crossPathValidationTargets() []interfaces.ValidationTarget {
	return []interfaces.ValidationTarget{
		validationTarget(testValidationCodeDuplicateIdentifier, "duplicate identifier", interfaces.ValidationSubjectTypeWorker, "worker-a", interfaces.ValidationSubjectLocationDefinition),
		validationTarget(testValidationCodeDanglingWorkerReference, "dangling worker", interfaces.ValidationSubjectTypeWorkstation, "process", interfaces.ValidationSubjectLocationDefinition),
		validationTarget(testValidationCodeDanglingPlaceReference, "dangling route", interfaces.ValidationSubjectTypeRoute, "process->missing", interfaces.ValidationSubjectLocationOutputs),
		validationTarget(testValidationCodeWorkstationMissingFailureRoute, "missing failure route", interfaces.ValidationSubjectTypeWorkstation, "process", interfaces.ValidationSubjectLocationOnFailure),
		validationTarget(testValidationCodeWorkTypeMissingCompletionState, "missing completion state", interfaces.ValidationSubjectTypeWorkType, "task", interfaces.ValidationSubjectLocationStates),
		validationTarget(testValidationCodeWorkTypeMissingFailureState, "missing failure state", interfaces.ValidationSubjectTypeWorkType, "task", interfaces.ValidationSubjectLocationStates),
		validationTarget(testValidationCodeWorkStateMissingTerminalPath, "missing terminal path", interfaces.ValidationSubjectTypeWorkState, "queued", interfaces.ValidationSubjectLocationTerminal),
	}
}

func TestValidateFactory_RoutelessCronAndLogicalMove_ReturnMissingOutputRoutesAtOutputs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		body        string
		workstation string
	}{
		{name: "routeless_cron", body: factoryfixtures.RoutelessCronFactoryJSON, workstation: "cron"},
		{name: "routeless_logical_move", body: factoryfixtures.RoutelessLogicalMoveFactoryJSON, workstation: "router"},
		{name: "routeless_logical_move_cron", body: factoryfixtures.RoutelessLogicalMoveCronFactoryJSON, workstation: "trigger-monkey"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := newAPIServerFromRoles(
				nil, nil, nil, nil, nil, nil, nil, nil,
				programmableFactoryValidator{result: interfaces.ValidationResult{Targets: []interfaces.ValidationTarget{
					validationTarget(
						testValidationCodeWorkstationMissingOutputRoutes,
						"workstation requires an output route",
						interfaces.ValidationSubjectTypeWorkstation,
						tc.workstation,
						interfaces.ValidationSubjectLocationOutputs,
					),
				}}},
				nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zap.NewNop(),
			)
			req := httptest.NewRequest(http.MethodPost, "/factory-validations", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("POST /factory-validations status = %d, want 200: %s", rec.Code, rec.Body.String())
			}

			result := decodeJSONResponse[factoryapi.FactoryValidationResult](t, rec)
			assertHasValidationTarget(
				t, result.Targets, testValidationCodeWorkstationMissingOutputRoutes,
				factoryapi.FactoryValidationSubjectTypeWorkstation, tc.workstation,
				factoryapi.FactoryValidationSubjectLocationOutputs,
				tc.workstation+" OUTPUTS missingOutputRoutes target",
			)
			for _, target := range result.Targets {
				if target.Code == testValidationCodeWorkstationMissingFailureRoute &&
					target.Subject.Type == factoryapi.FactoryValidationSubjectTypeWorkstation &&
					target.Subject.Id == tc.workstation &&
					target.Subject.Location == factoryapi.FactoryValidationSubjectLocationOnFailure {
					t.Fatalf("targets = %#v, want no missingFailureRoute at ON_FAILURE for routeless workstation", result.Targets)
				}
			}
		})
	}
}
