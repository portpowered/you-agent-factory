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
	testBackendScopeID = "local-00000000-0000-4000-8000-000000000030"
	testProvider       = "codex"
	testModel          = "gpt-5"
)

func testApplyDocumentUpdateInputJSON() string {
	return fmt.Sprintf(
		`{"path":%q,"expectedBackendScope":%q,"providerModel":{"provider":%q,"model":%q}}`,
		testConfigPath,
		testBackendScopeID,
		testProvider,
		testModel,
	)
}

func TestBind_ApplyDocumentUpdateSuccessReturnsPostUpdateFactsFromInjectedRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeSettingsRoot{
		invoked: &invoked,
		applyDocumentUpdate: func(request operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error) {
			if request.Path != testConfigPath {
				t.Fatalf("path = %q, want %q", request.Path, testConfigPath)
			}
			if request.ExpectedBackendScope != testBackendScopeID {
				t.Fatalf("ExpectedBackendScope = %q, want %q", request.ExpectedBackendScope, testBackendScopeID)
			}
			if request.ProviderModel.Provider == nil || *request.ProviderModel.Provider != testProvider {
				t.Fatalf("ProviderModel.Provider = %#v, want %q", request.ProviderModel.Provider, testProvider)
			}
			if request.ProviderModel.Model == nil || *request.ProviderModel.Model != testModel {
				t.Fatalf("ProviderModel.Model = %#v, want %q", request.ProviderModel.Model, testModel)
			}
			return operatorsettings.ApplyDocumentUpdateResult{
				Document: operatorsettings.Document{
					BackendScopeID: testBackendScopeID,
					Defaults: operatorsettings.DocumentDefaults{
						WorkerModelProvider: testProvider,
						WorkerModel:         testModel,
					},
				},
				Path:      request.Path,
				Persisted: true,
			}, nil
		},
	}
	raw := mustCallApplyDocumentUpdate(t, fake, testApplyDocumentUpdateInputJSON())
	if !invoked {
		t.Fatal("fake settings root was not invoked")
	}
	var response mcpoperatorsettings.ToolResponse[operatorsettings.ApplyDocumentUpdateResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("tool response = %s, want success envelope", raw)
	}
	if response.Result.Path != testConfigPath {
		t.Fatalf("Path = %q, want %q", response.Result.Path, testConfigPath)
	}
	if !response.Result.Persisted {
		t.Fatal("Persisted = false, want true")
	}
	if response.Result.Document.BackendScopeID != testBackendScopeID {
		t.Fatalf("BackendScopeID = %q, want %q", response.Result.Document.BackendScopeID, testBackendScopeID)
	}
	if response.Result.Document.Defaults.WorkerModelProvider != testProvider ||
		response.Result.Document.Defaults.WorkerModel != testModel {
		t.Fatalf("Document.Defaults = %#v, want provider %q and model %q", response.Result.Document.Defaults, testProvider, testModel)
	}
}

func TestBind_ApplyDocumentUpdateMalformedReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeSettingsRoot{
		applyDocumentUpdate: func(_ operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error) {
			return operatorsettings.ApplyDocumentUpdateResult{}, operatorsettings.ErrDocumentMalformed
		},
	}
	raw := mustCallApplyDocumentUpdate(t, fake, testApplyDocumentUpdateInputJSON())
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"operator_settings.document.malformed",
		false,
		testConfigPath,
	)
	if envelope.Message != "operator document is malformed" {
		t.Fatalf("error.message = %q, want malformed document message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_ApplyDocumentUpdateUnsupportedReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeSettingsRoot{
		applyDocumentUpdate: func(_ operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error) {
			return operatorsettings.ApplyDocumentUpdateResult{}, operatorsettings.ErrDocumentUnsupported
		},
	}
	raw := mustCallApplyDocumentUpdate(t, fake, testApplyDocumentUpdateInputJSON())
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"operator_settings.document.unsupported",
		false,
		testConfigPath,
	)
	if envelope.Message != "operator document update is unsupported" {
		t.Fatalf("error.message = %q, want unsupported document message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_ApplyDocumentUpdateConflictReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeSettingsRoot{
		applyDocumentUpdate: func(_ operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error) {
			return operatorsettings.ApplyDocumentUpdateResult{}, operatorsettings.ErrDocumentConflict
		},
	}
	raw := mustCallApplyDocumentUpdate(t, fake, testApplyDocumentUpdateInputJSON())
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"operator_settings.document.conflict",
		false,
		testConfigPath,
	)
	if envelope.Message != "operator document persist conflict" {
		t.Fatalf("error.message = %q, want conflict document message; envelope = %#v", envelope.Message, envelope)
	}
}

