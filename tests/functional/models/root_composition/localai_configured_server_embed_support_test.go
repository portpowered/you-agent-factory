package root_composition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

type localAIConfiguredEmbedTypedBackend struct {
	mu        sync.Mutex
	fixture   *localai.Fixture
	nonFinite bool
	requests  []models.EmbeddingBackendRequest
}

func (backend *localAIConfiguredEmbedTypedBackend) Invoke(
	ctx context.Context,
	request models.EmbeddingBackendRequest,
) (models.EmbeddingBackendResponse, error) {
	backend.mu.Lock()
	backend.requests = append(backend.requests, models.EmbeddingBackendRequest{
		Text: request.Text, Parameters: cloneLocalAIConfiguredEmbedParameters(request.Parameters),
	})
	fixture := backend.fixture
	nonFinite := backend.nonFinite
	backend.mu.Unlock()
	if fixture == nil {
		return models.EmbeddingBackendResponse{}, fmt.Errorf("configured EMBED typed fixture is nil")
	}
	outputs, _, err := fixture.InvocationBackend(ctx, models.InvokeModelRequest{
		Model:     models.ModelReference{NameOrURI: models.BuiltInModelNameEmbed},
		Operation: models.OperationEMBED,
		Inputs: []models.InferenceInput{{
			Name: "text", Modality: models.ModalityText,
			ContentType: "text/plain", MediaType: "text/plain", Content: request.Text,
		}},
	})
	if err != nil {
		return models.EmbeddingBackendResponse{}, err
	}
	if nonFinite {
		return models.EmbeddingBackendResponse{Embeddings: []float64{math.NaN(), 0.2}}, nil
	}
	if len(outputs) != 1 {
		return models.EmbeddingBackendResponse{}, fmt.Errorf("configured EMBED typed fixture returned %d outputs", len(outputs))
	}
	var values []float64
	if err := json.Unmarshal([]byte(outputs[0].Content), &values); err != nil {
		return models.EmbeddingBackendResponse{}, err
	}
	return models.EmbeddingBackendResponse{Embeddings: values}, nil
}

func cloneLocalAIConfiguredEmbedParameters(parameters map[string]any) map[string]any {
	if parameters == nil {
		return nil
	}
	cloned := make(map[string]any, len(parameters))
	for key, value := range parameters {
		cloned[key] = value
	}
	return cloned
}

func (backend *localAIConfiguredEmbedTypedBackend) Requests() []models.EmbeddingBackendRequest {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	requests := make([]models.EmbeddingBackendRequest, len(backend.requests))
	for index, request := range backend.requests {
		requests[index] = models.EmbeddingBackendRequest{
			Text: request.Text, Parameters: cloneLocalAIConfiguredEmbedParameters(request.Parameters),
		}
	}
	return requests
}

func (backend *localAIConfiguredEmbedTypedBackend) Signatures() []string {
	requests := backend.Requests()
	result := make([]string, 0, len(requests))
	for _, request := range requests {
		encoded, err := json.Marshal(request)
		if err != nil {
			continue
		}
		result = append(result, string(encoded))
	}
	return result
}

func assertLocalAIConfiguredEmbedFixtureCalls(t *testing.T, calls []localai.Call, wantEmbeddings int) {
	t.Helper()
	embeddings := 0
	for _, call := range calls {
		if call.Method != "Embedding" {
			continue
		}
		embeddings++
		if call.Prompt != localAIConfiguredEmbedPrompt {
			t.Fatalf("configured EMBED fixture prompt = %q, want %q", call.Prompt, localAIConfiguredEmbedPrompt)
		}
	}
	if embeddings != wantEmbeddings {
		t.Fatalf("configured EMBED fixture embedding calls = %d, want %d (all calls=%#v)", embeddings, wantEmbeddings, calls)
	}
}

type localAIConfiguredEmbedInvokeResult struct {
	Observation localAIConfiguredEmbedObservation
	Response    factoryapi.GenericModelInvocationResponse
	Stdout      string
	Stderr      string
	Err         error
}

type localAIConfiguredEmbedObservation struct {
	Name        string
	Modality    factoryapi.ModelInvocationContentType
	ContentType string
	MediaType   string
	Content     string
}

