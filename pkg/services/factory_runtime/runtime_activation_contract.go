package factory

import (
	"context"
	"errors"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// RuntimeLifecycleState is the process-root lifecycle state returned by the
// explicit Runtime activation boundary.
type RuntimeLifecycleState string

const (
	RuntimeLifecycleStateActive  RuntimeLifecycleState = "ACTIVE"
	RuntimeLifecycleStateStopped RuntimeLifecycleState = "STOPPED"
)

// RuntimeActivationRequest is the complete value request required to publish
// one Runtime. The Definitions snapshot is detached before it reaches the
// activation operation; no service, loader, executor, or mutable source is
// carried across this boundary.
type RuntimeActivationRequest struct {
	RuntimeID        string
	FactorySessionID string
	Snapshot         factorydefinitions.RuntimeSnapshot
	Runtime          RuntimeOpeningRequest
}

// RuntimeActivationResult reports the identity and state of a successful
// activation.
type RuntimeActivationResult struct {
	RuntimeID string
	State     RuntimeLifecycleState
	// Runtime contains the initialized, root-owned capabilities needed by the
	// caller to finish composing its public session view. Cleanup is deliberately
	// absent; the Runtime root retains that ownership and performs it through
	// Deactivate.
	Runtime RuntimeActivationView
}

// RuntimeActivationView is the published handoff for one successfully
// initialized Runtime. The view contains already-constructed runtime
// capabilities; callers cannot use it to replace the root's active delegate
// or take ownership of cleanup.
type RuntimeActivationView struct {
	RuntimeID        string
	FactorySessionID string
	Service          Service
	HostedInstance   HostedInstance
	Replacement      ReplacementBuilder
	BuildSpec        SessionBuildSpec
	Lifecycle        Lifecycle
	Sidecars         Sidecars
}

// RuntimeDeactivationRequest selects the Runtime whose owned resources should
// be closed. Deactivation is deliberately separate from a control terminate so
// cleanup ownership is explicit at the root boundary.
type RuntimeDeactivationRequest struct {
	RuntimeID string
}

// RuntimeDeactivationResult reports the state after owned cleanup completes.
type RuntimeDeactivationResult struct {
	RuntimeID string
	State     RuntimeLifecycleState
}

// RuntimeActivationErrorKind distinguishes deterministic activation and
// deactivation failures.
type RuntimeActivationErrorKind string

const (
	RuntimeActivationErrorMissingParameters  RuntimeActivationErrorKind = "MISSING_PARAMETERS"
	RuntimeActivationErrorInvalidSnapshot    RuntimeActivationErrorKind = "INVALID_SNAPSHOT"
	RuntimeActivationErrorUnavailable        RuntimeActivationErrorKind = "UNAVAILABLE"
	RuntimeActivationErrorAlreadyActive      RuntimeActivationErrorKind = "ALREADY_ACTIVE"
	RuntimeActivationErrorConflict           RuntimeActivationErrorKind = "CONFLICT"
	RuntimeActivationErrorFailed             RuntimeActivationErrorKind = "FAILED"
	RuntimeActivationErrorNotActive          RuntimeActivationErrorKind = "NOT_ACTIVE"
	RuntimeActivationErrorDeactivationFailed RuntimeActivationErrorKind = "DEACTIVATION_FAILED"
)

var (
	// ErrRuntimeActivationFailed is the stable umbrella for activation errors.
	ErrRuntimeActivationFailed = errors.New("Factory Runtime activation failed")
	// ErrRuntimeActivationMissingParameters identifies an incomplete request.
	ErrRuntimeActivationMissingParameters = errors.New("Factory Runtime activation parameters are missing")
	// ErrRuntimeActivationInvalidSnapshot identifies a snapshot that cannot be
	// used to initialize a Runtime.
	ErrRuntimeActivationInvalidSnapshot = errors.New("Factory Runtime activation snapshot is invalid")
	// ErrRuntimeActivationUnavailable identifies a root without an injected
	// activation operation.
	ErrRuntimeActivationUnavailable = errors.New("Factory Runtime activation operation is unavailable")
	// ErrRuntimeAlreadyActive identifies an exact duplicate activation.
	ErrRuntimeAlreadyActive = errors.New("Factory Runtime is already active")
	// ErrRuntimeActivationConflict identifies a different request for an active
	// Runtime identity or a conflicting active Runtime.
	ErrRuntimeActivationConflict = errors.New("Factory Runtime activation conflicts with active state")
	// ErrRuntimeNotActive identifies deactivation before activation.
	ErrRuntimeNotActive = errors.New("Factory Runtime is not active")
	// ErrRuntimeDeactivationFailed is the stable umbrella for cleanup failures.
	ErrRuntimeDeactivationFailed = errors.New("Factory Runtime deactivation failed")
)

// RuntimeActivationError is a typed lifecycle failure that callers can branch
// on without parsing error text.
type RuntimeActivationError struct {
	Kind      RuntimeActivationErrorKind
	RuntimeID string
	Message   string
	Cause     error
}

func (e *RuntimeActivationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Kind != "" {
		return string(e.Kind)
	}
	return ErrRuntimeActivationFailed.Error()
}

func (e *RuntimeActivationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *RuntimeActivationError) Is(target error) bool {
	if e == nil {
		return false
	}
	if target == ErrRuntimeActivationFailed && e.Kind != RuntimeActivationErrorDeactivationFailed {
		return true
	}
	if target == ErrRuntimeDeactivationFailed && e.Kind == RuntimeActivationErrorDeactivationFailed {
		return true
	}
	switch e.Kind {
	case RuntimeActivationErrorMissingParameters:
		return target == ErrRuntimeActivationMissingParameters
	case RuntimeActivationErrorInvalidSnapshot:
		return target == ErrRuntimeActivationInvalidSnapshot
	case RuntimeActivationErrorUnavailable:
		return target == ErrRuntimeActivationUnavailable
	case RuntimeActivationErrorAlreadyActive:
		return target == ErrRuntimeAlreadyActive
	case RuntimeActivationErrorConflict:
		return target == ErrRuntimeActivationConflict
	case RuntimeActivationErrorNotActive:
		return target == ErrRuntimeNotActive
	default:
		return false
	}
}

// RuntimeActivation is the result of the root-injected start operation. The
// operation owns construction and may return a cleanup function that the root
// invokes during deactivation or failed publication.
type RuntimeActivation struct {
	Service Service
	// The remaining fields are returned as a RuntimeActivationView after the
	// root publishes the delegate. They are optional for narrow callers that
	// only need the Service contract.
	HostedInstance HostedInstance
	Replacement    ReplacementBuilder
	BuildSpec      SessionBuildSpec
	Lifecycle      Lifecycle
	Sidecars       Sidecars
	Close          func(context.Context) error
}

// RuntimeActivationOperation is injected when the process root is composed.
// It receives only explicit Runtime values and returns an initialized Runtime
// plus its owned cleanup operation.
type RuntimeActivationOperation func(
	context.Context,
	RuntimeActivationRequest,
) (*RuntimeActivation, error)

// Root is the process-scoped Factory Runtime contract. Service operations are
// available after Activate publishes a fully initialized Runtime delegate.
type Root interface {
	Service
	Activate(context.Context, RuntimeActivationRequest) (RuntimeActivationResult, error)
	Deactivate(context.Context, RuntimeDeactivationRequest) (RuntimeDeactivationResult, error)
}

// NormalizeRuntimeActivationRequest validates and detaches an activation
// request before it is handed to the root's injected start operation.
func NormalizeRuntimeActivationRequest(request RuntimeActivationRequest) (RuntimeActivationRequest, error) {
	runtimeID := strings.TrimSpace(request.RuntimeID)
	sessionID := strings.TrimSpace(request.FactorySessionID)
	switch {
	case runtimeID == "":
		return RuntimeActivationRequest{}, runtimeActivationError(
			RuntimeActivationErrorMissingParameters,
			"activate Factory Runtime: Runtime ID is required",
			nil,
		)
	case sessionID == "":
		return RuntimeActivationRequest{}, runtimeActivationError(
			RuntimeActivationErrorMissingParameters,
			"activate Factory Runtime: Factory Session ID is required",
			nil,
		)
	}

	snapshot := request.Snapshot
	snapshot.FactoryDir = strings.TrimSpace(snapshot.FactoryDir)
	snapshot.RuntimeBaseDir = strings.TrimSpace(snapshot.RuntimeBaseDir)
	if snapshot.FactoryDir == "" || snapshot.RuntimeBaseDir == "" ||
		snapshot.EffectiveFactory.Name == "" || snapshot.DefinitionVersion == nil {
		return RuntimeActivationRequest{}, runtimeActivationError(
			RuntimeActivationErrorInvalidSnapshot,
			"activate Factory Runtime: a resolved Factory snapshot with identity, base directory, name, and version is required",
			nil,
		)
	}
	if snapshot.Invocation.FactorySessionID != "" &&
		strings.TrimSpace(snapshot.Invocation.FactorySessionID) != sessionID {
		return RuntimeActivationRequest{}, runtimeActivationError(
			RuntimeActivationErrorInvalidSnapshot,
			"activate Factory Runtime: snapshot Factory Session ID does not match the activation request",
			nil,
		)
	}
	if runtimeInstanceID := strings.TrimSpace(request.Runtime.RuntimeInstanceID); runtimeInstanceID != "" && runtimeInstanceID != runtimeID {
		return RuntimeActivationRequest{}, runtimeActivationError(
			RuntimeActivationErrorConflict,
			"activate Factory Runtime: Runtime instance ID does not match Runtime ID",
			nil,
		)
	}

	cloned, err := factorydefinitions.CloneRuntimeSnapshot(snapshot)
	if err != nil {
		return RuntimeActivationRequest{}, runtimeActivationError(
			RuntimeActivationErrorInvalidSnapshot,
			"activate Factory Runtime: snapshot could not be detached",
			err,
		)
	}
	cloned.Invocation.FactorySessionID = sessionID
	request.RuntimeID = runtimeID
	request.FactorySessionID = sessionID
	request.Runtime.RuntimeInstanceID = runtimeID
	request.Snapshot = cloned
	return request, nil
}

func runtimeActivationError(kind RuntimeActivationErrorKind, message string, cause error) error {
	return &RuntimeActivationError{Kind: kind, Message: message, Cause: cause}
}
