package wire

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	localai "github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	"google.golang.org/protobuf/proto"
)

type recordingInvocationProtocolClient struct {
	request  models.InvocationProtocolRequest
	response models.InvocationProtocolResponse
	err      error
}

type story001InvocationDialer struct {
	endpoint string
	calls    int
	response []byte
}

func (dialer *story001InvocationDialer) Dial(
	_ context.Context,
	endpoint string,
) (platformgrpc.Connection, error) {
	dialer.endpoint = endpoint
	dialer.calls++
	return story001InvocationConnection{response: append([]byte(nil), dialer.response...)}, nil
}

type story001InvocationConnection struct {
	response []byte
}

func (connection story001InvocationConnection) Invoke(context.Context, string, []byte) ([]byte, error) {
	return append([]byte(nil), connection.response...), nil
}

func (story001InvocationConnection) Close() error { return nil }

type story001InvocationObservation struct {
	Operation       string
	SelectedRuntime string
	BackendBound    bool
	BackendCalls    int
	EndpointPresent bool
	InputSHA256     string
	OutputSHA256    string
	OutputLineage   string
}

func (client *recordingInvocationProtocolClient) Predict(
	_ context.Context,
	request models.InvocationProtocolRequest,
) (models.InvocationProtocolResponse, error) {
	client.request = request
	return client.response, client.err
}

func TestNewInvocationRuntimeFailsClosedWhenOmniProtocolIsUnbound(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("scope:unbound-omni")
	if err != nil {
		t.Fatalf("scope.Parse: %v", err)
	}
	operation, ok := (models.GenericOperationCatalog{}).GenericOperationContract(models.OperationOMNI)
	if !ok {
		t.Fatal("GenericOperationContract(OMNI) = false")
	}
	runtime := newInvocationRuntime(nil, nil)
	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request: models.InvokeModelRequest{
			Scope: scope, Holder: "unbound-test", Model: models.ModelReference{NameOrURI: "llm"},
			Operation: models.OperationOMNI,
			Inputs: []models.InferenceInput{
				{Name: "prompt", Modality: models.ModalityText, Content: "Write a haiku"},
			},
		},
		Operation: operation,
	})
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassBackendProtocol {
		t.Fatalf("Invoke error = %v, failure = %#v, want typed backend-protocol failure", err, failure)
	}
	if !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("Invoke error = %v, want ErrUnavailable cause", err)
	}
	if len(result.Content) != 0 {
		t.Fatalf("Invoke result = %#v, want no fallback content", result)
	}
}

