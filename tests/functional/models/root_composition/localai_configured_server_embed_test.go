package root_composition_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

const (
	localAIConfiguredEmbedOperatorSource = "hf://characterization/localai-embed.gguf"
	localAIConfiguredEmbedRevision       = "0123456789abcdef0123456789abcdef01234567"
	localAIConfiguredEmbedPrompt         = "configured EMBED parity prompt"
	localAIConfiguredEmbedParameters     = `{"dimensions":5,"normalize":true}`
)

// TestLocalAIConfiguredServerEMBEDParity proves the composed customer path
// for the built-in EMBED model. Direct CLI and an independent thin client use
// separate roots, while the server owns the only actual HTTP listener and the
// controlled LocalAI fixture remains the sole protocol dependency.
func TestLocalAIConfiguredServerEMBEDParity(t *testing.T) {
	t.Parallel()

	fixture := functionalStartLocalAI(t, localai.Options{EmbeddingDimensions: 5})
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())

	directHome := functionalTempDir(t)
	directResolver, directBackendSelection := prepareLocalAIConfiguredEmbedProfile(t, directHome)
	directLauncher := &localAIHostLauncher{endpoint: fixture.Endpoint()}
	directInvocation := &localAIConfiguredEmbedInvocationRecorder{
		next: serviceedges.ModelInvocationBackend(fixture.InvocationBackend),
	}
	directEdges, directNetwork, _, _ := localAIConfiguredEmbedEdges(
		directHome, fixture, directResolver, directBackendSelection, directLauncher, directInvocation,
	)
	directProcess := functionalBuildProcess(t, directEdges)
	warmLocalAIConfiguredEmbedCache(t, directProcess, factoryDir, cleanModelsEnvironment(directHome), "", true, directInvocation)
	direct := executeLocalAIConfiguredEmbedInvoke(
		t, directProcess, factoryDir, cleanModelsEnvironment(directHome), "", true,
	)
	assertLocalAIConfiguredEmbedSuccess(t, direct, "direct")
	assertLocalAIConfiguredEmbedSelection(
		t, directHome, directResolver, directBackendSelection, directLauncher, directNetwork,
	)
	directRequests := directInvocation.Requests()
	if len(directRequests) != 1 {
		t.Fatalf("direct configured EMBED backend requests = %d, want one", len(directRequests))
	}
	assertLocalAIConfiguredEmbedRequest(t, "direct", directRequests[0])
	directRequest := directInvocation.Signatures()
	if err := directProcess.Close(context.Background()); err != nil {
		t.Fatalf("close direct configured EMBED process: %v", err)
	}
	assertLocalAIDirectHostReleased(t, directLauncher)

	serverHome := functionalTempDir(t)
	serverResolver, serverBackendSelection := prepareLocalAIConfiguredEmbedProfile(t, serverHome)
	serverLauncher := &localAIHostLauncher{endpoint: fixture.Endpoint()}
	serverInvocation := &localAIConfiguredEmbedInvocationRecorder{
		next: serviceedges.ModelInvocationBackend(fixture.InvocationBackend),
	}
	serverEdges, serverNetwork, _, _ := localAIConfiguredEmbedEdges(
		serverHome, fixture, serverResolver, serverBackendSelection, serverLauncher, serverInvocation,
	)
	server := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		ServerReadyTimeout:        60 * time.Second,
		Env:                       cleanModelsEnvironment(serverHome),
		Edges:                     serverEdges,
	})
	serverURL := server.URL()
	assertLocalAIConfiguredServerLoopbackPort(t, serverURL)

	clientHome := functionalTempDir(t)
	clientLauncher := &cacheSelectionHostLauncherFailure{}
	clientEdges, clientNetwork, _, _ := localAIConfiguredEmbedEdges(
		clientHome, fixture, nil, nil, clientLauncher, nil,
	)
	clientEdges.ModelInvocationBackend = nil
	clientEdges.ModelInvocationProtocolClient = nil
	clientEdges.ModelInvocationGRPCDialer = nil
	clientProcess := functionalBuildProcess(t, clientEdges)
	warmLocalAIConfiguredEmbedCache(t, clientProcess, factoryDir, cleanModelsEnvironment(clientHome), serverURL, false, serverInvocation)
	remote := executeLocalAIConfiguredEmbedInvoke(
		t, clientProcess, factoryDir, cleanModelsEnvironment(clientHome), serverURL, false,
	)
	assertLocalAIConfiguredEmbedSuccess(t, remote, "explicit --server")

	if got := serverInvocation.Signatures(); len(got) != 1 {
		t.Fatalf("configured-server EMBED backend requests = %d, want one", len(got))
	} else if len(directRequest) != 1 || got[0] != directRequest[0] {
		t.Fatalf("direct/configured-server EMBED request signatures = %#v / %#v, want identical ordered inputs", directRequest, got)
	}
	serverRequests := serverInvocation.Requests()
	assertLocalAIConfiguredEmbedRequest(t, "explicit --server", serverRequests[0])
	if direct.Observation != remote.Observation {
		t.Fatalf("direct/configured-server EMBED observations differ: %#v / %#v", direct.Observation, remote.Observation)
	}
	assertLocalAIConfiguredEmbedSelection(
		t, serverHome, serverResolver, serverBackendSelection, serverLauncher, serverNetwork,
	)
	assertLocalAIConfiguredEmbedFixtureCalls(t, fixture.Calls(), 2)
	if clientLauncher.called {
		t.Fatal("explicit --server EMBED opened a client-local model host")
	}
	if clientNetwork.Calls() != 0 {
		t.Fatalf("explicit --server client model-asset network calls = %d, want zero", clientNetwork.Calls())
	}
	clientCacheRoot := filepath.Join(clientHome, ".agent-factory", "models")
	if _, err := os.Stat(clientCacheRoot); !os.IsNotExist(err) {
		t.Fatalf("explicit --server client model cache root = %q, %v; want absent", clientCacheRoot, err)
	}

	if err := clientProcess.Close(context.Background()); err != nil {
		t.Fatalf("close configured EMBED client process: %v", err)
	}
	server.Close(t)
	assertLocalAIConfiguredServerReleased(t, server, serverURL)
	assertLocalAIDirectHostReleased(t, serverLauncher)
	if directNetwork.Calls()+serverNetwork.Calls() != 0 {
		t.Fatalf("configured EMBED model-asset network calls = direct:%d server:%d, want zero", directNetwork.Calls(), serverNetwork.Calls())
	}
	if err := fixture.Close(); err != nil {
		t.Fatalf("close configured EMBED LocalAI fixture: %v", err)
	}
	assertLocalAIOMNIFixtureListenerReleased(t, fixture.Endpoint())
}

