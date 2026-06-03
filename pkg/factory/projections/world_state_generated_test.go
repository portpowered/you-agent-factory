package projections

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestWorkstationResultFromGenerated_MapsProviderFailureOnlyWireToFailureMetadata(t *testing.T) {
	family := factoryapi.WorkFailureFamily(interfaces.WorkFailureFamilyRetryable)
	failureType := factoryapi.WorkFailureType(interfaces.WorkFailureTypeTimeout)
	payload := factoryapi.DispatchResponseEventPayload{
		Outcome: factoryapi.WorkOutcomeFailed,
		ProviderFailure: &factoryapi.ProviderFailureMetadata{
			Family: &family,
			Type:   &failureType,
		},
	}

	got := workstationResultFromGenerated(payload)
	if got.FailureMetadata == nil {
		t.Fatal("projected failure metadata = nil, want retryable/timeout from wire provider_failure")
	}
	if got.FailureMetadata.Family != interfaces.WorkFailureFamilyRetryable {
		t.Fatalf("projected family = %q, want retryable", got.FailureMetadata.Family)
	}
	if got.FailureMetadata.Type != interfaces.WorkFailureTypeTimeout {
		t.Fatalf("projected type = %q, want timeout", got.FailureMetadata.Type)
	}
	if got.ProviderFailure != nil {
		t.Fatalf("projected provider failure = %#v, want nil internal field", got.ProviderFailure)
	}
}

func TestWorkstationResultFromGenerated_OmitsFailureMetadataWhenWireUnset(t *testing.T) {
	got := workstationResultFromGenerated(factoryapi.DispatchResponseEventPayload{
		Outcome: factoryapi.WorkOutcomeAccepted,
	})
	if got.FailureMetadata != nil {
		t.Fatalf("projected failure metadata = %#v, want nil", got.FailureMetadata)
	}
}
