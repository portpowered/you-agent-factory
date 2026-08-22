package root_composition_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	operatorsettingshttp "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/http"
	mcpoperatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/mcp"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	transportActivationProviderAlias = "codex"
	transportActivationModel         = "transport-activation-model"
	mcpActivationBackendScope        = "local-00000000-0000-4000-8000-000000000042"
)

// TestHTTPSettingsTransportActivatesThroughRootBuildProcessAfterLifecycle proves
// the Settings-owned HTTP adapter activates after runtime lifecycle on a process
// composed only via root.BuildProcess with edges.Edges effect replacement.
// CLI Settings transport is covered by
// TestOperatorConfigDocumentUpdateActivatesThroughRootBuildProcessPublicCLISurface.
func TestHTTPSettingsTransportActivatesThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()

	homeDir := writeOperatorConfigForActivation(t, transportActivationProviderAlias, transportActivationModel)
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	recorder := newOperatorSettingsActivationRecorder()
	process := support.BuildProcess(t, recorder.edges())

	if got := recorder.fileSystemCalls(); got != 0 {
		t.Fatalf("operator-config filesystem effect calls = %d during BuildProcess, want 0", got)
	}

	runOperatorSettingsLifecycleInitialization(t, process, homeDir)

	beforeTransport := recorder.readFileCalls()
	providersRoot, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("providerswire.NewService() error = %v", err)
	}
	root, err := settingswire.NewServiceFromHomePorts(
		&operatorSettingsActivationFileSystem{recorder: recorder},
		globalconfigmapping.Decode,
		providersRoot,
		func() string { return "00000000-0000-4000-8000-000000000001" },
		logging.NoopLogger{},
	)
	if err != nil {
		t.Fatalf("NewServiceFromHomePorts() error = %v", err)
	}

	adapter := operatorsettingshttp.NewAdapterFromRoot(operatorsettingshttp.RootBinding{Settings: root})
	response, err := adapter.LoadDocument(t.Context(), operatorsettingshttp.LoadDocumentInput{
		Path:            configPath,
		RequireExisting: true,
	})
	if err != nil {
		t.Fatalf("HTTP LoadDocument() error = %v", err)
	}
	if !response.Found {
		t.Fatalf("HTTP LoadDocument() found = false, want true for %q", configPath)
	}
	if response.Path != configPath {
		t.Fatalf("HTTP LoadDocument() path = %q, want %q", response.Path, configPath)
	}
	if response.Document.Defaults == nil ||
		response.Document.Defaults.WorkerModel == nil ||
		*response.Document.Defaults.WorkerModel != transportActivationModel {
		t.Fatalf("HTTP LoadDocument() defaults = %#v, want model %q", response.Document.Defaults, transportActivationModel)
	}
	if got := recorder.readFileCalls() - beforeTransport; got == 0 {
		t.Fatalf("operator-config ReadFile calls during HTTP transport = %d, want > 0 via edges", got)
	}
}

// TestMCPSettingsTransportActivatesThroughRootBuildProcessAfterLifecycle proves
// the Settings-owned MCP adapter activates after runtime lifecycle on a process
// composed only via root.BuildProcess with edges.Edges effect replacement.
func TestMCPSettingsTransportActivatesThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()

	homeDir := writeOperatorConfigForActivation(t, transportActivationProviderAlias, transportActivationModel)
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	recorder := newOperatorSettingsActivationRecorder()
	process := support.BuildProcess(t, recorder.edges())

	if got := recorder.fileSystemCalls(); got != 0 {
		t.Fatalf("operator-config filesystem effect calls = %d during BuildProcess, want 0", got)
	}

	runOperatorSettingsLifecycleInitialization(t, process, homeDir)

	beforeTransport := recorder.readFileCalls()
	providersRoot, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("providerswire.NewService() error = %v", err)
	}
	root, err := settingswire.NewServiceFromHomePorts(
		&operatorSettingsActivationFileSystem{recorder: recorder},
		globalconfigmapping.Decode,
		providersRoot,
		func() string { return "00000000-0000-4000-8000-000000000001" },
		logging.NoopLogger{},
	)
	if err != nil {
		t.Fatalf("NewServiceFromHomePorts() error = %v", err)
	}

	operation := mcpoperatorsettings.Bind(mcpoperatorsettings.RootDependencies{Settings: root})
	raw, err := operation(
		t.Context(),
		mcpoperatorsettings.ToolLoadDocument,
		json.RawMessage(`{"path":`+jsonString(configPath)+`,"requireExisting":true}`),
	)
	if err != nil {
		t.Fatalf("MCP CallTool(load_document) transport error = %v", err)
	}

	var response mcpoperatorsettings.ToolResponse[operatorsettings.LoadDocumentResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode MCP tool response: %v\nraw=%s", err, raw)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("MCP CallTool(load_document) = %s, want success", raw)
	}
	if !response.Result.Found {
		t.Fatalf("MCP load_document found = false, want true for %q", configPath)
	}
	if response.Result.Document.Defaults.WorkerModel != transportActivationModel {
		t.Fatalf(
			"MCP load_document model = %q, want %q",
			response.Result.Document.Defaults.WorkerModel,
			transportActivationModel,
		)
	}
	if got := recorder.readFileCalls() - beforeTransport; got == 0 {
		t.Fatalf("operator-config ReadFile calls during MCP transport = %d, want > 0 via edges", got)
	}
}

