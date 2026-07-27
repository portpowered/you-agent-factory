// Package service implements deterministic Factory Runtime dispatch planning.
package service

import (
	"context"
	"fmt"
	"strings"

	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Planner translates complete runnable decisions without performing IO.
type Planner struct{}

var _ dispatchplanning.Service = (*Planner)(nil)

// New constructs an inert dispatch planner.
func New() *Planner {
	return &Planner{}
}

// Plan validates the entire batch before returning any outbox action.
func (p *Planner) Plan(ctx context.Context, req dispatchplanning.PlanRequest) (dispatchplanning.PlanResult, error) {
	if ctx == nil {
		return dispatchplanning.PlanResult{}, fmt.Errorf("%w: context is required", dispatchplanning.ErrInvalidRunnableDecision)
	}
	if err := ctx.Err(); err != nil {
		return dispatchplanning.PlanResult{}, err
	}
	if len(req.Decisions) == 0 {
		return dispatchplanning.PlanResult{}, nil
	}

	dispatchIDs := make(map[string]struct{}, len(req.Decisions))
	correlationIDs := make(map[string]struct{}, len(req.Decisions))
	for index, decision := range req.Decisions {
		if err := validateDecision(decision, dispatchIDs, correlationIDs); err != nil {
			return dispatchplanning.PlanResult{}, fmt.Errorf(
				"%w at scheduler position %d: %v",
				dispatchplanning.ErrInvalidRunnableDecision,
				index,
				err,
			)
		}
	}

	actions := make([]dispatchplanning.OutboxAction, 0, len(req.Decisions))
	for _, decision := range req.Decisions {
		execution := executionRequest(decision)
		actions = append(actions, dispatchplanning.OutboxAction{
			CorrelationID: decision.CorrelationID,
			Request: workers.WorkstationDispatchRequest{
				WorkstationName: execution.Dispatch.WorkstationName,
				Execution:       execution,
			},
		})
	}
	return dispatchplanning.PlanResult{Actions: actions}, nil
}

func validateDecision(
	decision dispatchplanning.RunnableDecision,
	dispatchIDs map[string]struct{},
	correlationIDs map[string]struct{},
) error {
	dispatch := decision.Dispatch
	required := []struct {
		name  string
		value string
	}{
		{name: "dispatch ID", value: dispatch.DispatchID},
		{name: "correlation ID", value: decision.CorrelationID},
		{name: "replay key", value: dispatch.Execution.ReplayKey},
		{name: "workstation name", value: dispatch.WorkstationName},
		{name: "worker type", value: decision.Execution.WorkerType},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s is required", field.name)
		}
	}
	if dispatch.WorkerType != "" && dispatch.WorkerType != decision.Execution.WorkerType {
		return fmt.Errorf("worker type conflicts with canonical dispatch")
	}
	if len(dispatch.Execution.WorkIDs) == 0 || containsBlank(dispatch.Execution.WorkIDs) {
		return fmt.Errorf("Work lineage is required")
	}
	if dispatch.InputTokens == nil {
		return fmt.Errorf("dispatch input payload is required")
	}
	if decision.Execution.InputPayload == nil {
		return fmt.Errorf("Workers input payload is required")
	}
	if _, exists := dispatchIDs[dispatch.DispatchID]; exists {
		return fmt.Errorf("dispatch ID %q appears more than once", dispatch.DispatchID)
	}
	if _, exists := correlationIDs[decision.CorrelationID]; exists {
		return fmt.Errorf("correlation ID %q appears more than once", decision.CorrelationID)
	}
	dispatchIDs[dispatch.DispatchID] = struct{}{}
	correlationIDs[decision.CorrelationID] = struct{}{}
	return nil
}

func executionRequest(decision dispatchplanning.RunnableDecision) workers.WorkstationExecutionRequest {
	facts := decision.Execution
	return workers.CloneWorkstationExecutionRequest(workers.WorkstationExecutionRequest{
		Dispatch:                 decision.Dispatch,
		WorkerType:               facts.WorkerType,
		WorkstationType:          facts.WorkstationType,
		RunnerID:                 facts.RunnerID,
		RunnerSelectionSource:    facts.RunnerSelectionSource,
		ProjectID:                facts.ProjectID,
		FactorySessionID:         facts.FactorySessionID,
		InputTokens:              facts.InputPayload,
		ModelOperation:           facts.ModelOperation,
		ModelBindings:            facts.ModelBindings,
		Model:                    facts.Model,
		ModelProvider:            facts.ModelProvider,
		SystemPrompt:             facts.SystemPrompt,
		UserMessage:              facts.UserMessage,
		OutputSchema:             facts.OutputSchema,
		EnvVars:                  facts.EnvVars,
		ProcessEnvironment:       facts.ProcessEnvironment,
		Worktree:                 facts.Worktree,
		WorkingDirectory:         facts.WorkingDirectory,
		WorkingDirectoryAuthored: facts.WorkingDirectoryAuthored,
	})
}

func containsBlank(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
	}
	return false
}
