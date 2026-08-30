// Package dispatch_planning defines the parent-private Factory Runtime
// capability that translates runnable decisions into canonical Workers
// requests and owns their outbox lifecycle. Execution remains behind the
// injected Workers boundary.
package dispatch_planning

import (
	"context"
	"errors"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// ErrInvalidRunnableDecision reports an incomplete or internally inconsistent
// decision. Planning rejects the complete batch when any decision is invalid.
var ErrInvalidRunnableDecision = errors.New("invalid Factory Runtime runnable decision")

// ErrDuplicateDispatchIntent reports reuse of a dispatch or correlation
// identity for content that differs from the accepted outbox intent.
var ErrDuplicateDispatchIntent = errors.New("factory runtime duplicate dispatch intent")

// ErrUnknownDispatchCorrelation reports a terminal result whose correlation
// identity is not present in the Runtime outbox.
var ErrUnknownDispatchCorrelation = errors.New("factory runtime unknown dispatch correlation")

// ErrInvalidDispatchResultBoundary reports a terminal result that conflicts
// with its accepted intent or falls outside the terminal-result vocabulary.
var ErrInvalidDispatchResultBoundary = errors.New("factory runtime invalid dispatch result boundary")

// ErrDispatchRuntimeStopped reports an attempt to accept or publish new work
// after cancellation or termination made the Runtime outbox permanently inert.
var ErrDispatchRuntimeStopped = errors.New("factory runtime dispatch outbox stopped")

// ExecutionFacts carries the detached worker-selection and invocation facts
// selected for one runnable decision. Runtime supplies values, not a Workers
// service, executor, provider, model, script, or hosted-runner implementation.
type ExecutionFacts struct {
	WorkerType               string
	WorkstationType          string
	RunnerID                 string
	RunnerSelectionSource    workers.RunnerSelectionSource
	ProjectID                string
	FactorySessionID         string
	RecordingID              string
	Capabilities             *workers.Capabilities
	InputPayload             []any
	ModelOperation           string
	ModelBindings            []workers.ResolvedModelOperationBinding
	Model                    string
	ModelProvider            string
	SystemPrompt             string
	UserMessage              string
	OutputSchema             string
	Timeout                  time.Duration
	EnvVars                  map[string]string
	ProcessEnvironment       []string
	Worktree                 string
	WorkingDirectory         string
	WorkingDirectoryAuthored bool
}

// RunnableDecision is the orchestration-neutral input for one scheduler-chosen
// dispatch. It carries canonical Work identity and plain execution facts
// without exposing an orchestrator implementation object.
type RunnableDecision struct {
	CorrelationID string
	Dispatch      work.WorkDispatch
	Execution     ExecutionFacts
}

// OutboxAction is one inert, ordered Workers publication action. It is not
// visible to Workers until the Runtime-owned outbox publishes it.
type OutboxAction struct {
	CorrelationID string
	Request       workers.WorkstationDispatchRequest
}

// PlanRequest preserves the order selected by the Runtime scheduler.
type PlanRequest struct {
	Decisions []RunnableDecision
}

// PlanResult contains one action per accepted decision in request order.
type PlanResult struct {
	Actions []OutboxAction
}

// WorkersPublisher is the exact Workers-facing publication edge. The outbox
// supplies a stable canonical request; routing, capacity, execution, and retry
// policy remain owned behind this boundary.
type WorkersPublisher func(context.Context, workers.WorkstationDispatchRequest) error

// WorkersCanceler is the exact Workers-facing cancellation edge. Workers owns
// cancellation execution and its idempotency policy; Runtime only forwards the
// stable dispatch identity it previously published.
type WorkersCanceler func(
	context.Context,
	workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error)

// PublicationOutcome describes whether an outbox intent was newly accepted or
// was an equivalent redelivery of an already accepted logical intent.
type PublicationOutcome string

const (
	PublicationOutcomeAccepted            PublicationOutcome = "ACCEPTED"
	PublicationOutcomeDuplicateIdempotent PublicationOutcome = "DUPLICATE_IDEMPOTENT"
)

// OutboxIntentStatus exposes the explicit retry state of an accepted intent.
type OutboxIntentStatus string

const (
	OutboxIntentStatusPending     OutboxIntentStatus = "PENDING"
	OutboxIntentStatusPublishing  OutboxIntentStatus = "PUBLISHING"
	OutboxIntentStatusPublished   OutboxIntentStatus = "PUBLISHED"
	OutboxIntentStatusInvalidated OutboxIntentStatus = "INVALIDATED"
	OutboxIntentStatusRetired     OutboxIntentStatus = "RETIRED"
)

// PublicationResult identifies the accepted logical intent and its outcome.
type PublicationResult struct {
	Outcome       PublicationOutcome
	DispatchID    string
	CorrelationID string
}

// OutboxIntent is a detached observation of one accepted Runtime intent.
type OutboxIntent struct {
	Action                OutboxAction
	Status                OutboxIntentStatus
	Attempts              int
	CancellationRequested bool
	Result                *TerminalResult
}

// WorkInvalidationOutcome describes whether a Work-scoped invalidation found
// any accepted Runtime intent. An invalidated intent remains addressable so a
// late result can be retired and classified by the terminal Work guard.
type WorkInvalidationOutcome string

const (
	WorkInvalidationOutcomeNoOp        WorkInvalidationOutcome = "NO_OP"
	WorkInvalidationOutcomeInvalidated WorkInvalidationOutcome = "INVALIDATED"
)

// WorkInvalidationIntent records the exact accepted intent observed during a
// Work-scoped invalidation, including tombstones that were already retired.
type WorkInvalidationIntent struct {
	DispatchID            string
	CorrelationID         string
	PreviousStatus        OutboxIntentStatus
	Status                OutboxIntentStatus
	CancellationRequested bool
}

// WorkInvalidationResult is a detached observation of one Work-scoped
// invalidation. Intents preserve their accepted publication order.
type WorkInvalidationResult struct {
	WorkID  string
	Outcome WorkInvalidationOutcome
	Intents []WorkInvalidationIntent
}

// RuntimeOutboxMode controls whether accepted intents may cross the Workers
// publication boundary.
type RuntimeOutboxMode string

const (
	RuntimeOutboxModeActive  RuntimeOutboxMode = "ACTIVE"
	RuntimeOutboxModePaused  RuntimeOutboxMode = "PAUSED"
	RuntimeOutboxModeStopped RuntimeOutboxMode = "STOPPED"
)

// RuntimeStopReason distinguishes customer cancellation from normal Runtime
// termination without changing the lossless outbox behavior.
type RuntimeStopReason string

const (
	RuntimeStopReasonCancelled  RuntimeStopReason = "CANCELLED"
	RuntimeStopReasonTerminated RuntimeStopReason = "TERMINATED"
)

// RuntimeOutboxState is a detached lifecycle observation.
type RuntimeOutboxState struct {
	Mode       RuntimeOutboxMode
	StopReason RuntimeStopReason
}

// TerminalResultOutcome is the orchestration-neutral terminal Workers result
// vocabulary accepted by dispatch planning.
type TerminalResultOutcome string

const (
	TerminalResultOutcomeSuccess   TerminalResultOutcome = "SUCCESS"
	TerminalResultOutcomeFailure   TerminalResultOutcome = "FAILURE"
	TerminalResultOutcomeCancelled TerminalResultOutcome = "CANCELLED"
)

// TerminalResult carries the identities, Work scope, and terminal fact needed
// to correlate one Workers result without exposing Workers implementation or
// orchestrator types.
type TerminalResult struct {
	DispatchID    string
	CorrelationID string
	WorkID        string
	Outcome       TerminalResultOutcome
}

// RetirementOutcome reports whether this delivery produced the one observable
// completion outcome or repeated the already accepted terminal fact.
type RetirementOutcome string

const (
	RetirementOutcomeRetired             RetirementOutcome = "RETIRED"
	RetirementOutcomeDuplicateIdempotent RetirementOutcome = "DUPLICATE_IDEMPOTENT"
)

// RetirementResult identifies the correlated intent and delivery outcome.
type RetirementResult struct {
	Outcome       RetirementOutcome
	DispatchID    string
	CorrelationID string
}

// Service owns deterministic decision validation and Workers request
// translation plus Runtime outbox identity and publication state. It invokes
// only the injected Workers-facing publisher, never an executor.
type Service interface {
	Plan(context.Context, PlanRequest) (PlanResult, error)
	Publish(context.Context, OutboxAction) (PublicationResult, error)
	Retry(context.Context, string) (PublicationResult, error)
	InvalidateWork(context.Context, string) (WorkInvalidationResult, error)
	Retire(context.Context, TerminalResult) (RetirementResult, error)
	Pause(context.Context) error
	Resume(context.Context) error
	Stop(context.Context, RuntimeStopReason) error
	State() RuntimeOutboxState
	Intent(string) (OutboxIntent, bool)
}