// TestLocalAIConfiguredServerEMBEDValidationParity proves the two CLI forms
// reject caller-owned input before any model host or backend effect. A
// malformed --parameter is transport preflight; a parameters-only input is a
// catalog-contract failure with the same public diagnostic on both routes.
func TestLocalAIConfiguredServerEMBEDValidationParity(t *testing.T) {
	t.Parallel()

	fixture := functionalStartLocalAI(t, localai.Options{EmbeddingDimensions: 5})
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())

	directHome := functionalTempDir(t)
	directResolver, directBackendSelection := prepareLocalAIConfiguredEmbedProfile(t, directHome)
	directLauncher := &localAIHostLauncher{endpoint: fixture.Endpoint()}
	directInvocation := &localAIConfiguredEmbedInvocationRecorder{
		next: serviceedges.ModelInvocationBackend(fixture.InvocationBackend),
	}
	directEdges, directNetwork, _, _ := localAIConfiguredEmbedEdges(
		directHome, fixture, directResolver, directBackendSelection, directLauncher, directInvocation,
	)
	directProcess := functionalBuildProcess(t, directEdges)

	serverHome := functionalTempDir(t)
	serverResolver, serverBackendSelection := prepareLocalAIConfiguredEmbedProfile(t, serverHome)
	serverLauncher := &localAIHostLauncher{endpoint: fixture.Endpoint()}
	serverInvocation := &localAIConfiguredEmbedInvocationRecorder{
		next: serviceedges.ModelInvocationBackend(fixture.InvocationBackend),
	}
	serverEdges, serverNetwork, _, _ := localAIConfiguredEmbedEdges(
		serverHome, fixture, serverResolver, serverBackendSelection, serverLauncher, serverInvocation,
	)
	server := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		ServerReadyTimeout:        60 * time.Second,
		Env:                       cleanModelsEnvironment(serverHome),
		Edges:                     serverEdges,
	})
	serverURL := server.URL()
	assertLocalAIConfiguredServerLoopbackPort(t, serverURL)

	clientHome := functionalTempDir(t)
	clientLauncher := &cacheSelectionHostLauncherFailure{}
	clientEdges, clientNetwork, _, _ := localAIConfiguredEmbedEdges(
		clientHome, fixture, nil, nil, clientLauncher, nil,
	)
	clientEdges.ModelInvocationBackend = nil
	clientEdges.ModelInvocationProtocolClient = nil
	clientEdges.ModelInvocationGRPCDialer = nil
	clientProcess := functionalBuildProcess(t, clientEdges)

	cases := []struct {
		name           string
		inputSpecs     []string
		parameterSpecs []string
		expected       factoryapi.ErrorResponse
	}{
		{
			name:       "missing required text input",
			inputSpecs: localAIConfiguredEmbedInputSpecs(t, false),
			expected: factoryapi.ErrorResponse{
				Code:    "BAD_REQUEST",
				Family:  factoryapi.ErrorFamilyBadRequest,
				Message: "required input slot is missing: text",
			},
		},
		{
			name:           "malformed parameter flag",
			inputSpecs:     localAIConfiguredEmbedInputSpecs(t, true),
			parameterSpecs: []string{"{"},
			expected: factoryapi.ErrorResponse{
				Code:    "BAD_REQUEST",
				Family:  factoryapi.ErrorFamilyBadRequest,
				Message: "parse --parameter 1: invalid JSON",
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			directCacheBefore := snapshotLocalAIConfiguredEmbedCache(t, directHome)
			serverCacheBefore := snapshotLocalAIConfiguredEmbedCache(t, serverHome)
			direct := executeLocalAIConfiguredEmbedCommand(
				t, directProcess, factoryDir, cleanModelsEnvironment(directHome), "", true,
				testCase.inputSpecs, testCase.parameterSpecs,
			)
			remote := executeLocalAIConfiguredEmbedCommand(
				t, clientProcess, factoryDir, cleanModelsEnvironment(clientHome), serverURL, false,
				testCase.inputSpecs, testCase.parameterSpecs,
			)
			assertLocalAIConfiguredEmbedValidationFailureParity(t, testCase.name, testCase.expected, direct, remote)
			assertLocalAIConfiguredEmbedCacheUnchanged(
				t, testCase.name+" direct", directCacheBefore, snapshotLocalAIConfiguredEmbedCache(t, directHome),
			)
			assertLocalAIConfiguredEmbedCacheUnchanged(
				t, testCase.name+" explicit --server", serverCacheBefore, snapshotLocalAIConfiguredEmbedCache(t, serverHome),
			)
		})
	}

	if len(directInvocation.Signatures()) != 0 || len(serverInvocation.Signatures()) != 0 {
		t.Fatalf("validation invoked backend: direct=%#v server=%#v", directInvocation.Signatures(), serverInvocation.Signatures())
	}
	if directLauncher.Starts() != 0 || serverLauncher.Starts() != 0 || clientLauncher.called {
		t.Fatalf("validation host effects = direct:%d server:%d clientCalled:%t, want zero", directLauncher.Starts(), serverLauncher.Starts(), clientLauncher.called)
	}
	if directNetwork.Calls()+serverNetwork.Calls()+clientNetwork.Calls() != 0 {
		t.Fatalf("validation model-asset network calls = direct:%d server:%d client:%d, want zero", directNetwork.Calls(), serverNetwork.Calls(), clientNetwork.Calls())
	}

	if err := clientProcess.Close(context.Background()); err != nil {
		t.Fatalf("close validation EMBED client process: %v", err)
	}
	if err := directProcess.Close(context.Background()); err != nil {
		t.Fatalf("close validation EMBED direct process: %v", err)
	}
	server.Close(t)
	assertLocalAIConfiguredServerReleased(t, server, serverURL)
	if err := fixture.Close(); err != nil {
		t.Fatalf("close validation EMBED LocalAI fixture: %v", err)
	}
	assertLocalAIOMNIFixtureListenerReleased(t, fixture.Endpoint())
}

