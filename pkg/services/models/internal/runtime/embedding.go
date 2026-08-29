package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai/codecs"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

// EmbeddingBackend is the private effect used after the EMBED codec has
// normalized one generic Models request. Protocol values never cross the
// Models service boundary.
type EmbeddingBackend func(
	context.Context,
	codecs.EmbeddingRequest,
) (codecs.EmbeddingResponse, error)

type embedding struct {
	codec   codecs.EmbedCodec
	backend EmbeddingBackend
}

// NewEmbedding constructs the Models-owned EMBED invocation runtime.
func NewEmbedding(backend EmbeddingBackend) (embedding, error) {
	if backend == nil {
		return embedding{}, models.ErrInvalidInferenceDependencies
	}
	return embedding{codec: codecs.NewEmbedCodec(), backend: backend}, nil
}

func (runtime embedding) Invoke(
	ctx context.Context,
	request inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if operation := strings.TrimSpace(request.Request.Operation); !strings.EqualFold(operation, models.OperationEMBED) {
		return inference.InvocationRuntimeResult{}, &models.InvocationFailure{
			Class:     models.InvocationFailureClassInvalidOperation,
			Operation: models.OperationEMBED,
			Message:   "EMBED runtime received an unsupported operation",
		}
	}
	if err := ctx.Err(); err != nil {
		return inference.InvocationRuntimeResult{}, err
	}

	backendRequest, err := runtime.codec.EncodeRequest(request.Request)
	if err != nil {
		return inference.InvocationRuntimeResult{}, err
	}
	backendResponse, err := runtime.backend(ctx, backendRequest)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return inference.InvocationRuntimeResult{}, err
		}
		var invocationFailure *models.InvocationFailure
		if errors.As(err, &invocationFailure) {
			return inference.InvocationRuntimeResult{}, enrichFailure(request.Request, invocationFailure)
		}
		model := request.Request.Model
		if model.IsZero() {
			model = models.ModelReference{NameOrURI: strings.TrimSpace(request.Request.ModelName)}
		}
		return inference.InvocationRuntimeResult{}, &models.InvocationFailure{
			Class:     models.InvocationFailureClassBackendProtocol,
			Model:     model,
			Operation: models.OperationEMBED,
			Message:   "EMBED backend invocation failed",
			Cause:     models.ErrInferenceFailed,
		}
	}
	if err := ctx.Err(); err != nil {
		return inference.InvocationRuntimeResult{}, err
	}

	content, err := runtime.codec.DecodeResponseValue(backendResponse)
	if err != nil {
		var invocationFailure *models.InvocationFailure
		if errors.As(err, &invocationFailure) {
			return inference.InvocationRuntimeResult{}, enrichFailure(request.Request, invocationFailure)
		}
		return inference.InvocationRuntimeResult{}, err
	}
	return inference.InvocationRuntimeResult{Content: []models.InferenceContent{content}}, nil
}

func enrichFailure(request models.InvokeModelRequest, failure *models.InvocationFailure) error {
	if failure == nil {
		return nil
	}
	clone := *failure
	if clone.Model.IsZero() {
		clone.Model = request.Model
		if clone.Model.IsZero() {
			clone.Model = models.ModelReference{NameOrURI: strings.TrimSpace(request.ModelName)}
		}
	}
	if strings.TrimSpace(clone.Operation) == "" {
		clone.Operation = models.OperationEMBED
	}
	clone.ValidNames = append([]string(nil), clone.ValidNames...)
	modelName := strings.TrimSpace(clone.Model.NameOrURI)
	if modelName == "" {
		modelName = strings.TrimSpace(request.ModelName)
	}
	if modelName == "" {
		modelName = models.BuiltInModelNameEmbed
	}
	if clone.Model.IsZero() {
		clone.Model = models.ModelReference{NameOrURI: modelName}
	}
	if modelName != "" && !strings.Contains(strings.ToLower(clone.Message), "for model") {
		clone.Message = fmt.Sprintf(
			"%s for model %q operation %q", clone.Message, modelName, clone.Operation,
		)
	}
	return &clone
}