// TestMCPSettingsTransportManagesDocumentAndEffectiveSelectionAfterLifecycle
// proves the published Operator Settings MCP binding carries a real document
// through load, guarded persistence, and effective-resolution operations on a
// root composed with the same public Wire ports used by the application.
func TestMCPSettingsTransportManagesDocumentAndEffectiveSelectionAfterLifecycle(t *testing.T) {
	t.Parallel()

	homeDir := writeScopedOperatorConfigForMCP(t)
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	recorder := newOperatorSettingsActivationRecorder()
	process := support.BuildProcess(t, recorder.edges())
	runOperatorSettingsLifecycleInitialization(t, process, homeDir)

	settingsRoot := newFullOperatorSettingsRoot(t, recorder)
	operation := mcpoperatorsettings.Bind(mcpoperatorsettings.RootDependencies{Settings: settingsRoot})

	loaded := loadMCPDocument(t, operation, configPath)
	assertInitialMCPDocument(t, loaded, configPath)

	updated := applyMCPDocumentUpdate(t, operation, configPath, loaded.Document.BackendScopeID)
	assertUpdatedMCPDocument(t, updated, loaded.Document, configPath)
	if reloaded := loadMCPDocument(t, operation, configPath); reloaded.Document.Defaults != updated.Document.Defaults {
		t.Fatalf("reloaded defaults = %#v, want accepted update %#v", reloaded.Document.Defaults, updated.Document.Defaults)
	}

	resolved := resolveMCPSelection(t, operation, configPath, updated.Document)
	assertMCPSelection(t, resolved.Selection, updated.Document)
	if reloaded := loadMCPDocument(t, operation, configPath); reloaded.Document.Defaults != updated.Document.Defaults {
		t.Fatalf("document changed during effective resolution: got %#v, want %#v", reloaded.Document.Defaults, updated.Document.Defaults)
	}

	assertStaleMCPUpdateDoesNotMutate(t, operation, configPath, updated.Document)
	assertMCPBoundaryFailures(t, operation, configPath)
}

func loadMCPDocument(
	t *testing.T,
	operation mcpoperatorsettings.ToolOperation,
	configPath string,
) operatorsettings.LoadDocumentResult {
	t.Helper()
	return decodeMCPResult[operatorsettings.LoadDocumentResult](t, callMCPTool(
		t,
		operation,
		mcpoperatorsettings.ToolLoadDocument,
		map[string]any{"path": configPath, "requireExisting": true},
	))
}

func assertInitialMCPDocument(
	t *testing.T,
	loaded operatorsettings.LoadDocumentResult,
	configPath string,
) {
	t.Helper()
	if !loaded.Found || loaded.Path != configPath {
		t.Fatalf("load result = %#v, want existing document at %q", loaded, configPath)
	}
	if !operatorsettings.IsLocalBackendScopeID(loaded.Document.BackendScopeID) {
		t.Fatalf("loaded backend scope = %q, want local scope", loaded.Document.BackendScopeID)
	}
	if loaded.Document.Defaults.WorkerModelProvider != transportActivationProviderAlias ||
		loaded.Document.Defaults.WorkerModel != transportActivationModel {
		t.Fatalf("loaded defaults = %#v, want %q/%q", loaded.Document.Defaults, transportActivationProviderAlias, transportActivationModel)
	}
}

