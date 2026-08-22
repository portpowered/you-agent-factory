package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionshttp "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/http"
	httpcompat "github.com/portpowered/infinite-you/pkg/transports/http/compat"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type validationDecodeCase struct {
	name        string
	body        string
	wantStatus  int
	wantCode    string
	wantMessage string
}

func TestValidateFactory_RejectsInvalidPayloadBeforeValidationInvoked(t *testing.T) {
	t.Parallel()

	cases := []validationDecodeCase{
		{
			name:        "malformed",
			body:        `{"name":"alpha"`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "invalid request payload",
		},
		{
			name:        "empty",
			body:        "",
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "invalid request payload",
		},
		{
			name:        "multi_object",
			body:        `{"name":"alpha"}{}`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "request payload must contain one JSON object",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			validation := &httpDefinitionsValidationFake{}
			handler := factorydefinitionshttp.NewHandlerFromRoot(
				factorydefinitionshttp.RootBinding{Validation: validation},
				zap.NewNop(),
			)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(
				http.MethodPost,
				"/factory-validations",
				strings.NewReader(tc.body),
			)
			request.Header.Set("Content-Type", "application/json")

			handler.ValidateFactory(recorder, request)

			if validation.invoked {
				t.Fatal("ValidateSubmittedDefinition was invoked before request decode succeeded")
			}
			assertValidationErrorResponse(
				t,
				recorder,
				tc.wantStatus,
				tc.wantCode,
				tc.wantMessage,
			)
		})
	}
}

func TestValidateFactory_AcceptsUnknownFieldsWithWarning(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.WarnLevel)
	validation := &capturingValidationFake{}
	handler := factorydefinitionshttp.NewHandlerFromRoot(
		factorydefinitionshttp.RootBinding{Validation: validation},
		zap.New(core),
	)
	body := strings.Replace(
		strings.Replace(minimalValidationFactoryBody, `"name": "alpha",`, `"name": "alpha", "futureRoot": "secret-root",`, 1),
		`"name": "task",`, `"name": "task", "futureNested": {"value":"secret-nested"},`, 1,
	)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/factory-validations", strings.NewReader(body))

	handler.ValidateFactory(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	warning := recorder.Header().Get("Warning")
	if !strings.Contains(warning, "299") ||
		!strings.Contains(warning, "$.futureRoot") ||
		!strings.Contains(warning, "$.workTypes[0].futureNested") {
		t.Fatalf("Warning = %q, want code 299 and both ignored paths", warning)
	}
	if validation.request.Config == nil || validation.request.Config.Name != "alpha" {
		t.Fatalf("decoded config = %#v, want known factory name alpha", validation.request.Config)
	}
	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("warning log count = %d, want one", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["warning_code"] != int64(httpcompat.WarningCode) ||
		fields["boundary"] != "factory_definitions.http" ||
		fields["operation"] != "validate_factory" {
		t.Fatalf("warning fields = %#v, want HTTP compatibility metadata", fields)
	}
	if got, ok := fields["json_paths"].([]interface{}); !ok || !reflect.DeepEqual(got, []interface{}{
		"$.futureRoot", "$.workTypes[0].futureNested",
	}) {
		t.Fatalf("json_paths = %#v, want sorted ignored paths", fields["json_paths"])
	}
	if strings.Contains(entries[0].Message, "secret") || strings.Contains(recorder.Body.String(), "secret") {
		t.Fatal("compatibility diagnostics exposed an ignored field value")
	}
}

func TestValidateFactory_EncodesValidationTargetsFromFakeRoot(t *testing.T) {
	t.Parallel()

	validation := &httpDefinitionsValidationFake{
		result: factorydefinitions.ValidationResult{
			Targets: []factorydefinitions.ValidationTarget{{
				Code:     "factory.validation.stub",
				Severity: factorydefinitions.ValidationSeverityError,
				Message:  "stub validation finding",
				Path:     "workers[0].model",
				Subject: factorydefinitions.ValidationSubject{
					Type:     factorydefinitions.ValidationSubjectTypeWorker,
					ID:       "planner",
					Location: factorydefinitions.ValidationSubjectLocationDefinition,
				},
			}},
		},
	}
	handler := factorydefinitionshttp.NewHandlerFromRoot(
		factorydefinitionshttp.RootBinding{Validation: validation},
		zap.NewNop(),
	)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/factory-validations",
		strings.NewReader(minimalValidationFactoryBody),
	)
	request.Header.Set("Content-Type", "application/json")

	handler.ValidateFactory(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("response = %d %s, want 200", recorder.Code, recorder.Body.String())
	}

	var result factoryapi.FactoryValidationResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result.Targets) != 1 {
		t.Fatalf("targets = %#v, want one encoded finding", result.Targets)
	}
	target := result.Targets[0]
	if target.Code != "factory.validation.stub" {
		t.Fatalf("target code = %q, want factory.validation.stub", target.Code)
	}
	if target.Message != "stub validation finding" {
		t.Fatalf("target message = %q, want stub validation finding", target.Message)
	}
	if target.Severity != factoryapi.FactoryValidationSeverityError {
		t.Fatalf("target severity = %q, want error", target.Severity)
	}
	if target.Path == nil || *target.Path != "workers[0].model" {
		t.Fatalf("target path = %#v, want workers[0].model", target.Path)
	}
	if target.Subject.Type != factoryapi.FactoryValidationSubjectTypeWorker {
		t.Fatalf("subject type = %q, want WORKER", target.Subject.Type)
	}
	if target.Subject.Id != "planner" {
		t.Fatalf("subject id = %q, want planner", target.Subject.Id)
	}
}