func TestStory001ProductionDefaultRuntimeCharacterization(t *testing.T) {
	t.Parallel()

	runtime, err := inferenceRuntime(invocationRuntimeOptions{})
	if err != nil {
		t.Fatalf("inferenceRuntime: %v", err)
	}
	composed, ok := runtime.(operationInvocationRuntime)
	if !ok {
		t.Fatalf("inferenceRuntime type = %T, want operationInvocationRuntime", runtime)
	}
	if composed.asr != nil || composed.embedding != nil {
		t.Fatalf("production default operation runtimes = asr:%T embed:%T, want unbound", composed.asr, composed.embedding)
	}
	if _, ok := composed.generic.(inference.InputEchoInvocationRuntime); !ok {
		t.Fatalf("production default generic runtime = %T, want InputEchoInvocationRuntime", composed.generic)
	}

	catalog := models.GenericOperationCatalog{}
	cases := []struct {
		operation string
		model     string
		name      string
		modality  models.Modality
		mediaType string
		content   string
	}{
		{operation: models.OperationASR, model: "asr", name: "audio", modality: models.ModalityAudio, mediaType: "audio/wav", content: string([]byte{0x00, 0xff, 0x01, 0x52, 0x49, 0x46, 0x46})},
		{operation: models.OperationEMBED, model: "embed", name: "text", modality: models.ModalityText, mediaType: "text/plain", content: "Find similar work"},
	}
	for _, testCase := range cases {
		t.Run(testCase.operation, func(t *testing.T) {
			operation, ok := catalog.GenericOperationContract(testCase.operation)
			if !ok {
				t.Fatalf("GenericOperationContract(%q) = false", testCase.operation)
			}
			request := models.InvokeModelRequest{
				Scope: mustRoutingScope(t), Holder: "story-001", Model: models.ModelReference{NameOrURI: testCase.model},
				Operation: testCase.operation,
				Inputs:    []models.InferenceInput{{Name: testCase.name, Modality: testCase.modality, ContentType: testCase.mediaType, MediaType: testCase.mediaType, Content: testCase.content}},
			}
			result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{Request: request, Operation: operation})
			if err != nil {
				t.Fatalf("Invoke error = %v", err)
			}
			if len(result.Content) != len(operation.Outputs) {
				t.Fatalf("Invoke outputs = %#v, want %d declared outputs", result.Content, len(operation.Outputs))
			}
			outputValues := make([]string, len(result.Content))
			allInputsEchoed := len(result.Content) > 0
			for index, output := range result.Content {
				outputValues[index] = output.Content
				if output.Content != testCase.content {
					allInputsEchoed = false
				}
			}
			lineage := "backend-generated"
			if allInputsEchoed {
				lineage = "input-echo"
			}
			observation := story001InvocationObservation{
				Operation: testCase.operation, SelectedRuntime: "InputEchoInvocationRuntime", BackendBound: false,
				BackendCalls: 0, EndpointPresent: false, InputSHA256: story001SHA256(testCase.content),
				OutputSHA256: story001SHA256(strings.Join(outputValues, "\n")), OutputLineage: lineage,
			}
			t.Logf("STORY-001-EVIDENCE ledger=%+v", observation)
			if !allInputsEchoed {
				t.Fatalf("default %s output lineage = %q, want input-echo characterization", testCase.operation, lineage)
			}
		})
	}
}

func TestInferenceRuntimeBindsProductionASRWhenProtocolAndStagingArePresent(t *testing.T) {
	t.Parallel()

	runtime, err := inferenceRuntime(invocationRuntimeOptions{
		Dialer:           &story001InvocationDialer{},
		ASRTempDirectory: func() string { return "temp" },
		ASRCreateTemp: func(string, string) (localai.TempFile, error) {
			return nil, nil
		},
		ASRWriteFile:  func(string, []byte) error { return nil },
		ASRRemoveFile: func(string) error { return nil },
	})
	if err != nil {
		t.Fatalf("inferenceRuntime: %v", err)
	}
	composed, ok := runtime.(operationInvocationRuntime)
	if !ok {
		t.Fatalf("inferenceRuntime type = %T, want operationInvocationRuntime", runtime)
	}
	if composed.asr == nil {
		t.Fatal("production ASR runtime = nil, want pinned adapter when protocol and staging are wired")
	}
}

func TestInferenceRuntimeRoutesDefaultASRThroughPinnedProtocol(t *testing.T) {
	t.Parallel()

	responsePayload, err := proto.Marshal(&localai.TranscriptResult{
		Text:     "routed transcript",
		Segments: []*localai.TranscriptSegment{{Id: 0, Start: 0, End: 100, Text: "routed transcript"}},
	})
	if err != nil {
		t.Fatalf("marshal ASR response: %v", err)
	}
	endpoint := "grpc://127.0.0.1:45906"
	dialer := &story001InvocationDialer{response: responsePayload}
	tempDir := t.TempDir()
	runtime, err := inferenceRuntime(invocationRuntimeOptions{
		Dialer:           dialer,
		ASRTempDirectory: func() string { return tempDir },
		ASRCreateTemp: func(directory, pattern string) (localai.TempFile, error) {
			return os.CreateTemp(directory, pattern)
		},
		ASRWriteFile: func(path string, content []byte) error {
			return os.WriteFile(path, content, 0o600)
		},
		ASRRemoveFile: os.Remove,
	})
	if err != nil {
		t.Fatalf("inferenceRuntime: %v", err)
	}
	operation, ok := (models.GenericOperationCatalog{}).GenericOperationContract(models.OperationASR)
	if !ok {
		t.Fatal("GenericOperationContract(ASR) = false")
	}
	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request: models.InvokeModelRequest{
			Scope: mustRoutingScope(t), Model: models.ModelReference{NameOrURI: "asr"},
			Operation: models.OperationASR,
			Inputs: []models.InferenceInput{{
				Name: "audio", Modality: models.ModalityAudio,
				ContentType: "audio/wav", MediaType: "audio/wav", Content: "audio",
			}},
		},
		Operation: operation,
		HostSlot:  inference.HostHandleSlot{Endpoint: endpoint},
	})
	if err != nil || len(result.Content) != 2 || result.Content[0].Content != "routed transcript" || dialer.endpoint != endpoint {
		t.Fatalf("default ASR route = result:%#v error:%v endpoint:%q, want pinned response and selected endpoint", result, err, dialer.endpoint)
	}
}

