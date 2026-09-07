package runtime

import (
	"context"
	"errors"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
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

type asr struct {
	codec   codecs.ASRCodec
	backend Backend
}

// New constructs the Models-owned ASR invocation runtime.
func New(backend Backend) (asr, error) {
	if backend == nil {
		return asr{}, models.ErrInvalidInferenceDependencies
	}
	return asr{codec: codecs.NewASRCodec(), backend: backend}, nil
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

	backendRequest, err := runtime.codec.EncodeRequest(request.Request)
	if err != nil {
		return inference.InvocationRuntimeResult{}, err
	}
	backendResponse, artifacts, err := runtime.backend(ctx, backendRequest)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return inference.InvocationRuntimeResult{}, err
		}
		return inference.InvocationRuntimeResult{}, &models.InvocationFailure{
			Class: models.InvocationFailureClassBackendProtocol, Operation: models.OperationASR,
			Message: "ASR backend invocation failed", Cause: models.ErrInferenceFailed,
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