func assertLocalAIConfiguredEmbedValidationFailureParity(
	t *testing.T,
	name string,
	expected factoryapi.ErrorResponse,
	direct, remote localAIConfiguredEmbedInvokeResult,
) {
	t.Helper()
	for path, result := range map[string]localAIConfiguredEmbedInvokeResult{
		"direct": direct, "explicit --server": remote,
	} {
		if result.Err == nil {
			t.Fatalf("%s %s validation error = nil, want failure", name, path)
		}
		if result.Stdout != "" {
			t.Fatalf("%s %s validation stdout = %q, want empty", name, path, result.Stdout)
		}
		if strings.TrimSpace(result.Stderr) == "" {
			t.Fatalf("%s %s validation stderr is empty, want one public diagnostic", name, path)
		}
	}
	directDiagnostic := decodeLocalAIConfiguredEmbedDiagnostic(t, name+" direct", direct.Stderr)
	remoteDiagnostic := decodeLocalAIConfiguredEmbedDiagnostic(t, name+" explicit --server", remote.Stderr)
	for path, diagnostic := range map[string]factoryapi.ErrorResponse{
		"direct": directDiagnostic, "explicit --server": remoteDiagnostic,
	} {
		if diagnostic.Code != expected.Code || diagnostic.Family != expected.Family || diagnostic.Message != expected.Message {
			t.Fatalf("%s %s validation diagnostic = %#v, want %#v", name, path, diagnostic, expected)
		}
	}
	if directDiagnostic.Code != remoteDiagnostic.Code ||
		directDiagnostic.Family != remoteDiagnostic.Family ||
		directDiagnostic.Message != remoteDiagnostic.Message {
		t.Fatalf("%s validation diagnostics differ: direct=%#v remote=%#v", name, directDiagnostic, remoteDiagnostic)
	}
}

func decodeLocalAIConfiguredEmbedDiagnostic(
	t *testing.T,
	path, stderr string,
) factoryapi.ErrorResponse {
	t.Helper()
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &response); err != nil {
		t.Fatalf("decode %s diagnostic: %v\nstderr=%q", path, err, stderr)
	}
	return response
}

// TestLocalAIConfiguredServerEMBEDFailureParity covers the operation-boundary
// failures supplied by the shared LocalAI gRPC fixture. Each row owns one
// fixture and one dynamic HTTP server so a failed invocation cannot hide a
// stale listener or lease behind another case.
func TestLocalAIConfiguredServerEMBEDFailureParity(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name string
		mode localai.Mode
	}{
		{name: "backend protocol", mode: localai.ModeProtocolMismatch},
		{name: "malformed response", mode: localai.ModeMalformedResponse},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runLocalAIConfiguredEmbedFailureParity(t, testCase.mode)
		})
	}
}

