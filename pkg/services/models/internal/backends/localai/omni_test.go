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
	result, err := codec.Invoke(context.Background(), request)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Content != "LOCALAI_FIXTURE_OMNI" {
		t.Fatalf("content = %#v, want fixture text", result.Content)
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

func TestOmniCodecReturnsSemanticTextAndUsage(t *testing.T) {
	t.Parallel()

	const wantText = "Résumé — 世界 🌍"
	const wantUsage = `{"tokens":3}`
	fixture := &protocolFixture{response: PredictResponse{Text: wantText, Usage: wantUsage}}
	result, err := invokeSemanticOmni(t, fixture)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if fixture.calls != 1 {
		t.Fatalf("protocol calls = %d, want exactly one", fixture.calls)
	}
	assertSemanticOmniContent(t, result.Content, wantText, wantUsage)
	assertSemanticOmniArtifact(t, result.Artifacts)
}

func invokeSemanticOmni(t *testing.T, fixture *protocolFixture) (OmniInvocationResult, error) {
	t.Helper()
	codec, err := NewOmniCodec(fixture, PinnedOmniCapability())
	if err != nil {
		t.Fatalf("NewOmniCodec: %v", err)
	}
	scope, err := (models.RuntimeScopeRef{}).Parse("scope:semantic-text")
	if err != nil {
		t.Fatalf("scope.Parse: %v", err)
	}
	operation, ok := (models.GenericOperationCatalog{}).GenericOperationContract(models.OperationOMNI)
	if !ok {
		t.Fatal("GenericOperationContract(OMNI) = false")
	}
	return codec.Invoke(context.Background(), models.InvokeModelRequest{
		Scope: scope, Holder: "semantic-text-test", Model: models.ModelReference{NameOrURI: "llm"},
		Operation: models.OperationOMNI,
		Inputs:    []models.InferenceInput{{Name: "prompt", Modality: models.ModalityText, Content: "describe"}},
	}, operation)
}

func assertSemanticOmniContent(
	t *testing.T,
	content []models.InferenceContent,
	wantText, wantUsage string,
) {
	t.Helper()
	if len(content) != 2 {
		t.Fatalf("content = %#v, want text and usage", content)
	}
	textOutput := content[0]
	if textOutput.Name != "text" || textOutput.Modality != models.ModalityText ||
		textOutput.ContentType != "text/plain" || textOutput.MediaType != "text/plain" ||
		textOutput.Content != wantText {
		t.Fatalf("text output = %#v, want exact semantic text metadata", textOutput)
	}
	usageOutput := content[1]
	if usageOutput.Name != "usage" || usageOutput.Modality != models.ModalityJSON ||
		usageOutput.ContentType != "application/json" || usageOutput.MediaType != "application/json" ||
		usageOutput.Content != wantUsage {
		t.Fatalf("usage output = %#v, want separate JSON usage", usageOutput)
	}
}

func assertSemanticOmniArtifact(t *testing.T, artifacts []models.InferenceArtifact) {
	t.Helper()
	if len(artifacts) != 1 {
		t.Fatalf("artifacts = %#v, want one detached text descriptor", artifacts)
	}
}

func TestOmniCodecBuildsUTF8TextArtifactDescriptor(t *testing.T) {
	t.Parallel()

	const wantText = "é界🙂"
	fixture := &protocolFixture{response: PredictResponse{Text: wantText, Usage: `{"tokens":4}`}}
	codec, err := NewOmniCodec(fixture, PinnedOmniCapability())
	if err != nil {
		t.Fatalf("NewOmniCodec: %v", err)
	}
	scope, err := (models.RuntimeScopeRef{}).Parse("scope:utf8-artifact")
	if err != nil {
		t.Fatalf("scope.Parse: %v", err)
	}
	result, err := codec.Invoke(context.Background(), models.InvokeModelRequest{
		Scope: scope, Holder: "utf8-artifact-test", Model: models.ModelReference{NameOrURI: "llm"},
		Operation: models.OperationOMNI,
		Inputs:    []models.InferenceInput{{Name: "prompt", Modality: models.ModalityText, Content: "describe"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("artifacts = %#v, want exactly one descriptor", result.Artifacts)
	}
	descriptor := result.Artifacts[0]
	if descriptor.Name != "text" || descriptor.MediaType != "text/plain" ||
		descriptor.SizeBytes != int64(len([]byte(wantText))) {
		t.Fatalf("descriptor = %#v, want text/plain UTF-8 byte size", descriptor)
	}
	if !descriptor.Artifact.IsZero() {
		t.Fatalf("descriptor identity = %q, want zero before registration", descriptor.Artifact.String())
	}
	if descriptor.Properties != nil {
		t.Fatalf("descriptor properties = %#v, want no unsafe metadata", descriptor.Properties)
	}
}

func TestOmniCodecReturnsZeroResultOnFailure(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("scope:atomic-result")
	if err != nil {
		t.Fatalf("scope.Parse: %v", err)
	}
	baseRequest := models.InvokeModelRequest{
		Scope: scope, Holder: "atomic-result-test", Model: models.ModelReference{NameOrURI: "llm"},
		Operation: models.OperationOMNI,
		Inputs:    []models.InferenceInput{{Name: "prompt", Modality: models.ModalityText, Content: "describe"}},
	}
	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	protocolErr := errors.New("fixture protocol failure")
	invalidRequest := baseRequest
	invalidRequest.Inputs = []models.InferenceInput{{Name: "unknown", Modality: models.ModalityText, Content: "invalid"}}
	tests := []struct {
		name      string
		ctx       context.Context
		request   models.InvokeModelRequest
		fixture   protocolFixture
		wantClass models.InvocationFailureClass
		wantCalls int
		wantCause error
	}{
		{
			name: "canceled context", ctx: canceledContext, request: baseRequest,
			fixture: protocolFixture{response: PredictResponse{Text: "late output"}}, wantCalls: 0,
			wantCause: context.Canceled,
		},
		{
			name: "encode failure", ctx: context.Background(), request: invalidRequest,
			fixture: protocolFixture{response: PredictResponse{Text: "must not be used"}}, wantCalls: 0,
		},
		{
			name: "protocol failure", ctx: context.Background(), request: baseRequest,
			fixture: protocolFixture{err: protocolErr}, wantCalls: 1, wantCause: protocolErr,
		},
		{
			name: "blank response", ctx: context.Background(), request: baseRequest,
			fixture: protocolFixture{response: PredictResponse{}}, wantCalls: 1,
			wantClass: models.InvocationFailureClassMalformedResponse,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := test.fixture
			codec, err := NewOmniCodec(&fixture, PinnedOmniCapability())
			if err != nil {
				t.Fatalf("NewOmniCodec: %v", err)
			}
			result, err := codec.Invoke(test.ctx, test.request)
			if err == nil {
				t.Fatal("Invoke error = nil, want failure")
			}
			if !reflect.DeepEqual(result, OmniInvocationResult{}) {
				t.Fatalf("Invoke result = %#v, want zero result", result)
			}
			if fixture.calls != test.wantCalls {
				t.Fatalf("protocol calls = %d, want %d", fixture.calls, test.wantCalls)
			}
			if test.wantCause != nil && !errors.Is(err, test.wantCause) {
				t.Fatalf("Invoke error = %v, want cause %v", err, test.wantCause)
			}
			if test.wantClass != "" {
				var failure *models.InvocationFailure
				if !errors.As(err, &failure) || failure.Class != test.wantClass {
					t.Fatalf("Invoke error = %v, failure = %#v, want class %q", err, failure, test.wantClass)
				}
			}
		})
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
	err      error
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
	return fixture.response, fixture.err
}