func applyMCPDocumentUpdate(
	t *testing.T,
	operation mcpoperatorsettings.ToolOperation,
	configPath string,
	expectedScope string,
) operatorsettings.ApplyDocumentUpdateResult {
	t.Helper()
	return decodeMCPResult[operatorsettings.ApplyDocumentUpdateResult](t, callMCPTool(
		t,
		operation,
		mcpoperatorsettings.ToolApplyDocumentUpdate,
		map[string]any{
			"path":                 configPath,
			"expectedBackendScope": expectedScope,
			"providerModel": map[string]any{
				"provider": "claude",
				"model":    "mcp-updated-model",
			},
		},
	))
}

func assertUpdatedMCPDocument(
	t *testing.T,
	updated operatorsettings.ApplyDocumentUpdateResult,
	loaded operatorsettings.Document,
	configPath string,
) {
	t.Helper()
	if !updated.Persisted || updated.Path != configPath {
		t.Fatalf("update result = %#v, want persisted document at %q", updated, configPath)
	}
	if updated.Document.BackendScopeID != loaded.BackendScopeID ||
		updated.Document.Defaults.WorkerModelProvider != string(providers.IDClaude) ||
		updated.Document.Defaults.WorkerModel != "mcp-updated-model" {
		t.Fatalf("updated document = %#v, want preserved scope and %q/mcp-updated-model", updated.Document, providers.IDClaude)
	}
}

func resolveMCPSelection(
	t *testing.T,
	operation mcpoperatorsettings.ToolOperation,
	configPath string,
	document operatorsettings.Document,
) operatorsettings.ResolveEffectiveResult {
	t.Helper()
	defaults := map[string]any{
		"workerModelProvider": document.Defaults.WorkerModelProvider,
		"workerModel":         document.Defaults.WorkerModel,
	}
	return decodeMCPResult[operatorsettings.ResolveEffectiveResult](t, callMCPTool(
		t,
		operation,
		mcpoperatorsettings.ToolResolveEffective,
		map[string]any{
			"documentBaseline":         defaults,
			"expectedDocumentBaseline": defaults,
			"backendScopeId":           document.BackendScopeID,
			"workerPresets": []map[string]any{{
				"id":              "fast",
				"modelProvider":   "codex",
				"model":           "preset-model",
				"reasoningEffort": "low",
			}},
			"environmentOverrides": map[string]any{
				"workerModelProvider": "openai",
				"workerModel":         "environment-model",
			},
			"invocationOverrides": map[string]any{"workerPresetId": "fast"},
			"configPath":          configPath,
		},
	))
}

func assertMCPSelection(
	t *testing.T,
	selection operatorsettings.EffectiveSelection,
	document operatorsettings.Document,
) {
	t.Helper()
	if selection.BackendScopeID != document.BackendScopeID ||
		selection.WorkerModelProvider != strings.ToUpper(string(providers.IDCodex)) ||
		selection.WorkerModel != "preset-model" {
		t.Fatalf("effective selection = %#v, want preset provider/model and accepted scope", selection)
	}
	if selection.WorkerModelProviderSource != operatorsettings.EffectiveLayerSourceFlag ||
		selection.WorkerModelSource != operatorsettings.EffectiveLayerSourceFlag ||
		len(selection.WorkerPresets) != 1 || selection.WorkerPresets[0].ID != "fast" {
		t.Fatalf("effective precedence facts = %#v, want flag sources and fast preset", selection)
	}
}

func assertStaleMCPUpdateDoesNotMutate(
	t *testing.T,
	operation mcpoperatorsettings.ToolOperation,
	configPath string,
	want operatorsettings.Document,
) {
	t.Helper()
	staleRaw := callMCPTool(t, operation, mcpoperatorsettings.ToolApplyDocumentUpdate, map[string]any{
		"path":                 configPath,
		"expectedBackendScope": "local-00000000-0000-4000-8000-000000000099",
		"providerModel": map[string]any{
			"provider": "gemini",
			"model":    "must-not-persist",
		},
	})
	assertMCPError(t, staleRaw, "operator_settings.document.conflict", "operator document persist conflict", false, configPath)
	unchanged := loadMCPDocument(t, operation, configPath)
	if unchanged.Document.Defaults != want.Defaults {
		t.Fatalf("stale update changed document: got %#v, want %#v", unchanged.Document.Defaults, want.Defaults)
	}
}