func runLocalAIConfiguredEmbedFailureParity(t *testing.T, mode localai.Mode) {
	t.Helper()
	fixture := functionalStartLocalAI(t, localai.Options{Mode: mode, EmbeddingDimensions: 5})
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())

	directHome := functionalTempDir(t)
	directResolver, directBackendSelection := prepareLocalAIConfiguredEmbedProfile(t, directHome)
	directLauncher := &localAIHostLauncher{endpoint: fixture.Endpoint()}
	directInvocation := &localAIConfiguredEmbedInvocationRecorder{
		next: serviceedges.ModelInvocationBackend(fixture.InvocationBackend),
	}
	directEdges, directNetwork, _, _ := localAIConfiguredEmbedEdges(
		directHome, fixture, directResolver, directBackendSelection, directLauncher, directInvocation,
	)
	directProcess := functionalBuildProcess(t, directEdges)
	warmLocalAIConfiguredEmbedCache(t, directProcess, factoryDir, cleanModelsEnvironment(directHome), "", true, directInvocation)
	directCacheBefore := snapshotLocalAIConfiguredEmbedCache(t, directHome)
	direct := executeLocalAIConfiguredEmbedInvoke(
		t, directProcess, factoryDir, cleanModelsEnvironment(directHome), "", true,
	)

	serverHome := functionalTempDir(t)
	serverResolver, serverBackendSelection := prepareLocalAIConfiguredEmbedProfile(t, serverHome)
	serverLauncher := &localAIHostLauncher{endpoint: fixture.Endpoint()}
	serverInvocation := &localAIConfiguredEmbedInvocationRecorder{
		next: serviceedges.ModelInvocationBackend(fixture.InvocationBackend),
	}
	serverEdges, serverNetwork, _, _ := localAIConfiguredEmbedEdges(
		serverHome, fixture, serverResolver, serverBackendSelection, serverLauncher, serverInvocation,
	)
	server := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		ServerReadyTimeout:        60 * time.Second,
		Env:                       cleanModelsEnvironment(serverHome),
		Edges:                     serverEdges,
	})
	serverURL := server.URL()
	assertLocalAIConfiguredServerLoopbackPort(t, serverURL)
	serverCacheBefore := snapshotLocalAIConfiguredEmbedCache(t, serverHome)

	clientHome := functionalTempDir(t)
	clientLauncher := &cacheSelectionHostLauncherFailure{}
	clientEdges, clientNetwork, _, _ := localAIConfiguredEmbedEdges(
		clientHome, fixture, nil, nil, clientLauncher, nil,
	)
	clientEdges.ModelInvocationBackend = nil
	clientEdges.ModelInvocationProtocolClient = nil
	clientEdges.ModelInvocationGRPCDialer = nil
	clientProcess := functionalBuildProcess(t, clientEdges)
	warmLocalAIConfiguredEmbedCache(t, clientProcess, factoryDir, cleanModelsEnvironment(clientHome), serverURL, false, serverInvocation)
	serverCacheBefore = snapshotLocalAIConfiguredEmbedCache(t, serverHome)
	remote := executeLocalAIConfiguredEmbedInvoke(
		t, clientProcess, factoryDir, cleanModelsEnvironment(clientHome), serverURL, false,
	)

	wantClass := models.InvocationFailureClassBackendProtocol
	if mode == localai.ModeMalformedResponse {
		wantClass = models.InvocationFailureClassMalformedResponse
	}
	assertLocalAIConfiguredEmbedFailure(t, direct, "direct", wantClass)
	assertLocalAIConfiguredEmbedFailure(t, remote, "explicit --server", wantClass)
	assertLocalAIConfiguredEmbedFailureDiagnostics(t, fixture, direct, remote)
	assertLocalAIConfiguredEmbedCacheUnchanged(
		t, "backend failure direct", directCacheBefore, snapshotLocalAIConfiguredEmbedCache(t, directHome),
	)
	assertLocalAIConfiguredEmbedCacheUnchanged(
		t, "backend failure explicit --server", serverCacheBefore, snapshotLocalAIConfiguredEmbedCache(t, serverHome),
	)
	if len(directInvocation.Signatures()) != 1 || len(serverInvocation.Signatures()) != 1 {
		t.Fatalf("configured EMBED failure backend requests = direct:%d server:%d, want one each", len(directInvocation.Signatures()), len(serverInvocation.Signatures()))
	}
	assertLocalAIConfiguredEmbedFixtureCalls(t, fixture.Calls(), 2)
	if clientLauncher.called || clientNetwork.Calls() != 0 {
		t.Fatalf("configured EMBED failure used client effects: host=%t network=%d", clientLauncher.called, clientNetwork.Calls())
	}
	clientCacheRoot := filepath.Join(clientHome, ".agent-factory", "models")
	if _, err := os.Stat(clientCacheRoot); !os.IsNotExist(err) {
		t.Fatalf("configured EMBED failure client model cache root = %q, %v; want absent", clientCacheRoot, err)
	}
	if directNetwork.Calls() != 0 || serverNetwork.Calls() != 0 {
		t.Fatalf("configured EMBED failure model-asset network calls = direct:%d server:%d, want zero", directNetwork.Calls(), serverNetwork.Calls())
	}

	if err := directProcess.Close(context.Background()); err != nil {
		t.Fatalf("close direct configured EMBED failure process: %v", err)
	}
	assertLocalAIDirectHostReleased(t, directLauncher)
	if err := clientProcess.Close(context.Background()); err != nil {
		t.Fatalf("close configured EMBED failure client process: %v", err)
	}
	server.Close(t)
	assertLocalAIConfiguredServerReleased(t, server, serverURL)
	assertLocalAIDirectHostReleased(t, serverLauncher)
	if err := fixture.Close(); err != nil {
		t.Fatalf("close configured EMBED failure LocalAI fixture: %v", err)
	}
	assertLocalAIOMNIFixtureListenerReleased(t, fixture.Endpoint())
}

func assertLocalAIConfiguredEmbedFailure(
	t *testing.T,
	result localAIConfiguredEmbedInvokeResult,
	path string,
	wantClass models.InvocationFailureClass,
) {
	t.Helper()
	if result.Err == nil {
		t.Fatalf("%s configured EMBED failure returned nil error", path)
	}
	if result.Stdout != "" {
		t.Fatalf("%s configured EMBED failure stdout = %q, want empty", path, result.Stdout)
	}
	var failure *models.InvocationFailure
	if !errors.As(result.Err, &failure) || failure == nil {
		if strings.HasPrefix(path, "explicit --server") {
			// The thin client intentionally receives the server's public
			// response diagnostic rather than the server's private typed value.
			return
		}
		t.Fatalf("%s configured EMBED error = %v (%T), want typed invocation failure", path, result.Err, result.Err)
	}
	if failure.Class != wantClass || failure.Operation != models.OperationEMBED ||
		(wantClass != models.InvocationFailureClassMalformedResponse &&
			failure.Model.NameOrURI != models.BuiltInModelNameEmbed) {
		t.Fatalf("%s configured EMBED typed failure = %#v, want %s/embed/EMBED", path, failure, wantClass)
	}
}

