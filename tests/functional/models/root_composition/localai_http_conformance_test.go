package root_composition_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/conformance"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

// TestLocalAIHTTPConformanceMatrixRunsThroughRootBuildProcess proves each
// catalog-declared pair reaches the real generic HTTP endpoint, the managed
// Models host lifecycle, the pinned LocalAI gRPC fixture, and the decoded
// public response. The fixture is supplied only through edges.Edges; no
// production operation vertical, registry, or artifact path is introduced.
func TestLocalAIHTTPConformanceMatrixRunsThroughRootBuildProcess(t *testing.T) {
	fixture := localai.Start(t, localai.Options{EmbeddingDimensions: 5})
	home := t.TempDir()
	writeGenericConformanceCaches(t, home)
	dir := support.ScaffoldFactory(t, genericConformanceFactoryConfig(fixture.Endpoint()))
	server, compatibility := startLocalAIConformanceServer(t, dir, home, fixture)

	matrix := conformance.Build(models.GenericOperationCatalog{})
	executor := func(row conformance.Row) (models.GenericInvocationResult, error) {
		response, failure, statusCode, err := postConformanceInvocation(
			t.Context(), server.URL()+"/models/invocations", row,
		)
		if err != nil {
			return models.GenericInvocationResult{}, err
		}
		if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
			return models.GenericInvocationResult{}, fmt.Errorf(
				"public generic invocation returned HTTP %d: %s (%s)",
				statusCode, failure.Message, failure.Code,
			)
		}
		if response.Failure != nil {
			return models.GenericInvocationResult{}, fmt.Errorf("public generic invocation failed: %s", response.Failure.Message)
		}
		if err := assertConformanceResponse(row, response); err != nil {
			return models.GenericInvocationResult{}, err
		}
		return models.GenericInvocationResult{Status: models.ModelInvocationStatusCompleted}, nil
	}

	report, err := matrix.Run(executor, conformance.ModeStrict)
	if err != nil {
		t.Fatalf("LocalAI HTTP conformance: %v", err)
	}
	if got, want := report.ImplementedCount(), len(matrix.Rows); got != want {
		t.Fatalf("implemented rows = %d, want %d", got, want)
	}
	if report.ExpectedUnimplementedCount() != 0 || report.UnexpectedFailureCount() != 0 {
		t.Fatalf("conformance report = %#v, want all implemented", report.Results)
	}

	assertFixtureConformanceCalls(t, fixture)
	if compatibility.Calls() == 0 {
		t.Fatal("managed host compatibility edge calls = 0, want at least one")
	}

	probe := runNoVerticalProbe(t, server.URL()+"/models/invocations")
	var output bytes.Buffer
	if _, err := report.WriteTo(&output); err != nil {
		t.Fatalf("write conformance report: %v", err)
	}
	fmt.Fprintf(&output, "current/no-vertical classification=%s\n", probe.Classification)
	t.Log(output.String())
	if probe.Classification != conformance.ClassificationExpectedUnimplemented {
		t.Fatalf("no-vertical probe classification = %s, want expected-unimplemented", probe.Classification)
	}
}

