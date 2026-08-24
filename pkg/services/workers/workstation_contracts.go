package workers

import (
	"context"
	"errors"
	"fmt"
	"time"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
)

// ModelInvoker executes one model operation through the configured Worker
// path.
type ModelInvoker interface {
	InvokeModel(context.Context, string, modelinference.Request) (modelinference.Result, error)
}

// WorkstationDispatchTerminalOutcome classifies the one terminal result
// committed for an accepted workstation dispatch.
type WorkstationDispatchTerminalOutcome string

// WorkstationDispatchReconciliationReason identifies a policy-free execution
// observation that caused Workers to synthesize a terminal dispatch result.
// Worker Sessions owns the session-level classification of this fact.
type WorkstationDispatchReconciliationReason string

// WorkstationDispatchCancelReason distinguishes an operator cancellation
// from an internal reconciliation request. The latter is allowed to commit a
// terminal result immediately because the executor may never return.
type WorkstationDispatchCancelReason string

// WorkstationDispatchCancelOutcome describes an idempotent explicit
// cancellation request.
type WorkstationDispatchCancelOutcome string

const (
	WorkstationDispatchTerminalOutcomeCompleted WorkstationDispatchTerminalOutcome = "COMPLETED"
	WorkstationDispatchTerminalOutcomeFailed    WorkstationDispatchTerminalOutcome = "FAILED"
	WorkstationDispatchTerminalOutcomeCanceled  WorkstationDispatchTerminalOutcome = "CANCELED"

	WorkstationDispatchReconciliationReasonProcessGone WorkstationDispatchReconciliationReason = "PROCESS_GONE"
	WorkstationDispatchReconciliationReasonTimeout     WorkstationDispatchReconciliationReason = "EXECUTION_TIMEOUT"

	WorkstationDispatchCancelReasonCanceled   WorkstationDispatchCancelReason = "CANCELED"
	WorkstationDispatchCancelReasonSuperseded WorkstationDispatchCancelReason = "SUPERSEDED"
	WorkstationDispatchCancelReasonTimeout    WorkstationDispatchCancelReason = "EXECUTION_TIMEOUT"

	WorkstationDispatchCancelOutcomeCanceled          WorkstationDispatchCancelOutcome = "CANCELED"
	WorkstationDispatchCancelOutcomeAlreadyCanceled   WorkstationDispatchCancelOutcome = "ALREADY_CANCELED"
	WorkstationDispatchCancelOutcomeReconciled        WorkstationDispatchCancelOutcome = "RECONCILED"
	WorkstationDispatchCancelOutcomeAlreadyReconciled WorkstationDispatchCancelOutcome = "ALREADY_RECONCILED"
	WorkstationDispatchCancelOutcomeAlreadyTerminal   WorkstationDispatchCancelOutcome = "ALREADY_TERMINAL"

	// DefaultWorkstationExecutionTimeout is the hard execution limit used when
	// an authored worker does not provide an explicit positive timeout.
	// Supervision and the workstation executor both consume this value so the
	// deadline is not silently changed at either boundary.
	DefaultWorkstationExecutionTimeout = 2 * time.Hour
)

var (
	// ErrInvalidWorkstationDispatch reports a malformed workstation dispatch.
	ErrInvalidWorkstationDispatch = errors.New("invalid Workers workstation dispatch request")
	// ErrUnknownWorkstationDispatch reports cancellation for an unaccepted ID.
	ErrUnknownWorkstationDispatch = errors.New("Workers workstation dispatch is unknown")
	// ErrWorkstationDispatchCanceled classifies the canonical cancelled terminal
	// result from caller, explicit, or pool-stop cancellation.
	ErrWorkstationDispatchCanceled = errors.New("Workers workstation dispatch is canceled")
	// ErrWorkstationDispatchAlreadyTerminal reports late cancellation after a
	// non-cancelled terminal result was committed.
	ErrWorkstationDispatchAlreadyTerminal = errors.New("Workers workstation dispatch is already terminal")
	// ErrWorkstationDispatchProcessGone reports that the executor's parent
	// process exited before the dispatch produced a terminal result.
	ErrWorkstationDispatchProcessGone = errors.New("Workers workstation process exited before dispatch completion")
	// ErrWorkstationDispatchTimeout reports that a detached execution exceeded
	// its resolved hard deadline before producing a terminal result.
	ErrWorkstationDispatchTimeout = errors.New("Workers workstation dispatch execution deadline exceeded")
)

// WorkstationDispatchRequest carries one detached execution request to the
// executor binding assembled for the named workstation.
type WorkstationDispatchRequest struct {
	WorkstationName string
	Execution       WorkstationExecutionRequest
}

