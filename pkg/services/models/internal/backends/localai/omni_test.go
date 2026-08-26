package localai

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

func TestProbePinnedOmniProtocolRecordsMediaCapability(t *testing.T) {
	t.Parallel()

	probe := ProbePinnedOmniProtocol()
	if probe.ProtocolVersion != "localai-backend-v1" || probe.ProtocolRevision != PinnedProtocolRevision ||
		probe.ProtocolPath != PinnedProtocolPath || probe.LocalAICommit != PinnedLocalAICommit {
		t.Fatalf("probe identity = %#v, want pinned protocol", probe)
	}
	if probe.PromptField != "Prompt" || probe.ImageField != "Images" ||
		probe.AudioField != "Audios" || probe.VideoField != "Videos" {
		t.Fatalf("probe fields = %#v, want pinned PredictOptions fields", probe)
	}
	if !probe.AudioSupported || !probe.VideoSupported {
		t.Fatalf("probe media support = audio:%t video:%t, want both supported", probe.AudioSupported, probe.VideoSupported)
	}
	if err := PinnedOmniCapability().Validate(); err != nil {
		t.Fatalf("PinnedOmniCapability().Validate(): %v", err)
	}
}

func TestProbePinnedOmniProtocolUsesFixtureAcceptanceAndNarrowsCodec(t *testing.T) {
	t.Parallel()

	fixture := &recordingConformanceProbe{
		accepted: map[models.Modality]bool{
			models.ModalityAudio: true,
			models.ModalityVideo: false,
		},
	}
	evidence := ProbePinnedOmniProtocol(fixture)
	if !evidence.AudioSupported || evidence.VideoSupported {
		t.Fatalf("fixture capability = %#v, want audio accepted and video rejected", evidence)
	}
	if len(fixture.requests) != 2 {
		t.Fatalf("conformance requests = %#v, want audio and video probes", fixture.requests)
	}
	wantRequests := []OmniConformanceRequest{
		{
			ProtocolVersion: modelseffects.PinnedHostProtocolVersion, ProtocolRevision: PinnedProtocolRevision,
			ProtocolPath: PinnedProtocolPath, LocalAICommit: PinnedLocalAICommit,
			Slot: "audio", Modality: models.ModalityAudio, ProtocolField: "Audios", MediaType: "audio/*",
		},
		{
			ProtocolVersion: modelseffects.PinnedHostProtocolVersion, ProtocolRevision: PinnedProtocolRevision,
			ProtocolPath: PinnedProtocolPath, LocalAICommit: PinnedLocalAICommit,
			Slot: "video", Modality: models.ModalityVideo, ProtocolField: "Videos", MediaType: "video/*",
		},
	}
	if !reflect.DeepEqual(fixture.requests, wantRequests) {
		t.Fatalf("conformance requests = %#v, want pinned audio/video field shapes", fixture.requests)
	}

	capability := CapabilityFromPinnedOmniProbe(evidence)
	if !capability.Supported("audio") || capability.Supported("video") {
		t.Fatalf("effective capability = %#v, want audio only", capability.Inputs)
	}
	codec, err := NewOmniCodec(&protocolFixture{response: PredictResponse{Text: "unused"}}, capability)
	if err != nil {
		t.Fatalf("NewOmniCodec: %v", err)
	}
	scope, err := (models.RuntimeScopeRef{}).Parse("scope:conformance")
	if err != nil {
		t.Fatalf("scope.Parse: %v", err)
	}
	_, err = codec.Encode(models.InvokeModelRequest{
		Scope: scope, Holder: "conformance-test", Model: models.ModelReference{NameOrURI: "llm"},
		Operation: models.OperationOMNI,
		Inputs: []models.InferenceInput{
			{Name: "prompt", Modality: models.ModalityText, Content: "describe"},
			{Name: "video", Modality: models.ModalityVideo, MediaType: "video/mp4", Content: "clip.mp4"},
		},
	})
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMediaCapability {
		t.Fatalf("Encode error = %v, failure = %#v, want fixture-driven media rejection", err, failure)
	}
}

func TestOmniCapabilityOperationNarrowsUnsupportedOptionalSlots(t *testing.T) {
	t.Parallel()

	capability := PinnedOmniCapability()
	for index := range capability.Inputs {
		if capability.Inputs[index].Slot == "video" {
			capability.Inputs[index].Supported = false
		}
	}
	operation := capability.Operation()
	if hasOperationSlot(operation, "video") {
		t.Fatalf("narrowed OMNI operation retained unsupported video slot: %#v", operation.Inputs)
	}
	if !hasOperationSlot(operation, "prompt") || !hasOperationSlot(operation, "image") {
		t.Fatalf("narrowed OMNI operation lost supported slots: %#v", operation.Inputs)
	}
}

