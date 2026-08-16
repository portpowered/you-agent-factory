package factory

import (
	"context"
	"errors"
	"fmt"
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
	Inputs           RuntimeActivationInputs
}

// RuntimeActivationResult reports the identity and state of a successful
// activation.
type RuntimeActivationResult struct {
	RuntimeID string
	State     RuntimeLifecycleState
	// Binding is the detached, per-activation capability that callers retain
	// for Runtime operations. Its identity and delegate are intentionally
	// unexported; callers can only use the published Service contract.
	Binding RuntimeBinding
	// Runtime contains the initialized Runtime service view needed by the root
	// to publish the binding. Cleanup is deliberately absent; the Runtime root
	// retains that ownership and performs it through Deactivate.
	Runtime RuntimeActivationView
}

// RuntimeBinding is an opaque capability for one activated Runtime. The
// process-scoped Runtime root owns the identity and lifecycle associated with
// bindings it publishes; callers cannot inspect hosted implementation state.
//
// The constructor is intentionally limited to attaching a published Service
// view. Runtime root implementations use it while publishing an activation;
// callers should retain the returned value and use Service for operations.
type RuntimeBinding struct {
	identity string
	service  Service
	owner    *runtimeBindingOwner
}

type runtimeBindingOwner struct {
	deactivate func(context.Context) (RuntimeDeactivationResult, error)
}

// NewRuntimeBinding creates a detached binding for a Runtime service view.
// It performs no lifecycle work and returns a zero binding for incomplete
// inputs. The identity is retained privately so bindings from distinct
// activations cannot be confused by the owning root.
func NewRuntimeBinding(
	identity string,
	service Service,
	deactivate ...func(context.Context) (RuntimeDeactivationResult, error),
) RuntimeBinding {
	identity = strings.TrimSpace(identity)
	if identity == "" || service == nil {
		return RuntimeBinding{}
	}
	var owner *runtimeBindingOwner
	if len(deactivate) > 0 && deactivate[0] != nil {
		owner = &runtimeBindingOwner{deactivate: deactivate[0]}
	}
	return RuntimeBinding{identity: identity, service: service, owner: owner}
}

// Service returns the detached Factory Runtime capability for this binding.
// A zero binding returns nil. A binding whose Runtime was deactivated retains
// its capability value, but the owning root rejects operations on it.
func (binding RuntimeBinding) Service() Service {
	return binding.service
}

// IsZero reports whether the binding contains no Runtime capability.
func (binding RuntimeBinding) IsZero() bool {
	return binding.identity == "" || binding.service == nil
}

// Deactivate asks the Runtime root that issued this binding to release the
// activation it owns. The capability keeps cleanup adjacent to the identity
// used for Runtime operations, so callers do not need a second process-wide
// lookup or a hosted Runtime handle.
func (binding RuntimeBinding) Deactivate(ctx context.Context) (RuntimeDeactivationResult, error) {
	if binding.IsZero() || binding.owner == nil || binding.owner.deactivate == nil {
		return RuntimeDeactivationResult{}, &RuntimeActivationError{
			Kind:    RuntimeActivationErrorNotActive,
			Message: "deactivate Factory Runtime: Runtime binding cannot deactivate its owner",
		}
	}
	return binding.owner.deactivate(ctx)
}

// Equal reports whether two bindings name the same Runtime activation.
func (binding RuntimeBinding) Equal(other RuntimeBinding) bool {
	return !binding.IsZero() && !other.IsZero() && binding.identity == other.identity
}

// RuntimeActivationView is the published handoff for one successfully
// initialized Runtime. The view contains already-constructed runtime
// capabilities; callers cannot use it to replace the root's active delegate
// or take ownership of cleanup.
type RuntimeActivationView struct {
	RuntimeID        string
	FactorySessionID string
	// Binding is the successor capability for callers migrating away from the
	// hosted handoff below.
	Binding RuntimeBinding
	Service Service
}

// RuntimeDeactivationRequest selects the Runtime whose owned resources should
// be closed. Deactivation is deliberately separate from a control terminate so
// cleanup ownership is explicit at the root boundary.
type RuntimeDeactivationRequest struct {
	// Binding is the preferred opaque selector. RuntimeID remains for
	// migration callers and failed-start cleanup that has no published binding.
	Binding   RuntimeBinding
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
	message := e.Message
	if message == "" && e.Kind != "" {
		message = string(e.Kind)
	}
	if message == "" {
		message = ErrRuntimeActivationFailed.Error()
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", message, e.Cause)
	}
	return message
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
	// Close is retained by the Runtime root as its private cleanup edge. It is
	// deliberately not returned to callers in RuntimeActivationResult.
	Close func(context.Context) error
}

// RuntimeActivationOperation is injected when the process root is composed.
// It receives only explicit Runtime values and returns an initialized Runtime
// plus its owned cleanup operation.
type RuntimeActivationOperation func(
	context.Context,
	RuntimeActivationRequest,
) (*RuntimeActivation, error)

// Normalize validates and detaches an activation request before it is handed
// to the root's injected start operation.
func (request RuntimeActivationRequest) Normalize() (RuntimeActivationRequest, error) {
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

	cloned, err := snapshot.Clone()
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
	request.Inputs = request.Inputs.Clone()
	request.Snapshot = cloned
	return request, nil
}

func runtimeActivationError(kind RuntimeActivationErrorKind, message string, cause error) error {
	return &RuntimeActivationError{Kind: kind, Message: message, Cause: cause}
}