func startLocalAIConformanceServer(
	t *testing.T,
	dir string,
	home string,
	fixture *localai.Fixture,
) (*support.FunctionalAPIServer, *joinedCompatibilityChecker) {
	t.Helper()
	rejectingNetwork := &rejectingModelAssetHTTP{}
	assetFiles := functionalModelAssetFileSystem{home: home}
	hostLauncher := &recordingModelHostLauncher{endpoint: fixture.Endpoint()}
	compatibility := &joinedCompatibilityChecker{}
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Env:                       functionalHomeEnvironment(home),
		Edges: serviceedges.Edges{
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
		},
	})
	if rejectingNetwork.Calls() != 0 {
		t.Fatalf("model asset network calls during server startup = %d, want 0", rejectingNetwork.Calls())
	}
	return server, compatibility
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
	request := factoryapi.GenericModelInvocationRequest{
		Holder: "localai-http-conformance", Inputs: &inputs,
		Model:     factoryapi.ModelReference{NameOrUri: modelName},
		Operation: factoryapi.ModelOperationName(row.Operation.Name), Scope: "factory-session:localai-conformance",
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
	findOutput := func(name string) (factoryapi.ModelInvocationOutput, error) {
		for _, output := range response.Outputs {
			if output.Name == name {
				return output, nil
			}
		}
		return factoryapi.ModelInvocationOutput{}, fmt.Errorf("%s response is missing %q output", row.Label, name)
	}
	content := func(output factoryapi.ModelInvocationOutput) (string, error) {
		if output.Content == nil {
			return "", fmt.Errorf("%s output %q has no inline content", row.Label, output.Name)
		}
		return *output.Content, nil
	}

	switch row.Operation.Name {
	case models.OperationOMNI:
		output, err := findOutput("text")
		if err != nil {
			return err
		}
		got, err := content(output)
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
	case models.OperationEMBED:
		output, err := findOutput("embedding")
		if err != nil {
			return err
		}
		got, err := content(output)
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
	case models.OperationTTS:
		output, err := findOutput("audio")
		if err != nil {
			return err
		}
		got, err := content(output)
		if err != nil {
			return err
		}
		audio, err := base64.StdEncoding.DecodeString(got)
		if err != nil {
			return fmt.Errorf("%s audio base64: %w", row.Label, err)
		}
		if len(audio) <= 44 || string(audio[:4]) != "RIFF" || string(audio[8:12]) != "WAVE" || string(audio[36:40]) != "data" {
			return fmt.Errorf("%s audio is not a non-trivial WAV (%d bytes)", row.Label, len(audio))
		}
	case models.OperationASR:
		transcript, err := findOutput("transcript")
		if err != nil {
			return err
		}
		transcriptText, err := content(transcript)
		if err != nil {
			return err
		}
		if transcriptText != localai.FixtureTranscript {
			return fmt.Errorf("%s transcript = %q, want %q", row.Label, transcriptText, localai.FixtureTranscript)
		}
		segments, err := findOutput("segments")
		if err != nil {
			return err
		}
		segmentJSON, err := content(segments)
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
	default:
		return fmt.Errorf("%s has no semantic assertion", row.Label)
	}
	return nil
}

func assertFixtureConformanceCalls(t *testing.T, fixture *localai.Fixture) {
	t.Helper()
	calls := fixture.Calls()
	var predicts, embeddings, tts, transcriptions int
	for _, call := range calls {
		switch call.Method {
		case "Predict":
			predicts++
			if len(call.Images) == 2 && (call.Images[0] != "fixture image first" || call.Images[1] != "fixture image second") {
				t.Fatalf("multiple-image fixture order = %#v", call.Images)
			}
		case "Embedding":
			embeddings++
		case "TTSStream":
			tts++
		case "AudioTranscription":
			transcriptions++
		}
	}
	if predicts != 4 || embeddings != 1 || tts != 1 || transcriptions != 1 {
		t.Fatalf("fixture calls = predict %d, embedding %d, tts %d, asr %d; want 4/1/1/1", predicts, embeddings, tts, transcriptions)
	}
}

func runNoVerticalProbe(t *testing.T, endpoint string) conformance.RowResult {
	t.Helper()
	row := conformance.Row{
		Label: "current/no-vertical", Variant: conformance.VariantDefault,
		Operation: models.Operation{Name: models.OperationEMBED}, ContractStatus: conformance.ContractSupported,
	}
	probe := row
	probe.Inputs = []models.InferenceInput{{Name: "text", Modality: models.ModalityText, ContentType: "TEXT", MediaType: "text/plain", Content: "no vertical"}}
	response, failure, statusCode, err := postConformanceInvocationForModel(t.Context(), endpoint, probe, models.BuiltInModelNameLLM)
	if err != nil {
		t.Fatalf("no-vertical probe request: %v", err)
	}
	if statusCode != http.StatusBadRequest || string(failure.Code) != "BAD_REQUEST" || len(response.Outputs) != 0 {
		t.Fatalf("no-vertical probe HTTP response = status %d, failure %#v, outputs %#v; want BAD_REQUEST", statusCode, failure, response.Outputs)
	}
	return conformance.Classify(row, conformance.InvocationOutcome{Err: models.ErrUnsupportedOperation})
}

type fixtureHostHTTPClient struct{}

func (fixtureHostHTTPClient) Do(request *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: request,
	}, nil
}
