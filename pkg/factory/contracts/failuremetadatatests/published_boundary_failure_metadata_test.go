package failuremetadatatests

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerdiagnosticsmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/workerdiagnostics"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

func TestWorkFailureMetadataFromGenerated_MapsProviderFailureOnlyWireInput(t *testing.T) {
	family := factoryapi.WorkFailureFamily(workerexecution.WorkFailureFamilyRetryable)
	failureType := factoryapi.WorkFailureType(workerexecution.WorkFailureTypeInternalServerError)
	wire := &factoryapi.ProviderFailureMetadata{
		Family: &family,
		Type:   &failureType,
	}

	got := workerdiagnosticsmapping.WorkFailureMetadataFromGenerated(wire)
	if got == nil {
		t.Fatal("ingress failure metadata = nil, want retryable/internal_server_error")
	}
	if got.Family != workerexecution.WorkFailureFamilyRetryable {
		t.Fatalf("ingress family = %q, want retryable", got.Family)
	}
	if got.Type != workerexecution.WorkFailureTypeInternalServerError {
		t.Fatalf("ingress type = %q, want internal_server_error", got.Type)
	}
}

func TestWorkFailureMetadataFromGenerated_ReturnsNilForNilWire(t *testing.T) {
	if got := workerdiagnosticsmapping.WorkFailureMetadataFromGenerated(nil); got != nil {
		t.Fatalf("ingress failure metadata = %#v, want nil", got)
	}
}

func TestGeneratedWorkFailureMetadata_MapsFailureMetadataToPublishedWire(t *testing.T) {
	failure := &workerexecution.WorkFailureMetadata{
		Family: workerexecution.WorkFailureFamilyThrottle,
		Type:   workerexecution.WorkFailureTypeThrottled,
	}

	got := workerdiagnosticsmapping.GeneratedWorkFailureMetadata(failure)
	if got == nil {
		t.Fatal("published provider failure = nil, want throttle/throttled metadata")
	}
	if got.Family == nil || string(*got.Family) != string(workerexecution.WorkFailureFamilyThrottle) {
		t.Fatalf("published family = %#v, want throttle", got.Family)
	}
	if got.Type == nil || string(*got.Type) != string(workerexecution.WorkFailureTypeThrottled) {
		t.Fatalf("published type = %#v, want throttled", got.Type)
	}
}

func TestGeneratedWorkFailureMetadata_OmitsWhenFailureMetadataUnset(t *testing.T) {
	if got := workerdiagnosticsmapping.GeneratedWorkFailureMetadata(nil); got != nil {
		t.Fatalf("published provider failure = %#v, want nil", got)
	}
}

func TestGeneratedWorkFailureMetadataAndWorkFailureMetadataFromGenerated_RoundTripPreservesFamilyAndType(t *testing.T) {
	original := &workerexecution.WorkFailureMetadata{
		Family: workerexecution.WorkFailureFamilyRetryable,
		Type:   workerexecution.WorkFailureTypeInternalServerError,
	}

	wire := workerdiagnosticsmapping.GeneratedWorkFailureMetadata(original)
	if wire == nil {
		t.Fatal("published provider failure = nil, want retryable/internal_server_error wire")
	}

	got := workerdiagnosticsmapping.WorkFailureMetadataFromGenerated(wire)
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
