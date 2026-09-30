package operatorsettingsmcp_test

import (
	"context"
	"encoding/json"
	"errors"
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

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
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

const testSubagentHome = "/home/operator"

type subagentSettingsFake struct {
	operatorsettings.Service
	path  string
	load  func(operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error)
	apply func(operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error)
	add   func(context.Context, string, operatorsettings.ACPIntegration) (operatorsettings.Document, error)
}

func (fake subagentSettingsFake) DefaultConfigPath(home string) string {
	if home != testSubagentHome {
		panic("unexpected home directory: " + home)
	}
	return fake.path
}

func (fake subagentSettingsFake) LoadDocument(request operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
	return fake.load(request)
}

func (fake subagentSettingsFake) ApplyDocumentUpdate(request operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error) {
	return fake.apply(request)
}

func (fake subagentSettingsFake) ConfigureACPIntegrationAdd(ctx context.Context, path string, integration operatorsettings.ACPIntegration) (operatorsettings.Document, error) {
	return fake.add(ctx, path, integration)
}

func TestConfigureSubagentsTool_SetsDefaultProviderAndModel(t *testing.T) {
	t.Parallel()
	var loaded, applied bool
	fake := subagentSettingsFake{
		path: testConfigPath,
		load: func(request operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
			loaded = true
			if request.Path != testConfigPath {
				t.Fatalf("LoadDocument path = %q", request.Path)
			}
			return operatorsettings.LoadDocumentResult{Document: operatorsettings.Document{BackendScopeID: testBackendScopeID}}, nil
		},
		apply: func(request operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error) {
			applied = true
			if request.Path != testConfigPath || request.ExpectedBackendScope != testBackendScopeID {
				t.Fatalf("ApplyDocumentUpdate request = %#v", request)
			}
			if request.ProviderModel.Provider == nil || *request.ProviderModel.Provider != "opencode-acp" {
				t.Fatalf("provider update = %#v", request.ProviderModel.Provider)
			}
			if request.ProviderModel.Model == nil || *request.ProviderModel.Model != "model-x" {
				t.Fatalf("model update = %#v", request.ProviderModel.Model)
			}
			return operatorsettings.ApplyDocumentUpdateResult{Path: request.Path, Persisted: true, Document: operatorsettings.Document{Defaults: operatorsettings.DocumentDefaults{WorkerModelProvider: "opencode-acp", WorkerModel: "model-x"}}}, nil
		},
	}
	raw, err := mcpoperatorsettings.ConfigureSubagentsTool(context.Background(), fake, testSubagentHome, mcpoperatorsettings.ToolSetSubagentDefaults, json.RawMessage(`{"provider":"opencode-acp","model":"model-x"}`), nil)
	if err != nil {
		t.Fatalf("ConfigureSubagentsTool() error = %v", err)
	}
	var result struct {
		Result struct {
			Path      string                    `json:"path"`
			Persisted bool                      `json:"persisted"`
			Document  operatorsettings.Document `json:"document"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !loaded || !applied || result.Result.Path != testConfigPath || !result.Result.Persisted || result.Result.Document.Defaults.WorkerModel != "model-x" {
		t.Fatalf("result = %#v; loaded=%v applied=%v", result, loaded, applied)
	}
}

func TestConfigureSubagentsTool_RejectsEmptyDefaultsAndPropagatesLoadFailure(t *testing.T) {
	t.Parallel()
	_, err := mcpoperatorsettings.ConfigureSubagentsTool(context.Background(), subagentSettingsFake{path: testConfigPath}, testSubagentHome, mcpoperatorsettings.ToolSetSubagentDefaults, json.RawMessage(`{}`), nil)
	if err == nil || err.Error() != "provider or model is required" {
		t.Fatalf("empty defaults error = %v", err)
	}
	wantErr := errors.New("load failed")
	fake := subagentSettingsFake{path: testConfigPath, load: func(operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
		return operatorsettings.LoadDocumentResult{}, wantErr
	}}
	_, err = mcpoperatorsettings.ConfigureSubagentsTool(context.Background(), fake, testSubagentHome, mcpoperatorsettings.ToolSetSubagentDefaults, json.RawMessage(`{"provider":"codex"}`), nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("load failure = %v, want %v", err, wantErr)
	}
}

func TestConfigureSubagentsTool_AddsACPProviderWithGeneratedIDAndDefaultTransport(t *testing.T) {
	t.Parallel()
	var called bool
	fake := subagentSettingsFake{
		path: testConfigPath,
		add: func(ctx context.Context, path string, integration operatorsettings.ACPIntegration) (operatorsettings.Document, error) {
			called = true
			if ctx == nil || path != testConfigPath {
				t.Fatalf("ctx=%v path=%q", ctx, path)
			}
			if integration.ID != "generated-id" || integration.Name != "Local OpenCode" || integration.Command != `C:\tools\opencode.exe acp` || integration.Transport != "stdio" {
				t.Fatalf("integration = %#v", integration)
			}
			return operatorsettings.Document{BackendScopeID: testBackendScopeID}, nil
		},
	}
	raw, err := mcpoperatorsettings.ConfigureSubagentsTool(context.Background(), fake, testSubagentHome, mcpoperatorsettings.ToolAddACPProvider, json.RawMessage(`{"name":"Local OpenCode","command":"C:\\tools\\opencode.exe acp"}`), func() string { return "generated-id" })
	if err != nil {
		t.Fatalf("ConfigureSubagentsTool() error = %v", err)
	}
	var result struct {
		Result struct {
			Path            string `json:"path"`
			Persisted       bool   `json:"persisted"`
			RequiresRestart bool   `json:"requiresRestart"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !called || result.Result.Path != testConfigPath || !result.Result.Persisted || !result.Result.RequiresRestart {
		t.Fatalf("result = %#v; called=%v", result, called)
	}
}

func TestConfigureSubagentsTool_ValidatesAndPropagatesACPFailures(t *testing.T) {
	t.Parallel()
	_, err := mcpoperatorsettings.ConfigureSubagentsTool(context.Background(), subagentSettingsFake{path: testConfigPath}, testSubagentHome, mcpoperatorsettings.ToolAddACPProvider, json.RawMessage(`{"name":"x"}`), nil)
	if err == nil || err.Error() != "ACP provider ID generator is required" {
		t.Fatalf("missing generator error = %v", err)
	}
	wantErr := errors.New("persist failed")
	fake := subagentSettingsFake{path: testConfigPath, add: func(context.Context, string, operatorsettings.ACPIntegration) (operatorsettings.Document, error) {
		return operatorsettings.Document{}, wantErr
	}}
	_, err = mcpoperatorsettings.ConfigureSubagentsTool(context.Background(), fake, testSubagentHome, mcpoperatorsettings.ToolAddACPProvider, json.RawMessage(`{"id":"provider-id","name":"x","command":"opencode acp","transport":"stdio"}`), nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ACP persist failure = %v, want %v", err, wantErr)
	}
}

func TestConfigureSubagentsTool_RejectsInvalidDependenciesAndUnknownTool(t *testing.T) {
	t.Parallel()
	if _, err := mcpoperatorsettings.ConfigureSubagentsTool(nil, nil, testSubagentHome, mcpoperatorsettings.ToolAddACPProvider, json.RawMessage(`{}`), nil); err == nil {
		t.Fatal("nil dependencies should fail")
	}
	if _, err := mcpoperatorsettings.ConfigureSubagentsTool(context.Background(), subagentSettingsFake{}, testSubagentHome, "unknown", json.RawMessage(`{}`), nil); err == nil {
		t.Fatal("unknown tool should fail")
	}
}
