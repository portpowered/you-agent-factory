package operatorsettingsmcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	mcpoperatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/mcp"
)

const (
	testResolveProvider       = "codex"
	testResolveModel          = "gpt-5"
	testInvocationProvider    = "openai"
	testInvocationModel       = "gpt-4o"
	testResolveWorkerPresetID = "preset-fast"
)

func testResolveEffectiveInputJSON() string {
	return fmt.Sprintf(
		`{"documentBaseline":{"workerModelProvider":%q,"workerModel":%q},"backendScopeId":%q,"configPath":%q,"invocationOverrides":{"workerModelProvider":%q,"workerModel":%q}}`,
		testResolveProvider,
		testResolveModel,
		testBackendScopeID,
		testConfigPath,
		testInvocationProvider,
		testInvocationModel,
	)
}

func TestBind_ResolveEffectiveSuccessReturnsSelectionFactsFromInjectedRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeSettingsRoot{
		invoked: &invoked,
		resolveEffective: func(request operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error) {
			if request.DocumentBaseline.WorkerModelProvider != testResolveProvider {
				t.Fatalf("DocumentBaseline.WorkerModelProvider = %q, want %q", request.DocumentBaseline.WorkerModelProvider, testResolveProvider)
			}
			if request.DocumentBaseline.WorkerModel != testResolveModel {
				t.Fatalf("DocumentBaseline.WorkerModel = %q, want %q", request.DocumentBaseline.WorkerModel, testResolveModel)
			}
			if request.BackendScopeID != testBackendScopeID {
				t.Fatalf("BackendScopeID = %q, want %q", request.BackendScopeID, testBackendScopeID)
			}
			if request.ConfigPath != testConfigPath {
				t.Fatalf("ConfigPath = %q, want %q", request.ConfigPath, testConfigPath)
			}
			if request.InvocationOverrides.WorkerModelProvider != testInvocationProvider {
				t.Fatalf("InvocationOverrides.WorkerModelProvider = %q, want %q", request.InvocationOverrides.WorkerModelProvider, testInvocationProvider)
			}
			if request.InvocationOverrides.WorkerModel != testInvocationModel {
				t.Fatalf("InvocationOverrides.WorkerModel = %q, want %q", request.InvocationOverrides.WorkerModel, testInvocationModel)
			}
			return operatorsettings.ResolveEffectiveResult{
				Selection: operatorsettings.EffectiveSelection{
					BackendScopeID:            testBackendScopeID,
					WorkerModelProvider:       testInvocationProvider,
					WorkerModel:               testInvocationModel,
					WorkerModelProviderSource: operatorsettings.EffectiveLayerSourceFlag,
					WorkerModelSource:         operatorsettings.EffectiveLayerSourceFlag,
					ConfigPath:                request.ConfigPath,
				},
			}, nil
		},
	}
	raw := mustCallResolveEffective(t, fake, testResolveEffectiveInputJSON())
	if !invoked {
		t.Fatal("fake settings root was not invoked")
	}
	var response mcpoperatorsettings.ToolResponse[operatorsettings.ResolveEffectiveResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("tool response = %s, want success envelope", raw)
	}
	if response.Result.Selection.WorkerModelProvider != testInvocationProvider {
		t.Fatalf("Selection.WorkerModelProvider = %q, want %q", response.Result.Selection.WorkerModelProvider, testInvocationProvider)
	}
	if response.Result.Selection.WorkerModel != testInvocationModel {
		t.Fatalf("Selection.WorkerModel = %q, want %q", response.Result.Selection.WorkerModel, testInvocationModel)
	}
	if response.Result.Selection.WorkerModelProviderSource != operatorsettings.EffectiveLayerSourceFlag {
		t.Fatalf("Selection.WorkerModelProviderSource = %q, want %q", response.Result.Selection.WorkerModelProviderSource, operatorsettings.EffectiveLayerSourceFlag)
	}
	if response.Result.Selection.ConfigPath != testConfigPath {
		t.Fatalf("Selection.ConfigPath = %q, want %q", response.Result.Selection.ConfigPath, testConfigPath)
	}
}

func TestBind_ResolveEffectiveInvalidInputReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeSettingsRoot{
		resolveEffective: func(_ operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error) {
			return operatorsettings.ResolveEffectiveResult{}, operatorsettings.ErrResolutionInvalidInput
		},
	}
	raw := mustCallResolveEffective(t, fake, testResolveEffectiveInputJSON())
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"operator_settings.resolution.invalid_input",
		false,
		"",
	)
	if envelope.Message != "operator effective resolution input is invalid" {
		t.Fatalf("error.message = %q, want invalid input message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_ResolveEffectiveUnsupportedOverrideReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeSettingsRoot{
		resolveEffective: func(_ operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error) {
			return operatorsettings.ResolveEffectiveResult{}, operatorsettings.ErrResolutionUnsupportedOverride
		},
	}
	raw := mustCallResolveEffective(t, fake, testResolveEffectiveInputJSON())
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"operator_settings.resolution.unsupported_override",
		false,
		"",
	)
	if envelope.Message != "operator effective resolution override is unsupported" {
		t.Fatalf("error.message = %q, want unsupported override message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_ResolveEffectiveConflictReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeSettingsRoot{
		resolveEffective: func(_ operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error) {
			return operatorsettings.ResolveEffectiveResult{}, operatorsettings.ErrResolutionConflict
		},
	}
	raw := mustCallResolveEffective(t, fake, testResolveEffectiveInputJSON())
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"operator_settings.resolution.conflict",
		false,
		"",
	)
	if envelope.Message != "operator effective resolution conflict" {
		t.Fatalf("error.message = %q, want conflict message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_ResolveEffectiveResolutionFailureKindsReturnTypedErrorEnvelopes(t *testing.T) {
	t.Parallel()

	invalidInputRaw := mustCallResolveEffective(t, fakeSettingsRoot{
		resolveEffective: func(_ operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error) {
			return operatorsettings.ResolveEffectiveResult{}, operatorsettings.ResolutionFailure{
				Kind:    operatorsettings.ResolutionFailureKindInvalidInput,
				Message: "symbolic DEFAULT requires a concrete provider",
				Field:   "workerModelProvider",
			}
		},
	}, testResolveEffectiveInputJSON())
	unsupportedRaw := mustCallResolveEffective(t, fakeSettingsRoot{
		resolveEffective: func(_ operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error) {
			return operatorsettings.ResolveEffectiveResult{}, operatorsettings.ResolutionFailure{
				Kind:    operatorsettings.ResolutionFailureKindUnsupportedOverride,
				Message: "unknown-provider",
				Field:   "workerModelProvider",
			}
		},
	}, testResolveEffectiveInputJSON())
	conflictRaw := mustCallResolveEffective(t, fakeSettingsRoot{
		resolveEffective: func(_ operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error) {
			return operatorsettings.ResolveEffectiveResult{}, operatorsettings.ResolutionFailure{
				Kind:    operatorsettings.ResolutionFailureKindConflict,
				Message: "document baseline mismatch",
				Field:   "documentBaseline",
			}
		},
	}, testResolveEffectiveInputJSON())

	assertTypedToolErrorEnvelopeWithField(t, invalidInputRaw, "operator_settings.resolution.invalid_input", false, "workerModelProvider")
	assertTypedToolErrorEnvelopeWithField(t, unsupportedRaw, "operator_settings.resolution.unsupported_override", false, "workerModelProvider")
	assertTypedToolErrorEnvelopeWithField(t, conflictRaw, "operator_settings.resolution.conflict", false, "documentBaseline")
}

func TestBind_ResolveEffectiveFailuresHaveDistinctTypedCodes(t *testing.T) {
	t.Parallel()

	invalidInputRaw := mustCallResolveEffective(t, fakeSettingsRoot{
		resolveEffective: func(_ operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error) {
			return operatorsettings.ResolveEffectiveResult{}, operatorsettings.ErrResolutionInvalidInput
		},
	}, testResolveEffectiveInputJSON())
	unsupportedRaw := mustCallResolveEffective(t, fakeSettingsRoot{
		resolveEffective: func(_ operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error) {
			return operatorsettings.ResolveEffectiveResult{}, operatorsettings.ErrResolutionUnsupportedOverride
		},
	}, testResolveEffectiveInputJSON())
	conflictRaw := mustCallResolveEffective(t, fakeSettingsRoot{
		resolveEffective: func(_ operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error) {
			return operatorsettings.ResolveEffectiveResult{}, operatorsettings.ErrResolutionConflict
		},
	}, testResolveEffectiveInputJSON())

	invalidInputEnvelope := assertTypedToolErrorEnvelope(t, invalidInputRaw, "operator_settings.resolution.invalid_input", false, "")
	unsupportedEnvelope := assertTypedToolErrorEnvelope(t, unsupportedRaw, "operator_settings.resolution.unsupported_override", false, "")
	conflictEnvelope := assertTypedToolErrorEnvelope(t, conflictRaw, "operator_settings.resolution.conflict", false, "")
	if invalidInputEnvelope.Code == unsupportedEnvelope.Code ||
		invalidInputEnvelope.Code == conflictEnvelope.Code ||
		unsupportedEnvelope.Code == conflictEnvelope.Code {
		t.Fatalf(
			"invalid input, unsupported override, and conflict error codes should differ: %#v vs %#v vs %#v",
			invalidInputEnvelope,
			unsupportedEnvelope,
			conflictEnvelope,
		)
	}
}

func TestBind_ResolveEffectiveInvalidJSONReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := mcpoperatorsettings.Bind(mcpoperatorsettings.RootDependencies{
		Settings: fakeSettingsRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		mcpoperatorsettings.ToolResolveEffective,
		json.RawMessage(`{"documentBaseline":`),
	)
	if err != nil {
		t.Fatalf("CallTool(resolve_effective) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false, "")
	if !strings.Contains(envelope.Message, "decode resolve effective input") {
		t.Fatalf("error.message = %q, want decode resolve effective input context", envelope.Message)
	}
	if invoked {
		t.Fatal("fake settings root was invoked for invalid JSON decode")
	}
}

func TestBind_ResolveEffectiveDoesNotInvokeDocumentMutateOperations(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeSettingsRoot{
		invoked: &invoked,
		loadDocument: func(_ operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
			t.Fatal("LoadDocument should not be invoked for resolve-effective")
			return operatorsettings.LoadDocumentResult{}, nil
		},
		applyDocumentUpdate: func(_ operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error) {
			t.Fatal("ApplyDocumentUpdate should not be invoked for resolve-effective")
			return operatorsettings.ApplyDocumentUpdateResult{}, nil
		},
		resolveEffective: func(request operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error) {
			return operatorsettings.ResolveEffectiveResult{
				Selection: operatorsettings.EffectiveSelection{
					WorkerModelProvider: request.DocumentBaseline.WorkerModelProvider,
					WorkerModel:         request.DocumentBaseline.WorkerModel,
					ConfigPath:          request.ConfigPath,
				},
			}, nil
		},
	}
	raw := mustCallResolveEffective(t, fake, testResolveEffectiveInputJSON())
	if !invoked {
		t.Fatal("fake settings root was not invoked")
	}
	var response mcpoperatorsettings.ToolResponse[operatorsettings.ResolveEffectiveResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("tool response = %s, want success envelope", raw)
	}
}

func TestResolveEffectiveErrorEnvelope_UsesResolutionFailureFieldWhenPresent(t *testing.T) {
	t.Parallel()

	failure := operatorsettings.ResolutionFailure{
		Kind:    operatorsettings.ResolutionFailureKindConflict,
		Message: "document baseline mismatch",
		Field:   "documentBaseline",
	}
	raw := mustCallResolveEffective(t, fakeSettingsRoot{
		resolveEffective: func(_ operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error) {
			return operatorsettings.ResolveEffectiveResult{}, failure
		},
	}, testResolveEffectiveInputJSON())
	envelope := assertTypedToolErrorEnvelopeWithField(
		t,
		raw,
		"operator_settings.resolution.conflict",
		false,
		"documentBaseline",
	)
	if envelope.Details["reason"] != failure.Error() {
		t.Fatalf("error.details.reason = %#v, want %q", envelope.Details["reason"], failure.Error())
	}
}

func mustCallResolveEffective(t *testing.T, fake fakeSettingsRoot, inputJSON string) json.RawMessage {
	t.Helper()

	operation := mcpoperatorsettings.Bind(mcpoperatorsettings.RootDependencies{Settings: fake})
	raw, err := operation(
		context.Background(),
		mcpoperatorsettings.ToolResolveEffective,
		json.RawMessage(inputJSON),
	)
	if err != nil {
		t.Fatalf("CallTool(resolve_effective) transport error = %v", err)
	}
	return raw
}

func assertTypedToolErrorEnvelopeWithField(
	t *testing.T,
	raw json.RawMessage,
	wantCode string,
	wantRetryable bool,
	wantField string,
) *mcpoperatorsettings.ToolErrorEnvelope {
	t.Helper()

	envelope := assertTypedToolErrorEnvelope(t, raw, wantCode, wantRetryable, "")
	if wantField != "" {
		field, ok := envelope.Details["field"].(string)
		if !ok || field != wantField {
			t.Fatalf("error.details.field = %#v, want %q; envelope = %#v", envelope.Details["field"], wantField, envelope)
		}
	}
	return envelope
}
