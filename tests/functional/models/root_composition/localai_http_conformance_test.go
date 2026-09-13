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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/conformance"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

func startLocalAIConformanceServer(
	t *testing.T,
	dir string,
	home string,
	fixture *localai.Fixture,
) (*support.FunctionalAPIServer, *joinedCompatibilityChecker) {
	t.Helper()
	edges, rejectingNetwork, compatibility, _ := localAIConformanceEdges(home, fixture)
	server := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		ServerReadyTimeout:        60 * time.Second,
		Env:                       functionalHomeEnvironment(home),
		Edges:                     edges,
	})
	if rejectingNetwork.Calls() != 0 {
		t.Fatalf("model asset network calls during server startup = %d, want 0", rejectingNetwork.Calls())
	}
	return server, compatibility
}

func localAIConformanceEdges(
	home string,
	fixture *localai.Fixture,
) (serviceedges.Edges, *rejectingModelAssetHTTP, *joinedCompatibilityChecker, *recordingModelHostLauncher) {
	rejectingNetwork := &rejectingModelAssetHTTP{}
	assetFiles := functionalModelAssetFileSystem{home: home}
	hostLauncher := &recordingModelHostLauncher{endpoint: fixture.Endpoint()}
	compatibility := &joinedCompatibilityChecker{}
	return serviceedges.Edges{
		ModelAssetHTTPClient:           rejectingNetwork,
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
		ModelHostProcessLauncher:       hostLauncher,
		ModelHostHTTPClient:            fixtureHostHTTPClient{},
		ModelHostGRPCDialer:            fixture.GRPCDialer(),
		ModelHostCompatibilityChecker:  compatibility,
		ModelRuntimeHTTPClient:         fixtureHostHTTPClient{},
		ModelResolveBackendArtifact:    conformanceBackendArtifactResolver,
		ModelInvocationBackend:         serviceedges.ModelInvocationBackend(fixture.InvocationBackend),
		ModelInvocationProtocolClient:  localAIInvocationProtocolClient{fixture: fixture},
	}, rejectingNetwork, compatibility, hostLauncher
}

type localAIInvocationProtocolClient struct {
	fixture *localai.Fixture
}

func (client localAIInvocationProtocolClient) Predict(
	ctx context.Context,
	request models.InvocationProtocolRequest,
) (models.InvocationProtocolResponse, error) {
	if client.fixture == nil {
		return models.InvocationProtocolResponse{}, fmt.Errorf("LocalAI conformance fixture is nil")
	}
	inputs := make([]models.InferenceInput, 0, len(request.Inputs))
	for _, input := range request.Inputs {
		inputs = append(inputs, models.InferenceInput{
			Name: input.Slot, Modality: input.Modality, MediaType: input.MediaType,
			ContentType: input.MediaType, Content: input.Content,
		})
	}
	outputs, _, err := client.fixture.InvocationBackend(ctx, models.InvokeModelRequest{
		Operation: request.Operation, Inputs: inputs, Parameters: request.Parameters,
	})
	if err != nil {
		return models.InvocationProtocolResponse{}, err
	}
	for _, output := range outputs {
		if output.Name == "text" {
			return models.InvocationProtocolResponse{Text: output.Content}, nil
		}
	}
	return models.InvocationProtocolResponse{}, fmt.Errorf("LocalAI conformance fixture returned no text output")
}

func writeGenericConformanceCaches(t *testing.T, home string) {
	t.Helper()
	for _, definition := range (models.BuiltInCatalog{}).ModelDefinitions() {
		writeGenericBuiltinModelCache(t, home, definition.Source)
		selection, body := fixtureBackendSelection(definition.Backend)
		writeGenericBackendCache(t, home, definition.Backend, selection, body)
	}
}

func fixtureBackendSelection(backend string) (serviceedges.ModelBackendArtifactSelection, []byte) {
	body := []byte("localai-backend-" + backend + "-fixture")
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	return serviceedges.ModelBackendArtifactSelection{
		Name: "fixture-" + backend + ".tar.gz", Location: "https://github.com/portpowered/infinite-you/releases/download/localai-fixture/fixture-" + backend + ".tar.gz",
		Bytes: int64(len(body)), SHA256: digest,
	}, body
}

