package root_composition_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const story004EmbedSource = "hf://Qwen/Qwen3-Embedding-0.6B-GGUF/Qwen3-Embedding-0.6B-f16.gguf@370f27d7550e0def9b39c1f16d3fbaa13aa67728"

func TestModelsEmbedRootCompositionBehavior(t *testing.T) {
	t.Parallel()
	t.Run("zero-configuration", testModelsEmbedZeroConfigurationJourneyThroughRootBuildProcess)
	t.Run("oversized-file", testModelsEmbedOversizedFileInputFailsBeforeBackendThroughRootBuildProcess)
	t.Run("invalid-vector", testModelsEmbedInvalidVectorUsesTypedRuntimeAndReleasesLease)
	t.Run("named-generic-http-parity", testModelsNamedAndGenericHTTPInvocationShareBuiltinResolution)
}

func testModelsEmbedZeroConfigurationJourneyThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	hostServer := story004HostServer(t)
	home := functionalTempDir(t)
	backendBody := []byte("story-004-localai-backend")
	selection := story004EmbedBackendSelection(backendBody)
	writeGenericBuiltinModelCache(t, home, story004EmbedSource)
	writeGenericBackendCache(t, home, "localai-llamacpp", selection, backendBody)

	assetNetwork := &rejectingModelAssetHTTP{}
	launcher := &recordingModelHostLauncher{endpoint: hostServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	fixture := newStory004EmbedFixture()
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	process := functionalBuildProcess(t, story004EmbedEdges(
		home, assetNetwork, hostServer.Client(), launcher, protocol, compatibility,
		selection, fixture,
	))
	support.CleanupProcess(t, process)
	environment := functionalHomeEnvironment(home)

	stdout, stderr, err := runStory004CLI(t, process, factoryDir, environment,
		[]string{"you", "models", "invoke", "embed", "--input", "text=Find similar work"})
	assertStory004PlainOutput(t, err, stdout, stderr)

	stdout, stderr, err = runStory004CLI(t, process, factoryDir, environment, []string{
		"you", "--json", "models", "invoke", "embed",
		"--input", "text=Find similar work", "--input", `parameters=json:{"normalize":true}`,
	})
	if err != nil {
		t.Fatalf("EMBED JSON command error = %v\nstdout=%q\nstderr=%q", err, stdout, stderr)
	}
	var jsonResponse factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal([]byte(stdout), &jsonResponse); err != nil {
		t.Fatalf("decode EMBED JSON command response: %v\n%s", err, stdout)
	}
	if len(jsonResponse.Outputs) != 1 || jsonResponse.Outputs[0].Name != "embedding" ||
		jsonResponse.Outputs[0].Content == nil || *jsonResponse.Outputs[0].Content != `[0.1,0.2,0.3,0.4]` {
		t.Fatalf("EMBED JSON response = %#v, want one named embedding vector", jsonResponse)
	}
	if stderr != "" {
		t.Fatalf("EMBED JSON stderr = %q, want empty", stderr)
	}

	exchanges := fixture.Exchanges()
	if len(exchanges) != 2 || exchanges[0].ProtocolJSON != `{"prompt":"Find similar work"}` ||
		exchanges[1].ProtocolJSON != `{"prompt":"Find similar work","parameters":{"normalize":true}}` {
		t.Fatalf("fixture protocol exchanges = %#v, want inline and parameterized EMBED requests", exchanges)
	}
	if assetNetwork.Calls() != 0 {
		t.Fatalf("cache-hit asset network calls = %d, want 0", assetNetwork.Calls())
	}
	if fixture.Calls() != 2 {
		t.Fatalf("fixture EMBED calls = %d, want two completed root invocations", fixture.Calls())
	}

	beforeCalls := fixture.Calls()
	beforeStarts := launcher.Calls()
	stdout, stderr, err = runStory004CLI(t, process, factoryDir, environment,
		[]string{"you", "models", "invoke", "embed", "--input", "unknown=not sent"})
	if err == nil {
		t.Fatal("unknown EMBED input error = nil, want preflight failure")
	}
	if stdout != "" {
		t.Fatalf("unknown EMBED input stdout = %q, want empty", stdout)
	}
	support.RequireSafeCLIDiagnostic(t, stderr)
	if fixture.Calls() != beforeCalls || launcher.Calls() != beforeStarts {
		t.Fatalf("unknown EMBED input effects = backend %d->%d starts %d->%d, want no post-preflight effects", beforeCalls, fixture.Calls(), beforeStarts, launcher.Calls())
	}

	fixture.SetFailure(errors.New("backend endpoint https://private.invalid token=secret cache=/private/cache: fixture failure"))
	stdout, stderr, err = runStory004CLI(t, process, factoryDir, environment,
		[]string{"you", "models", "invoke", "embed", "--input", "text=Find similar work"})
	if err == nil {
		t.Fatal("failed EMBED input error = nil, want backend failure")
	}
	if stdout != "" {
		t.Fatalf("failed EMBED stdout = %q, want empty", stdout)
	}
	requireEmbedTypedDiagnostic(t, stderr, "EMBED backend invocation failed")
	for _, secret := range []string{"private.invalid", "secret", "/private/cache"} {
		if strings.Contains(stderr, secret) {
			t.Fatalf("failed EMBED diagnostic leaked %q: %q", secret, stderr)
		}
	}

	fixture.SetFailure(nil)
	stdout, stderr, err = runStory004CLI(t, process, factoryDir, environment,
		[]string{"you", "models", "invoke", "embed", "--input", "text=Find similar work"})
	if err != nil || stdout != `[0.1,0.2,0.3,0.4]` || stderr != "" {
		t.Fatalf("EMBED after failure = err %v stdout %q stderr %q, want released successful invocation", err, stdout, stderr)
	}
}

