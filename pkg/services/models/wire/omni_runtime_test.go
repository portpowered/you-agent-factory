package wire

import (
	"context"
	"errors"
	"strings"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

type recordingInvocationProtocolClient struct {
	request  models.InvocationProtocolRequest
	response models.InvocationProtocolResponse
	err      error
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
	if len(result.Artifacts) != 1 {
		t.Fatalf("Invoke artifacts = %#v, want one forwarded descriptor", result.Artifacts)
	}
	artifact := result.Artifacts[0]
	if artifact.RefValue != "" || artifact.SourcePath != "" || artifact.Name != "text" ||
		artifact.MediaType != "text/plain" || artifact.SizeBytes != int64(len([]byte("fixture answer"))) {
		t.Fatalf("Invoke artifact = %#v, want detached zero-reference text metadata", artifact)
	}
	if client.request.Operation != models.OperationOMNI || client.request.Prompt != "compare these" {
		t.Fatalf("protocol request = %#v, want OMNI prompt", client.request)
	}
	if len(client.request.Inputs) != 2 || client.request.Inputs[0].Slot != "prompt" || client.request.Inputs[1].Slot != "image" || client.request.Inputs[1].MediaType != "image/png" {
		t.Fatalf("protocol inputs = %#v, want ordered prompt/image inputs", client.request.Inputs)
	}
}

func TestNewInvocationRuntimeUsesFailClosedFallbackForNonOmni(t *testing.T) {
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
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassConfiguration {
		t.Fatalf("non-OMNI Invoke error = %v, failure = %#v, want typed configuration failure", err, failure)
	}
	if !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("non-OMNI Invoke error = %v, want ErrUnavailable cause", err)
	}
	if len(result.Content) != 0 || len(result.Artifacts) != 0 {
		t.Fatalf("non-OMNI content = %#v artifacts = %#v, want no output", result.Content, result.Artifacts)
	}
}

func TestInferenceRuntimeFailsClosedWithoutProductionAdapters(t *testing.T) {
	t.Parallel()

	runtime, err := inferenceRuntime(invocationRuntimeOptions{})
	if err != nil {
		t.Fatalf("inferenceRuntime: %v", err)
	}
	composed, ok := runtime.(operationInvocationRuntime)
	if !ok {
		t.Fatalf("inferenceRuntime type = %T, want operationInvocationRuntime", runtime)
	}
	if _, ok := composed.generic.(failClosedInvocationRuntime); !ok {
		t.Fatalf("production default generic runtime = %T, want fail-closed runtime", composed.generic)
	}

	catalog := models.GenericOperationCatalog{}
	for _, operationName := range []string{models.OperationTTS, models.OperationASR, models.OperationEMBED} {
		t.Run(operationName, func(t *testing.T) {
			operation, ok := catalog.GenericOperationContract(operationName)
			if !ok {
				t.Fatalf("GenericOperationContract(%q) = false", operationName)
			}
			result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
				Request: models.InvokeModelRequest{
					Model: models.ModelReference{NameOrURI: "fixture-model"}, Operation: operationName,
					Inputs: []models.InferenceInput{{Name: "input", Content: "must not be echoed"}},
				},
				Operation: operation,
			})
			var failure *models.InvocationFailure
			if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassConfiguration {
				t.Fatalf("Invoke error = %v, failure = %#v, want typed configuration failure", err, failure)
			}
			if !errors.Is(err, models.ErrUnavailable) {
				t.Fatalf("Invoke error = %v, want ErrUnavailable cause", err)
			}
			if len(result.Content) != 0 || len(result.Artifacts) != 0 {
				t.Fatalf("Invoke result = %#v, want no partial output", result)
			}
		})
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
