package root_composition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestLocalAIConfiguredServerMatchesDirectConfigurationFacts proves that an
// explicit --server command keeps source, cache, backend, platform, protocol,
// and normalized result ownership in the server-built Models process. The
// client is given an empty model profile and a launcher that fails if the
// remote command opens a local host. The server uses only deterministic cached
// fixtures and rejecting model-asset HTTP effects.
func TestLocalAIConfiguredServerMatchesDirectConfigurationFacts(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name          string
		protocolError error
		wantOutput    string
		wantFailure   bool
	}{
		{
			name:       "semantic output",
			wantOutput: "configured server LocalAI characterization response",
		},
		{
			name:          "typed backend protocol failure",
			protocolError: errors.New("controlled configured-server LocalAI backend protocol failure"),
			wantFailure:   true,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runLocalAIConfiguredServerParity(t, test)
		})
	}
}

func runLocalAIConfiguredServerParity(
	t *testing.T,
	test struct {
		name          string
		protocolError error
		wantOutput    string
		wantFailure   bool
	},
) {
	t.Helper()

	direct := newLocalAIDirectConfigurationFactsScenario(t, test.protocolError, test.wantOutput)
	directResult := executeLocalAIConfigurationFactsInvoke(
		t, direct.process, direct.factoryDir, direct.environment, "", true,
	)
	assertLocalAIConfigurationFactsResult(t, directResult, test.wantOutput, test.wantFailure, true)
	assertLocalAIDirectConfigurationFacts(t, direct)
	directFacts := localAIConfigurationFactsSnapshot(t, direct)
	closeLocalAIConfigurationFactsProcess(t, direct.process, "direct")

	serverScenario := newLocalAIConfiguredServerScenario(t, direct, test)
	serverURL := serverScenario.server.URL()
	port := assertLocalAIConfiguredServerLoopbackPort(t, serverURL)
	client := newLocalAIConfiguredServerClient(t, direct)
	remoteResult := executeLocalAIConfigurationFactsInvoke(
		t, client.process, client.factoryDir, client.environment, serverURL, false,
	)
	assertLocalAIConfigurationFactsResult(t, remoteResult, test.wantOutput, test.wantFailure, false)
	assertLocalAIConfiguredServerClientUnused(t, client)
	assertLocalAIDirectConfigurationFacts(t, serverScenario.factsScenario())
	serverFacts := localAIConfigurationFactsSnapshot(t, serverScenario.factsScenario())
	if directFacts != serverFacts {
		t.Fatalf("direct/configured-server LocalAI facts differ: %#v / %#v", directFacts, serverFacts)
	}

	closeLocalAIConfigurationFactsProcess(t, client.process, "configured-server client")
	serverScenario.server.Close(t)
	assertLocalAIConfiguredServerReleased(t, serverScenario.server, serverURL)
	assertLocalAIDirectHostReleased(t, serverScenario.launcher)

	t.Logf(
		"configured-server LocalAI parity: source=%q revision=%q cache=%q modelPath=%q protocol=%q backend=%q platform=%s/%s directOffline=true serverCacheOnly=true loopbackPort=%d clientModelEffects=0 serverNetworkCalls=%d hostStarts=%d hostStops=%d hostWaits=%d",
		direct.source, direct.revision, serverScenario.modelCacheRoot,
		serverScenario.launcher.ModelPath(), serverScenario.negotiator.ProtocolVersion(),
		direct.backend, direct.platform.OperatingSystem, direct.platform.Architecture,
		port, serverScenario.assetNetwork.Calls(), serverScenario.launcher.Starts(),
		serverScenario.launcher.Stops(), serverScenario.launcher.Waits(),
	)
}

type localAIConfigurationFactsResult struct {
	response factoryapi.GenericModelInvocationResponse
	stdout   string
	stderr   string
	err      error
}

