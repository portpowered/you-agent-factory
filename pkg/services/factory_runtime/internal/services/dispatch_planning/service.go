// Package dispatch_planning defines the parent-private Factory Runtime
// capability that translates runnable decisions into canonical Workers
// requests. Publication and execution remain outside this planning boundary.
package dispatch_planning

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// ErrInvalidRunnableDecision reports an incomplete or internally inconsistent
// decision. Planning rejects the complete batch when any decision is invalid.
var ErrInvalidRunnableDecision = errors.New("invalid Factory Runtime runnable decision")

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
	InputPayload             []any
	ModelOperation           string
	ModelBindings            []workers.ResolvedModelOperationBinding
	Model                    string
	ModelProvider            string
	SystemPrompt             string
	UserMessage              string
	OutputSchema             string
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

// Service owns deterministic decision validation and Workers request
// translation. It neither publishes requests nor invokes Workers.
type Service interface {
	Plan(context.Context, PlanRequest) (PlanResult, error)
}