func TestOmniCodecForwardsOrderedMediaAndDetectedTypes(t *testing.T) {
	t.Parallel()

	fixture := &protocolFixture{response: PredictResponse{Text: "LOCALAI_FIXTURE_OMNI"}}
	codec, err := NewOmniCodec(fixture, PinnedOmniCapability())
	if err != nil {
		t.Fatalf("NewOmniCodec: %v", err)
	}
	scope, err := (models.RuntimeScopeRef{}).Parse("scope:omni")
	if err != nil {
		t.Fatalf("scope.Parse: %v", err)
	}
	request := models.InvokeModelRequest{
		Scope:     scope,
		Holder:    "omni-test",
		Model:     models.ModelReference{NameOrURI: models.BuiltInModelNameLLM},
		Operation: models.OperationOMNI,
		Inputs: []models.InferenceInput{
			{Name: "prompt", Modality: models.ModalityText, Content: "compare"},
			{Name: "image", Modality: models.ModalityImage, MediaType: "image/png", Content: "a.png"},
			{Name: "image", Modality: models.ModalityImage, MediaType: "image/jpeg", Content: "b.png"},
			{Name: "audio", Modality: models.ModalityAudio, MediaType: "audio/wav", Content: "voice.wav"},
			{Name: "video", Modality: models.ModalityVideo, MediaType: "video/mp4", Content: "clip.mp4"},
		},
	}
	outputs, err := codec.Invoke(context.Background(), request)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(outputs) != 1 || outputs[0].Content != "LOCALAI_FIXTURE_OMNI" {
		t.Fatalf("outputs = %#v, want fixture text", outputs)
	}
	want := PredictRequest{
		Prompt: "compare",
		Inputs: []ProtocolInput{
			{Slot: "prompt", Modality: models.ModalityText, MediaType: "text/plain", Content: "compare"},
			{Slot: "image", Modality: models.ModalityImage, MediaType: "image/png", Content: "a.png"},
			{Slot: "image", Modality: models.ModalityImage, MediaType: "image/jpeg", Content: "b.png"},
			{Slot: "audio", Modality: models.ModalityAudio, MediaType: "audio/wav", Content: "voice.wav"},
			{Slot: "video", Modality: models.ModalityVideo, MediaType: "video/mp4", Content: "clip.mp4"},
		},
	}
	if !reflect.DeepEqual(fixture.request, want) {
		t.Fatalf("protocol request = %#v, want %#v", fixture.request, want)
	}
}

func TestOmniCodecRejectsUnsupportedModalityBeforeProtocolCall(t *testing.T) {
	t.Parallel()

	fixture := &protocolFixture{response: PredictResponse{Text: "must not be used"}}
	capability := PinnedOmniCapability()
	for index := range capability.Inputs {
		if capability.Inputs[index].Slot == "audio" || capability.Inputs[index].Slot == "video" {
			capability.Inputs[index].Supported = false
		}
	}
	codec, err := NewOmniCodec(fixture, capability)
	if err != nil {
		t.Fatalf("NewOmniCodec: %v", err)
	}
	scope, err := (models.RuntimeScopeRef{}).Parse("scope:unsupported")
	if err != nil {
		t.Fatalf("scope.Parse: %v", err)
	}
	request := models.InvokeModelRequest{
		Scope: scope, Holder: "omni-test", Model: models.ModelReference{NameOrURI: "llm"},
		Operation: models.OperationOMNI,
		Inputs: []models.InferenceInput{
			{Name: "prompt", Modality: models.ModalityText, Content: "describe"},
			{Name: "video", Modality: models.ModalityVideo, MediaType: "video/mp4", Content: "clip.mp4"},
		},
	}
	_, err = codec.Invoke(context.Background(), request)
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMediaCapability {
		t.Fatalf("Invoke error = %v, failure = %#v, want typed media capability failure", err, failure)
	}
	if failure.Slot != "video" || fixture.calls != 0 {
		t.Fatalf("failure = %#v, fixture calls = %d, want video rejection before generation", failure, fixture.calls)
	}
}

func TestOmniCodecPreservesArtifactReferenceAndRejectsMalformedResponse(t *testing.T) {
	t.Parallel()

	artifact, err := (models.InferenceArtifactRef{}).Parse("models-input:clip")
	if err != nil {
		t.Fatalf("artifact.Parse: %v", err)
	}
	fixture := &protocolFixture{}
	codec, err := NewOmniCodec(fixture, PinnedOmniCapability())
	if err != nil {
		t.Fatalf("NewOmniCodec: %v", err)
	}
	scope, err := (models.RuntimeScopeRef{}).Parse("scope:artifact")
	if err != nil {
		t.Fatalf("scope.Parse: %v", err)
	}
	request := models.InvokeModelRequest{
		Scope: scope, Holder: "omni-test", Model: models.ModelReference{NameOrURI: "llm"},
		Inputs: []models.InferenceInput{
			{Name: "prompt", Modality: models.ModalityText, Content: "what"},
			{Name: "video", Modality: models.ModalityVideo, MediaType: "video/mp4", Artifact: &artifact},
		},
	}
	predict, err := codec.Encode(request)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if predict.Inputs[1].Reference != "models-input:clip" || predict.Inputs[1].Content != "" {
		t.Fatalf("artifact reference was not preserved: %#v", predict.Inputs)
	}

	fixture.response = PredictResponse{}
	_, err = codec.Invoke(context.Background(), request)
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMalformedResponse || failure.Slot != "text" {
		t.Fatalf("malformed response error = %v, failure = %#v, want typed text failure", err, failure)
	}
}

func hasOperationSlot(operation models.Operation, name string) bool {
	for _, slot := range operation.Inputs {
		if slot.Name == name {
			return true
		}
	}
	return false
}

type protocolFixture struct {
	request  PredictRequest
	response PredictResponse
	calls    int
}

type recordingConformanceProbe struct {
	requests []OmniConformanceRequest
	accepted map[models.Modality]bool
}

func (probe *recordingConformanceProbe) Accepts(request OmniConformanceRequest) bool {
	probe.requests = append(probe.requests, request)
	return probe.accepted[request.Modality]
}

func (fixture *protocolFixture) Predict(_ context.Context, request PredictRequest) (PredictResponse, error) {
	fixture.calls++
	fixture.request = request
	return fixture.response, nil
}