func executeLocalAIConfigurationFactsInvoke(
	t *testing.T,
	process support.Process,
	factoryDir string,
	environment []string,
	serverURL string,
	offline bool,
) localAIConfigurationFactsResult {
	t.Helper()

	args := []string{"you", "--json"}
	if strings.TrimSpace(serverURL) != "" {
		args = append(args, "--server", strings.TrimSuffix(serverURL, "/"))
	}
	args = append(args,
		"models", "invoke", models.BuiltInModelNameLLM,
	)
	if offline {
		args = append(args, "--offline")
	}
	args = append(args,
		"--operation", models.OperationOMNI,
		"--input", "prompt=direct LocalAI characterization prompt",
	)
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append([]string(nil), environment...)
	inputs.Input.WorkingDirectory = factoryDir
	var stdout, stderr bytes.Buffer
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr

	err := process.Execute(inputs.Input)
	result := localAIConfigurationFactsResult{
		stdout: stdout.String(), stderr: stderr.String(), err: err,
	}
	if strings.TrimSpace(result.stdout) == "" {
		return result
	}
	if decodeErr := json.Unmarshal([]byte(result.stdout), &result.response); decodeErr != nil {
		t.Fatalf("decode LocalAI configuration-facts response: %v\nstdout=%s", decodeErr, result.stdout)
	}
	return result
}

func assertLocalAIConfigurationFactsResult(
	t *testing.T,
	result localAIConfigurationFactsResult,
	wantOutput string,
	wantFailure, direct bool,
) {
	t.Helper()
	if wantFailure {
		if direct {
			assertLocalAIDirectTypedFailure(t, result.err, result.stdout)
		} else {
			assertLocalAIConfiguredServerFailure(t, result)
		}
		return
	}
	assertLocalAIDirectSemanticOutput(t, result.err, []byte(result.stdout), result.stderr, wantOutput)
}

func assertLocalAIConfiguredServerFailure(t *testing.T, result localAIConfigurationFactsResult) {
	t.Helper()
	if result.err == nil {
		t.Fatal("configured-server LocalAI protocol failure returned nil error")
	}
	if result.stdout != "" || len(result.response.Outputs) != 0 {
		t.Fatalf("configured-server LocalAI protocol failure published output: stdout=%q response=%#v", result.stdout, result.response)
	}
	details := cacheSelectionFailure(t, result.err)
	if details.Code != "MODEL_BACKEND_FAILURE" || details.Family != factoryapi.ErrorFamilyInternalServerError {
		t.Fatalf("configured-server LocalAI failure = %#v, want MODEL_BACKEND_FAILURE/INTERNAL_SERVER_ERROR", details)
	}
}

type localAIConfiguredServerScenario struct {
	server         *support.FunctionalAPIServer
	factoryDir     string
	home           string
	modelCacheRoot string
	source         string
	revision       string
	backend        string
	platform       models.AssetHostPlatform
	assetNetwork   *rejectingModelAssetHTTP
	resolver       *localAIRevisionRecorder
	backendSelect  *localAIBackendSelectionRecorder
	launcher       *localAIHostLauncher
	negotiator     *localAIHostProtocolRecorder
	compatibility  *localAIHostCompatibilityRecorder
	invocation     *localAIInvocationProtocolRecorder
}

func newLocalAIConfiguredServerScenario(
	t *testing.T,
	direct localAIDirectConfigurationFactsScenario,
	test struct {
		name          string
		protocolError error
		wantOutput    string
		wantFailure   bool
	},
) localAIConfiguredServerScenario {
	t.Helper()

	home := functionalTempDir(t)
	writeLocalAIModelOverlay(t, home, direct.source)
	writeGenericBuiltinModelCache(t, home, direct.source+"@"+direct.revision)
	selection := genericLlamaBackendSelection()
	writeGenericBackendCache(t, home, direct.backend, selection, []byte("localai-llamacpp/linux-amd64"))

	assetNetwork := &rejectingModelAssetHTTP{}
	resolver := &localAIRevisionRecorder{revision: direct.revision}
	backendSelect := &localAIBackendSelectionRecorder{selection: selection}
	launcher := &localAIHostLauncher{endpoint: "http://localai-configured-server.invalid"}
	negotiator := &localAIHostProtocolRecorder{}
	compatibility := &localAIHostCompatibilityRecorder{}
	invocation := &localAIInvocationProtocolRecorder{response: test.wantOutput, failure: test.protocolError}
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env:                       cleanModelsEnvironment(home),
		Edges: localAIConfigurationFactsEdges(
			home, assetNetwork, resolver, backendSelect, launcher, negotiator,
			compatibility, direct.platform, invocation,
		),
	})
	return localAIConfiguredServerScenario{
		server: server, factoryDir: factoryDir, home: home,
		modelCacheRoot: filepath.Join(home, ".agent-factory", "models"),
		source:         direct.source, revision: direct.revision, backend: direct.backend,
		platform: direct.platform, assetNetwork: assetNetwork, resolver: resolver,
		backendSelect: backendSelect, launcher: launcher, negotiator: negotiator,
		compatibility: compatibility, invocation: invocation,
	}
}

