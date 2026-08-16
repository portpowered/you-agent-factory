package factorysessions

import (
	"context"
	"fmt"
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
