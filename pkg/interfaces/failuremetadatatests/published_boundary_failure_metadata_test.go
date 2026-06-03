package failuremetadatatests

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestPublishedProviderFailureMetadata_EmitsFromFailureMetadata(t *testing.T) {
	failure := &interfaces.WorkFailureMetadata{
		Family: interfaces.WorkFailureFamilyThrottle,
		Type:   interfaces.WorkFailureTypeThrottled,
	}

	got := interfaces.PublishedProviderFailureMetadata(failure)
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

func TestPublishedProviderFailureMetadata_OmitsWhenFailureMetadataUnset(t *testing.T) {
	if got := interfaces.PublishedProviderFailureMetadata(nil); got != nil {
		t.Fatalf("published provider failure = %#v, want nil", got)
	}
}

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
