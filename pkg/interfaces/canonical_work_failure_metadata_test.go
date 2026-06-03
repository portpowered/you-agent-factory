package interfaces

import "testing"

func TestCanonicalWorkFailureMetadata_PrefersFailureMetadataWhenBothSet(t *testing.T) {
	failure := &WorkFailureMetadata{
		Family: WorkFailureFamilyRetryable,
		Type:   WorkFailureTypeTimeout,
	}
	providerFailure := &WorkFailureMetadata{
		Family: WorkFailureFamilyThrottle,
		Type:   WorkFailureTypeThrottled,
	}

	got := CanonicalWorkFailureMetadata(failure, providerFailure)
	if got != failure {
		t.Fatalf("canonical metadata = %#v, want failure_metadata pointer %#v", got, failure)
	}
}

func TestCanonicalWorkFailureMetadata_ReturnsFailureMetadataOnlyInput(t *testing.T) {
	failure := &WorkFailureMetadata{
		Family: WorkFailureFamilyRetryable,
		Type:   WorkFailureTypeInternalServerError,
	}

	got := CanonicalWorkFailureMetadata(failure, nil)
	if got != failure {
		t.Fatalf("canonical metadata = %#v, want failure_metadata %#v", got, failure)
	}
}

func TestCanonicalWorkFailureMetadata_FallsBackToLegacyProviderFailureOnlyInput(t *testing.T) {
	providerFailure := &WorkFailureMetadata{
		Family: WorkFailureFamilyThrottle,
		Type:   WorkFailureTypeThrottled,
	}

	got := CanonicalWorkFailureMetadata(nil, providerFailure)
	if got != providerFailure {
		t.Fatalf("canonical metadata = %#v, want legacy provider_failure %#v", got, providerFailure)
	}
}

func TestCanonicalWorkFailureMetadata_ReturnsNilWhenBothUnset(t *testing.T) {
	if got := CanonicalWorkFailureMetadata(nil, nil); got != nil {
		t.Fatalf("canonical metadata = %#v, want nil", got)
	}
}