func executeLocalAIConfiguredEmbedInvoke(
	t *testing.T,
	process support.Process,
	factoryDir string,
	environment []string,
	serverURL string,
	offline bool,
) localAIConfiguredEmbedInvokeResult {
	t.Helper()
	return executeLocalAIConfiguredEmbedCommand(
		t, process, factoryDir, environment, serverURL, offline,
		localAIConfiguredEmbedInputSpecs(t, true), nil,
	)
}

func executeLocalAIConfiguredEmbedCommand(
	t *testing.T,
	process support.Process,
	factoryDir string,
	environment []string,
	serverURL string,
	offline bool,
	inputSpecs []string,
	parameterSpecs []string,
) localAIConfiguredEmbedInvokeResult {
	t.Helper()
	args := []string{"you", "--json"}
	if strings.TrimSpace(serverURL) != "" {
		args = append(args, "--server", strings.TrimSuffix(serverURL, "/"))
	}
	args = append(args, "models", "invoke", models.BuiltInModelNameEmbed)
	if offline {
		args = append(args, "--offline")
	}
	args = append(args,
		"--operation", models.OperationEMBED,
	)
	for _, inputSpec := range inputSpecs {
		args = append(args, "--input", inputSpec)
	}
	for _, parameterSpec := range parameterSpecs {
		args = append(args, "--parameter", parameterSpec)
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append([]string(nil), environment...)
	inputs.Input.WorkingDirectory = factoryDir
	var stdout, stderr bytes.Buffer
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	invokeErr := process.Execute(inputs.Input)
	result := localAIConfiguredEmbedInvokeResult{
		Stdout: stdout.String(), Stderr: stderr.String(), Err: invokeErr,
	}
	if strings.TrimSpace(result.Stdout) != "" {
		if err := json.Unmarshal([]byte(result.Stdout), &result.Response); err != nil {
			t.Fatalf("decode configured EMBED response: %v\nstdout=%s", err, result.Stdout)
		}
		if len(result.Response.Outputs) == 1 {
			output := result.Response.Outputs[0]
			if output.Content == nil || output.ContentType == nil || output.MediaType == nil {
				t.Fatalf("configured EMBED output metadata = %#v, want content and existing media metadata", output)
			}
			result.Observation = localAIConfiguredEmbedObservation{
				Name: output.Name, Modality: output.Modality,
				ContentType: *output.ContentType, MediaType: *output.MediaType, Content: *output.Content,
			}
		}
	}
	return result
}

func localAIConfiguredEmbedInputSpecs(t *testing.T, includeText bool) []string {
	t.Helper()
	result := make([]string, 0, 2)
	if includeText {
		textInput, err := json.Marshal(map[string]string{
			"name": "text", "modality": "TEXT", "contentType": "text/plain",
			"mediaType": "text/plain", "content": localAIConfiguredEmbedPrompt,
		})
		if err != nil {
			t.Fatalf("marshal configured EMBED text input: %v", err)
		}
		result = append(result, string(textInput))
	}
	parametersInput, err := json.Marshal(map[string]string{
		"name": "parameters", "modality": "JSON", "contentType": "application/json",
		"mediaType": "application/json", "content": localAIConfiguredEmbedParameters,
	})
	if err != nil {
		t.Fatalf("marshal configured EMBED parameters input: %v", err)
	}
	return append(result, string(parametersInput))
}

func assertLocalAIConfiguredEmbedSuccess(
	t *testing.T,
	result localAIConfiguredEmbedInvokeResult,
	path string,
) {
	t.Helper()
	if result.Err != nil {
		t.Fatalf("%s configured EMBED error = %v\nstderr=%q", path, result.Err, result.Stderr)
	}
	if result.Stderr != "" {
		t.Fatalf("%s configured EMBED stderr = %q, want empty", path, result.Stderr)
	}
	if result.Response.Failure != nil || len(result.Response.Outputs) != 1 {
		t.Fatalf("%s configured EMBED response = %#v, want one successful output", path, result.Response)
	}
	if result.Observation.Name != "embedding" ||
		result.Observation.Modality != factoryapi.ModelInvocationContentTypeJSON ||
		result.Observation.ContentType != "application/json" ||
		result.Observation.MediaType != "application/json" {
		t.Fatalf("%s configured EMBED observation metadata = %#v, want named JSON output with existing metadata", path, result.Observation)
	}
	var values []float64
	if err := json.Unmarshal([]byte(result.Observation.Content), &values); err != nil {
		t.Fatalf("%s configured EMBED vector JSON: %v", path, err)
	}
	want := []float64{0.1, 0.2, 0.3, 0.4, 0.5}
	if len(values) != len(want) {
		t.Fatalf("%s configured EMBED vector dimensions = %d, want %d", path, len(values), len(want))
	}
	for index, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) || value != want[index] {
			t.Fatalf("%s configured EMBED vector[%d] = %v, want finite %v", path, index, value, want[index])
		}
	}
}

