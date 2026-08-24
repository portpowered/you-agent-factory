package root_composition_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

const story004EmbedSource = "hf://Qwen/Qwen3-Embedding-0.6B@97b0c614be4d77ee51c0cef4e5f07c00f9eb65b3"

func TestModelsEmbedZeroConfigurationJourneyThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	hostServer := story004HostServer(t)
	home := t.TempDir()
	backendBody := []byte("story-004-localai-backend")
	selection := story004EmbedBackendSelection(backendBody)
	writeGenericBuiltinModelCache(t, home, story004EmbedSource)
	writeGenericBackendCache(t, home, "localai-llamacpp", selection, backendBody)

	assetNetwork := &rejectingModelAssetHTTP{}
	launcher := &recordingModelHostLauncher{endpoint: hostServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	fixture := newStory004EmbedFixture()
	factoryDir := support.ScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	process := support.BuildProcess(t, story004EmbedEdges(
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
	support.RequireSafeCLIDiagnostic(t, stderr)
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

func assertStory004PlainOutput(t *testing.T, err error, stdout, stderr string) {
	t.Helper()
	if err != nil {
		t.Fatalf("documented EMBED command error = %v\nstdout=%q\nstderr=%q", err, stdout, stderr)
	}
	if stdout != `[0.1,0.2,0.3,0.4]` || stderr != "" {
		t.Fatalf("documented EMBED command streams = stdout %q stderr %q, want vector and empty stderr", stdout, stderr)
	}
}

func TestModelsEmbedCacheMissThenHitAvoidsNetworkThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	hostServer := story004HostServer(t)
	home := t.TempDir()
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
	factoryDir := support.ScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	process := support.BuildProcess(t, story004EmbedEdges(
		home, assetFixture, hostServer.Client(), launcher, protocol, compatibility,
		selection, fixture,
	))
	support.CleanupProcess(t, process)
	environment := functionalHomeEnvironment(home)

	stdout, stderr, err := runStory004CLI(t, process, factoryDir, environment,
		[]string{"you", "models", "invoke", "embed", "--input", "text=Find similar work"})
	if err != nil || stdout != `[0.1,0.2,0.3,0.4]` || stderr != "" {
		t.Fatalf("EMBED cache-miss invocation = err %v stdout %q stderr %q", err, stdout, stderr)
	}
	missCalls := assetFixture.Calls()
	if missCalls < 3 {
		t.Fatalf("EMBED cache-miss asset calls = %d, want manifest, model, and backend exchanges", missCalls)
	}
	if !assetFixture.SawPathSuffix("/models/Qwen/Qwen3-Embedding-0.6B") ||
		!assetFixture.SawPathSuffix("/Qwen/Qwen3-Embedding-0.6B/resolve/") ||
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
	home := t.TempDir()
	backendBody := []byte("story-004-localai-backend-http")
	selection := story004EmbedBackendSelection(backendBody)
	writeGenericBuiltinModelCache(t, home, story004EmbedSource)
	writeGenericBackendCache(t, home, "localai-llamacpp", selection, backendBody)
	assetNetwork := &rejectingModelAssetHTTP{}
	launcher := &recordingModelHostLauncher{endpoint: hostServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	fixture := newStory004EmbedFixture()
	factoryDir := support.ScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
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
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
		ModelInvocationBackend: fixture.Invoke,
	}
}

type story004EmbedFixture struct {
	mu        sync.Mutex
	response  []byte
	failure   error
	exchanges []story004EmbedExchange
}

type story004EmbedExchange struct {
	Model        string
	Operation    string
	ProtocolJSON string
}

func newStory004EmbedFixture() *story004EmbedFixture {
	return &story004EmbedFixture{response: []byte(`{"embeddings":[0.1,0.2,0.3,0.4]}`)}
}

func (fixture *story004EmbedFixture) Invoke(
	ctx context.Context,
	request models.InvokeModelRequest,
) ([]models.InferenceContent, []models.InferenceArtifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if request.Model.NameOrURI != "" && !strings.EqualFold(request.Model.NameOrURI, "embed") {
		return nil, nil, fmt.Errorf("fixture received model %q", request.Model.NameOrURI)
	}
	if request.Operation != models.OperationEMBED {
		return nil, nil, fmt.Errorf("fixture received operation %q", request.Operation)
	}

	protocolRequest := struct {
		Prompt     string         `json:"prompt"`
		Parameters map[string]any `json:"parameters,omitempty"`
	}{Parameters: map[string]any{}}
	for _, input := range request.Inputs {
		switch input.Name {
		case "text":
			if protocolRequest.Prompt != "" {
				return nil, nil, fmt.Errorf("fixture received repeated text input")
			}
			protocolRequest.Prompt = input.Content
		case "parameters":
			var parameters map[string]any
			if err := json.Unmarshal([]byte(input.Content), &parameters); err != nil {
				return nil, nil, fmt.Errorf("fixture received invalid parameter JSON: %w", err)
			}
			for name, value := range parameters {
				protocolRequest.Parameters[name] = value
			}
		default:
			return nil, nil, fmt.Errorf("fixture received unknown input %q", input.Name)
		}
	}
	if protocolRequest.Prompt == "" {
		return nil, nil, fmt.Errorf("fixture received empty text input")
	}
	if len(protocolRequest.Parameters) == 0 {
		protocolRequest.Parameters = nil
	}
	encoded, err := json.Marshal(protocolRequest)
	if err != nil {
		return nil, nil, fmt.Errorf("encode fixture protocol request: %w", err)
	}

	fixture.mu.Lock()
	fixture.exchanges = append(fixture.exchanges, story004EmbedExchange{
		Model: request.Model.NameOrURI, Operation: request.Operation, ProtocolJSON: string(encoded),
	})
	failure := fixture.failure
	response := append([]byte(nil), fixture.response...)
	fixture.mu.Unlock()
	if failure != nil {
		return nil, nil, failure
	}
	var protocolResponse struct {
		Embeddings []float64 `json:"embeddings"`
	}
	if err := json.Unmarshal(response, &protocolResponse); err != nil || len(protocolResponse.Embeddings) == 0 {
		if err == nil {
			err = errors.New("fixture returned no embeddings")
		}
		return nil, nil, fmt.Errorf("decode fixture protocol response: %w", err)
	}
	content, err := json.Marshal(protocolResponse.Embeddings)
	if err != nil {
		return nil, nil, fmt.Errorf("encode fixture embedding output: %w", err)
	}
	return []models.InferenceContent{{
		Name: "embedding", Modality: models.ModalityJSON,
		ContentType: "application/json", MediaType: "application/json", Content: string(content),
	}}, nil, nil
}

func (fixture *story004EmbedFixture) SetFailure(failure error) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.failure = failure
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
	if strings.HasSuffix(request.URL.Path, "/models/Qwen/Qwen3-Embedding-0.6B") {
		digest := fmt.Sprintf("%x", sha256.Sum256(client.modelBody))
		manifest := map[string]any{
			"sha": "97b0c614be4d77ee51c0cef4e5f07c00f9eb65b3",
			"siblings": []map[string]any{{
				"rfilename": "Qwen3-Embedding-0.6B.gguf",
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
	if strings.Contains(request.URL.Path, "/Qwen/Qwen3-Embedding-0.6B/resolve/") {
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
