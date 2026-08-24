package service

import (
	"context"
	"errors"
	"reflect"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai/codecs"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

// EmbeddingBackend is the narrow backend seam required by the EMBED runtime.
// Implementations may be a protocol adapter or a deterministic test fixture;
// neither is allowed to leak backend details through the Models contract.
type EmbeddingBackend interface {
	InvokeEmbedding(context.Context, codecs.EmbeddingRequest) (codecs.EmbeddingResponse, error)
}

// EmbeddingInvocationRuntime maps one generic EMBED invocation through the
// codec and an injected backend.
type EmbeddingInvocationRuntime struct {
	codec   codecs.EmbedCodec
	backend EmbeddingBackend
}

// NewEmbeddingInvocationRuntime constructs an EMBED runtime with an explicit
// backend dependency.
func NewEmbeddingInvocationRuntime(backend EmbeddingBackend) (EmbeddingInvocationRuntime, error) {
	if backend == nil || isNilEmbeddingBackend(backend) {
		return EmbeddingInvocationRuntime{}, models.ErrInvalidInferenceDependencies
	}
	return EmbeddingInvocationRuntime{
		codec:   codecs.NewEmbedCodec(),
		backend: backend,
	}, nil
}

// Invoke implements the Models inference runtime seam.
func (runtime EmbeddingInvocationRuntime) Invoke(
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
	backendResponse, err := runtime.backend.InvokeEmbedding(ctx, backendRequest)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return inference.InvocationRuntimeResult{}, err
		}
		return inference.InvocationRuntimeResult{}, &models.InvocationFailure{
			Class:     models.InvocationFailureClassBackendProtocol,
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
		return inference.InvocationRuntimeResult{}, err
	}
	return inference.InvocationRuntimeResult{Content: []models.InferenceContent{content}}, nil
}

func isNilEmbeddingBackend(backend EmbeddingBackend) bool {
	value := reflect.ValueOf(backend)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