func prepareLocalAIConfiguredEmbedProfile(
	t *testing.T,
	home string,
) (*localAIRevisionRecorder, *localAIBackendSelectionRecorder) {
	t.Helper()
	writeLocalAIEmbedModelOverlay(t, home, localAIConfiguredEmbedOperatorSource)
	writeGenericBuiltinModelCache(t, home, localAIConfiguredEmbedOperatorSource+"@"+localAIConfiguredEmbedRevision)
	selection := genericLlamaBackendSelection()
	writeGenericBackendCache(t, home, "localai-llamacpp", selection, []byte("localai-llamacpp/linux-amd64"))
	return &localAIRevisionRecorder{revision: localAIConfiguredEmbedRevision}, &localAIBackendSelectionRecorder{selection: selection}
}

func writeLocalAIEmbedModelOverlay(t *testing.T, home, source string) {
	t.Helper()
	configPath := filepath.Join(home, ".you-agent-factory", "config.json")
	config, err := json.Marshal(map[string]any{
		"models": map[string]any{
			models.BuiltInModelNameEmbed: map[string]any{"source": source},
		},
	})
	if err != nil {
		t.Fatalf("marshal configured EMBED operator config: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create configured EMBED operator config directory: %v", err)
	}
	if err := os.WriteFile(configPath, config, 0o600); err != nil {
		t.Fatalf("write configured EMBED operator config: %v", err)
	}
}

func localAIConfiguredEmbedEdges(
	home string,
	fixture *localai.Fixture,
	resolver *localAIRevisionRecorder,
	backendSelection *localAIBackendSelectionRecorder,
	launcher interface {
		Start(context.Context, serviceedges.HostProcessStartSpec) (interface {
			HealthEndpoint() string
			Wait() error
			Stop(context.Context) error
		}, error)
	},
	invocation *localAIConfiguredEmbedInvocationRecorder,
) (serviceedges.Edges, *rejectingModelAssetHTTP, *joinedCompatibilityChecker, *recordingModelHostLauncher) {
	edges, network, compatibility, defaultLauncher := localAIConformanceEdges(home, fixture)
	if launcher != nil {
		edges.ModelHostProcessLauncher = launcher
	}
	if resolver != nil {
		edges.ModelResolveHuggingFaceRevision = resolver.Resolve
	}
	if backendSelection != nil {
		edges.ModelResolveBackendArtifact = backendSelection.Resolve
	}
	if invocation != nil {
		edges.ModelInvocationBackend = invocation.Invoke
	}
	return edges, network, compatibility, defaultLauncher
}

type localAIConfiguredEmbedInvocationRecorder struct {
	mu       sync.Mutex
	next     serviceedges.ModelInvocationBackend
	requests []localAIConfiguredEmbedRequestObservation
	failure  error
}

type localAIConfiguredEmbedRequestObservation struct {
	Model      string                                   `json:"model"`
	Operation  string                                   `json:"operation"`
	Inputs     []localAIConfiguredEmbedInputObservation `json:"inputs"`
	Parameters []models.OperationParameter              `json:"parameters,omitempty"`
}

type localAIConfiguredEmbedInputObservation struct {
	Name        string          `json:"name"`
	Modality    models.Modality `json:"modality"`
	ContentType string          `json:"contentType"`
	MediaType   string          `json:"mediaType"`
	Content     string          `json:"content"`
}

