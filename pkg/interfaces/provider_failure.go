package interfaces

// WorkFailureFamily captures the runtime behavior category for a normalized
// work failure.
type WorkFailureFamily string

const (
	WorkFailureFamilyTerminal  WorkFailureFamily = "terminal"
	WorkFailureFamilyRetryable WorkFailureFamily = "retryable"
	WorkFailureFamilyThrottle  WorkFailureFamily = "throttle"
)

// ProviderErrorFamily remains as a compatibility alias while runtime paths
// transition to generalized work-failure naming.
type ProviderErrorFamily = WorkFailureFamily

const (
	ProviderErrorFamilyTerminal  ProviderErrorFamily = WorkFailureFamilyTerminal
	ProviderErrorFamilyRetryable ProviderErrorFamily = WorkFailureFamilyRetryable
	ProviderErrorFamilyThrottle  ProviderErrorFamily = WorkFailureFamilyThrottle
)

// WorkFailureType is the stable customer-facing normalized failure type for
// scoped runtime work execution paths.
type WorkFailureType string

const (
	WorkFailureTypeAuthFailure         WorkFailureType = "auth_failure"
	WorkFailureTypePermanentBadRequest WorkFailureType = "permanent_bad_request"
	WorkFailureTypeThrottled           WorkFailureType = "throttled"
	WorkFailureTypeInternalServerError WorkFailureType = "internal_server_error"
	WorkFailureTypeTimeout             WorkFailureType = "timeout"
	WorkFailureTypeUnknown             WorkFailureType = "unknown"
	WorkFailureTypeMisconfigured       WorkFailureType = "misconfigured"
)

// ProviderErrorType remains as a compatibility alias while runtime paths
// transition to generalized work-failure naming.
type ProviderErrorType = WorkFailureType

const (
	ProviderErrorTypeAuthFailure         ProviderErrorType = WorkFailureTypeAuthFailure
	ProviderErrorTypePermanentBadRequest ProviderErrorType = WorkFailureTypePermanentBadRequest
	ProviderErrorTypeThrottled           ProviderErrorType = WorkFailureTypeThrottled
	ProviderErrorTypeInternalServerError ProviderErrorType = WorkFailureTypeInternalServerError
	ProviderErrorTypeTimeout             ProviderErrorType = WorkFailureTypeTimeout
	ProviderErrorTypeUnknown             ProviderErrorType = WorkFailureTypeUnknown
	ProviderErrorTypeMisconfigured       ProviderErrorType = WorkFailureTypeMisconfigured
)

// WorkFailureDecision is the normalized behavior contract consumed by
// downstream retry, termination, and throttle-pause logic.
type WorkFailureDecision struct {
	Retryable             bool
	Terminal              bool
	TriggersThrottlePause bool
}

// ProviderFailureDecision remains as a compatibility alias while runtime paths
// transition to generalized work-failure naming.
type ProviderFailureDecision = WorkFailureDecision

// WorkFailureMetadata carries the normalized failure contract
// across runtime boundaries after the original error has been rendered.
type WorkFailureMetadata struct {
	Family WorkFailureFamily `json:"family"`
	Type   WorkFailureType   `json:"type"`
}

// ProviderFailureMetadata remains as a compatibility alias while runtime paths
// transition to generalized work-failure naming.
type ProviderFailureMetadata = WorkFailureMetadata

// CanonicalWorkFailureMetadata returns the generalized failure metadata from
// the runtime result, falling back to the legacy provider-named field while
// older callers are still being migrated.
func CanonicalWorkFailureMetadata(failure *WorkFailureMetadata, providerFailure *ProviderFailureMetadata) *WorkFailureMetadata {
	if failure != nil {
		return failure
	}
	return providerFailure
}