// TestLocalAIConfiguredServerEMBEDRecoveryAfterFailure proves that a typed
// backend failure is released before a subsequent direct and explicit-server
// request succeeds through the same composed roots and fixture.
func TestLocalAIConfiguredServerEMBEDRecoveryAfterFailure(t *testing.T) {
	t.Parallel()

	fixture := functionalStartLocalAI(t, localai.Options{EmbeddingDimensions: 5})
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())

	directHome := functionalTempDir(t)
	directResolver, directBackendSelection := prepareLocalAIConfiguredEmbedProfile(t, directHome)
	directLauncher := &localAIHostLauncher{endpoint: fixture.Endpoint()}
	directInvocation := &localAIConfiguredEmbedInvocationRecorder{
		next: serviceedges.ModelInvocationBackend(fixture.InvocationBackend),
	}
	directEdges, directNetwork, _, _ := localAIConfiguredEmbedEdges(
		directHome, fixture, directResolver, directBackendSelection, directLauncher, directInvocation,
	)
	directProcess := functionalBuildProcess(t, directEdges)
	warmLocalAIConfiguredEmbedCache(t, directProcess, factoryDir, cleanModelsEnvironment(directHome), "", true, directInvocation)
	directInvocation.FailNext(localAIConfiguredEmbedBackendFailure())
	directCacheBeforeFailure := snapshotLocalAIConfiguredEmbedCache(t, directHome)
	directFailure := executeLocalAIConfiguredEmbedInvoke(
		t, directProcess, factoryDir, cleanModelsEnvironment(directHome), "", true,
	)
	assertLocalAIConfiguredEmbedFailure(
		t, directFailure, "direct failure before recovery", models.InvocationFailureClassBackendProtocol,
	)
	assertLocalAIConfiguredEmbedCacheUnchanged(
		t, "recovery direct failure", directCacheBeforeFailure, snapshotLocalAIConfiguredEmbedCache(t, directHome),
	)
	if directLauncher.Active() {
		t.Fatal("direct configured EMBED host remained active after backend failure")
	}
	directSuccess := executeLocalAIConfiguredEmbedInvoke(
		t, directProcess, factoryDir, cleanModelsEnvironment(directHome), "", true,
	)
	assertLocalAIConfiguredEmbedSuccess(t, directSuccess, "direct recovery")

	serverHome := functionalTempDir(t)
	serverResolver, serverBackendSelection := prepareLocalAIConfiguredEmbedProfile(t, serverHome)
	serverLauncher := &localAIHostLauncher{endpoint: fixture.Endpoint()}
	serverInvocation := &localAIConfiguredEmbedInvocationRecorder{
		next: serviceedges.ModelInvocationBackend(fixture.InvocationBackend),
	}
	serverEdges, serverNetwork, _, _ := localAIConfiguredEmbedEdges(
		serverHome, fixture, serverResolver, serverBackendSelection, serverLauncher, serverInvocation,
	)
	server := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		ServerReadyTimeout:        60 * time.Second,
		Env:                       cleanModelsEnvironment(serverHome),
		Edges:                     serverEdges,
	})
	serverURL := server.URL()
	assertLocalAIConfiguredServerLoopbackPort(t, serverURL)
	serverCacheBeforeFailure := snapshotLocalAIConfiguredEmbedCache(t, serverHome)

	clientHome := functionalTempDir(t)
	clientLauncher := &cacheSelectionHostLauncherFailure{}
	clientEdges, clientNetwork, _, _ := localAIConfiguredEmbedEdges(
		clientHome, fixture, nil, nil, clientLauncher, nil,
	)
	clientEdges.ModelInvocationBackend = nil
	clientEdges.ModelInvocationProtocolClient = nil
	clientEdges.ModelInvocationGRPCDialer = nil
	clientProcess := functionalBuildProcess(t, clientEdges)
	warmLocalAIConfiguredEmbedCache(t, clientProcess, factoryDir, cleanModelsEnvironment(clientHome), serverURL, false, serverInvocation)
	serverCacheBeforeFailure = snapshotLocalAIConfiguredEmbedCache(t, serverHome)
	serverInvocation.FailNext(localAIConfiguredEmbedBackendFailure())
	remoteFailure := executeLocalAIConfiguredEmbedInvoke(
		t, clientProcess, factoryDir, cleanModelsEnvironment(clientHome), serverURL, false,
	)
	assertLocalAIConfiguredEmbedFailure(
		t, remoteFailure, "explicit --server failure before recovery", models.InvocationFailureClassBackendProtocol,
	)
	assertLocalAIConfiguredEmbedCacheUnchanged(
		t, "recovery explicit --server failure", serverCacheBeforeFailure, snapshotLocalAIConfiguredEmbedCache(t, serverHome),
	)
	remoteSuccess := executeLocalAIConfiguredEmbedInvoke(
		t, clientProcess, factoryDir, cleanModelsEnvironment(clientHome), serverURL, false,
	)
	assertLocalAIConfiguredEmbedSuccess(t, remoteSuccess, "explicit --server recovery")

	assertLocalAIConfiguredEmbedRecovery(t, fixture, directFailure, remoteFailure, directSuccess, remoteSuccess, directInvocation, serverInvocation, clientHome, clientLauncher, clientNetwork, directNetwork, serverNetwork)

	if err := directProcess.Close(context.Background()); err != nil {
		t.Fatalf("close recovery direct process: %v", err)
	}
	assertLocalAIDirectHostReleased(t, directLauncher)
	if err := clientProcess.Close(context.Background()); err != nil {
		t.Fatalf("close recovery client process: %v", err)
	}
	server.Close(t)
	assertLocalAIConfiguredServerReleased(t, server, serverURL)
	assertLocalAIDirectHostReleased(t, serverLauncher)
	if err := fixture.Close(); err != nil {
		t.Fatalf("close recovery LocalAI fixture: %v", err)
	}
	assertLocalAIOMNIFixtureListenerReleased(t, fixture.Endpoint())
}

func localAIConfiguredEmbedBackendFailure() error {
	return &models.InvocationFailure{
		Class:     models.InvocationFailureClassBackendProtocol,
		Message:   "EMBED backend invocation failed",
		Model:     models.ModelReference{NameOrURI: models.BuiltInModelNameEmbed},
		Operation: models.OperationEMBED,
		Cause:     models.ErrInferenceFailed,
	}
}

