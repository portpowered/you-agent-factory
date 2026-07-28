package factorydefinition_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionmcp "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mcp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestBind_ValidateToolEncodesValidationTargetsFromFakeRoot(t *testing.T) {
	t.Parallel()

	validation := &mcpDefinitionsValidationFake{
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
	operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Validation: validation})
	raw, err := operation(
		context.Background(),
		factorydefinitionmcp.ToolValidate,
		json.RawMessage(minimalValidationFactoryBody),
	)
	if err != nil {
		t.Fatalf("CallTool(validate) error = %v", err)
	}

	var response struct {
		Result *factoryapi.FactoryValidationResult `json:"result"`
		Error  *factorydefinitionmcp.ToolErrorEnvelope `json:"error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("tool response error = %#v, want success result", response.Error)
	}
	if response.Result == nil || len(response.Result.Targets) != 1 {
		t.Fatalf("result targets = %#v, want one encoded finding", response.Result)
	}
	target := response.Result.Targets[0]
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
}

func TestBind_ValidateToolInvalidFactoryDefinitionPayloadReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	validation := &mcpDefinitionsValidationErrorFake{
		err: factorydefinitions.ErrInvalidFactoryDefinitionPayload,
	}
	operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Validation: validation})
	raw, err := operation(
		context.Background(),
		factorydefinitionmcp.ToolValidate,
		json.RawMessage(minimalValidationFactoryBody),
	)
	if err != nil {
		t.Fatalf("CallTool(validate) error = %v", err)
	}

	envelope := assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false)
	if envelope.Message != "invalid request payload" {
		t.Fatalf("error.message = %q, want invalid request payload", envelope.Message)
	}
}

func TestBind_ValidateToolValidationFailedReturnsInvalidFactoryWithTargets(t *testing.T) {
	t.Parallel()

	validation := &mcpDefinitionsValidationErrorFake{
		err: &factorydefinitions.FactoryDefinitionValidationFailure{
			Validation: factorydefinitions.ValidationResult{
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
		},
	}
	operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Validation: validation})
	raw, err := operation(
		context.Background(),
		factorydefinitionmcp.ToolValidate,
		json.RawMessage(minimalValidationFactoryBody),
	)
	if err != nil {
		t.Fatalf("CallTool(validate) error = %v", err)
	}

	envelope := assertTypedToolErrorEnvelope(t, raw, "INVALID_FACTORY", false)
	if envelope.Message != "Factory payload is not a valid Agent Factory definition." {
		t.Fatalf("error.message = %q, want invalid factory message", envelope.Message)
	}
	targets, ok := envelope.Details["targets"].([]any)
	if !ok || len(targets) != 1 {
		t.Fatalf("error.details.targets = %#v, want one encoded finding", envelope.Details["targets"])
	}
	targetMap, ok := targets[0].(map[string]any)
	if !ok {
		t.Fatalf("target = %#v, want object", targets[0])
	}
	if targetMap["code"] != "factory.validation.stub" {
		t.Fatalf("target code = %#v, want factory.validation.stub", targetMap["code"])
	}
}

func TestBind_ValidateToolOpaqueRootFailureDoesNotLeakInternalDetails(t *testing.T) {
	t.Parallel()

	const leakedPath = "/var/lib/factory/pkg/services/factory_definitions/internal/catalog"
	validation := &mcpDefinitionsValidationErrorFake{
		err: fmt.Errorf("load catalog: %s", leakedPath),
	}
	operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Validation: validation})
	raw, err := operation(
		context.Background(),
		factorydefinitionmcp.ToolValidate,
		json.RawMessage(minimalValidationFactoryBody),
	)
	if err != nil {
		t.Fatalf("CallTool(validate) error = %v", err)
	}

	envelope := assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false)
	if envelope.Message != "invalid request payload" {
		t.Fatalf("error.message = %q, want opaque invalid request payload", envelope.Message)
	}
	if strings.Contains(string(raw), leakedPath) {
		t.Fatalf("tool response leaked internal path: %s", raw)
	}
	if strings.Contains(string(raw), "factory_definitions/internal") {
		t.Fatalf("tool response leaked internal package path: %s", raw)
	}
}

func TestBind_ValidateToolMalformedJSONReturnsBadRequestWithoutInvokingRoot(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{name: "malformed", body: `{"name":"alpha"`},
		{name: "empty", body: ""},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			validation := &mcpDefinitionsValidationFake{}
			operation := factorydefinitionmcp.Bind(factorydefinitionmcp.RootBinding{Validation: validation})
			raw, err := operation(
				context.Background(),
				factorydefinitionmcp.ToolValidate,
				json.RawMessage(tc.body),
			)
			if err != nil {
				t.Fatalf("CallTool(validate) error = %v", err)
			}
			if validation.invoked {
				t.Fatal("ValidateSubmittedDefinition was invoked before request decode succeeded")
			}
			envelope := assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false)
			if !strings.Contains(envelope.Message, "decode validate input") {
				t.Fatalf("error.message = %q, want decode validate input context", envelope.Message)
			}
		})
	}
}

type mcpDefinitionsValidationErrorFake struct {
	invoked bool
	err     error
}

func (fake *mcpDefinitionsValidationErrorFake) ValidateSubmittedDefinition(
	_ context.Context,
	_ factorydefinitions.SubmittedDefinitionValidationRequest,
) (factorydefinitions.ValidationResult, error) {
	fake.invoked = true
	return factorydefinitions.ValidationResult{}, fake.err
}

var _ factorydefinitions.SubmittedDefinitionValidationOperation = (*mcpDefinitionsValidationErrorFake)(nil)

func assertTypedToolErrorEnvelope(
	t *testing.T,
	raw json.RawMessage,
	wantCode string,
	wantRetryable bool,
) *factorydefinitionmcp.ToolErrorEnvelope {
	t.Helper()

	var response struct {
		Result *json.RawMessage                          `json:"result"`
		Error  *factorydefinitionmcp.ToolErrorEnvelope `json:"error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("tool response result = %s, want error envelope only", raw)
	}
	if response.Error == nil {
		t.Fatalf("tool response = %s, want typed error envelope", raw)
	}
	if response.Error.Code != wantCode {
		t.Fatalf("error.code = %q, want %q; envelope = %#v", response.Error.Code, wantCode, response.Error)
	}
	if response.Error.Retryable != wantRetryable {
		t.Fatalf("error.retryable = %v, want %v; envelope = %#v", response.Error.Retryable, wantRetryable, response.Error)
	}
	if strings.TrimSpace(response.Error.Message) == "" {
		t.Fatalf("error.message is required; envelope = %#v", response.Error)
	}
	return response.Error
}
