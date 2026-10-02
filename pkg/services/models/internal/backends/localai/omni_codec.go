package localai

import (
	"context"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

// ProtocolInput is the codec-owned representation of one ordered value sent
// to LocalAI Predict. It keeps the detected media type and opaque artifact
// reference beside the content until the protocol adapter forwards them.
type ProtocolInput struct {
	Slot      string
	Modality  models.Modality
	MediaType string
	Content   string
	Reference string
}

// PredictRequest is the private protocol request assembled by the OMNI
// codec. Inputs preserve command order; Prompt is also projected into the
// pinned protocol's named prompt field.
type PredictRequest struct {
	Prompt     string
	Inputs     []ProtocolInput
	Parameters []models.OperationParameter
}

// PredictResponse is the detached text response and safe reply-shape counts
// returned by LocalAI's protocol adapter.
type PredictResponse struct {
	Text            string
	Usage           string
	ReplyBytes      int
	MessageBytes    int
	ChatDeltaCount  int
	ReasoningBytes  int
	GeneratedTokens int32
	PromptTokens    int32
	AudioBytes      int
}

// OmniInvocationResult is private LocalAI/Models output. Artifact identity is
// intentionally zero here; the Models Inference registrar owns opaque
// identity assignment.
type OmniInvocationResult struct {
	Content   []models.InferenceContent
	Artifacts []models.InferenceArtifact
}

// ProtocolClient is the only execution dependency needed by the codec. A
// production adapter or a deterministic fixture may implement it; neither
// leaks LocalAI protocol types through the Models public boundary.
type ProtocolClient interface {
	Predict(context.Context, PredictRequest) (PredictResponse, error)
}

// VideoAudioExtractor returns WAV bytes for a video's embedded audio track.
// An empty result means the video has no audio track.
type VideoAudioExtractor func(context.Context, []byte) ([]byte, error)

// OmniCodec owns OMNI validation, provider mapping, and response shaping.
type OmniCodec struct {
	client       ProtocolClient
	capability   OmniCapability
	extractAudio VideoAudioExtractor
}

// NewOmniCodec constructs an inert codec around one recorded capability. It
// performs no network, process, filesystem, or backend-artifact work.
func NewOmniCodec(client ProtocolClient, capability OmniCapability, extractors ...VideoAudioExtractor) (*OmniCodec, error) {
	if err := capability.Validate(); err != nil {
		return nil, err
	}
	codec := &OmniCodec{client: client, capability: capability.Clone()}
	if len(extractors) > 0 {
		codec.extractAudio = extractors[0]
	}
	return codec, nil
}

// NewPinnedOmniCodec constructs the codec with the pinned protocol capability.
// A nil client is allowed so callers can use Operation and Encode for pure
// validation/mapping tests; Invoke reports ErrUnavailable until a client is
// supplied.
func NewPinnedOmniCodec(client ProtocolClient, extractors ...VideoAudioExtractor) *OmniCodec {
	codec, err := NewOmniCodec(client, PinnedOmniCapability(), extractors...)
	if err != nil {
		return nil
	}
	return codec
}

// Capability returns detached capability facts used by this codec.
func (codec *OmniCodec) Capability() OmniCapability {
	if codec == nil {
		return OmniCapability{}
	}
	return codec.capability.Clone()
}

// Operation returns the effective provider-neutral OMNI contract selected by
// the pinned capability result.
func (codec *OmniCodec) Operation() models.Operation {
	if codec == nil {
		return models.Operation{}
	}
	return codec.capability.Operation()
}

// Encode validates and maps one generic provider-neutral request to the
// private LocalAI protocol request. Capability failures are returned before a
// protocol client can be called.
func (codec *OmniCodec) Encode(request models.InvokeModelRequest) (PredictRequest, error) {
	if codec == nil {
		return PredictRequest{}, models.ErrUnavailable
	}
	inputs := request.Inputs
	if inputs == nil && !isZeroInput(request.Input) {
		inputs = []models.InferenceInput{request.Input}
	}
	request.Inputs = inputs
	if err := codec.validateCapabilityInputs(request); err != nil {
		return PredictRequest{}, err
	}
	prepared, _, err := models.PrepareGenericInvocation(request, models.ModelDefinition{
		Name:       request.Model.NameOrURI,
		Operations: []models.Operation{codec.Operation()},
	})
	if err != nil {
		return PredictRequest{}, err
	}
	predict, err := mapPredictRequest(prepared)
	if err != nil {
		return PredictRequest{}, err
	}
	if _, err := omniGrammar(predict); err != nil {
		return PredictRequest{}, err
	}
	return predict, nil
}

// Invoke performs one protocol call after Encode has completed all local
// capability and slot validation, then returns provider-neutral content and
// detached artifact metadata. An optional effective operation lets response
// metadata be retained only when the caller's declared output contract
// includes that slot.
func (codec *OmniCodec) Invoke(
	ctx context.Context,
	request models.InvokeModelRequest,
	operations ...models.Operation,
) (OmniInvocationResult, error) {
	if err := ctx.Err(); err != nil {
		return OmniInvocationResult{}, err
	}
	predict, err := codec.Encode(request)
	if err != nil {
		return OmniInvocationResult{}, err
	}
	if codec == nil || codec.client == nil {
		return OmniInvocationResult{}, &models.InvocationFailure{
			Class:     models.InvocationFailureClassBackendProtocol,
			Model:     request.Model,
			Operation: models.OperationOMNI,
			Message:   "OMNI protocol client is unavailable",
			Cause:     models.ErrUnavailable,
		}
	}
	response, err := codec.invokePredict(ctx, request, predict)
	if err != nil {
		return OmniInvocationResult{}, err
	}
	if err := validateTextResponse(request, response); err != nil {
		return OmniInvocationResult{}, err
	}
	content := []models.InferenceContent{{
		Name:        "text",
		Modality:    models.ModalityText,
		ContentType: "text/plain",
		MediaType:   "text/plain",
		Content:     response.Text,
	}}
	if strings.TrimSpace(response.Usage) != "" && declaresOutputSlot(operations, "usage") {
		content = append(content, models.InferenceContent{
			Name: "usage", Modality: models.ModalityJSON,
			ContentType: "application/json", MediaType: "application/json",
			Content: response.Usage,
		})
	}
	return OmniInvocationResult{
		Content: content,
		Artifacts: []models.InferenceArtifact{{
			Name:      "text",
			MediaType: "text/plain",
			SizeBytes: int64(len([]byte(response.Text))),
		}},
	}, nil
}

type omniAudioSource struct {
	label string
	input ProtocolInput
}

func (codec *OmniCodec) invokePredict(
	ctx context.Context, request models.InvokeModelRequest, predict PredictRequest,
) (PredictResponse, error) {
	var audios []omniAudioSource
	var visuals []ProtocolInput
	videoCount := 0
	for _, input := range predict.Inputs {
		switch input.Modality {
		case models.ModalityAudio:
			audios = append(audios, omniAudioSource{
				label: fmt.Sprintf("Audio input %d", len(audios)+1), input: input,
			})
		case models.ModalityVideo:
			videoCount++
			visuals = append(visuals, input)
			if codec.extractAudio == nil || input.Content == "" {
				continue
			}
			if err := ctx.Err(); err != nil {
				return PredictResponse{}, err
			}
			wav, err := codec.extractAudio(ctx, []byte(input.Content))
			if err != nil {
				return PredictResponse{}, fmt.Errorf("extract audio from video input %d: %w", videoCount, err)
			}
			if len(wav) > 0 {
				audios = append(audios, omniAudioSource{
					label: fmt.Sprintf("Embedded audio from video input %d", videoCount),
					input: ProtocolInput{Slot: "audio", Modality: models.ModalityAudio,
						MediaType: "audio/wav", Content: string(wav)},
				})
			}
		case models.ModalityImage:
			visuals = append(visuals, input)
		}
	}
	if videoCount == 0 || len(audios) == 0 {
		return codec.predict(ctx, predict)
	}
	for _, parameter := range request.Parameters {
		if strings.TrimSpace(parameter.Name) == omniMaxTokensParameter {
			return PredictResponse{}, &models.InvocationFailure{
				Class: models.InvocationFailureClassInvalidParameter, Model: request.Model,
				Operation: models.OperationOMNI, Parameter: omniMaxTokensParameter,
				Message: "OMNI max_tokens is unsupported for combined audio and video because the response joins separate observations",
			}
		}
	}
	return codec.predictAudioVideo(ctx, request, predict, audios, visuals)
}

func (codec *OmniCodec) predictAudioVideo(
	ctx context.Context, request models.InvokeModelRequest, original PredictRequest,
	audios []omniAudioSource, visuals []ProtocolInput,
) (PredictResponse, error) {
	var observations strings.Builder
	for _, source := range audios {
		prompt := "Transcribe spoken words verbatim using words rather than digits where possible. " +
			"Describe other salient sounds. Only report audible facts."
		response, err := codec.predict(ctx, PredictRequest{
			Prompt: prompt, Parameters: original.Parameters,
			Inputs: []ProtocolInput{{Slot: "prompt", Modality: models.ModalityText,
				MediaType: "text/plain", Content: prompt}, source.input},
		})
		if err != nil {
			return PredictResponse{}, err
		}
		if err := validateTextResponse(request, response); err != nil {
			return PredictResponse{}, err
		}
		fmt.Fprintf(&observations, "%s: %s\n", source.label, strings.TrimSpace(response.Text))
	}
	videoPrompt := "Report visible events, on-screen words, colors, and their temporal order. " +
		"Only report visual facts."
	videoInputs := append([]ProtocolInput{{
		Slot: "prompt", Modality: models.ModalityText,
		MediaType: "text/plain", Content: videoPrompt,
	}}, visuals...)
	video, err := codec.predict(ctx, PredictRequest{
		Prompt: videoPrompt, Parameters: original.Parameters, Inputs: videoInputs,
	})
	if err != nil {
		return PredictResponse{}, err
	}
	if err := validateTextResponse(request, video); err != nil {
		return PredictResponse{}, err
	}
	// Preserve both observed modalities without another model pass that might
	// incorrectly deny an already observed audio source. Per-pass usage cannot
	// be represented as one reliable total in the single-response contract.
	return PredictResponse{Text: observations.String() + "Video: " + strings.TrimSpace(video.Text)}, nil
}

func (codec *OmniCodec) predict(ctx context.Context, request PredictRequest) (PredictResponse, error) {
	if err := ctx.Err(); err != nil {
		return PredictResponse{}, err
	}
	return codec.client.Predict(ctx, request)
}

func validateTextResponse(request models.InvokeModelRequest, response PredictResponse) error {
	if strings.TrimSpace(response.Text) != "" {
		return nil
	}
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassMalformedResponse, Model: request.Model,
		Operation: models.OperationOMNI, Slot: "text",
		Message: fmt.Sprintf(
			"OMNI response did not contain text output (reply_bytes=%d message_bytes=%d chat_delta_count=%d generated_tokens=%d prompt_tokens=%d audio_bytes=%d reasoning_bytes=%d)",
			response.ReplyBytes, response.MessageBytes, response.ChatDeltaCount,
			response.GeneratedTokens, response.PromptTokens, response.AudioBytes, response.ReasoningBytes,
		),
	}
}

