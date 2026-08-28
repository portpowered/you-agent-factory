package models

import "fmt"

// InferenceFailureClass is the stable customer-facing inference failure
// category. It classifies why one model operation could not produce a result,
// and outward adapters branch on it to choose a customer-facing status.
type InferenceFailureClass string

const (
	InferenceFailureClassMissingModel         InferenceFailureClass = "missing_model"
	InferenceFailureClassLoadingModel         InferenceFailureClass = "loading_model"
	InferenceFailureClassUnsupportedOperation InferenceFailureClass = "unsupported_operation"
	InferenceFailureClassTimeout              InferenceFailureClass = "timeout"
	InferenceFailureClassRuntimeFailure       InferenceFailureClass = "runtime_failure"
)

// InferenceFailure is the detached, customer-safe outcome of inference failure
// classification. It is the classified counterpart of TargetError: TargetError
// retains identity around an unclassified cause, and InferenceFailure carries
// the same identity plus the settled category and public message.
//
// The value lives with the Models inference vocabulary (Request, Result,
// ErrMissing, ErrLoading, ErrUnsupported, TargetError) rather than with the
// service that performs the classification, so that every producer and every
// outward adapter can name one type without depending on each other.
type InferenceFailure struct {
	Class      InferenceFailureClass
	Message    string
	ModelName  string
	WorkerName string
	Operation  string
	Cause      error
}

func (e *InferenceFailure) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *InferenceFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// InvocationFailureClass is the provider-neutral identity of a generic model
// invocation contract failure. The values intentionally describe the public
// failure boundary rather than a resolver, cache, process, or backend type.
type InvocationFailureClass string

const (
	InvocationFailureClassInvalidModelReference InvocationFailureClass = "INVALID_MODEL_REFERENCE"
	InvocationFailureClassRevisionResolution    InvocationFailureClass = "REVISION_RESOLUTION"
	InvocationFailureClassInvalidOperation      InvocationFailureClass = "INVALID_OPERATION"
	InvocationFailureClassInvalidSlot           InvocationFailureClass = "INVALID_SLOT"
	InvocationFailureClassSlotArity             InvocationFailureClass = "SLOT_ARITY"
	InvocationFailureClassInvalidParameter      InvocationFailureClass = "INVALID_PARAMETER"
	InvocationFailureClassMediaCapability       InvocationFailureClass = "MEDIA_CAPABILITY"
	InvocationFailureClassConfiguration         InvocationFailureClass = "CONFIGURATION"
	InvocationFailureClassOfflineCache          InvocationFailureClass = "OFFLINE_CACHE"
	InvocationFailureClassArtifact              InvocationFailureClass = "ARTIFACT"
	InvocationFailureClassAssetPreparation      InvocationFailureClass = "ASSET_PREPARATION"
	InvocationFailureClassBackendReadiness      InvocationFailureClass = "BACKEND_READINESS"
	InvocationFailureClassBackendProtocol       InvocationFailureClass = "BACKEND_PROTOCOL"
	InvocationFailureClassCancellation          InvocationFailureClass = "CANCELLATION"
	InvocationFailureClassTimeout               InvocationFailureClass = "TIMEOUT"
	InvocationFailureClassMalformedResponse     InvocationFailureClass = "MALFORMED_RESPONSE"
)

// Short aliases keep call sites readable while the Class-prefixed constants
// make the taxonomy discoverable beside InvocationFailureClass.
const (
	InvocationFailureInvalidModelReference = InvocationFailureClassInvalidModelReference
	InvocationFailureRevisionResolution    = InvocationFailureClassRevisionResolution
	InvocationFailureInvalidOperation      = InvocationFailureClassInvalidOperation
	InvocationFailureInvalidSlot           = InvocationFailureClassInvalidSlot
	InvocationFailureSlotArity             = InvocationFailureClassSlotArity
	InvocationFailureInvalidParameter      = InvocationFailureClassInvalidParameter
	InvocationFailureMediaCapability       = InvocationFailureClassMediaCapability
	InvocationFailureConfiguration         = InvocationFailureClassConfiguration
	InvocationFailureOfflineCache          = InvocationFailureClassOfflineCache
	InvocationFailureArtifact              = InvocationFailureClassArtifact
	InvocationFailureAssetPreparation      = InvocationFailureClassAssetPreparation
	InvocationFailureBackendReadiness      = InvocationFailureClassBackendReadiness
	InvocationFailureBackendProtocol       = InvocationFailureClassBackendProtocol
	InvocationFailureCancellation          = InvocationFailureClassCancellation
	InvocationFailureTimeout               = InvocationFailureClassTimeout
	InvocationFailureMalformedResponse     = InvocationFailureClassMalformedResponse
)

// InvocationFailure is a detached, customer-safe typed failure for the
// generic invocation contract. Only stable identity and caller-relevant
// coordinates are retained; implementation details such as cache paths,
// backend addresses, credentials, and concrete protocol errors are excluded
// from Error().
type InvocationFailure struct {
	Class      InvocationFailureClass
	Message    string
	Model      ModelReference
	Operation  string
	Slot       string
	Parameter  string
	Field      string
	ValidNames []string
	Cause      error
}

func newInvocationFailure(class InvocationFailureClass, message string) *InvocationFailure {
	return &InvocationFailure{Class: class, Message: message}
}

func (e *InvocationFailure) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Class == "" {
		return "model invocation failed"
	}
	return fmt.Sprintf("model invocation failed: %s", e.Class)
}

func (e *InvocationFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// GenericInvocationFailure is the descriptive alias used by future generic
// transports without creating a second failure identity.
type GenericInvocationFailure = InvocationFailure