func TestStory001PinnedOmniCharacterizationRecordsHostEndpointAndBackendCall(t *testing.T) {
	t.Parallel()

	response, err := proto.Marshal(&localai.Reply{Message: []byte("generated response")})
	if err != nil {
		t.Fatalf("marshal fixture LocalAI response: %v", err)
	}
	dialer := &story001InvocationDialer{response: response}
	runtime := newInvocationRuntime(nil, dialer)
	operation, ok := (models.GenericOperationCatalog{}).GenericOperationContract(models.OperationOMNI)
	if !ok {
		t.Fatal("GenericOperationContract(OMNI) = false")
	}
	input := "describe the image"
	endpoint := "grpc://127.0.0.1:45901"
	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request: models.InvokeModelRequest{
			Scope: mustRoutingScope(t), Holder: "story-001", Model: models.ModelReference{NameOrURI: "llm"},
			Operation: models.OperationOMNI,
			Inputs:    []models.InferenceInput{{Name: "prompt", Modality: models.ModalityText, ContentType: "text/plain", MediaType: "text/plain", Content: input}},
		},
		Operation: operation,
		HostSlot:  inference.HostHandleSlot{Endpoint: endpoint},
	})
	if err != nil {
		t.Fatalf("OMNI Invoke error = %v", err)
	}
	if dialer.endpoint != endpoint {
		t.Fatal("OMNI transport did not receive the selected host endpoint")
	}
	if dialer.calls != 1 {
		t.Fatalf("OMNI backend calls = %d, want 1", dialer.calls)
	}
	if len(result.Content) != 1 || result.Content[0].Content != "generated response" {
		t.Fatalf("OMNI output = %#v, want generated protocol response", result.Content)
	}
	observation := story001InvocationObservation{
		Operation: models.OperationOMNI, SelectedRuntime: "omniInvocationRuntime", BackendBound: true,
		BackendCalls: dialer.calls, EndpointPresent: strings.TrimSpace(dialer.endpoint) != "",
		InputSHA256: story001SHA256(input), OutputSHA256: story001SHA256(result.Content[0].Content),
		OutputLineage: "backend-generated",
	}
	t.Logf("STORY-001-EVIDENCE ledger=%+v", observation)
	if !observation.EndpointPresent {
		t.Fatal("OMNI characterization endpoint-present = false, want true")
	}
}

