package failuremetadatatests

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestWorkFailureMetadataFromGenerated_MapsProviderFailureOnlyWireInput(t *testing.T) {
	family := factoryapi.WorkFailureFamily(interfaces.WorkFailureFamilyRetryable)
	failureType := factoryapi.WorkFailureType(interfaces.WorkFailureTypeInternalServerError)
	wire := &factoryapi.ProviderFailureMetadata{
		Family: &family,
		Type:   &failureType,
	}

	got := interfaces.WorkFailureMetadataFromGenerated(wire)
	if got == nil {
		t.Fatal("ingress failure metadata = nil, want retryable/internal_server_error")
	}
	if got.Family != interfaces.WorkFailureFamilyRetryable {
		t.Fatalf("ingress family = %q, want retryable", got.Family)
	}
	if got.Type != interfaces.WorkFailureTypeInternalServerError {
		t.Fatalf("ingress type = %q, want internal_server_error", got.Type)
	}
}

func TestWorkFailureMetadataFromGenerated_ReturnsNilForNilWire(t *testing.T) {
	if got := interfaces.WorkFailureMetadataFromGenerated(nil); got != nil {
		t.Fatalf("ingress failure metadata = %#v, want nil", got)
	}
}

func TestGeneratedWorkFailureMetadata_MapsFailureMetadataToPublishedWire(t *testing.T) {
	failure := &interfaces.WorkFailureMetadata{
		Family: interfaces.WorkFailureFamilyThrottle,
		Type:   interfaces.WorkFailureTypeThrottled,
	}

	got := interfaces.GeneratedWorkFailureMetadata(failure)
	if got == nil {
		t.Fatal("published provider failure = nil, want throttle/throttled metadata")
	}
	if got.Family == nil || string(*got.Family) != string(interfaces.WorkFailureFamilyThrottle) {
		t.Fatalf("published family = %#v, want throttle", got.Family)
	}
	if got.Type == nil || string(*got.Type) != string(interfaces.WorkFailureTypeThrottled) {
		t.Fatalf("published type = %#v, want throttled", got.Type)
	}
}

func TestGeneratedWorkFailureMetadata_OmitsWhenFailureMetadataUnset(t *testing.T) {
	if got := interfaces.GeneratedWorkFailureMetadata(nil); got != nil {
		t.Fatalf("published provider failure = %#v, want nil", got)
	}
}

func TestGeneratedWorkFailureMetadataAndWorkFailureMetadataFromGenerated_RoundTripPreservesFamilyAndType(t *testing.T) {
	original := &interfaces.WorkFailureMetadata{
		Family: interfaces.WorkFailureFamilyRetryable,
		Type:   interfaces.WorkFailureTypeInternalServerError,
	}

	wire := interfaces.GeneratedWorkFailureMetadata(original)
	if wire == nil {
		t.Fatal("published provider failure = nil, want retryable/internal_server_error wire")
	}

	got := interfaces.WorkFailureMetadataFromGenerated(wire)
	if got == nil {
		t.Fatal("ingress failure metadata = nil, want retryable/internal_server_error")
	}
	if got.Family != original.Family {
		t.Fatalf("round-trip family = %q, want %q", got.Family, original.Family)
	}
	if got.Type != original.Type {
		t.Fatalf("round-trip type = %q, want %q", got.Type, original.Type)
	}
}