func conformanceBackendArtifactResolver(
	ctx context.Context,
	request serviceedges.ModelBackendArtifactSelectionRequest,
) (serviceedges.ModelBackendArtifactSelection, error) {
	if err := ctx.Err(); err != nil {
		return serviceedges.ModelBackendArtifactSelection{}, err
	}
	selection, _ := fixtureBackendSelection(request.Backend)
	return selection, nil
}

func genericConformanceFactoryConfig(endpoint string) map[string]any {
	resources := make([]map[string]any, 0)
	workers := make([]map[string]any, 0)
	for _, definition := range (models.BuiltInCatalog{}).ModelDefinitions() {
		resourceName := definition.Name + "-cache"
		resources = append(resources, map[string]any{
			"name": resourceName, "type": interfaces.ResourceTypeModel, "capacity": 1,
			"model": definition.Name, "backend": definition.Backend,
			"loadPolicy": string(definition.LoadPolicy),
		})
		operation := definition.Operations[0]
		workers = append(workers, map[string]any{
			"name": definition.Name + "-worker", "type": interfaces.WorkerTypeModel,
			"model": definition.Name, "modelProvider": "CODEX", "modelLocality": interfaces.ModelLocalityLocal,
			"command": "localai-fixture", "args": []string{"--grpc-endpoint", endpoint},
			"resources":  []map[string]any{{"name": resourceName, "capacity": 1}},
			"operations": []map[string]any{authoredOperation(operation)},
		})
	}
	return map[string]any{
		"name": "localai-http-conformance",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"resources": resources,
		"workers":   workers,
	}
}

func authoredOperation(operation models.Operation) map[string]any {
	return map[string]any{
		"name":    operation.Name,
		"inputs":  authoredOperationSlots(operation.Inputs),
		"outputs": authoredOperationSlots(operation.Outputs),
	}
}

func authoredOperationSlots(slots []models.OperationSlot) []map[string]any {
	result := make([]map[string]any, 0, len(slots))
	for _, slot := range slots {
		required := slot.Required != nil && *slot.Required
		result = append(result, map[string]any{
			"name": slot.Name, "contentTypes": authoredContentTypes(slot.ContentTypes), "required": required,
		})
	}
	return result
}

func authoredContentTypes(contentTypes []string) []string {
	result := append([]string(nil), contentTypes...)
	for index, contentType := range result {
		// Factory-authored worker contracts predate the provider-neutral VIDEO
		// modality. The generic Models catalog remains the source of truth for
		// the HTTP request; this scaffold uses the closest accepted binary
		// declaration only to pass Factory schema validation.
		if contentType == string(models.ModalityVideo) {
			result[index] = interfaces.ModelOperationContentTypeBinary
		}
	}
	return result
}

func postConformanceInvocation(
	ctx context.Context,
	endpoint string,
	row conformance.Row,
) (factoryapi.GenericModelInvocationResponse, factoryapi.ErrorResponse, int, error) {
	return postConformanceInvocationForModel(ctx, endpoint, row, conformanceModelName(row.Operation.Name))
}

