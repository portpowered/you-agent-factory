package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai/codecs"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

// Backend is the private effect used after the ASR codec has normalized one
// generic Models request. Its protocol values never cross the Models service
// boundary.
type Backend func(
	context.Context,
	codecs.ASRRequest,
) (codecs.ASRResponse, []models.InferenceArtifact, error)

type VideoAudioExtractor func(context.Context, []byte) ([]byte, error)

type asr struct {
	extractAudio   VideoAudioExtractor
	normalizeAudio VideoAudioExtractor
	codec          codecs.ASRCodec
	backend        Backend
}

// New constructs the Models-owned ASR invocation runtime.
func New(backend Backend, extractAudio, normalizeAudio VideoAudioExtractor) (asr, error) {
	if backend == nil {
		return asr{}, models.ErrInvalidInferenceDependencies
	}
	return asr{codec: codecs.NewASRCodec(), backend: backend, extractAudio: extractAudio, normalizeAudio: normalizeAudio}, nil
}

func (runtime asr) Invoke(
	ctx context.Context,
	request inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if operation := strings.TrimSpace(request.Request.Operation); !strings.EqualFold(operation, models.OperationASR) {
		return inference.InvocationRuntimeResult{}, &models.InvocationFailure{
			Class: models.InvocationFailureClassInvalidOperation, Operation: models.OperationASR,
			Message: "ASR runtime received an unsupported operation",
		}
	}
	if err := ctx.Err(); err != nil {
		return inference.InvocationRuntimeResult{}, err
	}

	prepared, err := runtime.prepareVideoAudio(ctx, request.Request)
	if err != nil {
		return inference.InvocationRuntimeResult{}, err
	}
	backendRequest, err := runtime.codec.EncodeRequest(prepared)
	if err != nil {
		return inference.InvocationRuntimeResult{}, err
	}
	backendResponse, artifacts, err := runtime.backend(ctx, backendRequest)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return inference.InvocationRuntimeResult{}, err
		}
		message := "ASR backend invocation failed"
		var failure *models.InvocationFailure
		if errors.As(err, &failure) && failure.Message != "" {
			message = failure.Message
		}
		return inference.InvocationRuntimeResult{}, &models.InvocationFailure{
			Class: models.InvocationFailureClassBackendProtocol, Operation: models.OperationASR,
			Message: message, Cause: errors.Join(models.ErrInferenceFailed, err),
		}
	}
	if err := ctx.Err(); err != nil {
		return inference.InvocationRuntimeResult{}, err
	}
	content, err := runtime.codec.DecodeResponseValueWithinAudio(backendResponse, backendRequest.Audio)
	if err != nil {
		return inference.InvocationRuntimeResult{}, err
	}
	return inference.InvocationRuntimeResult{
		Content:   content,
		Artifacts: artifactSources(artifacts),
	}, nil
}

func artifactSources(artifacts []models.InferenceArtifact) []inference.InvocationArtifactSource {
	if len(artifacts) == 0 {
		return nil
	}
	sources := make([]inference.InvocationArtifactSource, 0, len(artifacts))
	for _, artifact := range artifacts {
		clone := artifact.Clone()
		sources = append(sources, inference.InvocationArtifactSource{
			RefValue:   clone.Artifact.String(),
			Name:       clone.Name,
			MediaType:  clone.MediaType,
			SizeBytes:  clone.SizeBytes,
			Properties: clone.Properties,
		})
	}
	return sources
}

// prepareVideoAudio replaces only the declared audio slot's video container.
// The codec continues to validate the resulting audio and all other slots.
func (runtime asr) prepareVideoAudio(ctx context.Context, request models.InvokeModelRequest) (models.InvokeModelRequest, error) {
	inputs := request.Inputs
	if len(inputs) == 0 {
		inputs = []models.InferenceInput{request.Input}
	}
	prepared := append([]models.InferenceInput(nil), inputs...)
	for index, input := range prepared {
		extract, required := runtime.audioPreparation(ctx, input)
		if !required {
			continue
		}
		if extract == nil {
			return request, asrVideoFailure("ASR audio normalization is unavailable", nil)
		}
		wav, err := extract(ctx, []byte(input.Content))
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return request, err
			}
			return request, asrVideoFailure(fmt.Sprintf("ASR video audio extraction failed: %v", err), err)
		}
		if len(wav) == 0 {
			return request, asrVideoFailure("ASR video input has no audio track", nil)
		}
		input.Modality, input.MediaType, input.ContentType, input.Content = models.ModalityAudio, "audio/wav", "audio/wav", string(wav)
		prepared[index] = input
	}
	if len(request.Inputs) == 0 {
		request.Input = prepared[0]
	} else {
		request.Inputs = prepared
	}
	return request, nil
}

func (runtime asr) audioPreparation(ctx context.Context, input models.InferenceInput) (VideoAudioExtractor, bool) {
	if asrVideoInput(input) {
		return runtime.extractAudio, true
	}
	media := strings.ToLower(strings.TrimSpace(input.MediaType))
	if media == "" {
		media = strings.ToLower(strings.TrimSpace(input.ContentType))
	}
	if localai.InvocationBackend(ctx) == "localai-qwen3-asr-cpp" && strings.TrimSpace(input.Name) == "audio" &&
		input.Modality == models.ModalityAudio && input.Artifact == nil && strings.HasPrefix(media, "audio/") && len(input.Content) > 0 &&
		!codecs.IsMono16KPCMWAV([]byte(input.Content)) {
		return runtime.normalizeAudio, true
	}
	return nil, false
}

