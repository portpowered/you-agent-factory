package root_composition_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelservice "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	removeParityModelName       = "LLM"
	removeParityExpectedMessage = "model cache is not installed; run you models pull LLM first"
)

// TestModelsActualLocalAndConfiguredServerRemoveMissingCacheParity proves the
// absent-cache remove journey through both customer placements. The configured
// path uses a real root-composed API server and the server's dynamic URL; the
// test never supplies a canned response. Direct and configured calls are kept
// sequential because the repeated absence of this same cache is the invariant
// under test.
func TestModelsActualLocalAndConfiguredServerRemoveMissingCacheParity(t *testing.T) {
	t.Parallel()

	home := functionalTempDir(t)
	cacheRoot := filepath.Join(home, "missing-model-cache")
	environment := isolatedModelEnvironment(home, cacheRoot)
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	effects := newRemoveParityEffects()
	edges := effects.edges()

	t.Logf(
		"remove-cache parity environment: platform=%s/%s artifact=go-test-functional-package cache=%s listener=dynamic-loopback timeout=60s process-lifetime=one-server-command network=rejecting-external model-download-budget=0 max-calls=2",
		runtime.GOOS, runtime.GOARCH, cacheRoot,
	)
	assertRemoveParityCacheAbsent(t, cacheRoot, "before direct removal")

	directProcess := functionalBuildProcess(t, edges)
	direct := executeRemoveParityCommand(
		t, directProcess, factoryDir, environment,
		[]string{"you", "--json", "models", "remove", removeParityModelName},
		"direct",
	)
	assertDirectRemoveParityFailure(t, direct)
	closeRootProcess(t, directProcess, "close direct remove root process")
	assertRemoveParityCacheAbsent(t, cacheRoot, "after direct removal")

	server := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		Env:                       environment,
		WaitForServiceModeRuntime: true,
		ServerReadyTimeout:        60 * time.Second,
		Edges:                     edges,
	})
	serverURL := server.URL()
	assertRemoveParityDynamicServerURL(t, serverURL)
	t.Logf(
		"remove-cache parity listener: url=%s handler=production-root-composed dynamic=true configured-request-budget=1",
		serverURL,
	)

	configured := executeRemoveParityCommand(
		t, removeParityExecutor(func(input root.Input) error {
			return server.Execute(t, input)
		}), factoryDir, environment,
		[]string{"you", "--server", serverURL, "--json", "models", "remove", removeParityModelName},
		"configured --server",
	)
	assertConfiguredRemoveParityFailure(t, configured, serverURL)

	if direct.ExitCode != configured.ExitCode {
		t.Fatalf("direct/configured normalized exit codes = %d/%d, want both 1", direct.ExitCode, configured.ExitCode)
	}
	if direct.Stdout != configured.Stdout || direct.Stderr != configured.Stderr {
		t.Fatalf("direct/configured streams differ: direct stdout=%q stderr=%q; configured stdout=%q stderr=%q", direct.Stdout, direct.Stderr, configured.Stdout, configured.Stderr)
	}
	if direct.Diagnostic != configured.Diagnostic {
		t.Fatalf("direct/configured diagnostics differ: direct=%#v configured=%#v", direct.Diagnostic, configured.Diagnostic)
	}
	if direct.Diagnostic.Code != factoryapi.ErrorResponseCodeMODELCACHENOTFOUND ||
		direct.Diagnostic.Family != factoryapi.ErrorFamilyNotFound ||
		direct.Diagnostic.Message != removeParityExpectedMessage {
		t.Fatalf("normalized remove diagnostic = %#v, want MODEL_CACHE_NOT_FOUND/NOT_FOUND/%q", direct.Diagnostic, removeParityExpectedMessage)
	}

	server.Close(t)
	assertRemoveParityCacheAbsent(t, cacheRoot, "after configured-server removal")
	effects.assertNoActivation(t)
}

type removeParityCLIResult struct {
	Err        error
	Stdout     string
	Stderr     string
	Diagnostic factoryapi.ErrorResponse
	ExitCode   int
}

type removeParityExecutor func(root.Input) error

func (executor removeParityExecutor) Execute(input root.Input) error {
	return executor(input)
}