func story001SHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func TestNewInvocationRuntimeForwardsOmniInputsAndDeclaredUsage(t *testing.T) {
	t.Parallel()

	client := &recordingInvocationProtocolClient{
		response: models.InvocationProtocolResponse{
			Text:  "fixture answer",
			Usage: `{"tokens":3}`,
		},
	}
	runtime := newInvocationRuntime(client, nil)
	scope, err := (models.RuntimeScopeRef{}).Parse("scope:bound-omni")
	if err != nil {
		t.Fatalf("scope.Parse: %v", err)
	}
	operation, ok := (models.GenericOperationCatalog{}).GenericOperationContract(models.OperationOMNI)
	if !ok {
		t.Fatal("GenericOperationContract(OMNI) = false")
	}
	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request: models.InvokeModelRequest{
			Scope: scope, Holder: "bound-test", Model: models.ModelReference{NameOrURI: "llm"},
			Operation: models.OperationOMNI,
			Inputs: []models.InferenceInput{
				{Name: "prompt", Modality: models.ModalityText, ContentType: "text/plain", MediaType: "text/plain", Content: "compare these"},
				{Name: "image", Modality: models.ModalityImage, ContentType: "image/png", MediaType: "image/png", Content: "PNG-A"},
			},
		},
		Operation: operation,
	})
	if err != nil {
		t.Fatalf("Invoke error = %v", err)
	}
	if len(result.Content) != 2 || result.Content[0].Name != "text" || result.Content[0].Content != "fixture answer" || result.Content[1].Name != "usage" {
		t.Fatalf("Invoke content = %#v, want text and declared usage", result.Content)
	}
	if client.request.Operation != models.OperationOMNI || client.request.Prompt != "compare these" {
		t.Fatalf("protocol request = %#v, want OMNI prompt", client.request)
	}
	if len(client.request.Inputs) != 2 || client.request.Inputs[0].Slot != "prompt" || client.request.Inputs[1].Slot != "image" || client.request.Inputs[1].MediaType != "image/png" {
		t.Fatalf("protocol inputs = %#v, want ordered prompt/image inputs", client.request.Inputs)
	}
}

func TestNewInvocationRuntimeUsesGenericFallbackForNonOmni(t *testing.T) {
	t.Parallel()

	runtime := newInvocationRuntime(nil, nil)
	operation, ok := (models.GenericOperationCatalog{}).GenericOperationContract(models.OperationTTS)
	if !ok {
		t.Fatal("GenericOperationContract(TTS) = false")
	}
	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request: models.InvokeModelRequest{
			Operation: models.OperationTTS,
			Inputs:    []models.InferenceInput{{Name: "text", Modality: models.ModalityText, Content: "hello"}},
		},
		Operation: operation,
	})
	if err != nil {
		t.Fatalf("non-OMNI Invoke error = %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Content != "hello" {
		t.Fatalf("non-OMNI content = %#v, want input echo", result.Content)
	}
}

func TestInferenceRuntimeRoutesOmniBeforeGenericBackend(t *testing.T) {
	t.Parallel()

	scope := mustRoutingScope(t)
	genericCalls := 0
	runtime, err := inferenceRuntime(invocationRuntimeOptions{
		Backend: func(context.Context, models.InvokeModelRequest) ([]models.InferenceContent, []models.InferenceArtifact, error) {
			genericCalls++
			return []models.InferenceContent{{Name: "generic", Content: "generic answer"}}, nil, nil
		},
		Client: &recordingInvocationProtocolClient{response: models.InvocationProtocolResponse{Text: "omni answer"}},
	})
	if err != nil {
		t.Fatalf("inferenceRuntime: %v", err)
	}
	catalog := models.GenericOperationCatalog{}
	omni, _ := catalog.GenericOperationContract(models.OperationOMNI)
	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request:   models.InvokeModelRequest{Scope: scope, Holder: "routing-test", Model: models.ModelReference{NameOrURI: "llm"}, Operation: models.OperationOMNI, Inputs: []models.InferenceInput{{Name: "prompt", Modality: models.ModalityText, Content: "hello"}}},
		Operation: omni,
	})
	if err != nil || len(result.Content) != 1 || result.Content[0].Content != "omni answer" {
		t.Fatalf("OMNI route = result:%#v error:%v", result, err)
	}
	if genericCalls != 0 {
		t.Fatalf("generic calls after OMNI = %d, want 0", genericCalls)
	}

	tts, _ := catalog.GenericOperationContract(models.OperationTTS)
	result, err = runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request:   models.InvokeModelRequest{Scope: scope, Holder: "routing-test", Model: models.ModelReference{NameOrURI: "tts"}, Operation: models.OperationTTS, Inputs: []models.InferenceInput{{Name: "text", Modality: models.ModalityText, Content: "speak"}}},
		Operation: tts,
	})
	if err != nil || len(result.Content) != 1 || result.Content[0].Content != "generic answer" {
		t.Fatalf("generic route = result:%#v error:%v", result, err)
	}
	if genericCalls != 1 {
		t.Fatalf("generic calls = %d, want 1", genericCalls)
	}
}

