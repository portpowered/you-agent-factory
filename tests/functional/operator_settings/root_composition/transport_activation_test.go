package root_composition_test

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	operatorsettingshttp "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/http"
	mcpoperatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/mcp"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

const (
	transportActivationProviderAlias = "codex"
	transportActivationModel         = "transport-activation-model"
)

// TestHTTPSettingsTransportActivatesThroughRootBuildProcessAfterLifecycle proves
// the Settings-owned HTTP adapter activates after runtime lifecycle on a process
// composed only via the canonical process construction with edges.Edges effect replacement.
// CLI Settings transport is covered by
// TestOperatorConfigDocumentUpdateActivatesThroughRootBuildProcessPublicCLISurface.
func TestHTTPSettingsTransportActivatesThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()

	homeDir := writeOperatorConfigForActivation(t, transportActivationProviderAlias, transportActivationModel)
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	fixture := ensureSharedOperatorSettingsFixture(t)
	fixture.withOperatorSettingsRoute(
		t,
		"HTTP settings transport",
		homeDir,
		homeDir,
		identityActivationGeneratedUUID,
		nil,
		func(_ *operatorSettingsEffectRoute) {
			beforeTransport := fixture.router.readFileCalls.Load()
			settingsRoot := newRoutedOperatorSettingsRoot(t, fixture)
			adapter := operatorsettingshttp.NewAdapterFromRoot(operatorsettingshttp.RootBinding{Settings: settingsRoot})
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
			if got := fixture.router.readFileCalls.Load() - beforeTransport; got == 0 {
				t.Fatalf("operator-config ReadFile calls during HTTP transport = %d, want > 0 via edges", got)
			}
		},
	)
}

// TestMCPSettingsTransportActivatesThroughRootBuildProcessAfterLifecycle proves
// the Settings-owned MCP adapter activates after runtime lifecycle on a process
// composed only via the canonical process construction with edges.Edges effect replacement.
func TestMCPSettingsTransportActivatesThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()

	homeDir := writeOperatorConfigForActivation(t, transportActivationProviderAlias, transportActivationModel)
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	fixture := ensureSharedOperatorSettingsFixture(t)
	fixture.withOperatorSettingsRoute(
		t,
		"MCP settings transport",
		homeDir,
		homeDir,
		identityActivationGeneratedUUID,
		nil,
		func(_ *operatorSettingsEffectRoute) {
			beforeTransport := fixture.router.readFileCalls.Load()
			settingsRoot := newRoutedOperatorSettingsRoot(t, fixture)
			operation := mcpoperatorsettings.Bind(mcpoperatorsettings.RootDependencies{Settings: settingsRoot})
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
			if got := fixture.router.readFileCalls.Load() - beforeTransport; got == 0 {
				t.Fatalf("operator-config ReadFile calls during MCP transport = %d, want > 0 via edges", got)
			}
		},
	)
}

func newRoutedOperatorSettingsRoot(t *testing.T, fixture *sharedOperatorSettingsFixture) operatorsettings.Service {
	t.Helper()
	providersRoot, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("providerswire.NewService() error = %v", err)
	}
	root, err := settingswire.NewServiceFromHomePorts(
		fixture.router,
		globalconfigmapping.Decode,
		providersRoot,
		fixture.router.generateOperatorID,
		logging.NoopLogger{},
	)
	if err != nil {
		t.Fatalf("NewServiceFromHomePorts() error = %v", err)
	}
	return root
}

func jsonString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