func executeRemoveParityCommand(
	t *testing.T,
	process support.Process,
	factoryDir string,
	environment []string,
	args []string,
	label string,
) removeParityCLIResult {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append([]string(nil), environment...)
	inputs.Input.WorkingDirectory = factoryDir
	err := process.Execute(inputs.Input)
	result := removeParityCLIResult{
		Err:      err,
		Stdout:   inputs.Stdout(),
		Stderr:   inputs.Stderr(),
		ExitCode: 0,
	}
	if err != nil {
		// Process.Execute exposes a command failure as an error. The public CLI
		// manifest maps that failure to exit status 1; numeric OS exit behavior
		// remains owned by the rebuilt-artifact validation gate.
		result.ExitCode = 1
	}
	if err == nil {
		t.Fatalf("%s Process.Execute(models remove) error = nil, want missing-cache failure", label)
	}
	result.Diagnostic = decodeFirstDiagnostic(t, result.Stderr)
	if result.Stdout != "" {
		t.Fatalf("%s models remove stdout = %q, want empty on failure", label, result.Stdout)
	}
	if nonEmptyDiagnosticLines(result.Stderr) != 1 {
		t.Fatalf("%s models remove stderr = %q, want one JSON diagnostic line", label, result.Stderr)
	}
	for _, forbidden := range []string{"managed model cache not found", "CLI_COMMAND_FAILED", "INTERNAL_SERVER_ERROR", "command failed", "\\"} {
		if strings.Contains(result.Stderr, forbidden) {
			t.Fatalf("%s models remove stderr contains forbidden %q: %q", label, forbidden, result.Stderr)
		}
	}
	return result
}

func assertDirectRemoveParityFailure(t *testing.T, result removeParityCLIResult) {
	t.Helper()
	if result.ExitCode != 1 {
		t.Fatalf("direct normalized exit code = %d, want 1", result.ExitCode)
	}
	if !errors.Is(result.Err, modelscli.ErrModelCacheNotFound) {
		t.Fatalf("direct remove error = %v, want CLI ErrModelCacheNotFound", result.Err)
	}
	if !errors.Is(result.Err, modelservice.ErrModelCacheNotFound) {
		t.Fatalf("direct remove error = %v, want Models ErrModelCacheNotFound cause", result.Err)
	}
}

func assertConfiguredRemoveParityFailure(t *testing.T, result removeParityCLIResult, serverURL string) {
	t.Helper()
	if result.ExitCode != 1 {
		t.Fatalf("configured-server normalized exit code = %d, want 1", result.ExitCode)
	}
	if !errors.Is(result.Err, modelscli.ErrModelCacheNotFound) {
		t.Fatalf("configured-server remove error = %v, want CLI ErrModelCacheNotFound", result.Err)
	}
	var httpFailure interface {
		CLIHTTPMethod() string
		CLIHTTPURL() string
		CLIHTTPStatus() int
	}
	if !errors.As(result.Err, &httpFailure) || httpFailure == nil {
		t.Fatalf("configured-server remove error = %T %v, want HTTP diagnostic metadata", result.Err, result.Err)
	}
	requestURL, err := url.Parse(httpFailure.CLIHTTPURL())
	if err != nil {
		t.Fatalf("configured-server request URL %q: %v", httpFailure.CLIHTTPURL(), err)
	}
	serverAddress, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("configured server URL %q: %v", serverURL, err)
	}
	if httpFailure.CLIHTTPMethod() != "DELETE" || httpFailure.CLIHTTPStatus() != 404 ||
		requestURL.Scheme != serverAddress.Scheme || requestURL.Host != serverAddress.Host ||
		requestURL.Path != "/models/"+removeParityModelName {
		t.Fatalf("configured-server HTTP witness = method:%q status:%d url:%q, want DELETE 404 %s/models/%s", httpFailure.CLIHTTPMethod(), httpFailure.CLIHTTPStatus(), httpFailure.CLIHTTPURL(), serverURL, removeParityModelName)
	}
}

func assertRemoveParityDynamicServerURL(t *testing.T, serverURL string) {
	t.Helper()
	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse dynamic server URL %q: %v", serverURL, err)
	}
	if parsed.Scheme != "http" || parsed.Path != "" {
		t.Fatalf("dynamic server URL = %q, want an HTTP loopback base URL", serverURL)
	}
	host := parsed.Hostname()
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			t.Fatalf("dynamic server host = %q, want loopback", host)
		}
	}
	if _, port, err := net.SplitHostPort(parsed.Host); err != nil || port == "7437" {
		t.Fatalf("dynamic server address = %q, want a dynamically bound non-7437 port", parsed.Host)
	}
}

func assertRemoveParityCacheAbsent(t *testing.T, cacheRoot, phase string) {
	t.Helper()
	if _, err := os.Stat(cacheRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s cache root %q stat error = %v, want absent", phase, cacheRoot, err)
	}
}