func TestInferenceRuntimeRoutesASRBeforeGenericBackend(t *testing.T) {
	t.Parallel()

	scope := mustRoutingScope(t)
	asrCalls := 0
	runtime, err := inferenceRuntime(invocationRuntimeOptions{
		Backend: func(context.Context, models.InvokeModelRequest) ([]models.InferenceContent, []models.InferenceArtifact, error) {
			return []models.InferenceContent{{Name: "generic"}}, nil, nil
		},
		ASR: func(context.Context, models.ASRBackendRequest) (models.ASRBackendResponse, error) {
			asrCalls++
			return models.ASRBackendResponse{
				Text:     "transcript",
				Segments: []models.ASRBackendSegment{{ID: 1, Start: 0, End: 100, Text: "transcript"}},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("inferenceRuntime: %v", err)
	}
	asr, _ := (models.GenericOperationCatalog{}).GenericOperationContract(models.OperationASR)
	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request: models.InvokeModelRequest{Scope: scope, Holder: "routing-test", Model: models.ModelReference{NameOrURI: "asr"}, Operation: models.OperationASR, Inputs: []models.InferenceInput{
			{Name: "audio", Modality: models.ModalityAudio, ContentType: "audio/wav", MediaType: "audio/wav", Content: string([]byte{0, 1, 2})},
		}},
		Operation: asr,
	})
	if err != nil || len(result.Content) != 2 || result.Content[0].Name != "transcript" || result.Content[1].Name != "segments" {
		t.Fatalf("ASR route = result:%#v error:%v", result, err)
	}
	if asrCalls != 1 {
		t.Fatalf("ASR calls = %d, want 1", asrCalls)
	}
}

func mustRoutingScope(t *testing.T) models.RuntimeScopeRef {
	t.Helper()
	scope, err := (models.RuntimeScopeRef{}).Parse("scope:runtime-routing")
	if err != nil {
		t.Fatalf("scope.Parse: %v", err)
	}
	return scope
}

func TestInvocationProtocolAdapterPreservesFailureAndOperationFallback(t *testing.T) {
	t.Parallel()

	if !isOMNIOperation(inference.InvocationRuntimeRequest{Request: models.InvokeModelRequest{Operation: models.OperationOMNI}}) {
		t.Fatal("isOMNIOperation should inspect the request operation when the catalog operation is empty")
	}
	scope, err := (models.RuntimeScopeRef{}).Parse("scope:protocol-error")
	if err != nil {
		t.Fatalf("scope.Parse: %v", err)
	}
	client := &recordingInvocationProtocolClient{err: errors.New("fixture protocol failure")}
	runtime := newInvocationRuntime(client, nil)
	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request:   models.InvokeModelRequest{Scope: scope, Holder: "protocol-error", Model: models.ModelReference{NameOrURI: "llm"}, Operation: models.OperationOMNI, Inputs: []models.InferenceInput{{Name: "prompt", Modality: models.ModalityText, Content: "hello"}}},
		Operation: models.Operation{},
	})
	if result.Content != nil || err == nil || !strings.Contains(err.Error(), "fixture protocol failure") {
		t.Fatalf("protocol failure = result:%#v error:%v, want typed failure preserving cause", result, err)
	}
}

func TestNewServiceWithInvocationProtocolConstructsRoot(t *testing.T) {
	t.Parallel()

	service, err := validConstructionEdges().newServiceWithInvocationProtocol(&recordingInvocationProtocolClient{})
	if err != nil {
		t.Fatalf("NewServiceWithBackendArtifactResolverAndInvocationProtocolAndDialer: %v", err)
	}
	if service == nil {
		t.Fatal("NewServiceWithBackendArtifactResolverAndInvocationProtocolAndDialer returned nil service")
	}
}

func TestPinnedHostProtocolNegotiatorWrapperRejectsNilDialer(t *testing.T) {
	t.Parallel()

	if negotiator := NewPinnedGRPCHostProtocolNegotiator(nil); negotiator != nil {
		t.Fatalf("NewPinnedGRPCHostProtocolNegotiator(nil) = %T, want nil", negotiator)
	}
}
