package root_composition_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