func (recorder *localAIConfiguredEmbedInvocationRecorder) Invoke(
	ctx context.Context,
	request models.InvokeModelRequest,
) ([]models.InferenceContent, []models.InferenceArtifact, error) {
	inputs := make([]localAIConfiguredEmbedInputObservation, 0, len(request.Inputs))
	for _, input := range request.Inputs {
		inputs = append(inputs, localAIConfiguredEmbedInputObservation{
			Name: input.Name, Modality: input.Modality, ContentType: input.ContentType,
			MediaType: input.MediaType, Content: input.Content,
		})
	}
	parameters := make([]models.OperationParameter, 0, len(request.Parameters))
	for _, parameter := range request.Parameters {
		parameters = append(parameters, parameter.Clone())
	}
	observation := localAIConfiguredEmbedRequestObservation{
		Model: request.Model.NameOrURI, Operation: request.Operation,
		Inputs: inputs, Parameters: parameters,
	}
	recorder.mu.Lock()
	recorder.requests = append(recorder.requests, observation)
	next := recorder.next
	recorder.mu.Unlock()
	if next == nil {
		return nil, nil, fmt.Errorf("configured EMBED invocation recorder has no backend")
	}
	content, artifacts, err := next(ctx, request)
	if err != nil {
		return nil, nil, err
	}
	recorder.mu.Lock()
	failure := recorder.failure
	recorder.failure = nil
	recorder.mu.Unlock()
	if failure != nil {
		return nil, nil, failure
	}
	return content, artifacts, nil
}

func (recorder *localAIConfiguredEmbedInvocationRecorder) FailNext(err error) {
	recorder.mu.Lock()
	recorder.failure = err
	recorder.mu.Unlock()
}

func (recorder *localAIConfiguredEmbedInvocationRecorder) Signatures() []string {
	recorder.mu.Lock()
	requests := append([]localAIConfiguredEmbedRequestObservation(nil), recorder.requests...)
	recorder.mu.Unlock()
	signatures := make([]string, 0, len(requests))
	for _, request := range requests {
		encoded, err := json.Marshal(request)
		if err != nil {
			continue
		}
		signatures = append(signatures, string(encoded))
	}
	return signatures
}

func assertLocalAIConfiguredEmbedSelection(
	t *testing.T,
	home string,
	resolver *localAIRevisionRecorder,
	backendSelection *localAIBackendSelectionRecorder,
	launcher interface {
		Starts() int
		Stops() int
		Waits() int
		Active() bool
		LastSpec() (serviceedges.HostProcessStartSpec, bool)
	},
	network *rejectingModelAssetHTTP,
) {
	t.Helper()
	sources := resolver.Sources()
	if len(sources) == 0 {
		t.Fatal("configured EMBED revision resolver recorded no source")
	}
	for _, source := range sources {
		if source != localAIConfiguredEmbedOperatorSource {
			t.Fatalf("configured EMBED resolved source = %q, want operator override %q", source, localAIConfiguredEmbedOperatorSource)
		}
	}
	requests := backendSelection.Requests()
	if len(requests) == 0 {
		t.Fatal("configured EMBED backend selection recorded no request")
	}
	for _, request := range requests {
		if request.Backend != "localai-llamacpp" {
			t.Fatalf("configured EMBED backend selection = %#v, want localai-llamacpp", request)
		}
	}
	spec, ok := launcher.LastSpec()
	if !ok || spec.Backend != "localai-llamacpp" || strings.TrimSpace(spec.ModelPath) == "" {
		t.Fatalf("configured EMBED host spec = %#v, want selected operator model and backend", spec)
	}
	cacheRoot := filepath.Join(home, ".agent-factory", "models")
	if !pathWithinLocalAIRoot(cacheRoot, spec.ModelPath) {
		t.Fatalf("configured EMBED host model path = %q, want inside %q", spec.ModelPath, cacheRoot)
	}
	if network.Calls() != 0 {
		t.Fatalf("configured EMBED model-asset network calls = %d, want zero", network.Calls())
	}
}
