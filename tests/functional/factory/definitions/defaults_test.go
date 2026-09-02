package definitions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const globalDefaultsPresetWorkflow = `return (async function () {
  const defaults = await agent.run({prompt: "use operator defaults", label: "defaults"});
  const preset = await agent.run({prompt: "use configured preset", label: "preset", preset: "careful-review"});
  return {defaults, preset};
})();`

const (
	globalDefaultsProviderAlias = "codex"
	globalDefaultsModel         = "operator-default-model"
	factoryOverrideProvider     = modelprovider.ProviderClaude
	factoryOverrideModel        = "factory-authored-model"
)

// TestGlobalConfigSuppliesDefaultProviderAndModel proves workers that omit
// provider and model inherit operator global defaults at run time and dispatch
// through the resolved provider-process edge with the configured default model.
func TestGlobalConfigSuppliesDefaultProviderAndModel(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldFactory(t, defaultsFactoryConfig())
	support.WriteAgentConfig(t, dir, "worker-a", `---
type: MODEL_WORKER
stopToken: COMPLETE
---
Process the input task.
`)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"global defaults supply provider and model"}`))

	homeDir := writeOperatorGlobalDefaultsConfig(t, globalDefaultsProviderAlias, globalDefaultsModel)
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout("Done. COMPLETE"),
	})

	_, listed := runFactoryWithOperatorHome(
		t,
		dir,
		homeDir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		15*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:complete"); got != 1 {
		t.Fatalf("completed work tokens = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work tokens = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", runner.CallCount())
	}

	call := runner.LastRequest()
	if call.Command != string(modelprovider.ProviderCodex) {
		t.Fatalf("command = %q, want global default provider %q", call.Command, modelprovider.ProviderCodex)
	}
	support.AssertArgsContainSequence(t, call.Args, []string{"--model", globalDefaultsModel})
}

// TestExplicitFactoryConfigOverridesGlobalDefaults proves Factory-authored
// provider and model on a worker win over operator global defaults at run time
// and dispatch through the resolved provider-process edge with the authored
// model selection.
func TestExplicitFactoryConfigOverridesGlobalDefaults(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldFactory(t, defaultsFactoryConfig())
	support.WriteAgentConfig(
		t,
		dir,
		"worker-a",
		support.BuildModelWorkerConfig(factoryOverrideProvider, factoryOverrideModel),
	)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"explicit factory config overrides global defaults"}`))

	homeDir := writeOperatorGlobalDefaultsConfig(t, globalDefaultsProviderAlias, globalDefaultsModel)
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.ClaudeSuccessStdout("Done. COMPLETE"),
	})

	_, listed := runFactoryWithOperatorHome(
		t,
		dir,
		homeDir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		15*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:complete"); got != 1 {
		t.Fatalf("completed work tokens = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work tokens = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", runner.CallCount())
	}

	call := runner.LastRequest()
	if call.Command != string(factoryOverrideProvider) {
		t.Fatalf(
			"command = %q, want factory-authored provider %q (not global default %q)",
			call.Command,
			factoryOverrideProvider,
			modelprovider.ProviderCodex,
		)
	}
	support.AssertArgsContainSequence(t, call.Args, []string{"--model", factoryOverrideModel})
}

// TestOperatorGlobalDefaultsAndWorkerPresetResolveAtProviderEdge proves
// operator global defaults and named worker presets supply observable
// provider/model selection at the public process and provider-edge boundary
// through a JavaScript workflow executed against a root-built API server.
func TestOperatorGlobalDefaultsAndWorkerPresetResolveAtProviderEdge(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldFactory(t, defaultsFactoryConfig())
	support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\n---\n")
	homeDir := writeOperatorGlobalDefaultsAndPresetsConfig(t)

	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("defaults complete")},
		platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("preset complete")},
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
		Env:                       defaultsOperatorEnvironment(homeDir),
		BeforeStart: func(tb testing.TB, process support.Process, inputs root.Input) {
			// Complete the scenario-owned customer bootstrap before starting the
			// hosted readiness clock. The behavior under test is defaults/preset
			// selection, not first-run packaged Factory installation, and a loaded
			// race run must not turn that unrelated blocking setup into a server
			// readiness failure.
			support.InitializeCustomerHomeWithProcess(
				tb,
				process,
				inputs.Env,
				inputs.WorkingDirectory,
			)
		},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startDefaultsPresetWorkflow(t, server.URL())
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", started.Status)
	}
	if runner.CallCount() != 2 {
		t.Fatalf("provider command runner calls = %d, want 2", runner.CallCount())
	}

	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+started.SessionId+"/dispatches",
	)
	assertGlobalDefaultsPresetDispatches(t, dispatches.Dispatches)
	assertGlobalDefaultsPresetProviderCommands(t, runner.Requests())
}