func (scenario localAIConfiguredServerScenario) factsScenario() localAIDirectConfigurationFactsScenario {
	return localAIDirectConfigurationFactsScenario{
		factoryDir: scenario.factoryDir, home: scenario.home,
		modelCacheRoot: scenario.modelCacheRoot, source: scenario.source,
		revision: scenario.revision, backend: scenario.backend,
		platform: scenario.platform, assetNetwork: scenario.assetNetwork,
		resolver: scenario.resolver, backendSelect: scenario.backendSelect,
		launcher: scenario.launcher, negotiator: scenario.negotiator,
		compatibility: scenario.compatibility, invocation: scenario.invocation,
	}
}

type localAIConfiguredServerClient struct {
	process        support.ApplicationProcess
	factoryDir     string
	home           string
	modelCacheRoot string
	environment    []string
	assetNetwork   *rejectingModelAssetHTTP
	resolver       *localAIRevisionRecorder
	backendSelect  *localAIBackendSelectionRecorder
	launcher       *cacheSelectionHostLauncherFailure
	negotiator     *localAIHostProtocolRecorder
	compatibility  *localAIHostCompatibilityRecorder
	invocation     *localAIInvocationProtocolRecorder
}

func newLocalAIConfiguredServerClient(
	t *testing.T,
	direct localAIDirectConfigurationFactsScenario,
) localAIConfiguredServerClient {
	t.Helper()
	home := functionalTempDir(t)
	selection := genericLlamaBackendSelection()
	assetNetwork := &rejectingModelAssetHTTP{}
	resolver := &localAIRevisionRecorder{revision: direct.revision}
	backendSelect := &localAIBackendSelectionRecorder{selection: selection}
	launcher := &cacheSelectionHostLauncherFailure{}
	negotiator := &localAIHostProtocolRecorder{}
	compatibility := &localAIHostCompatibilityRecorder{}
	invocation := &localAIInvocationProtocolRecorder{}
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	process := functionalBuildProcess(t, localAIConfigurationFactsEdges(
		home, assetNetwork, resolver, backendSelect, launcher, negotiator,
		compatibility, direct.platform, invocation,
	))
	return localAIConfiguredServerClient{
		process: process, factoryDir: factoryDir, home: home,
		modelCacheRoot: filepath.Join(home, ".agent-factory", "models"),
		environment:    cleanModelsEnvironment(home), assetNetwork: assetNetwork,
		resolver: resolver, backendSelect: backendSelect, launcher: launcher,
		negotiator: negotiator, compatibility: compatibility, invocation: invocation,
	}
}

func assertLocalAIConfiguredServerClientUnused(t *testing.T, client localAIConfiguredServerClient) {
	t.Helper()
	if client.launcher.called {
		t.Fatal("explicit --server invocation opened the client LocalAI host")
	}
	if client.assetNetwork.Calls() != 0 || len(client.resolver.Sources()) != 0 ||
		len(client.backendSelect.Requests()) != 0 || len(client.negotiator.Requests()) != 0 ||
		len(client.compatibility.Requests()) != 0 || client.invocation.Calls() != 0 {
		t.Fatalf("explicit --server invocation used client LocalAI effects: network=%d sources=%d backend=%d negotiation=%d compatibility=%d invocation=%d",
			client.assetNetwork.Calls(), len(client.resolver.Sources()),
			len(client.backendSelect.Requests()), len(client.negotiator.Requests()),
			len(client.compatibility.Requests()), client.invocation.Calls())
	}
	if _, err := os.Stat(client.modelCacheRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("client LocalAI model cache root = %q, %v; want absent", client.modelCacheRoot, err)
	}
}