func testModelsEmbedOversizedFileInputFailsBeforeBackendThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	hostServer := story004HostServer(t)
	home := functionalTempDir(t)
	backendBody := []byte("story-004-localai-backend-oversized-input")
	selection := story004EmbedBackendSelection(backendBody)
	writeGenericBuiltinModelCache(t, home, story004EmbedSource)
	writeGenericBackendCache(t, home, "localai-llamacpp", selection, backendBody)
	assetNetwork := &rejectingModelAssetHTTP{}
	launcher := &recordingModelHostLauncher{endpoint: hostServer.URL}
	fixture := newStory004EmbedFixture()
	edges := story004EmbedEdges(
		home, assetNetwork, hostServer.Client(), launcher, &joinedProtocolNegotiator{},
		&joinedCompatibilityChecker{}, selection, fixture,
	)
	var receivedLimit int64
	edges.ModelCLIInputReadFile = func(_ context.Context, _ string, maxBytes int64) ([]byte, error) {
		receivedLimit = maxBytes
		return bytes.Repeat([]byte{'x'}, int(maxBytes+1)), nil
	}
	process := functionalBuildProcess(t, edges)
	support.CleanupProcess(t, process)
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())

	stdout, stderr, err := runStory004CLI(t, process, factoryDir, functionalHomeEnvironment(home),
		[]string{"you", "models", "invoke", "embed", "--input", "text=@oversized.txt"})
	if err == nil {
		t.Fatal("oversized EMBED file error = nil, want local preflight failure")
	}
	if stdout != "" {
		t.Fatalf("oversized EMBED file stdout = %q, want empty", stdout)
	}
	var diagnostic factoryapi.ErrorResponse
	if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &diagnostic); decodeErr != nil {
		t.Fatalf("decode oversized EMBED diagnostic: %v\nstderr=%q", decodeErr, stderr)
	}
	if diagnostic.Code != factoryapi.ErrorResponseCode("CLI_LOCAL_INPUT_FAILED") ||
		diagnostic.Family != factoryapi.ErrorFamilyBadRequest ||
		!strings.Contains(diagnostic.Message, "failed to load --input") {
		t.Fatalf("oversized EMBED diagnostic = %#v, want customer-safe local input failure", diagnostic)
	}
	if receivedLimit <= 0 || fixture.Calls() != 0 || launcher.Calls() != 0 || assetNetwork.Calls() != 0 {
		t.Fatalf("oversized EMBED effects = limit:%d backend:%d starts:%d assets:%d, want positive limit and no downstream effects", receivedLimit, fixture.Calls(), launcher.Calls(), assetNetwork.Calls())
	}
}