// WorkstationDispatchResult attributes one executor result to its original
// dispatch and workstation route.
type WorkstationDispatchResult struct {
	DispatchID      string
	WorkstationName string
	TerminalOutcome WorkstationDispatchTerminalOutcome
	// ReconciliationReason is set only when Workers owns a terminal result
	// synthesized from an execution-lifecycle observation.
	ReconciliationReason WorkstationDispatchReconciliationReason
	Cancellation         *DispatchCancellation
	Result               WorkResult
	// ProposedOutput is a transient detached Worker proposal. Runtime passes it
	// to Work for validation and canonical identity assignment before applying
	// the result; it must never cross a durable WorkResult boundary directly.
	ProposedOutput *ProposedOutput `json:"-"`
}

// WorkstationDispatchCancelRequest identifies one accepted dispatch.
type WorkstationDispatchCancelRequest struct {
	DispatchID string
	// Reason is empty for an explicit operator cancellation. Workers-owned
	// timeout reconciliation supplies WorkstationDispatchCancelReasonTimeout.
	Reason WorkstationDispatchCancelReason
}

// WorkstationDispatchCancelResult reports whether cancellation was newly
// committed, already committed, or arrived after another terminal result.
type WorkstationDispatchCancelResult struct {
	DispatchID string
	Outcome    WorkstationDispatchCancelOutcome
}

// ProviderInvocationRoute is the reserved route name for Workers that have no
// authored workstation behind them. A JavaScript workflow child names it
// instead of a workstation, so it reaches the same Workers execution route,
// admission and cancellation, and the same Worker Session supervision as a
// Petri Worker while skipping workstation prompt rendering it has no
// definition for.
//
// It is deliberately one route rather than one per child. Workstation routes
// come from the Factory definition and are snapshotted immutably when a session
// opens, but agent.run children are discovered while the workflow runs; giving
// each its own route would mean mutating a snapshot that is immutable by
// contract. Everything that varies per child already travels on the execution
// request, so one route serves all of them.
//
// The double-underscore fencing marks it as reserved rather than authored. It
// deliberately carries no leading or trailing whitespace: Worker Sessions'
// request validation compares the raw nested dispatch name against the trimmed
// route name, so a padded value would be rejected before it ever reached the
// pool.
const ProviderInvocationRoute = "__provider_invocation__"

// ErrMissingRunnerSelection reports that an execution request omitted the
// runner selection needed by Workers.
var ErrMissingRunnerSelection = errors.New("Workers execution missing runner selection")

// ErrUnknownRunnerSelection reports that an execution request named a runner
// identity Workers does not recognize.
var ErrUnknownRunnerSelection = errors.New("Workers execution unknown runner selection")

// ErrUnsupportedRunnerCapability reports that a selected runner cannot satisfy
// one explicitly required optional capability.
var ErrUnsupportedRunnerCapability = errors.New("Workers runner capability unsupported")

// UnsupportedRunnerCapabilityError carries detached, customer-safe selection
// context for one unsupported required capability.
type UnsupportedRunnerCapabilityError struct {
	RunnerID   string
	Capability RunnerOptionalCapability
}

func (e *UnsupportedRunnerCapabilityError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf(
		"runner %q does not support capability %q",
		e.RunnerID,
		e.Capability,
	)
}

func (e *UnsupportedRunnerCapabilityError) Unwrap() error {
	if e == nil {
		return nil
	}
	return ErrUnsupportedRunnerCapability
}

// ErrInvalidRunnerRegistration reports that a private Workers runner
// registration contains malformed identity, metadata, capabilities, or a nil
// implementation.
var ErrInvalidRunnerRegistration = errors.New("invalid Workers runner registration")

// ErrConflictingRunnerRegistration reports that a registration's explicit
// identity disagrees with its metadata identity.
var ErrConflictingRunnerRegistration = errors.New("conflicting Workers runner registration")

// ErrDuplicateRunnerRegistration reports that registry construction received
// more than one registration for the same canonical runner identity.
var ErrDuplicateRunnerRegistration = errors.New("duplicate Workers runner registration")

// Service is the customer-facing request-scoped Worker execution boundary.
// Provider factories, command runners, and workstation builders remain
// implementation details or explicit Worker subservices.
type Service interface {
	// Execute runs one complete, detached Worker attempt. The operation is
	// request-scoped; callers retain dispatch lifecycle, scheduling, and retry
	// state outside Workers.
	Execute(context.Context, ExecuteRequest) (ExecuteResult, error)

	ModelInvoker
}