// TestSingleDiscoveredProviderIsUsedWhenNoDefaultExists proves that when no
// operator or Factory default provider is configured and exactly one supported
// provider is discoverable, model-backed Work dispatches through that provider.
func TestSingleDiscoveredProviderIsUsedWhenNoDefaultExists(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldFactory(t, defaultsFactoryConfig())
	support.WriteAgentConfig(t, dir, "worker-a", `---
type: MODEL_WORKER
stopToken: COMPLETE
---
Process the input task.
`)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"single discovered provider when no default exists"}`))

	homeDir := t.TempDir()
	discoveredProvider := string(modelprovider.ProviderCodex)
	codexPath := writeFixtureExecutable(t, discoveredProvider)
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout("Done. COMPLETE"),
	})

	_, listed := runFactoryWithOperatorHome(
		t,
		dir,
		homeDir,
		serviceedges.Edges{
			ProviderCommandRunner: runner,
			WorkersExecutableLocator: singleCommandExecutableLocator{
				command: discoveredProvider,
				path:    codexPath,
			},
		},
		15*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:complete"); got != 1 {
		t.Fatalf("completed work tokens = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work tokens = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", runner.CallCount())
	}

	call := runner.LastRequest()
	if call.Command != discoveredProvider {
		t.Fatalf("command = %q, want discovered provider %q", call.Command, discoveredProvider)
	}
}

func defaultsFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func writeOperatorGlobalDefaultsAndPresetsConfig(t *testing.T) string {
	t.Helper()

	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir global config directory: %v", err)
	}
	config := []byte(`{
  "defaults": {"workerModelProvider": "openai", "workerModel": "default-model"},
  "workerPresets": [{
    "id": "careful-review",
    "modelProvider": "codex",
    "model": "preset-model",
    "reasoningEffort": "medium"
  }]
}`)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	return homeDir
}

func startDefaultsPresetWorkflow(
	t *testing.T,
	serverURL string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	dialect := "you-workflow-v1"
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "global-config-runtime-selection",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				Dialect: &dialect,
				InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
					Inline:   globalDefaultsPresetWorkflow,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal global defaults preset workflow request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build global defaults preset workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start global defaults preset workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start global defaults preset workflow status = %d: %s", response.StatusCode, body.String())
	}
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode global defaults preset workflow response: %v", err)
	}
	return started
}

func assertGlobalDefaultsPresetDispatches(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
) {
	t.Helper()

	if len(dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want 2", len(dispatches))
	}
	assertGlobalDefaultsPresetDispatch(t, dispatches[0], "", "codex", "default-model", "")
	assertGlobalDefaultsPresetDispatch(t, dispatches[1], "careful-review", "codex", "preset-model", "medium")
}

func assertGlobalDefaultsPresetDispatch(
	t *testing.T,
	dispatch factoryapi.FactorySessionDispatchSummary,
	wantPreset, wantProvider, wantModel, wantEffort string,
) {
	t.Helper()

	gotPreset := dereferenceDefaultsValue(dispatch.PresetId)
	gotProvider := dereferenceDefaultsValue(dispatch.ModelProvider)
	gotModel := dereferenceDefaultsValue(dispatch.Model)
	gotEffort := dereferenceDefaultsValue(dispatch.ReasoningEffort)
	if gotPreset != wantPreset || gotProvider != wantProvider || gotModel != wantModel || gotEffort != wantEffort {
		t.Fatalf(
			"dispatch selection = preset=%q provider=%q model=%q effort=%q, want preset=%q provider=%q model=%q effort=%q",
			gotPreset, gotProvider, gotModel, gotEffort,
			wantPreset, wantProvider, wantModel, wantEffort,
		)
	}
}

func assertGlobalDefaultsPresetProviderCommands(
	t *testing.T,
	requests []platformprocess.CommandRequest,
) {
	t.Helper()

	if len(requests) != 2 {
		t.Fatalf("provider command count = %d, want 2", len(requests))
	}
	if requests[0].Command != string(modelprovider.ProviderCodex) {
		t.Fatalf(
			"default provider command = %q, want %q",
			requests[0].Command,
			modelprovider.ProviderCodex,
		)
	}
	support.AssertArgsContainSequence(t, requests[0].Args, []string{"--model", "default-model"})
	if requests[1].Command != string(modelprovider.ProviderCodex) {
		t.Fatalf(
			"preset provider command = %q, want %q",
			requests[1].Command,
			modelprovider.ProviderCodex,
		)
	}
	support.AssertArgsContainSequence(t, requests[1].Args, []string{"--model", "preset-model"})
}

func dereferenceDefaultsValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func writeOperatorGlobalDefaultsConfig(t *testing.T, providerAlias, model string) string {
	t.Helper()

	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir global config directory: %v", err)
	}
	config := []byte(`{
  "defaults": {
    "workerModelProvider": "` + providerAlias + `",
    "workerModel": "` + model + `"
  }
}`)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	return homeDir
}

func runFactoryWithOperatorHome(
	t *testing.T,
	dir string,
	homeDir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse) {
	t.Helper()

	server := support.NewProcessAPIServer()
	overrides.APIServerStarter = server.Start
	process := support.BuildProcess(t, overrides)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", dir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	inputs.Input.Env = defaultsOperatorEnvironment(homeDir)
	inputs.Input.WorkingDirectory = dir
	daemon := support.StartProcessCommand(t, process, inputs.Input)
	baseURL := server.WaitForURL(t)
	support.WaitForTerminalStatus(t, baseURL, timeout)

	session := support.GetDefaultSession(t, baseURL)
	work := support.ListDefaultSessionWork(t, baseURL)
	daemon.Stop(t)
	return session, work
}

func defaultsOperatorEnvironment(homeDir string) []string {
	blocked := map[string]struct{}{
		"HOME": {}, "USERPROFILE": {},
		strings.ToUpper(operatorsettings.EnvDefaultWorkerModelProvider): {},
		strings.ToUpper(operatorsettings.EnvDefaultWorkerModel):         {},
	}
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name := strings.ToUpper(strings.SplitN(entry, "=", 2)[0])
		if _, skip := blocked[name]; skip {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
}

type singleCommandExecutableLocator struct {
	command string
	path    string
}

func (l singleCommandExecutableLocator) LookPath(file string) (string, error) {
	if file == l.command {
		return l.path, nil
	}
	return "", fmt.Errorf("executable %q not found", file)
}

func writeFixtureExecutable(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(name+"-fixture-executable\n"), 0o755); err != nil {
		t.Fatalf("write %s fixture executable: %v", name, err)
	}
	return path
}