func testModelsEmbedInvalidVectorUsesTypedRuntimeAndReleasesLease(t *testing.T) {
	t.Parallel()

	hostServer := story004HostServer(t)
	home := functionalTempDir(t)
	backendBody := []byte("story-004-localai-backend-invalid-vector")
	selection := story004EmbedBackendSelection(backendBody)
	writeGenericBuiltinModelCache(t, home, story004EmbedSource)
	writeGenericBackendCache(t, home, "localai-llamacpp", selection, backendBody)
	assetNetwork := &rejectingModelAssetHTTP{}
	launcher := &recordingModelHostLauncher{endpoint: hostServer.URL}
	fixture := newStory004EmbedFixture()
	fixture.SetResponse(models.EmbeddingBackendResponse{
		Embeddings: []float64{math.NaN()},
	})
	process := functionalBuildProcess(t, story004EmbedEdges(
		home, assetNetwork, hostServer.Client(), launcher, &joinedProtocolNegotiator{},
		&joinedCompatibilityChecker{}, selection, fixture,
	))
	support.CleanupProcess(t, process)
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	environment := functionalHomeEnvironment(home)

	stdout, stderr, err := runStory004CLI(t, process, factoryDir, environment,
		[]string{"you", "models", "invoke", "embed", "--input", "text=Find similar work"})
	if err == nil {
		t.Fatal("invalid EMBED vector error = nil, want typed codec failure")
	}
	if stdout != "" {
		t.Fatalf("invalid EMBED vector stdout = %q, want empty", stdout)
	}
	requireEmbedTypedDiagnostic(t, stderr, "EMBED backend response is malformed")
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMalformedResponse {
		t.Fatalf("invalid EMBED vector error = %v, failure = %#v, want malformed response", err, failure)
	}
	failureCalls := fixture.Calls()

	fixture.SetResponse(models.EmbeddingBackendResponse{
		Embeddings: []float64{0.1, 0.2, 0.3, 0.4},
	})
	stdout, stderr, err = runStory004CLI(t, process, factoryDir, environment,
		[]string{"you", "models", "invoke", "embed", "--input", "text=Find similar work"})
	if err != nil || stdout != `[0.1,0.2,0.3,0.4]` || stderr != "" {
		t.Fatalf("EMBED after invalid vector = err %v stdout %q stderr %q, want released successful invocation", err, stdout, stderr)
	}
	if fixture.Calls() != failureCalls+1 {
		t.Fatalf("fixture calls after invalid vector = %d, want failed call followed by one successful call", fixture.Calls())
	}
}

func assertStory004PlainOutput(t *testing.T, err error, stdout, stderr string) {
	t.Helper()
	if err != nil {
		t.Fatalf("documented EMBED command error = %v\nstdout=%q\nstderr=%q", err, stdout, stderr)
	}
	if stdout != `[0.1,0.2,0.3,0.4]` || stderr != "" {
		t.Fatalf("documented EMBED command streams = stdout %q stderr %q, want vector and empty stderr", stdout, stderr)
	}
}

func requireEmbedTypedDiagnostic(t testing.TB, stderr, wantMessage string) factoryapi.ErrorResponse {
	t.Helper()
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &response); err != nil {
		t.Fatalf("decode typed EMBED diagnostic: %v\nstderr=%q", err, stderr)
	}
	if response.Code != factoryapi.ErrorResponseCode("MODEL_BACKEND_FAILURE") ||
		response.Family != factoryapi.ErrorFamilyInternalServerError ||
		!strings.Contains(response.Message, wantMessage) {
		t.Fatalf("typed EMBED diagnostic = %#v, want backend failure containing %q", response, wantMessage)
	}
	return response
}

