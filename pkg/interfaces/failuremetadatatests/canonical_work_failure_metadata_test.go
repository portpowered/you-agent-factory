package failuremetadatatests

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestCanonicalWorkFailureMetadata_PrefersFailureMetadataWhenBothSet(t *testing.T) {
	failure := &interfaces.WorkFailureMetadata{
		Family: interfaces.WorkFailureFamilyRetryable,
		Type:   interfaces.WorkFailureTypeTimeout,
	}
	providerFailure := &interfaces.WorkFailureMetadata{
		Family: interfaces.WorkFailureFamilyThrottle,
		Type:   interfaces.WorkFailureTypeThrottled,
	}

	got := interfaces.CanonicalWorkFailureMetadata(failure, providerFailure)
	if got != failure {
		t.Fatalf("canonical metadata = %#v, want failure_metadata pointer %#v", got, failure)
	}
}

func TestCanonicalWorkFailureMetadata_ReturnsFailureMetadataOnlyInput(t *testing.T) {
	failure := &interfaces.WorkFailureMetadata{
		Family: interfaces.WorkFailureFamilyRetryable,
		Type:   interfaces.WorkFailureTypeInternalServerError,
	}

	got := interfaces.CanonicalWorkFailureMetadata(failure, nil)
	if got != failure {
		t.Fatalf("canonical metadata = %#v, want failure_metadata %#v", got, failure)
	}
}

func TestCanonicalWorkFailureMetadata_FallsBackToLegacyProviderFailureOnlyInput(t *testing.T) {
	providerFailure := &interfaces.WorkFailureMetadata{
		Family: interfaces.WorkFailureFamilyThrottle,
		Type:   interfaces.WorkFailureTypeThrottled,
	}

	got := interfaces.CanonicalWorkFailureMetadata(nil, providerFailure)
	if got != providerFailure {
		t.Fatalf("canonical metadata = %#v, want legacy provider_failure %#v", got, providerFailure)
	}
}

func TestCanonicalWorkFailureMetadata_ReturnsNilWhenBothUnset(t *testing.T) {
	if got := interfaces.CanonicalWorkFailureMetadata(nil, nil); got != nil {
		t.Fatalf("canonical metadata = %#v, want nil", got)
	}
}

func TestGeneratedWorkFailureMetadata_MapsFailureMetadataOnlyInternalInput(t *testing.T) {
	failure := &interfaces.WorkFailureMetadata{
		Family: interfaces.WorkFailureFamilyThrottle,
		Type:   interfaces.WorkFailureTypeThrottled,
	}

	got := interfaces.GeneratedWorkFailureMetadata(interfaces.CanonicalWorkFailureMetadata(failure, nil))
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

func TestGeneratedWorkFailureMetadata_FallsBackToLegacyProviderFailureOnlyInput(t *testing.T) {
	providerFailure := &interfaces.WorkFailureMetadata{
		Family: interfaces.WorkFailureFamilyRetryable,
		Type:   interfaces.WorkFailureTypeInternalServerError,
	}

	got := interfaces.GeneratedWorkFailureMetadata(interfaces.CanonicalWorkFailureMetadata(nil, providerFailure))
	if got == nil {
		t.Fatal("published provider failure = nil, want legacy metadata on wire")
	}
	if got.Family == nil || string(*got.Family) != string(interfaces.WorkFailureFamilyRetryable) {
		t.Fatalf("published family = %#v, want retryable", got.Family)
	}
	if got.Type == nil || string(*got.Type) != string(interfaces.WorkFailureTypeInternalServerError) {
		t.Fatalf("published type = %#v, want internal_server_error", got.Type)
	}
}

func TestGeneratedWorkFailureMetadata_PrefersFailureMetadataWhenBothSet(t *testing.T) {
	failure := &interfaces.WorkFailureMetadata{
		Family: interfaces.WorkFailureFamilyRetryable,
		Type:   interfaces.WorkFailureTypeTimeout,
	}
	providerFailure := &interfaces.WorkFailureMetadata{
		Family: interfaces.WorkFailureFamilyThrottle,
		Type:   interfaces.WorkFailureTypeThrottled,
	}

	got := interfaces.GeneratedWorkFailureMetadata(interfaces.CanonicalWorkFailureMetadata(failure, providerFailure))
	if got == nil {
		t.Fatal("published provider failure = nil")
	}
	if got.Type == nil || string(*got.Type) != string(interfaces.WorkFailureTypeTimeout) {
		t.Fatalf("published type = %#v, want failure_metadata precedence (timeout)", got.Type)
	}
}