// TestLocalAIConfiguredServerEMBEDUnavailableAddressIsRedacted proves the
// retained thin client turns a closed explicit-server endpoint into the safe
// fallback diagnostic without opening any client-local model state.
func TestLocalAIConfiguredServerEMBEDUnavailableAddressIsRedacted(t *testing.T) {
	t.Parallel()

	fixture := functionalStartLocalAI(t, localai.Options{EmbeddingDimensions: 5})
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	serverHome := functionalTempDir(t)
	serverResolver, serverBackendSelection := prepareLocalAIConfiguredEmbedProfile(t, serverHome)
	serverLauncher := &localAIHostLauncher{endpoint: fixture.Endpoint()}
	serverInvocation := &localAIConfiguredEmbedInvocationRecorder{
		next: serviceedges.ModelInvocationBackend(fixture.InvocationBackend),
	}
	serverEdges, _, _, _ := localAIConfiguredEmbedEdges(
		serverHome, fixture, serverResolver, serverBackendSelection, serverLauncher, serverInvocation,
	)
	server := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		ServerReadyTimeout:        60 * time.Second,
		Env:                       cleanModelsEnvironment(serverHome),
		Edges:                     serverEdges,
	})
	serverURL := server.URL()
	assertLocalAIConfiguredServerLoopbackPort(t, serverURL)

	clientHome := functionalTempDir(t)
	clientLauncher := &cacheSelectionHostLauncherFailure{}
	clientEdges, clientNetwork, _, _ := localAIConfiguredEmbedEdges(
		clientHome, fixture, nil, nil, clientLauncher, nil,
	)
	clientEdges.ModelInvocationBackend = nil
	clientEdges.ModelInvocationProtocolClient = nil
	clientEdges.ModelInvocationGRPCDialer = nil
	clientProcess := functionalBuildProcess(t, clientEdges)
	warmLocalAIConfiguredEmbedCache(t, clientProcess, factoryDir, cleanModelsEnvironment(clientHome), serverURL, false, serverInvocation)
	server.Close(t)
	assertLocalAIConfiguredServerReleased(t, server, serverURL)
	serverCacheBefore := snapshotLocalAIConfiguredEmbedCache(t, serverHome)
	serverHostStartsBeforeClosedRequest := serverLauncher.Starts()
	clientCacheBefore := snapshotLocalAIConfiguredEmbedCache(t, clientHome)
	const (
		endpointCanary = "SERVER_PARITY_ENDPOINT_CANARY"
		tokenCanary    = "SERVER_PARITY_TOKEN_CANARY"
		rootCanary     = "SERVER_PARITY_ROOT_CANARY"
	)
	closedServerURL := strings.TrimSuffix(serverURL, "/") + "/" + endpointCanary + "/" + rootCanary + "/" + tokenCanary
	result := executeLocalAIConfiguredEmbedInvoke(
		t, clientProcess, factoryDir, cleanModelsEnvironment(clientHome), closedServerURL, false,
	)
	if result.Err == nil {
		t.Fatal("explicit --server EMBED against closed listener returned nil error")
	}
	if result.Stdout != "" {
		t.Fatalf("closed explicit-server EMBED stdout = %q, want empty", result.Stdout)
	}
	diagnostic := decodeLocalAIConfiguredEmbedDiagnostic(t, "closed explicit-server EMBED", result.Stderr)
	if diagnostic.Code != "CLI_COMMAND_FAILED" ||
		diagnostic.Family != factoryapi.ErrorFamilyInternalServerError ||
		diagnostic.Message != "command failed" {
		t.Fatalf("closed explicit-server EMBED diagnostic = %#v, want CLI_COMMAND_FAILED/INTERNAL_SERVER_ERROR/command failed", diagnostic)
	}
	for _, canary := range []string{endpointCanary, tokenCanary, rootCanary, serverURL, clientHome} {
		if strings.Contains(result.Stdout, canary) || strings.Contains(result.Stderr, canary) {
			t.Fatalf("closed explicit-server EMBED leaked %q: stdout=%q stderr=%q", canary, result.Stdout, result.Stderr)
		}
	}
	if clientLauncher.called || clientNetwork.Calls() != 0 || len(serverInvocation.Signatures()) != 0 {
		t.Fatalf("closed explicit-server EMBED effects = clientHost:%t clientNetwork:%d serverInvocations:%d, want zero", clientLauncher.called, clientNetwork.Calls(), len(serverInvocation.Signatures()))
	}
	clientCacheRoot := filepath.Join(clientHome, ".agent-factory", "models")
	if _, err := os.Stat(clientCacheRoot); !os.IsNotExist(err) {
		t.Fatalf("closed explicit-server EMBED client model cache root = %q, %v; want absent", clientCacheRoot, err)
	}
	assertLocalAIConfiguredEmbedCacheUnchanged(
		t, "closed explicit-server server cache", serverCacheBefore, snapshotLocalAIConfiguredEmbedCache(t, serverHome),
	)
	assertLocalAIConfiguredEmbedCacheUnchanged(
		t, "closed explicit-server client cache", clientCacheBefore, snapshotLocalAIConfiguredEmbedCache(t, clientHome),
	)
	if serverLauncher.Starts() != serverHostStartsBeforeClosedRequest || serverLauncher.Active() {
		t.Fatalf("closed explicit-server EMBED server host lifecycle = starts:%d active:%t, want no additional host start after warm-up (%d)", serverLauncher.Starts(), serverLauncher.Active(), serverHostStartsBeforeClosedRequest)
	}

	if err := clientProcess.Close(context.Background()); err != nil {
		t.Fatalf("close closed-server EMBED client process: %v", err)
	}
	if err := fixture.Close(); err != nil {
		t.Fatalf("close closed-server EMBED LocalAI fixture: %v", err)
	}
	assertLocalAIOMNIFixtureListenerReleased(t, fixture.Endpoint())
}