func TestValidateFactory_DecodesFactoryIntoSubmittedDefinitionValidationRequest(t *testing.T) {
	t.Parallel()

	validation := &capturingValidationFake{}
	handler := factorydefinitionshttp.NewHandlerFromRoot(
		factorydefinitionshttp.RootBinding{Validation: validation},
		zap.NewNop(),
	)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/factory-validations",
		strings.NewReader(minimalValidationFactoryBody),
	)
	request.Header.Set("Content-Type", "application/json")

	handler.ValidateFactory(recorder, request)

	if !validation.invoked {
		t.Fatal("ValidateSubmittedDefinition was not invoked")
	}
	if validation.request.Config == nil {
		t.Fatal("decoded Config is nil")
	}
	if validation.request.Config.Name != "alpha" {
		t.Fatalf("decoded factory name = %q, want alpha", validation.request.Config.Name)
	}
	if len(validation.request.Config.WorkTypes) != 1 || validation.request.Config.WorkTypes[0].Name != "task" {
		t.Fatalf("decoded work types = %#v, want task work type", validation.request.Config.WorkTypes)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("response = %d %s, want 200", recorder.Code, recorder.Body.String())
	}
}

type capturingValidationFake struct {
	invoked bool
	request factorydefinitions.SubmittedDefinitionValidationRequest
}

func (fake *capturingValidationFake) ValidateSubmittedDefinition(
	_ context.Context,
	request factorydefinitions.SubmittedDefinitionValidationRequest,
) (factorydefinitions.ValidationResult, error) {
	fake.invoked = true
	fake.request = request
	return factorydefinitions.ValidationResult{}, nil
}

var _ factorydefinitions.SubmittedDefinitionValidationOperation = (*capturingValidationFake)(nil)

func assertValidationErrorResponse(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	wantStatus int,
	wantCode string,
	wantMessage string,
) {
	t.Helper()

	if recorder.Code != wantStatus {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, wantStatus, recorder.Body.String())
	}

	var errResp factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if string(errResp.Code) != wantCode {
		t.Fatalf("code = %q, want %q", errResp.Code, wantCode)
	}
	if errResp.Message != wantMessage {
		t.Fatalf("message = %q, want %q", errResp.Message, wantMessage)
	}
	if errResp.Family != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("family = %q, want bad_request", errResp.Family)
	}
}