func assertMCPBoundaryFailures(
	t *testing.T,
	operation mcpoperatorsettings.ToolOperation,
	configPath string,
) {
	t.Helper()
	malformedRaw, err := operation(t.Context(), mcpoperatorsettings.ToolResolveEffective, json.RawMessage(`{"documentBaseline":`))
	if err != nil {
		t.Fatalf("MCP malformed resolve request transport error = %v", err)
	}
	assertMCPError(t, malformedRaw, "BAD_REQUEST", "decode resolve effective input", false, "")

	canceledContext, cancel := context.WithCancel(t.Context())
	cancel()
	canceledRaw, err := operation(canceledContext, mcpoperatorsettings.ToolLoadDocument, json.RawMessage(`{"path":`+jsonString(configPath)+`}`))
	if err != nil {
		t.Fatalf("MCP canceled load request transport error = %v", err)
	}
	assertMCPError(t, canceledRaw, "operator_settings.request.canceled", "operator settings request was canceled", false, "")
}

func newFullOperatorSettingsRoot(t *testing.T, recorder *operatorSettingsActivationRecorder) operatorsettings.Service {
	t.Helper()

	providersRoot, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("providerswire.NewService() error = %v", err)
	}
	providerCatalog := func(value string) (string, bool) {
		resolved, resolveErr := providersRoot.ResolveIdentity(
			context.Background(),
			providers.ResolveIdentityRequest{Identity: value},
		)
		if resolveErr != nil {
			return "", false
		}
		return resolved.ID.String(), true
	}
	root, err := settingswire.NewServiceFromConfigDocument(
		operatorsettings.ConfigDocumentService{
			Files:                 &operatorSettingsActivationFileSystem{recorder: recorder},
			CreateTemp:            recorder.createTemporaryFile,
			Providers:             providerCatalog,
			Decoder:               globalconfigmapping.Decode,
			Encoder:               globalconfigmapping.Encode,
			PreserveUnknownFields: globalconfigmapping.PreserveUnknownFields,
		},
		providersRoot,
		func() string { return "00000000-0000-4000-8000-000000000001" },
		logging.NoopLogger{},
	)
	if err != nil {
		t.Fatalf("NewServiceFromConfigDocument() error = %v", err)
	}
	return root
}

func callMCPTool(t *testing.T, operation mcpoperatorsettings.ToolOperation, name string, input any) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal MCP %s input: %v", name, err)
	}
	raw, err := operation(t.Context(), name, payload)
	if err != nil {
		t.Fatalf("MCP %s transport error = %v", name, err)
	}
	return raw
}

func decodeMCPResult[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var response mcpoperatorsettings.ToolResponse[T]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode MCP response: %v\nraw=%s", err, raw)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("MCP response = %s, want success result", raw)
	}
	return *response.Result
}

func assertMCPError(t *testing.T, raw json.RawMessage, code, message string, retryable bool, path string) {
	t.Helper()
	var response struct {
		Result *json.RawMessage                       `json:"result"`
		Error  *mcpoperatorsettings.ToolErrorEnvelope `json:"error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode MCP error response: %v\nraw=%s", err, raw)
	}
	if response.Result != nil || response.Error == nil {
		t.Fatalf("MCP response = %s, want error-only envelope", raw)
	}
	if response.Error.Code != code || !strings.HasPrefix(response.Error.Message, message) || response.Error.Retryable != retryable {
		t.Fatalf("MCP error = %#v, want code=%q message=%q retryable=%v", response.Error, code, message, retryable)
	}
	if path != "" {
		if got, ok := response.Error.Details["path"].(string); !ok || got != path {
			t.Fatalf("MCP error details.path = %#v, want %q", response.Error.Details["path"], path)
		}
	}
}

func writeScopedOperatorConfigForMCP(t *testing.T) string {
	t.Helper()

	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir MCP operator config directory: %v", err)
	}
	config := []byte(`{
  "backendScopeID": "` + mcpActivationBackendScope + `",
  "defaults": {
    "workerModelProvider": "` + transportActivationProviderAlias + `",
    "workerModel": "` + transportActivationModel + `"
  }
}`)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
		t.Fatalf("write MCP operator config: %v", err)
	}
	return homeDir
}

func jsonString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