// TestLocalAIConfiguredServerEMBEDNonFiniteFailureParity proves the typed
// EMBED codec rejects a non-finite backend value on both the direct and actual
// server paths without publishing a partial vector.
func TestLocalAIConfiguredServerEMBEDNonFiniteFailureParity(t *testing.T) {
	t.Parallel()

	fixture := functionalStartLocalAI(t, localai.Options{EmbeddingDimensions: 5})
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())

	directHome := functionalTempDir(t)
	directResolver, directBackendSelection := prepareLocalAIConfiguredEmbedProfile(t, directHome)
	directLauncher := &localAIHostLauncher{endpoint: fixture.Endpoint()}
	directBackend := &localAIConfiguredEmbedTypedBackend{fixture: fixture, nonFinite: true}
	directEdges, directNetwork, _, _ := localAIConfiguredEmbedEdges(
		directHome, fixture, directResolver, directBackendSelection, directLauncher, nil,
	)
	directEdges.ModelInvocationBackend = nil
	directEdges.ModelEmbeddingBackend = directBackend.Invoke
	directProcess := functionalBuildProcess(t, directEdges)
	warmLocalAIConfiguredEmbedTypedCache(t, directProcess, factoryDir, cleanModelsEnvironment(directHome), "", true, directBackend)
	directCacheBefore := snapshotLocalAIConfiguredEmbedCache(t, directHome)
	direct := executeLocalAIConfiguredEmbedInvoke(
		t, directProcess, factoryDir, cleanModelsEnvironment(directHome), "", true,
	)
	assertLocalAIConfiguredEmbedFailure(
		t, direct, "direct non-finite response", models.InvocationFailureClassMalformedResponse,
	)

	serverHome := functionalTempDir(t)
	serverResolver, serverBackendSelection := prepareLocalAIConfiguredEmbedProfile(t, serverHome)
	serverLauncher := &localAIHostLauncher{endpoint: fixture.Endpoint()}
	serverBackend := &localAIConfiguredEmbedTypedBackend{fixture: fixture, nonFinite: true}
	serverEdges, serverNetwork, _, _ := localAIConfiguredEmbedEdges(
		serverHome, fixture, serverResolver, serverBackendSelection, serverLauncher, nil,
	)
	serverEdges.ModelInvocationBackend = nil
	serverEdges.ModelEmbeddingBackend = serverBackend.Invoke
	server := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		ServerReadyTimeout:        60 * time.Second,
		Env:                       cleanModelsEnvironment(serverHome),
		Edges:                     serverEdges,
	})
	serverURL := server.URL()
	assertLocalAIConfiguredServerLoopbackPort(t, serverURL)
	serverCacheBefore := snapshotLocalAIConfiguredEmbedCache(t, serverHome)

	clientHome := functionalTempDir(t)
	clientLauncher := &cacheSelectionHostLauncherFailure{}
	clientEdges, clientNetwork, _, _ := localAIConfiguredEmbedEdges(
		clientHome, fixture, nil, nil, clientLauncher, nil,
	)
	clientEdges.ModelInvocationBackend = nil
	clientEdges.ModelEmbeddingBackend = nil
	clientEdges.ModelInvocationProtocolClient = nil
	clientEdges.ModelInvocationGRPCDialer = nil
	clientProcess := functionalBuildProcess(t, clientEdges)
	warmLocalAIConfiguredEmbedTypedCache(t, clientProcess, factoryDir, cleanModelsEnvironment(clientHome), serverURL, false, serverBackend)
	serverCacheBefore = snapshotLocalAIConfiguredEmbedCache(t, serverHome)
	remote := executeLocalAIConfiguredEmbedInvoke(
		t, clientProcess, factoryDir, cleanModelsEnvironment(clientHome), serverURL, false,
	)
	assertLocalAIConfiguredEmbedFailure(
		t, remote, "explicit --server non-finite response", models.InvocationFailureClassMalformedResponse,
	)
	assertLocalAIConfiguredEmbedCacheUnchanged(
		t, "non-finite direct", directCacheBefore, snapshotLocalAIConfiguredEmbedCache(t, directHome),
	)
	assertLocalAIConfiguredEmbedCacheUnchanged(
		t, "non-finite explicit --server", serverCacheBefore, snapshotLocalAIConfiguredEmbedCache(t, serverHome),
	)
	assertLocalAIConfiguredEmbedNonFiniteObservations(t, fixture, direct, remote, directBackend, serverBackend)
	if clientLauncher.called || clientNetwork.Calls() != 0 {
		t.Fatalf("non-finite explicit --server used client effects: host=%t network=%d", clientLauncher.called, clientNetwork.Calls())
	}
	clientCacheRoot := filepath.Join(clientHome, ".agent-factory", "models")
	if _, err := os.Stat(clientCacheRoot); !os.IsNotExist(err) {
		t.Fatalf("non-finite explicit --server client model cache root = %q, %v; want absent", clientCacheRoot, err)
	}
	if directNetwork.Calls() != 0 || serverNetwork.Calls() != 0 {
		t.Fatalf("non-finite EMBED model-asset network calls = direct:%d server:%d, want zero", directNetwork.Calls(), serverNetwork.Calls())
	}

	if err := directProcess.Close(context.Background()); err != nil {
		t.Fatalf("close direct non-finite EMBED process: %v", err)
	}
	assertLocalAIDirectHostReleased(t, directLauncher)
	if err := clientProcess.Close(context.Background()); err != nil {
		t.Fatalf("close non-finite EMBED client process: %v", err)
	}
	server.Close(t)
	assertLocalAIConfiguredServerReleased(t, server, serverURL)
	assertLocalAIDirectHostReleased(t, serverLauncher)
	if err := fixture.Close(); err != nil {
		t.Fatalf("close non-finite EMBED LocalAI fixture: %v", err)
	}
	assertLocalAIOMNIFixtureListenerReleased(t, fixture.Endpoint())
}

