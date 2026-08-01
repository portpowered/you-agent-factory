package factorysessions

import (
	"fmt"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// RuntimeSidecars owns runtime-scoped background services without exposing
// their concrete host implementation to Factory Sessions consumers.
type RuntimeSidecars = factoryruntime.Sidecars

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
// Invocation remains part of the singular root Service aggregate. This file
// does not publish a separate peer-facing invoker interface.

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
