package wire

import (
	"context"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	localai "github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai"
	modelcodecs "github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai/codecs"
	modelsruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/runtime"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

func invocationArtifactSources(artifacts []models.InferenceArtifact) []inference.InvocationArtifactSource {
	if len(artifacts) == 0 {
		return nil
	}
	sources := make([]inference.InvocationArtifactSource, 0, len(artifacts))
	for _, artifact := range artifacts {
		sources = append(sources, inference.InvocationArtifactSource{
			RefValue:   artifact.Artifact.String(),
			Name:       artifact.Name,
			MediaType:  artifact.MediaType,
			SizeBytes:  artifact.SizeBytes,
			Properties: artifact.Properties,
		})
	}
	return sources
}

// NewGenericInvocationRuntime normalizes the optional public override.
func NewGenericInvocationRuntime(backend InvocationBackend) GenericInvocationRuntime {
	if backend == nil {
		return modelsruntime.UnavailableInvocationRuntime{}
	}
	return modelsruntime.NewGenericInvocationRuntime(modelsruntime.InvocationBackend(backend))
}

// NewUnavailableInvocationRuntime supplies an explicit fail-closed role.
func NewUnavailableInvocationRuntime() InvocationRuntime {
	return modelsruntime.UnavailableInvocationRuntime{}
}

func NewASRInvocationRuntime(backend ASRBackend, extract, normalize InvocationAudioTransform) (ASRInvocationRuntime, error) {
	if backend == nil {
		return nil, models.ErrInvalidInferenceDependencies
	}
	return modelsruntime.New(func(
		ctx context.Context,
		request modelcodecs.ASRRequest,
	) (modelcodecs.ASRResponse, []models.InferenceArtifact, error) {
		response, err := backend(ctx, models.ASRBackendRequest{
			Audio: append([]byte(nil), request.Audio...), MediaType: request.MediaType,
			Prompt: request.Prompt, Parameters: cloneInvocationParameters(request.Parameters),
		})
		if err != nil {
			return modelcodecs.ASRResponse{}, nil, err
		}
		segments := make([]modelcodecs.ASRSegment, len(response.Segments))
		for index, segment := range response.Segments {
			segments[index] = modelcodecs.ASRSegment{
				ID: segment.ID, Start: segment.Start, End: segment.End, Text: segment.Text,
			}
		}
		return modelcodecs.ASRResponse{Text: response.Text, Segments: segments}, response.Artifacts, nil
	}, modelsruntime.VideoAudioExtractor(extract), modelsruntime.VideoAudioExtractor(normalize))
}

func NewEmbeddingInvocationRuntime(backend EmbeddingBackend) (EmbeddingInvocationRuntime, error) {
	if backend == nil {
		return nil, models.ErrInvalidInferenceDependencies
	}
	return modelsruntime.NewEmbedding(func(
		ctx context.Context,
		request modelcodecs.EmbeddingRequest,
	) (modelcodecs.EmbeddingResponse, error) {
		response, err := backend(ctx, models.EmbeddingBackendRequest{
			Text:       request.Prompt,
			Parameters: cloneInvocationParameters(request.Parameters),
		})
		if err != nil {
			return modelcodecs.EmbeddingResponse{}, err
		}
		return modelcodecs.EmbeddingResponse{
			Embeddings: append([]float64(nil), response.Embeddings...),
		}, nil
	})
}

type InvocationProtocol = localai.ProtocolClient
type InvocationOMNICodec = localai.OmniCodec
type InvocationAudioTransform = localai.VideoAudioExtractor
type InvocationTTSBackend = modelsruntime.TTSBackend

func NewTTSInvocationRuntime(backend InvocationTTSBackend) (TTSInvocationRuntime, error) {
	return modelsruntime.NewTTS(backend)
}

type omniInvocationRuntime struct{ codec *InvocationOMNICodec }

func NewOMNIInvocationRuntime(codec *InvocationOMNICodec) OMNIInvocationRuntime {
	return omniInvocationRuntime{codec: codec}
}

func NewInvocationProtocolAdapter(client InvocationProtocolClient) InvocationProtocol {
	return invocationProtocolAdapter{client: client}
}

func NewPinnedInvocationProtocol(dialer InvocationProtocolDialer) InvocationProtocol {
	return localai.NewPinnedGRPCProtocolClient(dialer)
}

func NewInvocationOMNICodec(client InvocationProtocol, extract InvocationAudioTransform) *InvocationOMNICodec {
	return localai.NewPinnedOmniCodec(client, extract)
}

func (runtime omniInvocationRuntime) Invoke(
	ctx context.Context,
	request inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	if !isOMNIOperation(request) {
		return modelsruntime.UnavailableInvocationRuntime{}.Invoke(ctx, request)
	}
	if runtime.codec == nil {
		return inference.InvocationRuntimeResult{}, models.ErrUnavailable
	}
	ctx = localai.WithInvocationEndpoint(ctx, request.HostSlot.Endpoint)
	omniResult, err := runtime.codec.Invoke(ctx, request.Request, request.Operation)
	if err != nil {
		return inference.InvocationRuntimeResult{}, err
	}
	return inference.InvocationRuntimeResult{
		Content:   omniResult.Content,
		Artifacts: invocationArtifactSources(omniResult.Artifacts),
	}, nil
}

type invocationProtocolAdapter struct {
	client InvocationProtocolClient
}

func (adapter invocationProtocolAdapter) Predict(
	ctx context.Context,
	request localai.PredictRequest,
) (localai.PredictResponse, error) {
	inputs := make([]models.InvocationProtocolInput, len(request.Inputs))
	for index, input := range request.Inputs {
		inputs[index] = models.InvocationProtocolInput{
			Slot: input.Slot, Modality: input.Modality, MediaType: input.MediaType,
			Content: input.Content, Reference: input.Reference,
		}
	}
	response, err := adapter.client.Predict(ctx, models.InvocationProtocolRequest{
		Operation: models.OperationOMNI,
		Prompt:    request.Prompt, Inputs: inputs, Parameters: request.Parameters,
	})
	if err != nil {
		return localai.PredictResponse{}, err
	}
	return localai.PredictResponse{Text: response.Text, Usage: response.Usage}, nil
}

func isOMNIOperation(request inference.InvocationRuntimeRequest) bool {
	return invocationOperationName(request) == models.OperationOMNI
}

func invocationOperationName(request inference.InvocationRuntimeRequest) string {
	operation := request.Operation.Name
	if operation == "" {
		operation = request.Request.Operation
	}
	return strings.ToUpper(strings.TrimSpace(operation))
}

func cloneInvocationParameters(parameters map[string]any) map[string]any {
	if parameters == nil {
		return nil
	}
	cloned := make(map[string]any, len(parameters))
	for name, value := range parameters {
		cloned[name] = cloneInvocationParameterValue(value)
	}
	return cloned
}

func cloneInvocationParameterValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneInvocationParameters(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneInvocationParameterValue(item)
		}
		return cloned
	default:
		return value
	}
}