func TestModelsEmbedCacheMissThenHitAvoidsNetworkThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	hostServer := story004HostServer(t)
	home := functionalTempDir(t)
	modelBody := []byte("story-004-embedding-model-download")
	backendBody := []byte("story-004-localai-backend-download")
	selection := story004EmbedBackendSelection(backendBody)
	assetFixture := &story004EmbedAssetHTTP{
		modelBody:   modelBody,
		backendBody: backendBody,
		selection:   selection,
	}
	launcher := &recordingModelHostLauncher{endpoint: hostServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	fixture := newStory004EmbedFixture()
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	process := functionalBuildProcess(t, story004EmbedEdges(
		home, assetFixture, hostServer.Client(), launcher, protocol, compatibility,
		selection, fixture,
	))
	support.CleanupProcess(t, process)
	environment := functionalHomeEnvironment(home)

	stdout, stderr, err := runStory004CLI(t, process, factoryDir, environment,
		[]string{"you", "models", "invoke", "embed", "--input", "text=Find similar work"})
	wantEstimate := "models asset estimate modelName=\"embed\" backendBytes=34 modelBytes=34 totalBytes=68\n"
	if err != nil || stdout != `[0.1,0.2,0.3,0.4]` || stderr != wantEstimate {
		t.Fatalf("EMBED cache-miss invocation = err %v stdout %q stderr %q; want estimate %q", err, stdout, stderr, wantEstimate)
	}
	missCalls := assetFixture.Calls()
	if missCalls < 3 {
		t.Fatalf("EMBED cache-miss asset calls = %d, want manifest, model, and backend exchanges", missCalls)
	}
	if !assetFixture.SawPathSuffix("/models/Qwen/Qwen3-Embedding-0.6B-GGUF") ||
		!assetFixture.SawPathSuffix("/Qwen/Qwen3-Embedding-0.6B-GGUF/resolve/") ||
		!assetFixture.SawPathSuffix("/"+selection.Name) {
		t.Fatalf("EMBED cache-miss asset paths = %#v, want manifest, model, and pinned backend paths", assetFixture.URLs())
	}

	stdout, stderr, err = runStory004CLI(t, process, factoryDir, environment,
		[]string{"you", "models", "invoke", "embed", "--input", "text=Find similar work"})
	if err != nil || stdout != `[0.1,0.2,0.3,0.4]` || stderr != "" {
		t.Fatalf("EMBED cache-hit invocation = err %v stdout %q stderr %q", err, stdout, stderr)
	}
	if got := assetFixture.Calls(); got != missCalls {
		t.Fatalf("EMBED cache-hit asset calls = %d, want unchanged %d", got, missCalls)
	}
}

func TestModelsEmbedHTTPParityUsesTheSameFixtureThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	hostServer := story004HostServer(t)
	home := functionalTempDir(t)
	backendBody := []byte("story-004-localai-backend-http")
	selection := story004EmbedBackendSelection(backendBody)
	writeGenericBuiltinModelCache(t, home, story004EmbedSource)
	writeGenericBackendCache(t, home, "localai-llamacpp", selection, backendBody)
	assetNetwork := &rejectingModelAssetHTTP{}
	launcher := &recordingModelHostLauncher{endpoint: hostServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	fixture := newStory004EmbedFixture()
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	server := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env:                       functionalHomeEnvironment(home),
		Edges: story004EmbedEdges(
			home, assetNetwork, hostServer.Client(), launcher, protocol, compatibility,
			selection, fixture,
		),
	})

	textValue := "Find similar work"
	inputs := []factoryapi.ModelInvocationInput{{
		Name: "text", Modality: factoryapi.ModelInvocationContentTypeText, Content: &textValue,
	}}
	response := postFunctionalJSON[factoryapi.GenericModelInvocationResponse](
		t, server.URL()+"/models/invocations",
		factoryapi.GenericModelInvocationRequest{
			Scope: "factory-session:caller-supplied", Holder: "functional-embed-http",
			Model: factoryapi.ModelReference{NameOrUri: "embed"}, Inputs: &inputs,
		},
		"POST /models/invocations EMBED",
	)
	if len(response.Outputs) != 1 || response.Outputs[0].Name != "embedding" ||
		response.Outputs[0].Modality != factoryapi.ModelInvocationContentTypeJSON ||
		response.Outputs[0].Content == nil || *response.Outputs[0].Content != `[0.1,0.2,0.3,0.4]` {
		t.Fatalf("HTTP EMBED response = %#v, want one named JSON vector", response)
	}
	if assetNetwork.Calls() != 0 {
		t.Fatalf("HTTP EMBED cache-hit asset network calls = %d, want 0", assetNetwork.Calls())
	}
	exchanges := fixture.Exchanges()
	if len(exchanges) != 1 || exchanges[0].ProtocolJSON != `{"prompt":"Find similar work"}` {
		t.Fatalf("HTTP EMBED fixture exchanges = %#v, want one canonical protocol request", exchanges)
	}
}