func localAIConfigurationFactsEdges(
	home string,
	assetNetwork *rejectingModelAssetHTTP,
	resolver *localAIRevisionRecorder,
	backendSelect *localAIBackendSelectionRecorder,
	launcher interface {
		Start(context.Context, serviceedges.HostProcessStartSpec) (interface {
			HealthEndpoint() string
			Wait() error
			Stop(context.Context) error
		}, error)
	},
	negotiator *localAIHostProtocolRecorder,
	compatibility *localAIHostCompatibilityRecorder,
	platform models.AssetHostPlatform,
	invocation *localAIInvocationProtocolRecorder,
) serviceedges.Edges {
	assetFiles := functionalModelAssetFileSystem{home: home}
	return serviceedges.Edges{
		ModelAssetHTTPClient:            assetNetwork,
		ModelAssetMakeDirectories:       assetFiles.MkdirAll,
		ModelAssetInspectPath:           assetFiles.Stat,
		ModelAssetResolveHomeDirectory:  assetFiles.UserHomeDir,
		ModelAssetResolveEnvironment:    func(string) string { return "" },
		ModelAssetWriteFile:             assetFiles.WriteFile,
		ModelAssetRenamePath:            assetFiles.Rename,
		ModelAssetRemovePath:            assetFiles.Remove,
		ModelAssetReadFile:              assetFiles.ReadFile,
		ModelAssetReadDirectory:         assetFiles.ReadDir,
		ModelAssetCreateFile:            assetFiles.Create,
		ModelAssetOpenFile:              assetFiles.Open,
		ModelHostProcessLauncher:        launcher,
		ModelHostProtocolNegotiator:     negotiator,
		ModelHostCompatibilityChecker:   compatibility,
		ModelAssetHostPlatform:          platform,
		ModelResolveBackendArtifact:     backendSelect.Resolve,
		ModelResolveHuggingFaceRevision: resolver.Resolve,
		ModelHostHTTPClient:             assetNetwork,
		ModelRuntimeHTTPClient:          assetNetwork,
		ModelInvocationProtocolClient:   invocation,
	}
}

type localAIConfigurationFactsObservation struct {
	source            string
	revision          string
	backend           string
	platform          models.AssetHostPlatform
	protocol          string
	modelRelativePath string
	backendRelative   string
}

func localAIConfigurationFactsSnapshot(
	t *testing.T,
	scenario localAIDirectConfigurationFactsScenario,
) localAIConfigurationFactsObservation {
	t.Helper()
	sources := scenario.resolver.Sources()
	backendRequests := scenario.backendSelect.Requests()
	compatibilityRequests := scenario.compatibility.Requests()
	negotiationRequests := scenario.negotiator.Requests()
	spec, ok := scenario.launcher.LastSpec()
	if !ok || len(sources) == 0 || len(backendRequests) == 0 ||
		len(compatibilityRequests) == 0 || len(negotiationRequests) == 0 {
		t.Fatal("LocalAI configuration facts snapshot is incomplete")
	}
	modelRelativePath, err := filepath.Rel(scenario.modelCacheRoot, spec.ModelPath)
	if err != nil {
		t.Fatalf("relativize LocalAI model path: %v", err)
	}
	backendRoot := filepath.Join(scenario.modelCacheRoot, "backend-artifacts")
	backendRelativePath, err := filepath.Rel(backendRoot, spec.BackendFiles[0])
	if err != nil {
		t.Fatalf("relativize LocalAI backend path: %v", err)
	}
	return localAIConfigurationFactsObservation{
		source: sources[0], revision: compatibilityRequests[0].Revision,
		backend: backendRequests[0].Backend, platform: backendRequests[0].Platform,
		protocol:          negotiationRequests[0].ProtocolVersion,
		modelRelativePath: filepath.ToSlash(modelRelativePath),
		backendRelative:   filepath.ToSlash(backendRelativePath),
	}
}

func assertLocalAIConfiguredServerLoopbackPort(t *testing.T, serverURL string) int {
	t.Helper()
	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse configured-server URL %q: %v", serverURL, err)
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		t.Fatalf("configured-server URL = %q, want an OS-assigned loopback listener", serverURL)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 {
		t.Fatalf("configured-server port %q: %v", portText, err)
	}
	if port == 7437 {
		t.Fatalf("configured-server port = %d, want OS-assigned port rather than fixed 7437", port)
	}
	return port
}

func assertLocalAIConfiguredServerReleased(
	t *testing.T,
	server *support.FunctionalAPIServer,
	serverURL string,
) {
	t.Helper()
	select {
	case <-server.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("configured-server process did not finish after Close")
	}
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(strings.TrimSuffix(serverURL, "/") + "/status")
	if err == nil {
		response.Body.Close()
		t.Fatalf("configured-server listener still accepted requests after Close")
	}
}

func closeLocalAIConfigurationFactsProcess(
	t *testing.T,
	process support.ApplicationProcess,
	label string,
) {
	t.Helper()
	if err := process.Close(context.Background()); err != nil {
		t.Fatalf("close %s LocalAI characterization process: %v", label, err)
	}
}
