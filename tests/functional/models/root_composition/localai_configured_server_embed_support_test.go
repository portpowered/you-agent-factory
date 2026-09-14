package root_composition_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
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
	warmup    bool
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
	warmup := backend.warmup
	backend.warmup = false
	backend.mu.Unlock()
	if warmup {
		return models.EmbeddingBackendResponse{Embeddings: []float64{0.1, 0.2, 0.3, 0.4, 0.5}}, nil
	}
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

func (backend *localAIConfiguredEmbedTypedBackend) WarmNext() {
	backend.mu.Lock()
	backend.warmup = true
	backend.mu.Unlock()
}

func (backend *localAIConfiguredEmbedTypedBackend) Reset() {
	backend.mu.Lock()
	backend.requests = nil
	backend.warmup = false
	backend.mu.Unlock()
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

func assertLocalAIConfiguredEmbedRequest(
	t *testing.T,
	path string,
	request localAIConfiguredEmbedRequestObservation,
) {
	t.Helper()
	if request.Model != models.BuiltInModelNameEmbed || request.Operation != models.OperationEMBED {
		t.Fatalf("%s configured EMBED request = %#v, want model=%q operation=%q", path, request, models.BuiltInModelNameEmbed, models.OperationEMBED)
	}
	if len(request.Inputs) != 2 {
		t.Fatalf("%s configured EMBED inputs = %#v, want ordered text and parameters inputs", path, request.Inputs)
	}
	wantInputs := []localAIConfiguredEmbedInputObservation{
		{
			Name: "text", Modality: models.ModalityText, ContentType: "text/plain",
			MediaType: "text/plain", Content: localAIConfiguredEmbedPrompt,
		},
		{
			Name: "parameters", Modality: models.ModalityJSON, ContentType: "application/json",
			MediaType: "application/json", Content: localAIConfiguredEmbedParameters,
		},
	}
	if !reflect.DeepEqual(request.Inputs, wantInputs) {
		t.Fatalf("%s configured EMBED inputs = %#v, want ordered %#v", path, request.Inputs, wantInputs)
	}
	if len(request.Parameters) != 0 {
		t.Fatalf("%s configured EMBED named parameters = %#v, want parameters input only", path, request.Parameters)
	}
	var parameters map[string]any
	if err := json.Unmarshal([]byte(request.Inputs[1].Content), &parameters); err != nil {
		t.Fatalf("decode %s configured EMBED parameters: %v", path, err)
	}
	if dimensions, ok := parameters["dimensions"].(float64); !ok || dimensions != 5 {
		t.Fatalf("%s configured EMBED dimensions = %#v, want 5", path, parameters["dimensions"])
	}
	if normalize, ok := parameters["normalize"].(bool); !ok || !normalize {
		t.Fatalf("%s configured EMBED normalize = %#v, want true", path, parameters["normalize"])
	}
}

type localAIConfiguredEmbedCacheSnapshot map[string]string

func snapshotLocalAIConfiguredEmbedCache(t *testing.T, home string) localAIConfiguredEmbedCacheSnapshot {
	t.Helper()
	root := filepath.Join(home, ".agent-factory", "models")
	snapshot := make(localAIConfiguredEmbedCacheSnapshot)
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		snapshot["<root>"] = "absent"
		return snapshot
	} else if err != nil {
		t.Fatalf("inspect configured EMBED cache root %q: %v", root, err)
	}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			snapshot[relative] = "directory:" + entry.Type().String()
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			snapshot[relative] = "symlink:" + target
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(body)
		snapshot[relative] = fmt.Sprintf("file:%d:%s:%x", info.Size(), info.Mode().String(), digest)
		return nil
	}); err != nil {
		t.Fatalf("snapshot configured EMBED cache root %q: %v", root, err)
	}
	return snapshot
}

func assertLocalAIConfiguredEmbedCacheUnchanged(
	t *testing.T,
	path string,
	before, after localAIConfiguredEmbedCacheSnapshot,
) {
	t.Helper()
	for label, snapshot := range map[string]localAIConfiguredEmbedCacheSnapshot{
		"before": before, "after": after,
	} {
		for name := range snapshot {
			normalized := filepath.ToSlash(name)
			if strings.Contains(normalized, ".partial") || strings.Contains(normalized, ".previous") {
				t.Fatalf("%s configured EMBED owner cache %s contains temporary artifact %q", path, label, name)
			}
		}
	}
	if !reflect.DeepEqual(
		localAIConfiguredEmbedPersistentCacheSnapshot(before),
		localAIConfiguredEmbedPersistentCacheSnapshot(after),
	) {
		t.Fatalf("%s configured EMBED owner cache changed across failure: before=%#v after=%#v", path, before, after)
	}
}