func asrVideoFailure(message string, cause error) error {
	return &models.InvocationFailure{Class: models.InvocationFailureClassMediaCapability, Operation: models.OperationASR, Message: message, Cause: cause}
}

func asrVideoInput(input models.InferenceInput) bool {
	media := strings.TrimSpace(input.MediaType)
	if media == "" {
		media = strings.TrimSpace(input.ContentType)
	}
	return strings.TrimSpace(input.Name) == "audio" && strings.HasPrefix(strings.ToLower(media), "video/") && input.Artifact == nil && len(input.Content) > 0 && (input.Modality == models.ModalityAudio || input.Modality == models.ModalityVideo)
}

// InvocationRuntime is one completed Models operation role.
type InvocationRuntime interface {
	Invoke(context.Context, inference.InvocationRuntimeRequest) (inference.InvocationRuntimeResult, error)
}

type InvocationBackend func(context.Context, models.InvokeModelRequest) ([]models.InferenceContent, []models.InferenceArtifact, error)

// NewInvocationRuntime validates captured roles once, before any operation.
func NewInvocationRuntime(generic, omni, asr, embedding, tts InvocationRuntime) (InvocationRuntime, error) {
	for _, role := range []InvocationRuntime{generic, omni, asr, embedding, tts} {
		if role == nil {
			return nil, models.ErrInvalidInferenceDependencies
		}
		value := reflect.ValueOf(role)
		switch value.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
			if value.IsNil() {
				return nil, models.ErrInvalidInferenceDependencies
			}
		}
	}
	return operationInvocationRuntime{generic: generic, omni: omni, asr: asr, embedding: embedding, tts: tts}, nil
}

// NewGenericInvocationRuntime captures a required backend effect.
func NewGenericInvocationRuntime(backend InvocationBackend) InvocationRuntime {
	return backendInvocationRuntime{backend: backend}
}

type backendInvocationRuntime struct {
	backend InvocationBackend
}

func (runtime backendInvocationRuntime) Invoke(
	ctx context.Context,
	request inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	content, artifacts, err := runtime.backend(ctx, request.Request)
	if err != nil {
		return inference.InvocationRuntimeResult{}, err
	}
	return inference.InvocationRuntimeResult{
		Content:   content,
		Artifacts: artifactSources(artifacts),
	}, nil
}

type operationInvocationRuntime struct {
	generic   InvocationRuntime
	omni      InvocationRuntime
	asr       InvocationRuntime
	embedding InvocationRuntime
	tts       InvocationRuntime
}

func (runtime operationInvocationRuntime) Invoke(
	ctx context.Context,
	request inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	if isASROperation(request) {
		return runtime.asr.Invoke(
			localai.WithInvocationEndpoint(ctx, request.HostSlot.Endpoint), request,
		)
	}
	if isOMNIOperation(request) {
		return runtime.omni.Invoke(ctx, request)
	}
	if isEmbeddingOperation(request) {
		return runtime.embedding.Invoke(
			localai.WithInvocationEndpoint(ctx, request.HostSlot.Endpoint), request,
		)
	}
	if isTTSOperation(request) {
		return runtime.tts.Invoke(
			localai.WithInvocationEndpoint(ctx, request.HostSlot.Endpoint), request,
		)
	}
	return runtime.generic.Invoke(ctx, request)
}

// UnavailableInvocationRuntime is the production default when no operation
// adapter was composed. It intentionally emits no content, preserving the
// distinction between an unavailable backend and generated model output.
type UnavailableInvocationRuntime struct{}

func (UnavailableInvocationRuntime) Invoke(
	ctx context.Context,
	request inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return inference.InvocationRuntimeResult{}, err
		}
	}
	operation := strings.TrimSpace(request.Operation.Name)
	if operation == "" {
		operation = strings.TrimSpace(request.Request.Operation)
	}
	return inference.InvocationRuntimeResult{}, &models.InvocationFailure{
		Class:     models.InvocationFailureClassConfiguration,
		Operation: operation,
		Message:   "model operation backend is unavailable",
		Cause:     models.ErrUnavailable,
	}
}

func isOMNIOperation(request inference.InvocationRuntimeRequest) bool {
	return invocationOperationName(request) == models.OperationOMNI
}

func isASROperation(request inference.InvocationRuntimeRequest) bool {
	return invocationOperationName(request) == models.OperationASR
}

func isEmbeddingOperation(request inference.InvocationRuntimeRequest) bool {
	return invocationOperationName(request) == models.OperationEMBED
}

func isTTSOperation(request inference.InvocationRuntimeRequest) bool {
	return invocationOperationName(request) == models.OperationTTS
}

func invocationOperationName(request inference.InvocationRuntimeRequest) string {
	operation := request.Operation.Name
	if operation == "" {
		operation = request.Request.Operation
	}
	return strings.ToUpper(strings.TrimSpace(operation))
}