type removeParityEffects struct {
	assetNetwork     *rejectingModelAssetHTTP
	hostHTTP         *rejectingModelAssetHTTP
	runtimeHTTP      *rejectingModelAssetHTTP
	hostLauncher     *recordingModelHostLauncher
	protocol         *joinedProtocolNegotiator
	compatibility    *joinedCompatibilityChecker
	runtimeCommands  *support.RecordingCommandRunner
	backendResolvers atomic.Int32
	invocations      atomic.Int32
	asrInvocations   atomic.Int32
	embedInvocations atomic.Int32
}

func newRemoveParityEffects() *removeParityEffects {
	return &removeParityEffects{
		assetNetwork:    &rejectingModelAssetHTTP{},
		hostHTTP:        &rejectingModelAssetHTTP{},
		runtimeHTTP:     &rejectingModelAssetHTTP{},
		hostLauncher:    &recordingModelHostLauncher{endpoint: "http://127.0.0.1:1"},
		protocol:        &joinedProtocolNegotiator{},
		compatibility:   &joinedCompatibilityChecker{},
		runtimeCommands: support.NewRecordingCommandRunner("unexpected model runtime command"),
	}
}

func (effects *removeParityEffects) edges() serviceedges.Edges {
	return serviceedges.Edges{
		ModelAssetHTTPClient:          effects.assetNetwork,
		ModelHostHTTPClient:           effects.hostHTTP,
		ModelRuntimeHTTPClient:        effects.runtimeHTTP,
		ModelHostProcessLauncher:      effects.hostLauncher,
		ModelHostProtocolNegotiator:   effects.protocol,
		ModelHostCompatibilityChecker: effects.compatibility,
		ModelRuntimeCommandRunner:     effects.runtimeCommands,
		ModelResolveBackendArtifact: func(context.Context, serviceedges.ModelBackendArtifactSelectionRequest) (serviceedges.ModelBackendArtifactSelection, error) {
			effects.backendResolvers.Add(1)
			return serviceedges.ModelBackendArtifactSelection{}, fmt.Errorf("remove-cache parity must not resolve a backend")
		},
		ModelInvocationBackend: func(context.Context, modelservice.InvokeModelRequest) ([]modelservice.InferenceContent, []modelservice.InferenceArtifact, error) {
			effects.invocations.Add(1)
			return nil, nil, fmt.Errorf("remove-cache parity must not invoke a model")
		},
		ModelASRBackend: func(context.Context, modelservice.ASRBackendRequest) (modelservice.ASRBackendResponse, error) {
			effects.asrInvocations.Add(1)
			return modelservice.ASRBackendResponse{}, fmt.Errorf("remove-cache parity must not invoke ASR")
		},
		ModelEmbeddingBackend: func(context.Context, modelservice.EmbeddingBackendRequest) (modelservice.EmbeddingBackendResponse, error) {
			effects.embedInvocations.Add(1)
			return modelservice.EmbeddingBackendResponse{}, fmt.Errorf("remove-cache parity must not invoke embeddings")
		},
	}
}

func (effects *removeParityEffects) assertNoActivation(t *testing.T) {
	t.Helper()
	if effects.assetNetwork.Calls() != 0 || effects.hostHTTP.Calls() != 0 || effects.runtimeHTTP.Calls() != 0 {
		t.Fatalf("remove-cache external model network calls = asset:%d host:%d runtime:%d, want all zero", effects.assetNetwork.Calls(), effects.hostHTTP.Calls(), effects.runtimeHTTP.Calls())
	}
	if effects.hostLauncher.Calls() != 0 || effects.protocol.Calls() != 0 || effects.compatibility.Calls() != 0 {
		t.Fatalf("remove-cache backend lifecycle calls = host:%d protocol:%d compatibility:%d, want all zero", effects.hostLauncher.Calls(), effects.protocol.Calls(), effects.compatibility.Calls())
	}
	if effects.runtimeCommands.CallCount() != 0 || effects.backendResolvers.Load() != 0 || effects.invocations.Load() != 0 || effects.asrInvocations.Load() != 0 || effects.embedInvocations.Load() != 0 {
		t.Fatalf("remove-cache activation calls = runtime:%d resolve:%d invoke:%d asr:%d embed:%d, want all zero", effects.runtimeCommands.CallCount(), effects.backendResolvers.Load(), effects.invocations.Load(), effects.asrInvocations.Load(), effects.embedInvocations.Load())
	}
}
