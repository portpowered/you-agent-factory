package acp_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestPackagedSpawnRunsPlannerChildrenAndMergerThroughPersistentACPStdio(t *testing.T) {
	fixture := functionalACPFixture("spawn")
	var starts atomic.Int32
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("create operator config directory: %v", err)
	}
	config := []byte(`{"workers":{"acp":{"integrations":[{"id":"cursor-functional","name":"cursor-acp","transport":"stdio","command":"cursor-agent acp"}]}}}`)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
		t.Fatalf("write operator config: %v", err)
	}
	factoryDir := support.InstallPackagedFactory(t, homeDir, factorydefinitions.PackagedSpawnFactoryName)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env:                       packagedACPEnvironment(homeDir),
		Edges: serviceedges.Edges{
			PlatformProcessCommandFactory: acpHelperCommandFactory(&starts, fixture),
			ProvidersExecutableLocator:    availableExecutableLocator{},
			ProviderCatalogCapabilityOverrides: []providerswire.CatalogCapabilityOverride{{
				Provider: "cursor-acp",
				Capabilities: []providers.Capability{
					providers.CapabilityPromptSubmission,
					providers.CapabilityImageInput,
					providers.CapabilityStructuredOutput,
					providers.CapabilityPermissionBypass,
				},
			}},
		},
	})
	factory := support.GetJSON[factoryapi.Factory](t, server.URL()+"/factory-sessions/~default/factory")
	javascript := factory.Orchestrator.Javascript
	args := map[string]any{
		"request": "research the best places to travel", "count": 2,
		"executorProvider": "ACP", "modelProvider": "cursor-acp",
	}
	request := factoryapi.FactorySessionExecutionRequest{
		RequestId: "packaged-spawn-acp-stdio",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				Dialect: javascript.Dialect, Entrypoint: javascript.Entrypoint,
				InlineSource: *javascript.InlineSource, Metadata: javascript.Metadata,
			},
		},
		Args: &args, Orchestrator: factory.Orchestrator,
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal packaged spawn request: %v", err)
	}
	response, err := http.Post(server.URL()+"/factory-sessions/sync", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("invoke packaged spawn: %v", err)
	}
	defer response.Body.Close()
	var result factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode packaged spawn response: %v", err)
	}
	if result.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		eventsResponse, eventsErr := http.Get(server.URL() + "/factory-sessions/" + result.SessionId + "/events")
		if eventsErr == nil {
			defer eventsResponse.Body.Close()
			var eventBody bytes.Buffer
			_, _ = eventBody.ReadFrom(eventsResponse.Body)
			t.Logf("failed packaged spawn session events: %s", eventBody.String())
		}
		t.Fatalf("packaged spawn status = %q, want SUCCEEDED; result=%#v", result.Status, result)
	}
	if result.Result == nil || result.Result.PrimaryResult == nil || len(*result.Result.PrimaryResult) != 1 {
		t.Fatalf("packaged spawn primary result = %#v, want one merged text part", result.Result)
	}
	part, err := (*result.Result.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("packaged spawn primary result is not CLI-renderable text: %v", err)
	}
	if part.Text != "merged travel answer" {
		t.Fatalf("packaged spawn primary result = %q, want merged travel answer", part.Text)
	}
	if starts.Load() != 1 {
		t.Fatalf("ACP process starts = %d, want one persistent stdio peer for four agents", starts.Load())
	}
	// Stop the daemon before TempDir cleanup so the persistent ACP peer releases
	// its working-directory handle on Windows.
	server.Stop(t)
}

func packagedACPEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
}