func runStory004CLI(
	t testing.TB,
	process support.Process,
	factoryDir string,
	environment []string,
	args []string,
) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	inputs := support.FakeInputs(context.Background(), args)
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = factoryDir
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	err := process.Execute(inputs.Input)
	return stdout.String(), stderr.String(), err
}

func story004HostServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := functionalNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(server.Close)
	return server
}

func story004EmbedBackendSelection(body []byte) serviceedges.ModelBackendArtifactSelection {
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	return serviceedges.ModelBackendArtifactSelection{
		Name:     "localai-backend-localai-llamacpp-functional.tar.gz",
		Location: "https://github.com/portpowered/infinite-you/releases/download/lmx-v1-embeddings-end-to-end/localai-backend-localai-llamacpp-functional.tar.gz",
		Bytes:    int64(len(body)),
		SHA256:   digest,
	}
}

func story004EmbedEdges(
	home string,
	assetHTTP interface {
		Do(*http.Request) (*http.Response, error)
	},
	hostHTTP interface {
		Do(*http.Request) (*http.Response, error)
	},
	launcher *recordingModelHostLauncher,
	protocol *joinedProtocolNegotiator,
	compatibility *joinedCompatibilityChecker,
	selection serviceedges.ModelBackendArtifactSelection,
	fixture *story004EmbedFixture,
) serviceedges.Edges {
	assetFiles := functionalModelAssetFileSystem{home: home}
	return serviceedges.Edges{
		ModelAssetHTTPClient:           assetHTTP,
		ModelAssetMakeDirectories:      assetFiles.MkdirAll,
		ModelAssetInspectPath:          assetFiles.Stat,
		ModelAssetResolveHomeDirectory: assetFiles.UserHomeDir,
		ModelAssetResolveEnvironment:   func(string) string { return "" },
		ModelAssetWriteFile:            assetFiles.WriteFile,
		ModelAssetRenamePath:           assetFiles.Rename,
		ModelAssetRemovePath:           assetFiles.Remove,
		ModelAssetReadFile:             assetFiles.ReadFile,
		ModelAssetReadDirectory:        assetFiles.ReadDir,
		ModelAssetCreateFile:           assetFiles.Create,
		ModelAssetOpenFile:             assetFiles.Open,
		ModelAssetHostPlatform:         models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelHostProcessLauncher:       launcher,
		ModelHostHTTPClient:            hostHTTP,
		ModelHostProtocolNegotiator:    protocol,
		ModelHostCompatibilityChecker:  compatibility,
		ModelRuntimeHTTPClient:         hostHTTP,
		ModelResolveBackendArtifact: func(
			_ context.Context,
			request serviceedges.ModelBackendArtifactSelectionRequest,
		) (serviceedges.ModelBackendArtifactSelection, error) {
			if request.Backend != "localai-llamacpp" || request.ProtocolVersion != "localai-backend-v1" {
				return serviceedges.ModelBackendArtifactSelection{}, fmt.Errorf("unexpected EMBED backend selection request")
			}
			return selection, nil
		},
		ModelEmbeddingBackend: fixture.InvokeEmbedding,
	}
}

type story004EmbedFixture struct {
	mu        sync.Mutex
	response  models.EmbeddingBackendResponse
	failure   error
	exchanges []story004EmbedExchange
}

type story004EmbedExchange struct {
	Model        string
	Operation    string
	ProtocolJSON string
}

func newStory004EmbedFixture() *story004EmbedFixture {
	return &story004EmbedFixture{response: models.EmbeddingBackendResponse{
		Embeddings: []float64{0.1, 0.2, 0.3, 0.4},
	}}
}