func assertLocalAIConfiguredEmbedFailureDiagnostics(
	t *testing.T,
	fixture *localai.Fixture,
	direct, remote localAIConfiguredEmbedInvokeResult,
) {
	t.Helper()
	directDiagnostic := decodeLocalAIConfiguredEmbedDiagnostic(t, "direct configured EMBED failure", direct.Stderr)
	remoteDiagnostic := decodeLocalAIConfiguredEmbedDiagnostic(t, "explicit --server configured EMBED failure", remote.Stderr)
	if directDiagnostic.Code != remoteDiagnostic.Code ||
		directDiagnostic.Family != remoteDiagnostic.Family ||
		directDiagnostic.Message != remoteDiagnostic.Message {
		t.Fatalf("configured EMBED failure diagnostics differ: direct=%#v remote=%#v", directDiagnostic, remoteDiagnostic)
	}
	if directDiagnostic.Code != "MODEL_BACKEND_FAILURE" ||
		directDiagnostic.Family != factoryapi.ErrorFamilyInternalServerError {
		t.Fatalf("configured EMBED failure diagnostic = %#v, want MODEL_BACKEND_FAILURE/INTERNAL_SERVER_ERROR", directDiagnostic)
	}
	for _, value := range []string{fixture.Endpoint(), localAIConfiguredEmbedOperatorSource, localAIConfiguredEmbedRevision} {
		if strings.Contains(directDiagnostic.Message, value) || strings.Contains(remoteDiagnostic.Message, value) {
			t.Fatalf("configured EMBED failure leaked %q: direct=%q remote=%q", value, directDiagnostic.Message, remoteDiagnostic.Message)
		}
	}
}

func assertLocalAIConfiguredEmbedRecovery(
	t *testing.T,
	fixture *localai.Fixture,
	directFailure, remoteFailure, directSuccess, remoteSuccess localAIConfiguredEmbedInvokeResult,
	directInvocation, serverInvocation *localAIConfiguredEmbedInvocationRecorder,
	clientHome string,
	clientLauncher *cacheSelectionHostLauncherFailure,
	clientNetwork, directNetwork, serverNetwork *rejectingModelAssetHTTP,
) {
	t.Helper()
	assertLocalAIConfiguredEmbedFailureDiagnostics(t, fixture, directFailure, remoteFailure)
	if directSuccess.Observation != remoteSuccess.Observation {
		t.Fatalf("recovery success observations differ: direct=%#v remote=%#v", directSuccess.Observation, remoteSuccess.Observation)
	}
	directRequests := directInvocation.Signatures()
	serverRequests := serverInvocation.Signatures()
	if len(directRequests) != 2 || len(serverRequests) != 2 ||
		directRequests[0] != serverRequests[0] || directRequests[1] != serverRequests[1] {
		t.Fatalf("recovery request signatures = direct:%#v server:%#v, want matching failure and recovery inputs", directRequests, serverRequests)
	}
	assertLocalAIConfiguredEmbedFixtureCalls(t, fixture.Calls(), 4)
	if clientLauncher.called || clientNetwork.Calls() != 0 {
		t.Fatalf("recovery explicit --server used client effects: host=%t network=%d", clientLauncher.called, clientNetwork.Calls())
	}
	clientCacheRoot := filepath.Join(clientHome, ".agent-factory", "models")
	if _, err := os.Stat(clientCacheRoot); !os.IsNotExist(err) {
		t.Fatalf("recovery explicit --server client model cache root = %q, %v; want absent", clientCacheRoot, err)
	}
	if directNetwork.Calls() != 0 || serverNetwork.Calls() != 0 {
		t.Fatalf("recovery model-asset network calls = direct:%d server:%d, want zero", directNetwork.Calls(), serverNetwork.Calls())
	}
	if len(fixture.Calls()) < 4 {
		t.Fatalf("recovery LocalAI fixture calls = %d, want failed and recovered direct/server protocol calls", len(fixture.Calls()))
	}
}

func assertLocalAIConfiguredEmbedNonFiniteObservations(
	t *testing.T,
	fixture *localai.Fixture,
	direct, remote localAIConfiguredEmbedInvokeResult,
	directBackend, serverBackend *localAIConfiguredEmbedTypedBackend,
) {
	t.Helper()
	directDiagnostic := decodeLocalAIConfiguredEmbedDiagnostic(t, "direct non-finite EMBED", direct.Stderr)
	remoteDiagnostic := decodeLocalAIConfiguredEmbedDiagnostic(t, "explicit --server non-finite EMBED", remote.Stderr)
	if directDiagnostic.Code != remoteDiagnostic.Code ||
		directDiagnostic.Family != remoteDiagnostic.Family ||
		directDiagnostic.Message != remoteDiagnostic.Message {
		t.Fatalf("non-finite EMBED diagnostics differ: direct=%#v remote=%#v", directDiagnostic, remoteDiagnostic)
	}
	if directDiagnostic.Code != "MODEL_BACKEND_FAILURE" ||
		directDiagnostic.Family != factoryapi.ErrorFamilyInternalServerError {
		t.Fatalf("non-finite EMBED diagnostic = %#v, want MODEL_BACKEND_FAILURE/INTERNAL_SERVER_ERROR", directDiagnostic)
	}
	directRequests := directBackend.Signatures()
	serverRequests := serverBackend.Signatures()
	if len(directRequests) != 1 || len(serverRequests) != 1 || directRequests[0] != serverRequests[0] {
		t.Fatalf("non-finite EMBED typed requests = direct:%#v server:%#v, want identical text and parameters", directRequests, serverRequests)
	}
	request := directBackend.Requests()[0]
	if request.Text != localAIConfiguredEmbedPrompt ||
		fmt.Sprint(request.Parameters["dimensions"]) != "5" || request.Parameters["normalize"] != true {
		t.Fatalf("non-finite EMBED typed request = %#v, want text and normalized dimensions parameters", request)
	}
	assertLocalAIConfiguredEmbedFixtureCalls(t, fixture.Calls(), 2)
}