func declaresOutputSlot(operations []models.Operation, name string) bool {
	if len(operations) == 0 {
		return true
	}
	for _, output := range operations[0].Outputs {
		if strings.EqualFold(strings.TrimSpace(output.Name), name) {
			return true
		}
	}
	return false
}

func (codec *OmniCodec) validateCapabilityInputs(request models.InvokeModelRequest) error {
	for _, input := range request.Inputs {
		if input.Modality != models.ModalityAudio && input.Modality != models.ModalityVideo {
			continue
		}
		if codec.capability.ModalitySupported(input.Modality) {
			continue
		}
		name := strings.TrimSpace(input.Name)
		return &models.InvocationFailure{
			Class:      models.InvocationFailureClassMediaCapability,
			Model:      request.Model,
			Operation:  models.OperationOMNI,
			Slot:       name,
			ValidNames: []string{string(input.Modality)},
			Message: fmt.Sprintf(
				"OMNI input slot %q uses unsupported modality %q for the resolved model capability",
				name, input.Modality,
			),
		}
	}
	return nil
}

func mapPredictRequest(request models.InvokeModelRequest) (PredictRequest, error) {
	predict := PredictRequest{
		Inputs:     make([]ProtocolInput, 0, len(request.Inputs)),
		Parameters: cloneParameters(request.Parameters),
	}
	for _, input := range request.Inputs {
		value, reference, err := inputValue(input)
		if err != nil {
			return PredictRequest{}, err
		}
		mapped := ProtocolInput{
			Slot:      strings.TrimSpace(input.Name),
			Modality:  input.Modality,
			MediaType: detectedMediaType(input),
			Content:   value,
			Reference: reference,
		}
		predict.Inputs = append(predict.Inputs, mapped)
		if mapped.Slot == "prompt" && mapped.Modality == models.ModalityText {
			predict.Prompt = mapped.Content
			if predict.Prompt == "" {
				predict.Prompt = mapped.Reference
			}
		}
	}
	return predict, nil
}

