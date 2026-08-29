package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai/codecs"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

func TestEmbeddingRuntimePreservesTypedFailureIdentityAndModel(t *testing.T) {
	runtime, err := NewEmbedding(func(context.Context, codecs.EmbeddingRequest) (codecs.EmbeddingResponse, error) {
		return codecs.EmbeddingResponse{}, nil
	})
	if err != nil {
		t.Fatalf("NewEmbedding() error = %v", err)
	}
	_, err = runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request: models.InvokeModelRequest{
			ModelName: "embed", Operation: models.OperationEMBED,
			Inputs: []models.InferenceInput{{
				Name: "text", Modality: models.ModalityText,
				ContentType: "text/plain", MediaType: "text/plain", Content: "query",
			}},
		},
	})
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMalformedResponse {
		t.Fatalf("Invoke() error = %v, failure = %#v, want malformed response", err, failure)
	}
	if failure.Model.NameOrURI != "embed" || !strings.Contains(failure.Message, "embed") || !strings.Contains(failure.Message, "EMBED") {
		t.Fatalf("failure = %#v, want model and operation identity", failure)
	}
}
