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

func TestPublishedProviderFailureMetadata_MapsFailureMetadataOnlyInternalInput(t *testing.T) {
	failure := &WorkFailureMetadata{
		Family: WorkFailureFamilyThrottle,
		Type:   WorkFailureTypeThrottled,
	}

	got := PublishedProviderFailureMetadata(failure, nil)
	if got == nil {
		t.Fatal("published provider failure = nil, want throttle/throttled metadata")
	}
	if got.Family == nil || string(*got.Family) != string(WorkFailureFamilyThrottle) {
		t.Fatalf("published family = %#v, want throttle", got.Family)
	}
	if got.Type == nil || string(*got.Type) != string(WorkFailureTypeThrottled) {
		t.Fatalf("published type = %#v, want throttled", got.Type)
	}
}

func TestPublishedProviderFailureMetadata_FallsBackToLegacyProviderFailureOnlyInput(t *testing.T) {
	providerFailure := &WorkFailureMetadata{
		Family: WorkFailureFamilyRetryable,
		Type:   WorkFailureTypeInternalServerError,
	}

	got := PublishedProviderFailureMetadata(nil, providerFailure)
	if got == nil {
		t.Fatal("published provider failure = nil, want legacy metadata on wire")
	}
	if got.Family == nil || string(*got.Family) != string(WorkFailureFamilyRetryable) {
		t.Fatalf("published family = %#v, want retryable", got.Family)
	}
	if got.Type == nil || string(*got.Type) != string(WorkFailureTypeInternalServerError) {
		t.Fatalf("published type = %#v, want internal_server_error", got.Type)
	}
}

func TestPublishedProviderFailureMetadata_PrefersFailureMetadataWhenBothSet(t *testing.T) {
	failure := &WorkFailureMetadata{
		Family: WorkFailureFamilyRetryable,
		Type:   WorkFailureTypeTimeout,
	}
	providerFailure := &WorkFailureMetadata{
		Family: WorkFailureFamilyThrottle,
		Type:   WorkFailureTypeThrottled,
	}

	got := PublishedProviderFailureMetadata(failure, providerFailure)
	if got == nil {
		t.Fatal("published provider failure = nil")
	}
	if got.Type == nil || string(*got.Type) != string(WorkFailureTypeTimeout) {
		t.Fatalf("published type = %#v, want failure_metadata precedence (timeout)", got.Type)
	}
}
