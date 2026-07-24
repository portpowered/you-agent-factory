package factorysessions

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// RuntimeSidecars owns runtime-scoped background services without exposing
// their concrete host implementation to Factory Sessions consumers.
type RuntimeSidecars = factoryruntime.Sidecars

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
// Invocation remains part of the singular root Service aggregate. This file
// does not publish a separate peer-facing invoker interface.

// InvocationResult is the plain root session-scoped outcome of one Factory
// Session invocation after input resolution and result selection. It stays
// interchangeable with factorydefinitions.FactoryInvocationResult so the
// private invocation subservice can publish CTR-SES outcomes without a second
// peer-facing result shape.
type InvocationResult = factorydefinitions.FactoryInvocationResult

// InvocationTerminalStatus is the Factory Session-owned terminal outcome for
// one invocation on the published root slice.
type InvocationTerminalStatus = factorydefinitions.InvocationTerminalStatus

const (
	InvocationTerminalStatusCanceled  = factorydefinitions.InvocationTerminalStatusCanceled
	InvocationTerminalStatusCompleted = factorydefinitions.InvocationTerminalStatusCompleted
	InvocationTerminalStatusFailed    = factorydefinitions.InvocationTerminalStatusFailed
	InvocationTerminalStatusTimedOut  = factorydefinitions.InvocationTerminalStatusTimedOut
)

// InvocationErrorCode is the stable Factory Session-owned failure code emitted
// with a non-completed invocation result on the published root slice.
type InvocationErrorCode string

const (
	InvocationErrorCodeCanceled       InvocationErrorCode = InvocationErrorCode(factorydefinitions.InvocationErrorCodeCanceled)
	InvocationErrorCodeRuntimeFailure InvocationErrorCode = InvocationErrorCode(factorydefinitions.InvocationErrorCodeRuntimeFailure)
	InvocationErrorCodeTimedOut       InvocationErrorCode = InvocationErrorCode(factorydefinitions.InvocationErrorCodeTimedOut)
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