func (fixture *story004EmbedFixture) InvokeEmbedding(
	ctx context.Context,
	request models.EmbeddingBackendRequest,
) (models.EmbeddingBackendResponse, error) {
	if err := ctx.Err(); err != nil {
		return models.EmbeddingBackendResponse{}, err
	}

	protocolRequest := struct {
		Prompt     string         `json:"prompt"`
		Parameters map[string]any `json:"parameters,omitempty"`
	}{Prompt: request.Text, Parameters: request.Parameters}
	encoded, err := json.Marshal(protocolRequest)
	if err != nil {
		return models.EmbeddingBackendResponse{}, fmt.Errorf("encode fixture protocol request: %w", err)
	}

	fixture.mu.Lock()
	fixture.exchanges = append(fixture.exchanges, story004EmbedExchange{
		Model: "embed", Operation: models.OperationEMBED, ProtocolJSON: string(encoded),
	})
	failure := fixture.failure
	response := models.EmbeddingBackendResponse{
		Embeddings: append([]float64(nil), fixture.response.Embeddings...),
	}
	fixture.mu.Unlock()
	if failure != nil {
		return models.EmbeddingBackendResponse{}, failure
	}
	return response, nil
}

func (fixture *story004EmbedFixture) SetFailure(failure error) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.failure = failure
}

func (fixture *story004EmbedFixture) SetResponse(response models.EmbeddingBackendResponse) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.response = models.EmbeddingBackendResponse{
		Embeddings: append([]float64(nil), response.Embeddings...),
	}
}

func (fixture *story004EmbedFixture) Calls() int {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return len(fixture.exchanges)
}

func (fixture *story004EmbedFixture) Exchanges() []story004EmbedExchange {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return append([]story004EmbedExchange(nil), fixture.exchanges...)
}

type story004EmbedAssetHTTP struct {
	mu          sync.Mutex
	modelBody   []byte
	backendBody []byte
	selection   serviceedges.ModelBackendArtifactSelection
	urls        []string
}

func (client *story004EmbedAssetHTTP) Do(request *http.Request) (*http.Response, error) {
	client.mu.Lock()
	client.urls = append(client.urls, request.URL.String())
	client.mu.Unlock()
	if strings.HasSuffix(request.URL.Path, "/models/Qwen/Qwen3-Embedding-0.6B-GGUF") {
		digest := fmt.Sprintf("%x", sha256.Sum256(client.modelBody))
		manifest := map[string]any{
			"sha": "370f27d7550e0def9b39c1f16d3fbaa13aa67728",
			"siblings": []map[string]any{{
				"rfilename": "Qwen3-Embedding-0.6B-f16.gguf",
				"size":      len(client.modelBody),
				"lfs":       map[string]any{"oid": digest, "size": len(client.modelBody)},
			}},
		}
		body, err := json.Marshal(manifest)
		if err != nil {
			return nil, err
		}
		return story004HTTPResponse(request, http.StatusOK, "application/json", body), nil
	}
	if strings.Contains(request.URL.Path, "/Qwen/Qwen3-Embedding-0.6B-GGUF/resolve/") {
		return story004HTTPResponse(request, http.StatusOK, "application/octet-stream", client.modelBody), nil
	}
	if strings.HasSuffix(request.URL.Path, "/"+client.selection.Name) {
		return story004HTTPResponse(request, http.StatusOK, "application/octet-stream", client.backendBody), nil
	}
	return story004HTTPResponse(request, http.StatusNotFound, "text/plain", []byte("fixture asset not found")), nil
}

func story004HTTPResponse(
	request *http.Request,
	status int,
	contentType string,
	body []byte,
) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{contentType}},
		Request:    request,
	}
}

func (client *story004EmbedAssetHTTP) Calls() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return len(client.urls)
}

func (client *story004EmbedAssetHTTP) URLs() []string {
	client.mu.Lock()
	defer client.mu.Unlock()
	return append([]string(nil), client.urls...)
}

func (client *story004EmbedAssetHTTP) SawPathSuffix(suffix string) bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	for _, value := range client.urls {
		if strings.Contains(value, suffix) {
			return true
		}
	}
	return false
}