func inputValue(input models.InferenceInput) (string, string, error) {
	if strings.TrimSpace(input.Content) != "" {
		return input.Content, "", nil
	}
	if input.Artifact != nil && !input.Artifact.IsZero() {
		return "", input.Artifact.String(), nil
	}
	return input.Content, "", nil
}

func detectedMediaType(input models.InferenceInput) string {
	if mediaType := strings.TrimSpace(input.MediaType); mediaType != "" {
		return mediaType
	}
	if contentType := strings.TrimSpace(input.ContentType); contentType != "" && strings.Contains(contentType, "/") {
		return contentType
	}
	switch input.Modality {
	case models.ModalityText:
		return "text/plain"
	case models.ModalityImage:
		return "image/*"
	case models.ModalityAudio:
		return "audio/*"
	case models.ModalityVideo:
		return "video/*"
	case models.ModalityJSON:
		return "application/json"
	default:
		return "application/octet-stream"
	}
}

func cloneParameters(parameters []models.OperationParameter) []models.OperationParameter {
	if parameters == nil {
		return nil
	}
	cloned := make([]models.OperationParameter, len(parameters))
	for index, parameter := range parameters {
		cloned[index] = parameter.Clone()
	}
	return cloned
}

func isZeroInput(input models.InferenceInput) bool {
	return input.Name == "" && input.Modality == "" && input.ContentType == "" &&
		input.MediaType == "" && input.Content == "" && input.Artifact == nil
}