func TestBind_ApplyDocumentUpdateDocumentFailureKindsReturnTypedErrorEnvelopes(t *testing.T) {
	t.Parallel()

	malformedRaw := mustCallApplyDocumentUpdate(t, fakeSettingsRoot{
		applyDocumentUpdate: func(_ operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error) {
			return operatorsettings.ApplyDocumentUpdateResult{}, operatorsettings.DocumentFailure{
				Kind:    operatorsettings.DocumentFailureKindMalformed,
				Message: "at least one provider/model field is required",
				Path:    testConfigPath,
			}
		},
	}, testApplyDocumentUpdateInputJSON())
	unsupportedRaw := mustCallApplyDocumentUpdate(t, fakeSettingsRoot{
		applyDocumentUpdate: func(_ operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error) {
			return operatorsettings.ApplyDocumentUpdateResult{}, operatorsettings.DocumentFailure{
				Kind:    operatorsettings.DocumentFailureKindUnsupported,
				Message: "provider not supported",
				Path:    testConfigPath,
			}
		},
	}, testApplyDocumentUpdateInputJSON())
	conflictRaw := mustCallApplyDocumentUpdate(t, fakeSettingsRoot{
		applyDocumentUpdate: func(_ operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error) {
			return operatorsettings.ApplyDocumentUpdateResult{}, operatorsettings.DocumentFailure{
				Kind:    operatorsettings.DocumentFailureKindConflict,
				Message: "backend scope mismatch",
				Path:    testConfigPath,
			}
		},
	}, testApplyDocumentUpdateInputJSON())

	assertTypedToolErrorEnvelope(t, malformedRaw, "operator_settings.document.malformed", false, testConfigPath)
	assertTypedToolErrorEnvelope(t, unsupportedRaw, "operator_settings.document.unsupported", false, testConfigPath)
	assertTypedToolErrorEnvelope(t, conflictRaw, "operator_settings.document.conflict", false, testConfigPath)
}

func TestBind_ApplyDocumentUpdateFailuresHaveDistinctTypedCodes(t *testing.T) {
	t.Parallel()

	malformedRaw := mustCallApplyDocumentUpdate(t, fakeSettingsRoot{
		applyDocumentUpdate: func(_ operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error) {
			return operatorsettings.ApplyDocumentUpdateResult{}, operatorsettings.ErrDocumentMalformed
		},
	}, testApplyDocumentUpdateInputJSON())
	unsupportedRaw := mustCallApplyDocumentUpdate(t, fakeSettingsRoot{
		applyDocumentUpdate: func(_ operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error) {
			return operatorsettings.ApplyDocumentUpdateResult{}, operatorsettings.ErrDocumentUnsupported
		},
	}, testApplyDocumentUpdateInputJSON())
	conflictRaw := mustCallApplyDocumentUpdate(t, fakeSettingsRoot{
		applyDocumentUpdate: func(_ operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error) {
			return operatorsettings.ApplyDocumentUpdateResult{}, operatorsettings.ErrDocumentConflict
		},
	}, testApplyDocumentUpdateInputJSON())

	malformedEnvelope := assertTypedToolErrorEnvelope(t, malformedRaw, "operator_settings.document.malformed", false, testConfigPath)
	unsupportedEnvelope := assertTypedToolErrorEnvelope(t, unsupportedRaw, "operator_settings.document.unsupported", false, testConfigPath)
	conflictEnvelope := assertTypedToolErrorEnvelope(t, conflictRaw, "operator_settings.document.conflict", false, testConfigPath)
	if malformedEnvelope.Code == unsupportedEnvelope.Code ||
		malformedEnvelope.Code == conflictEnvelope.Code ||
		unsupportedEnvelope.Code == conflictEnvelope.Code {
		t.Fatalf(
			"malformed, unsupported, and conflict error codes should differ: %#v vs %#v vs %#v",
			malformedEnvelope,
			unsupportedEnvelope,
			conflictEnvelope,
		)
	}
}

func TestBind_ApplyDocumentUpdateInvalidJSONReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := mcpoperatorsettings.Bind(mcpoperatorsettings.RootDependencies{
		Settings: fakeSettingsRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		mcpoperatorsettings.ToolApplyDocumentUpdate,
		json.RawMessage(`{"path":`),
	)
	if err != nil {
		t.Fatalf("CallTool(apply_document_update) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false, "")
	if !strings.Contains(envelope.Message, "decode apply document update input") {
		t.Fatalf("error.message = %q, want decode apply document update input context", envelope.Message)
	}
	if invoked {
		t.Fatal("fake settings root was invoked for invalid JSON decode")
	}
}

func TestApplyDocumentUpdateErrorEnvelope_UsesDocumentFailurePathWhenPresent(t *testing.T) {
	t.Parallel()

	failure := operatorsettings.DocumentFailure{
		Kind: operatorsettings.DocumentFailureKindConflict,
		Path: "/custom/path.json",
	}
	raw := mustCallApplyDocumentUpdate(t, fakeSettingsRoot{
		applyDocumentUpdate: func(_ operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error) {
			return operatorsettings.ApplyDocumentUpdateResult{}, failure
		},
	}, testApplyDocumentUpdateInputJSON())
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"operator_settings.document.conflict",
		false,
		"/custom/path.json",
	)
	if envelope.Details["reason"] != failure.Error() {
		t.Fatalf("error.details.reason = %#v, want %q", envelope.Details["reason"], failure.Error())
	}
}

func mustCallApplyDocumentUpdate(t *testing.T, fake fakeSettingsRoot, inputJSON string) json.RawMessage {
	t.Helper()

	operation := mcpoperatorsettings.Bind(mcpoperatorsettings.RootDependencies{Settings: fake})
	raw, err := operation(
		context.Background(),
		mcpoperatorsettings.ToolApplyDocumentUpdate,
		json.RawMessage(inputJSON),
	)
	if err != nil {
		t.Fatalf("CallTool(apply_document_update) transport error = %v", err)
	}
	return raw
}
