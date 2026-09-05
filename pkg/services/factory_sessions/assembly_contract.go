package factorysessions

import (
	"context"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// RuntimeSidecars owns runtime-scoped background services without exposing
// their concrete host implementation to Factory Sessions consumers.
type RuntimeSidecars = runtimeports.RuntimeSidecarService

type DefinitionHost = factorydefinitions.SessionHost

// --- merged from invocation_contract.go ---

// Invocation root slice freezes request, resolved-input, result, timeout, and
// cancellation/error vocabulary on the singular Service aggregate. Peers
// consume these plain root contracts without importing private invocation
// subservice types or Work implementation packages beyond approved peer root
// contracts already present in root signatures:
//
//   - Request: InvocationRequest
//   - Resolved input: ResolvedInvocationInput
//   - Result: InvocationResult (+ InvocationTerminalStatus / InvocationErrorCode)
//   - Timeout: InvocationTimeout / DefaultInvocationTimeout (and TimeoutMillis on
//     InvocationRequest)
//   - Invalid input: *InvocationValidationError
//   - Timeout / caller-cancellation outcomes: InvocationResult with distinct
//     Status and ErrorCode values (TIMED_OUT / CANCELED)
//
// InvocationService is the narrow, owner-published Factory Sessions capability
// for one-shot invocation. It retains the singular Service as the only session
// authority: Service satisfies this interface structurally, and the interface
// neither constructs nor locates a session service.
//
// It intentionally exposes no live-session control, durable execution,
// opening, listing, or inspection method. Consumers that only invoke a captured
// Factory Session can therefore depend on this single owner-published operation.
type InvocationService interface {
	InvokeFactorySession(context.Context, string, InvocationRequest) (InvocationResult, error)
}

// Service satisfies InvocationService structurally. Keep this assertion at the
// owner root so a change to the published invocation contract cannot drift from
// the singular service implementation.
var _ InvocationService = (Service)(nil)

// InvocationResult is the plain root session-scoped outcome of one Factory
// Session invocation after input resolution and result selection.
type InvocationResult struct {
	RequestID     string
	TraceID       string
	Status        InvocationTerminalStatus
	PrimaryResult []work.WorkContentPart
	ErrorCode     string
	Message       string
	SessionID     string
	WorkID        string
	WorkName      string
	WorkState     string
}

// InvocationTerminalStatus is the Factory Session-owned terminal outcome for
// one invocation on the published root slice.
type InvocationTerminalStatus string

const (
	InvocationTerminalStatusCanceled  InvocationTerminalStatus = "CANCELED"
	InvocationTerminalStatusCompleted InvocationTerminalStatus = "COMPLETED"
	InvocationTerminalStatusFailed    InvocationTerminalStatus = "FAILED"
	InvocationTerminalStatusTimedOut  InvocationTerminalStatus = "TIMED_OUT"
)

// InvocationErrorCode is the stable Factory Session-owned failure code emitted
// with a non-completed invocation result on the published root slice.
type InvocationErrorCode string

const (
	InvocationErrorCodeCanceled       InvocationErrorCode = "INVOCATION_CANCELED"
	InvocationErrorCodeRuntimeFailure InvocationErrorCode = "INVOCATION_RUNTIME_FAILURE"
	InvocationErrorCodeTimedOut       InvocationErrorCode = "INVOCATION_TIMED_OUT"
)

// InvocationTimeout is the published root name for the Factory Sessions-owned
// lifecycle budget applied to one invocation.
type InvocationTimeout = ModelInvocationTimeout

// DefaultInvocationTimeout is the published default invocation lifecycle budget.
const DefaultInvocationTimeout = DefaultModelInvocationTimeout

// InvocationValidationError is the typed invalid-input failure published on the
// invocation root slice. Peers match it with errors.As without importing private
// invocation subservice types.
type InvocationValidationError struct {
	Field   string
	Message string
}

func (err *InvocationValidationError) Error() string {
	if err == nil {
		return "invocation validation error"
	}
	if err.Field == "" {
		return err.Message
	}
	if err.Message == "" {
		return fmt.Sprintf("invocation validation failed for %s", err.Field)
	}
	return fmt.Sprintf("%s: %s", err.Field, err.Message)
}

// DetachedOperations is a one-way compatibility view over an already-composed
// Factory Sessions Service. It retains only the singular owner: it does not
// construct a service, registry, lifecycle, or mode-specific sidecar.
type DetachedOperations struct {
	owner Service
}

// Bind attaches the detached view to the already-composed Sessions root.
// Binding is inert and performs no capability discovery or child construction.
func (operations *DetachedOperations) Bind(owner Service) (DetachedService, error) {
	if owner == nil {
		return nil, ErrDetachedServiceUnavailable
	}
	if operations == nil {
		operations = &DetachedOperations{}
	}
	operations.owner = owner
	return operations, nil
}

func (operations *DetachedOperations) service() (Service, error) {
	if operations == nil || operations.owner == nil {
		return nil, ErrDetachedServiceUnavailable
	}
	return operations.owner, nil
}

// Start forwards directly to the canonical Service operation. Validation and
// mode selection remain owned by the singular Service.
func (operations *DetachedOperations) Start(
	ctx context.Context,
	request SessionStartRequest,
) (SessionStartResult, error) {
	owner, err := operations.service()
	if err != nil {
		return SessionStartResult{}, err
	}
	return owner.Start(ctx, request)
}

// Invoke forwards directly to the canonical Service operation.
func (operations *DetachedOperations) Invoke(
	ctx context.Context,
	request SessionInvokeRequest,
) (InvocationResult, error) {
	owner, err := operations.service()
	if err != nil {
		return InvocationResult{}, err
	}
	return owner.Invoke(ctx, request)
}

// Activate remains a temporary named-activation compatibility exception. The
// target canonical vocabulary intentionally excludes named activation; FSCP-08
// owns its eventual migration.
func (operations *DetachedOperations) Activate(
	ctx context.Context,
	request SessionActivateRequest,
) (SessionActivateResult, error) {
	if err := validateSessionID(request.SessionID); err != nil {
		return SessionActivateResult{}, err
	}
	owner, err := operations.service()
	if err != nil {
		return SessionActivateResult{}, err
	}
	name := strings.TrimSpace(request.FactoryName)
	if name == "" {
		name = strings.TrimSpace(request.Definition.FactoryID)
	}
	if name == "" {
		return SessionActivateResult{}, detachedRequestError("factoryName", "factory name is required")
	}
	if err := owner.ActivateNamedFactory(ctx, name); err != nil {
		return SessionActivateResult{}, err
	}
	return SessionActivateResult{
		SessionID:   request.SessionID,
		FactoryName: name,
		Activated:   true,
	}, nil
}

// Get forwards directly to the canonical Service operation.
func (operations *DetachedOperations) Get(
	ctx context.Context,
	request SessionGetRequest,
) (SessionGetResult, error) {
	owner, err := operations.service()
	if err != nil {
		return SessionGetResult{}, err
	}
	return owner.Get(ctx, request)
}

// List forwards directly to the canonical Service operation.
func (operations *DetachedOperations) List(
	ctx context.Context,
	request SessionListRequest,
) (SessionListResult, error) {
	owner, err := operations.service()
	if err != nil {
		return SessionListResult{}, err
	}
	return owner.List(ctx, request)
}

// Control forwards directly to the canonical Service operation.
func (operations *DetachedOperations) Control(
	ctx context.Context,
	request SessionControlRequest,
) (SessionControlResult, error) {
	owner, err := operations.service()
	if err != nil {
		return SessionControlResult{}, err
	}
	return owner.Control(ctx, request)
}

// ReadResult forwards directly to the canonical Service operation.
func (operations *DetachedOperations) ReadResult(
	ctx context.Context,
	request SessionResultReadRequest,
) (SessionResultReadResult, error) {
	owner, err := operations.service()
	if err != nil {
		return SessionResultReadResult{}, err
	}
	return owner.ReadResult(ctx, request)
}

// PrepareSync is intentionally pure. It selects the synchronous wait values
// and clones caller-owned input without constructing or invoking a service.
func (operations *DetachedOperations) PrepareSync(
	_ context.Context,
	request SessionSyncPreparationRequest,
) (SessionPreparedSyncStart, error) {
	start := cloneDetachedStartRequest(request.Start)
	if start.Mode != SessionOperationModeDurable {
		return SessionPreparedSyncStart{}, detachedRequestError("start.mode", "synchronous preparation requires durable mode")
	}
	if start.Correlation.RequestID == "" {
		return SessionPreparedSyncStart{}, detachedRequestError("start.correlation.requestId", "request id is required")
	}
	if start.Wait.TimeoutMillis < 0 || request.Wait.TimeoutMillis < 0 {
		return SessionPreparedSyncStart{}, detachedRequestError("wait.timeoutMillis", "timeout must not be negative")
	}
	if request.Wait.TimeoutMillis > 0 || request.Wait.CancelOnTimeout {
		start.Wait = request.Wait
	}
	start.Synchronous = true
	return SessionPreparedSyncStart{Request: start, Wait: start.Wait}, nil
}

// Subscribe forwards directly to the canonical response-subscription operation.
func (operations *DetachedOperations) Subscribe(
	ctx context.Context,
	request SessionResponseSubscriptionRequest,
) (SessionResponseSubscriptionResult, error) {
	owner, err := operations.service()
	if err != nil {
		return SessionResponseSubscriptionResult{}, err
	}
	return owner.SubscribeResponses(ctx, request)
}

func validateSessionID(sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return detachedRequestError("sessionId", "session id is required")
	}
	return nil
}

func detachedRequestError(field, message string) error {
	return &DetachedRequestError{Field: field, Message: message}
}

// These cloning helpers are shared by the value-owned Session* request
// preparation code in types.go. Canonical operation forwarding itself does not
// translate or clone requests; the Service owner owns that boundary.
func clonePreparedInput(input *work.PreparedInvocationInput) *work.PreparedInvocationInput {
	if input == nil {
		return nil
	}
	return input.Clone()
}

func cloneAnyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = cloneAnyValue(value)
	}
	return cloned
}

func cloneAnyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneAnyValue(item)
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
