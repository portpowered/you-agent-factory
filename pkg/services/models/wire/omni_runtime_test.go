package wire

import (
	"context"
	"errors"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

func TestNewInvocationRuntimeFailsClosedWhenOmniProtocolIsUnbound(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("scope:unbound-omni")
	if err != nil {
		t.Fatalf("scope.Parse: %v", err)
	}
	operation, ok := (models.GenericOperationCatalog{}).GenericOperationContract(models.OperationOMNI)
	if !ok {
		t.Fatal("GenericOperationContract(OMNI) = false")
	}
	runtime := newInvocationRuntime(nil)
	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request: models.InvokeModelRequest{
			Scope: scope, Holder: "unbound-test", Model: models.ModelReference{NameOrURI: "llm"},
			Operation: models.OperationOMNI,
			Inputs: []models.InferenceInput{
				{Name: "prompt", Modality: models.ModalityText, Content: "Write a haiku"},
			},
		},
		Operation: operation,
	})
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassBackendProtocol {
		t.Fatalf("Invoke error = %v, failure = %#v, want typed backend-protocol failure", err, failure)
	}
	if !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("Invoke error = %v, want ErrUnavailable cause", err)
	}
	if len(result.Content) != 0 {
		t.Fatalf("Invoke result = %#v, want no fallback content", result)
	}
}
