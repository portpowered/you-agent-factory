package localai

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
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
	if len(result.Content) != 1 || result.Content[0].Content !=
		"Audio input 1: LOCALAI_FIXTURE_OMNI\nVideo: LOCALAI_FIXTURE_OMNI" {
		t.Fatalf("content = %#v, want composed fixture observations", result.Content)
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
	encoded, err := codec.Encode(request)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !reflect.DeepEqual(encoded, want) {
		t.Fatalf("encoded request = %#v, want %#v", encoded, want)
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

func TestOmniCodecEmptyTextErrorContainsOnlyReplyShape(t *testing.T) {
	t.Parallel()
	fixture := &protocolFixture{response: PredictResponse{
		Text: " \t", Usage: `{"secret":"private-usage"}`,
		ReplyBytes: 42, MessageBytes: 2, ChatDeltaCount: 3, ReasoningBytes: 17,
		GeneratedTokens: 5, PromptTokens: 7, AudioBytes: 11,
	}}
	_, err := invokeSemanticOmni(t, fixture)
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMalformedResponse {
		t.Fatalf("Invoke error = %v, want malformed response", err)
	}
	const want = "OMNI response did not contain text output (reply_bytes=42 message_bytes=2 chat_delta_count=3 generated_tokens=5 prompt_tokens=7 audio_bytes=11 reasoning_bytes=17)"
	if failure.Message != want || err.Error() != want {
		t.Fatalf("failure message = %q, error = %q, want %q", failure.Message, err.Error(), want)
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

type scriptedOmniProtocol struct {
	requests  []PredictRequest
	responses []PredictResponse
	errors    map[int]error
	afterCall func(int)
}

func (fixture *scriptedOmniProtocol) Predict(_ context.Context, request PredictRequest) (PredictResponse, error) {
	index := len(fixture.requests)
	fixture.requests = append(fixture.requests, request)
	if fixture.afterCall != nil {
		fixture.afterCall(index)
	}
	if err := fixture.errors[index]; err != nil {
		return PredictResponse{}, err
	}
	return fixture.responses[index], nil
}

func omniMediaRequest(t *testing.T, inputs ...models.InferenceInput) models.InvokeModelRequest {
	t.Helper()
	scope, err := (models.RuntimeScopeRef{}).Parse("scope:media-composition")
	if err != nil {
		t.Fatal(err)
	}
	return models.InvokeModelRequest{
		Scope: scope, Holder: "media-composition", Model: models.ModelReference{NameOrURI: "llm"},
		Operation: models.OperationOMNI,
		Inputs: append([]models.InferenceInput{{Name: "prompt", Modality: models.ModalityText,
			Content: "Name the spoken word and visual phases"}}, inputs...),
	}
}

func TestOmniCodecComposesAudioAndVideoObservations(t *testing.T) {
	t.Parallel()
	fixture := &scriptedOmniProtocol{responses: []PredictResponse{
		{Text: "zero", Usage: `{"tokens":2}`},
		{Text: "red PHASE 1, blue PHASE 2", Usage: `{"tokens":4}`},
	}}
	codec := NewPinnedOmniCodec(fixture)
	result, err := codec.Invoke(context.Background(), omniMediaRequest(t,
		models.InferenceInput{Name: "audio", Modality: models.ModalityAudio, MediaType: "audio/wav", Content: "WAV"},
		models.InferenceInput{Name: "video", Modality: models.ModalityVideo, MediaType: "video/mp4", Content: "MP4"},
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(fixture.requests) != 2 || len(result.Content) != 1 ||
		result.Content[0].Content != "Audio input 1: zero\nVideo: red PHASE 1, blue PHASE 2" {
		t.Fatalf("calls = %#v, result = %#v", fixture.requests, result)
	}
	if len(fixture.requests[0].Inputs) != 2 || fixture.requests[0].Inputs[1].Modality != models.ModalityAudio ||
		len(fixture.requests[1].Inputs) != 2 || fixture.requests[1].Inputs[1].Modality != models.ModalityVideo {
		t.Fatalf("media passes = %#v, want audio and video only", fixture.requests)
	}
	if !strings.Contains(fixture.requests[0].Prompt, "using words rather than digits") ||
		!strings.Contains(fixture.requests[1].Prompt, "on-screen words, colors, and their temporal order") ||
		strings.Contains(fixture.requests[0].Prompt, "Name the spoken word and visual phases") ||
		strings.Contains(fixture.requests[1].Prompt, "Name the spoken word and visual phases") ||
		strings.Contains(fixture.requests[1].Prompt, "audio") ||
		strings.Contains(fixture.requests[1].Prompt, "speech") {
		t.Fatalf("modality prompts = %#v", fixture.requests)
	}
}

func TestOmniCodecCombinedMediaRejectsMaxTokensBeforePredict(t *testing.T) {
	t.Parallel()
	request := omniMediaRequest(t,
		models.InferenceInput{Name: "audio", Modality: models.ModalityAudio, Content: "WAV"},
		models.InferenceInput{Name: "video", Modality: models.ModalityVideo, Content: "MP4"},
	)
	request.Parameters = []models.OperationParameter{
		{Name: "max_tokens", Value: 1}, {Name: "temperature", Value: 0.2},
	}
	for _, test := range []struct {
		name      string
		request   models.InvokeModelRequest
		extractor VideoAudioExtractor
	}{
		{name: "explicit audio", request: request},
		{name: "embedded audio", request: func() models.InvokeModelRequest {
			videoOnly := request
			videoOnly.Inputs = append([]models.InferenceInput(nil), request.Inputs[0], request.Inputs[2])
			return videoOnly
		}(), extractor: func(context.Context, []byte) ([]byte, error) { return []byte("WAV"), nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := &scriptedOmniProtocol{}
			result, err := NewPinnedOmniCodec(fixture, test.extractor).Invoke(context.Background(), test.request)
			var failure *models.InvocationFailure
			if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassInvalidParameter ||
				failure.Parameter != "max_tokens" || len(fixture.requests) != 0 ||
				!reflect.DeepEqual(result, OmniInvocationResult{}) {
				t.Fatalf("result = %#v, error = %v, calls = %d", result, err, len(fixture.requests))
			}
		})
	}
}

func TestOmniCodecSingleModalityPreservesMaxTokens(t *testing.T) {
	t.Parallel()
	for _, modality := range []models.Modality{models.ModalityAudio, models.ModalityVideo} {
		fixture := &scriptedOmniProtocol{responses: []PredictResponse{{Text: "answer"}}}
		request := omniMediaRequest(t, models.InferenceInput{Name: strings.ToLower(string(modality)),
			Modality: modality, Content: "media"})
		request.Parameters = []models.OperationParameter{{Name: "max_tokens", Value: 1}}
		_, err := NewPinnedOmniCodec(fixture).Invoke(context.Background(), request)
		if err != nil || len(fixture.requests) != 1 ||
			!reflect.DeepEqual(fixture.requests[0].Parameters, request.Parameters) {
			t.Fatalf("modality %s: error = %v, calls = %#v", modality, err, fixture.requests)
		}
	}
}

func TestOmniCodecExtractsAndLabelsEmbeddedAudioSeparately(t *testing.T) {
	t.Parallel()
	var extracted int
	extractor := func(_ context.Context, video []byte) ([]byte, error) {
		extracted++
		if string(video) != "MP4" {
			t.Fatalf("extractor video = %q", video)
		}
		return []byte("EMBEDDED-WAV"), nil
	}
	fixture := &scriptedOmniProtocol{responses: []PredictResponse{
		{Text: "separate audio says zero"}, {Text: "embedded audio says one"},
		{Text: "red then blue"},
	}}
	codec := NewPinnedOmniCodec(fixture, extractor)
	request := omniMediaRequest(t,
		models.InferenceInput{Name: "audio", Modality: models.ModalityAudio, Content: "EXPLICIT-WAV"},
		models.InferenceInput{Name: "video", Modality: models.ModalityVideo, Content: "MP4"},
	)
	if _, err := codec.Encode(request); err != nil || extracted != 0 {
		t.Fatalf("Encode error = %v, extraction calls = %d, want pure Encode", err, extracted)
	}
	result, err := codec.Invoke(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if extracted != 1 || len(fixture.requests) != 3 ||
		fixture.requests[0].Inputs[1].Content != "EXPLICIT-WAV" ||
		fixture.requests[1].Inputs[1].Content != "EMBEDDED-WAV" {
		t.Fatalf("extraction = %d, requests = %#v", extracted, fixture.requests)
	}
	if len(result.Content) != 1 ||
		result.Content[0].Content != "Audio input 1: separate audio says zero\n"+
			"Embedded audio from video input 1: embedded audio says one\nVideo: red then blue" {
		t.Fatalf("composed output lost source labels: %#v", result.Content)
	}
}

func TestOmniCodecVideoWithoutAudioStaysSinglePass(t *testing.T) {
	t.Parallel()
	fixture := &scriptedOmniProtocol{responses: []PredictResponse{{Text: "visual answer"}}}
	codec := NewPinnedOmniCodec(fixture, func(context.Context, []byte) ([]byte, error) {
		return nil, nil
	})
	result, err := codec.Invoke(context.Background(), omniMediaRequest(t,
		models.InferenceInput{Name: "video", Modality: models.ModalityVideo, Content: "MP4"},
	))
	if err != nil || len(fixture.requests) != 1 || result.Content[0].Content != "visual answer" {
		t.Fatalf("result = %#v, error = %v, requests = %#v", result, err, fixture.requests)
	}
}

func TestOmniCodecMultiPassFailsAtomically(t *testing.T) {
	t.Parallel()
	request := omniMediaRequest(t,
		models.InferenceInput{Name: "audio", Modality: models.ModalityAudio, Content: "WAV"},
		models.InferenceInput{Name: "video", Modality: models.ModalityVideo, Content: "MP4"},
	)
	backendErr := errors.New("backend failed")
	for _, step := range []int{0, 1} {
		fixture := &scriptedOmniProtocol{responses: []PredictResponse{{Text: "audio"}, {Text: "video"}},
			errors: map[int]error{step: backendErr}}
		codec := NewPinnedOmniCodec(fixture)
		result, err := codec.Invoke(context.Background(), request)
		if !errors.Is(err, backendErr) || !reflect.DeepEqual(result, OmniInvocationResult{}) ||
			len(fixture.requests) != step+1 {
			t.Fatalf("step %d: result = %#v, error = %v, calls = %d", step, result, err, len(fixture.requests))
		}
	}
	fixture := &scriptedOmniProtocol{responses: []PredictResponse{{Text: "audio"}, {Text: "video"}}}
	ctx, cancel := context.WithCancel(context.Background())
	fixture.afterCall = func(index int) {
		if index == 0 {
			cancel()
		}
	}
	result, err := NewPinnedOmniCodec(fixture).Invoke(ctx, request)
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(result, OmniInvocationResult{}) ||
		len(fixture.requests) != 1 {
		t.Fatalf("canceled result = %#v, error = %v, calls = %d", result, err, len(fixture.requests))
	}
	fixture = &scriptedOmniProtocol{responses: []PredictResponse{{Text: "unused"}}}
	result, err = NewPinnedOmniCodec(fixture, func(context.Context, []byte) ([]byte, error) {
		return nil, backendErr
	}).Invoke(context.Background(), request)
	if !errors.Is(err, backendErr) || !reflect.DeepEqual(result, OmniInvocationResult{}) || len(fixture.requests) != 0 {
		t.Fatalf("extraction result = %#v, error = %v, calls = %d", result, err, len(fixture.requests))
	}
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

func TestOmniPredictOptionsLeavesTokensUnboundedByDefault(t *testing.T) {
	t.Parallel()

	options, err := predictOptions(PredictRequest{
		Prompt:     "hello",
		Parameters: []models.OperationParameter{{Name: "temperature", Value: 0.2}},
	})
	if err != nil {
		t.Fatalf("predictOptions() error = %v", err)
	}
	if options.Tokens != 0 {
		t.Fatalf("Tokens = %d, want unbounded default 0", options.Tokens)
	}
	if options.Metadata["temperature"] != "0.2" {
		t.Fatalf("Metadata = %#v, want temperature passthrough", options.Metadata)
	}
	if _, ok := options.Metadata["max_tokens"]; ok {
		t.Fatalf("Metadata = %#v, want no max_tokens entry", options.Metadata)
	}
}

func TestOmniPredictOptionsMapsExplicitMaxTokens(t *testing.T) {
	t.Parallel()

	for _, value := range []any{int(128), int32(64), int64(256), float64(512), json.Number("1024")} {
		options, err := predictOptions(PredictRequest{
			Prompt: "hello",
			Parameters: []models.OperationParameter{
				{Name: "temperature", Value: 0.2},
				{Name: "max_tokens", Value: value},
			},
		})
		if err != nil {
			t.Fatalf("predictOptions(max_tokens=%v) error = %v", value, err)
		}
		var want int32
		switch raw := value.(type) {
		case int:
			want = int32(raw)
		case int32:
			want = raw
		case int64:
			want = int32(raw)
		case float64:
			want = int32(raw)
		case json.Number:
			want = 1024
		}
		if options.Tokens != want {
			t.Fatalf("predictOptions(max_tokens=%v) Tokens = %d, want %d", value, options.Tokens, want)
		}
		if _, ok := options.Metadata["max_tokens"]; ok {
			t.Fatalf("predictOptions(max_tokens=%v) Metadata = %#v, want max_tokens excluded", value, options.Metadata)
		}
		if options.Metadata["temperature"] != "0.2" {
			t.Fatalf("predictOptions(max_tokens=%v) Metadata = %#v, want temperature passthrough", value, options.Metadata)
		}
	}
}

func TestOmniPredictOptionsAcceptsInt32UpperBound(t *testing.T) {
	t.Parallel()

	options, err := predictOptions(PredictRequest{
		Parameters: []models.OperationParameter{{Name: "max_tokens", Value: math.MaxInt32}},
	})
	if err != nil {
		t.Fatalf("predictOptions() error = %v", err)
	}
	if options.Tokens != math.MaxInt32 {
		t.Fatalf("Tokens = %d, want %d", options.Tokens, int32(math.MaxInt32))
	}
}

func TestOmniPredictOptionsRejectsInvalidMaxTokens(t *testing.T) {
	t.Parallel()

	invalid := []any{
		0, -1, -100,
		int64(0), int64(-5),
		float64(1.5), float32(2.5),
		"100", "128", true, nil,
		map[string]any{"n": 1}, []any{1},
		float64(math.NaN()), float64(math.Inf(1)),
		float64(math.MaxInt32) + 1,
		int64(math.MaxInt32) + 1,
		uint64(math.MaxInt32) + 1,
		json.Number("1.5"), json.Number("abc"), json.Number("0"),
	}
	for _, value := range invalid {
		_, err := predictOptions(PredictRequest{
			Prompt:     "hello",
			Parameters: []models.OperationParameter{{Name: "max_tokens", Value: value}},
		})
		var failure *models.InvocationFailure
		if !errors.As(err, &failure) ||
			failure.Class != models.InvocationFailureClassInvalidParameter ||
			failure.Operation != models.OperationOMNI ||
			failure.Parameter != "max_tokens" {
			t.Fatalf("predictOptions(max_tokens=%#v) error = %v, failure = %#v, want typed OMNI/max_tokens InvalidParameter", value, err, failure)
		}
	}
}

func TestOmniPredictSurfacesInvalidMaxTokensWithoutProtocolMask(t *testing.T) {
	t.Parallel()

	client := NewPinnedGRPCProtocolClient(recordingGRPCDialer{connection: &recordingGRPCConnection{}})
	_, err := client.Predict(
		WithInvocationEndpoint(context.Background(), "127.0.0.1:50051"),
		PredictRequest{
			Prompt:     "hello",
			Parameters: []models.OperationParameter{{Name: "max_tokens", Value: 0}},
		},
	)
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) ||
		failure.Class != models.InvocationFailureClassInvalidParameter ||
		failure.Operation != models.OperationOMNI ||
		failure.Parameter != "max_tokens" {
		t.Fatalf("Predict() error = %v, failure = %#v, want typed OMNI/max_tokens InvalidParameter", err, failure)
	}
}

func TestEmbeddingKeepsMaxTokensAsMetadata(t *testing.T) {
	t.Parallel()

	options, err := embeddingOptions(models.EmbeddingBackendRequest{
		Text:       "query",
		Parameters: map[string]any{"max_tokens": 128, "normalize": true},
	})
	if err != nil {
		t.Fatalf("embeddingOptions() error = %v", err)
	}
	if options.Tokens != 0 {
		t.Fatalf("Tokens = %d, want embedding default 0", options.Tokens)
	}
	if options.Metadata["max_tokens"] != "128" || options.Metadata["normalize"] != "true" {
		t.Fatalf("Metadata = %#v, want legacy max_tokens passthrough", options.Metadata)
	}
}