func postConformanceInvocationForModel(
	ctx context.Context,
	endpoint string,
	row conformance.Row,
	modelName string,
) (factoryapi.GenericModelInvocationResponse, factoryapi.ErrorResponse, int, error) {
	inputs := make([]factoryapi.ModelInvocationInput, 0, len(row.Inputs))
	for _, input := range row.Inputs {
		contentType, mediaType, content := input.ContentType, input.MediaType, input.Content
		inputs = append(inputs, factoryapi.ModelInvocationInput{
			Name: input.Name, Modality: factoryapi.ModelInvocationContentType(input.Modality),
			ContentType: &contentType, MediaType: &mediaType, Content: &content,
		})
	}
	parameterValues := conformanceParameters()
	parameters := make([]factoryapi.ModelInvocationParameter, 0, len(parameterValues))
	for _, parameter := range parameterValues {
		parameters = append(parameters, factoryapi.ModelInvocationParameter{
			Name: parameter.Name, Value: parameter.Value,
		})
	}
	operation := factoryapi.ModelOperationName(row.Operation.Name)
	request := factoryapi.GenericModelInvocationRequest{
		Holder: "localai-http-conformance", Inputs: &inputs, Parameters: &parameters,
		Model:     factoryapi.ModelReference{NameOrUri: modelName},
		Operation: &operation, Scope: "factory-session:localai-conformance",
	}
	offline := true
	request.Offline = &offline
	encoded, err := json.Marshal(request)
	if err != nil {
		return factoryapi.GenericModelInvocationResponse{}, factoryapi.ErrorResponse{}, 0, fmt.Errorf("marshal generic request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return factoryapi.GenericModelInvocationResponse{}, factoryapi.ErrorResponse{}, 0, fmt.Errorf("build generic request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return factoryapi.GenericModelInvocationResponse{}, factoryapi.ErrorResponse{}, 0, fmt.Errorf("POST generic model invocation: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return factoryapi.GenericModelInvocationResponse{}, factoryapi.ErrorResponse{}, response.StatusCode, fmt.Errorf("read generic response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var failure factoryapi.ErrorResponse
		if err := json.Unmarshal(body, &failure); err != nil {
			return factoryapi.GenericModelInvocationResponse{}, factoryapi.ErrorResponse{}, response.StatusCode, fmt.Errorf("decode generic failure: %w (%s)", err, body)
		}
		return factoryapi.GenericModelInvocationResponse{}, failure, response.StatusCode, nil
	}
	var result factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return factoryapi.GenericModelInvocationResponse{}, factoryapi.ErrorResponse{}, response.StatusCode, fmt.Errorf("decode generic response: %w (%s)", err, body)
	}
	return result, factoryapi.ErrorResponse{}, response.StatusCode, nil
}

func conformanceParameters() []models.OperationParameter {
	return []models.OperationParameter{{
		Name: "temperature", Value: map[string]any{"value": 0.2},
	}}
}

func conformanceModelName(operation string) string {
	switch strings.ToUpper(strings.TrimSpace(operation)) {
	case models.OperationOMNI:
		return models.BuiltInModelNameLLM
	case models.OperationEMBED:
		return models.BuiltInModelNameEmbed
	case models.OperationTTS:
		return models.BuiltInModelNameTTS
	case models.OperationASR:
		return models.BuiltInModelNameASR
	default:
		return "missing"
	}
}

func assertConformanceResponse(row conformance.Row, response factoryapi.GenericModelInvocationResponse) error {
	switch row.Operation.Name {
	case models.OperationOMNI:
		return assertOmniResponse(row, response)
	case models.OperationEMBED:
		return assertEmbeddingResponse(row, response)
	case models.OperationTTS:
		return assertTTSResponse(row, response)
	case models.OperationASR:
		return assertASRResponse(row, response)
	default:
		return fmt.Errorf("%s has no semantic assertion", row.Label)
	}
}

func conformanceOutput(row conformance.Row, response factoryapi.GenericModelInvocationResponse, name string) (string, error) {
	for _, output := range response.Outputs {
		if output.Name != name {
			continue
		}
		if output.Content == nil {
			return "", fmt.Errorf("%s output %q has no inline content", row.Label, name)
		}
		return *output.Content, nil
	}
	return "", fmt.Errorf("%s response is missing %q output", row.Label, name)
}

func assertOmniResponse(row conformance.Row, response factoryapi.GenericModelInvocationResponse) error {
	got, err := conformanceOutput(row, response, "text")
	if err != nil {
		return err
	}
	var prompt string
	var images, audios, videos []string
	for _, input := range row.Inputs {
		switch input.Modality {
		case models.ModalityText:
			prompt = input.Content
		case models.ModalityImage:
			images = append(images, input.Content)
		case models.ModalityAudio:
			audios = append(audios, input.Content)
		case models.ModalityVideo:
			videos = append(videos, input.Content)
		}
	}
	if want := localai.ExpectedOmniText(prompt, images, audios, videos); got != want {
		return fmt.Errorf("%s OMNI text = %q, want %q", row.Label, got, want)
	}
	return nil
}

func assertEmbeddingResponse(row conformance.Row, response factoryapi.GenericModelInvocationResponse) error {
	got, err := conformanceOutput(row, response, "embedding")
	if err != nil {
		return err
	}
	var values []float32
	if err := json.Unmarshal([]byte(got), &values); err != nil {
		return fmt.Errorf("%s embedding JSON: %w", row.Label, err)
	}
	if len(values) != 5 || values[0] == 0 {
		return fmt.Errorf("%s embedding dimensions/value = %d/%v, want 5/non-zero", row.Label, len(values), values)
	}
	return nil
}

func assertTTSResponse(row conformance.Row, response factoryapi.GenericModelInvocationResponse) error {
	got, err := conformanceOutput(row, response, "audio")
	if err != nil {
		return err
	}
	audio := []byte(got)
	if len(audio) <= 44 || string(audio[:4]) != "RIFF" || string(audio[8:12]) != "WAVE" || string(audio[36:40]) != "data" {
		return fmt.Errorf("%s audio is not a non-trivial WAV (%d bytes)", row.Label, len(audio))
	}
	return nil
}

func assertASRResponse(row conformance.Row, response factoryapi.GenericModelInvocationResponse) error {
	transcript, err := conformanceOutput(row, response, "transcript")
	if err != nil {
		return err
	}
	if transcript != localai.FixtureTranscript {
		return fmt.Errorf("%s transcript = %q, want %q", row.Label, transcript, localai.FixtureTranscript)
	}
	segmentJSON, err := conformanceOutput(row, response, "segments")
	if err != nil {
		return err
	}
	var values []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(segmentJSON), &values); err != nil {
		return fmt.Errorf("%s segments JSON: %w", row.Label, err)
	}
	if len(values) == 0 || values[0].Text != localai.FixtureTranscriptSegment {
		return fmt.Errorf("%s segments = %#v, want transcript segment", row.Label, values)
	}
	return nil
}

