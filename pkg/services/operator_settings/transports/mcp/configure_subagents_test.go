package operatorsettingsmcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	mcpoperatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/mcp"
)

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
