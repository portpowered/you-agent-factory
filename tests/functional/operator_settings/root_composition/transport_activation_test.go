package root_composition_test

import (
	"encoding/json"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorsettingshttp "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/http"
	mcpoperatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/mcp"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	transportActivationProviderAlias = "codex"
	transportActivationModel         = "transport-activation-model"
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
	root, err := settingswire.NewServiceFromHomePorts(
		&operatorSettingsActivationFileSystem{recorder: recorder},
		globalconfigmapping.Decode,
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
	root, err := settingswire.NewServiceFromHomePorts(
		&operatorSettingsActivationFileSystem{recorder: recorder},
		globalconfigmapping.Decode,
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

func jsonString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