func localAIConfiguredEmbedPersistentCacheSnapshot(
	snapshot localAIConfiguredEmbedCacheSnapshot,
) localAIConfiguredEmbedCacheSnapshot {
	const lockRoot = ".you-asset-locks"
	persistent := make(localAIConfiguredEmbedCacheSnapshot, len(snapshot))
	for name, value := range snapshot {
		normalized := filepath.ToSlash(name)
		if normalized == lockRoot || strings.HasPrefix(normalized, lockRoot+"/") {
			// Invocation leases are owned by the live process and can remain
			// until Process.Close. They are checked by the lifecycle assertions;
			// persistent cache entries must still remain byte-for-byte stable.
			continue
		}
		persistent[name] = value
	}
	return persistent
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

func warmLocalAIConfiguredEmbedCache(
	t *testing.T,
	process support.Process,
	factoryDir string,
	environment []string,
	serverURL string,
	offline bool,
	invocation *localAIConfiguredEmbedInvocationRecorder,
) {
	t.Helper()
	invocation.WarmNext()
	result := executeLocalAIConfiguredEmbedInvoke(t, process, factoryDir, environment, serverURL, offline)
	assertLocalAIConfiguredEmbedSuccess(t, result, "configured EMBED cache warm-up")
	invocation.Reset()
	if result.Stderr != "" {
		t.Fatalf("configured EMBED cache warm-up stderr = %q, want empty", result.Stderr)
	}
}

func warmLocalAIConfiguredEmbedTypedCache(
	t *testing.T,
	process support.Process,
	factoryDir string,
	environment []string,
	serverURL string,
	offline bool,
	backend *localAIConfiguredEmbedTypedBackend,
) {
	t.Helper()
	backend.WarmNext()
	result := executeLocalAIConfiguredEmbedInvoke(t, process, factoryDir, environment, serverURL, offline)
	assertLocalAIConfiguredEmbedSuccess(t, result, "typed configured EMBED cache warm-up")
	backend.Reset()
	if result.Stderr != "" {
		t.Fatalf("typed configured EMBED cache warm-up stderr = %q, want empty", result.Stderr)
	}
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
	warmup   bool
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
	warmup := recorder.warmup
	recorder.warmup = false
	recorder.mu.Unlock()
	if warmup {
		return []models.InferenceContent{{
			Name:        "embedding",
			Modality:    models.ModalityJSON,
			ContentType: "application/json",
			MediaType:   "application/json",
			Content:     `[0.1,0.2,0.3,0.4,0.5]`,
		}}, nil, nil
	}
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

func (recorder *localAIConfiguredEmbedInvocationRecorder) WarmNext() {
	recorder.mu.Lock()
	recorder.warmup = true
	recorder.mu.Unlock()
}

func (recorder *localAIConfiguredEmbedInvocationRecorder) Reset() {
	recorder.mu.Lock()
	recorder.requests = nil
	recorder.failure = nil
	recorder.warmup = false
	recorder.mu.Unlock()
}

func (recorder *localAIConfiguredEmbedInvocationRecorder) Requests() []localAIConfiguredEmbedRequestObservation {
	recorder.mu.Lock()
	requests := append([]localAIConfiguredEmbedRequestObservation(nil), recorder.requests...)
	recorder.mu.Unlock()
	cloned := make([]localAIConfiguredEmbedRequestObservation, len(requests))
	for index, request := range requests {
		cloned[index] = request
		cloned[index].Inputs = append([]localAIConfiguredEmbedInputObservation(nil), request.Inputs...)
		cloned[index].Parameters = make([]models.OperationParameter, len(request.Parameters))
		for parameterIndex, parameter := range request.Parameters {
			cloned[index].Parameters[parameterIndex] = parameter.Clone()
		}
	}
	return cloned
}

func (recorder *localAIConfiguredEmbedInvocationRecorder) Signatures() []string {
	requests := recorder.Requests()
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