type fixtureHostHTTPClient struct{}

func (fixtureHostHTTPClient) Do(request *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: request,
	}, nil
}

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
	direct := executeLocalAIConfiguredEmbedInvoke(
		t, directProcess, factoryDir, cleanModelsEnvironment(directHome), "", true,
	)
	assertLocalAIConfiguredEmbedSuccess(t, direct, "direct")
	assertLocalAIConfiguredEmbedSelection(
		t, directHome, directResolver, directBackendSelection, directLauncher, directNetwork,
	)
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
	remote := executeLocalAIConfiguredEmbedInvoke(
		t, clientProcess, factoryDir, cleanModelsEnvironment(clientHome), serverURL, false,
	)
	assertLocalAIConfiguredEmbedSuccess(t, remote, "explicit --server")

	if got := serverInvocation.Signatures(); len(got) != 1 {
		t.Fatalf("configured-server EMBED backend requests = %d, want one", len(got))
	} else if len(directRequest) != 1 || got[0] != directRequest[0] {
		t.Fatalf("direct/configured-server EMBED request signatures = %#v / %#v, want identical ordered inputs", directRequest, got)
	}
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
	}{
		{
			name:       "missing required text input",
			inputSpecs: localAIConfiguredEmbedInputSpecs(t, false),
		},
		{
			name:           "malformed parameter flag",
			inputSpecs:     localAIConfiguredEmbedInputSpecs(t, true),
			parameterSpecs: []string{"{"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			direct := executeLocalAIConfiguredEmbedCommand(
				t, directProcess, factoryDir, cleanModelsEnvironment(directHome), "", true,
				testCase.inputSpecs, testCase.parameterSpecs,
			)
			remote := executeLocalAIConfiguredEmbedCommand(
				t, clientProcess, factoryDir, cleanModelsEnvironment(clientHome), serverURL, false,
				testCase.inputSpecs, testCase.parameterSpecs,
			)
			assertLocalAIConfiguredEmbedValidationFailureParity(t, testCase.name, direct, remote)
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

	clientHome := functionalTempDir(t)
	clientLauncher := &cacheSelectionHostLauncherFailure{}
	clientEdges, clientNetwork, _, _ := localAIConfiguredEmbedEdges(
		clientHome, fixture, nil, nil, clientLauncher, nil,
	)
	clientEdges.ModelInvocationBackend = nil
	clientEdges.ModelInvocationProtocolClient = nil
	clientEdges.ModelInvocationGRPCDialer = nil
	clientProcess := functionalBuildProcess(t, clientEdges)
	remote := executeLocalAIConfiguredEmbedInvoke(
		t, clientProcess, factoryDir, cleanModelsEnvironment(clientHome), serverURL, false,
	)

	wantClass := models.InvocationFailureClassBackendProtocol
	if mode == localai.ModeMalformedResponse {
		wantClass = models.InvocationFailureClassMalformedResponse
	}
	assertLocalAIConfiguredEmbedFailure(t, direct, "direct", wantClass)
	assertLocalAIConfiguredEmbedFailure(t, remote, "explicit --server", wantClass)
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
	directInvocation.FailNext(localAIConfiguredEmbedBackendFailure())
	directFailure := executeLocalAIConfiguredEmbedInvoke(
		t, directProcess, factoryDir, cleanModelsEnvironment(directHome), "", true,
	)
	assertLocalAIConfiguredEmbedFailure(
		t, directFailure, "direct failure before recovery", models.InvocationFailureClassBackendProtocol,
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

	clientHome := functionalTempDir(t)
	clientLauncher := &cacheSelectionHostLauncherFailure{}
	clientEdges, clientNetwork, _, _ := localAIConfiguredEmbedEdges(
		clientHome, fixture, nil, nil, clientLauncher, nil,
	)
	clientEdges.ModelInvocationBackend = nil
	clientEdges.ModelInvocationProtocolClient = nil
	clientEdges.ModelInvocationGRPCDialer = nil
	clientProcess := functionalBuildProcess(t, clientEdges)
	serverInvocation.FailNext(localAIConfiguredEmbedBackendFailure())
	remoteFailure := executeLocalAIConfiguredEmbedInvoke(
		t, clientProcess, factoryDir, cleanModelsEnvironment(clientHome), serverURL, false,
	)
	assertLocalAIConfiguredEmbedFailure(
		t, remoteFailure, "explicit --server failure before recovery", models.InvocationFailureClassBackendProtocol,
	)
	remoteSuccess := executeLocalAIConfiguredEmbedInvoke(
		t, clientProcess, factoryDir, cleanModelsEnvironment(clientHome), serverURL, false,
	)
	assertLocalAIConfiguredEmbedSuccess(t, remoteSuccess, "explicit --server recovery")

	directFailureDiagnostic := decodeLocalAIConfiguredEmbedDiagnostic(t, "direct recovery failure", directFailure.Stderr)
	remoteFailureDiagnostic := decodeLocalAIConfiguredEmbedDiagnostic(t, "explicit --server recovery failure", remoteFailure.Stderr)
	if directFailureDiagnostic.Code != remoteFailureDiagnostic.Code ||
		directFailureDiagnostic.Family != remoteFailureDiagnostic.Family ||
		directFailureDiagnostic.Message != remoteFailureDiagnostic.Message {
		t.Fatalf("recovery failure diagnostics differ: direct=%#v remote=%#v", directFailureDiagnostic, remoteFailureDiagnostic)
	}
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
	server.Close(t)
	assertLocalAIConfiguredServerReleased(t, server, serverURL)

	clientHome := functionalTempDir(t)
	clientLauncher := &cacheSelectionHostLauncherFailure{}
	clientEdges, clientNetwork, _, _ := localAIConfiguredEmbedEdges(
		clientHome, fixture, nil, nil, clientLauncher, nil,
	)
	clientEdges.ModelInvocationBackend = nil
	clientEdges.ModelInvocationProtocolClient = nil
	clientEdges.ModelInvocationGRPCDialer = nil
	clientProcess := functionalBuildProcess(t, clientEdges)
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
	if serverLauncher.Starts() != 0 || serverLauncher.Active() {
		t.Fatalf("closed explicit-server EMBED server host lifecycle = starts:%d active:%t, want no invocation host", serverLauncher.Starts(), serverLauncher.Active())
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
	remote := executeLocalAIConfiguredEmbedInvoke(
		t, clientProcess, factoryDir, cleanModelsEnvironment(clientHome), serverURL, false,
	)
	assertLocalAIConfiguredEmbedFailure(
		t, remote, "explicit --server non-finite response", models.InvocationFailureClassMalformedResponse,
	)
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
	if directBackend.Requests()[0].Text != localAIConfiguredEmbedPrompt ||
		fmt.Sprint(directBackend.Requests()[0].Parameters["dimensions"]) != "5" ||
		directBackend.Requests()[0].Parameters["normalize"] != true {
		t.Fatalf("non-finite EMBED typed request = %#v, want text and normalized dimensions parameters", directBackend.Requests()[0])
	}
	assertLocalAIConfiguredEmbedFixtureCalls(t, fixture.Calls(), 2)
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
