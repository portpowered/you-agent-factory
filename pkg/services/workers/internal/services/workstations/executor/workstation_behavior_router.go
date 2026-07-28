package executor

import (
	"context"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// WorkstationBehaviorRouter delegates execution to the agent-run harness path or
// the inference provider path based on configured worker and workstation taxonomy.
type WorkstationBehaviorRouter struct {
	RuntimeConfig     interfaces.RuntimeDefinitionLookup
	InferenceExecutor WorkstationRequestExecutor
	AgentRunExecutor  WorkstationRequestExecutor
}

var _ WorkstationRequestExecutor = (*WorkstationBehaviorRouter)(nil)

func (router *WorkstationBehaviorRouter) Execute(ctx context.Context, request workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
	if router != nil && router.shouldUseAgentRunExecutor(request) {
		return router.AgentRunExecutor.Execute(ctx, request)
	}
	if router == nil || router.InferenceExecutor == nil {
		return workerexecution.WorkResult{
			DispatchID:   request.Dispatch.DispatchID,
			TransitionID: request.Dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        "inference executor unavailable",
		}, nil
	}
	return router.InferenceExecutor.Execute(ctx, request)
}

func (router *WorkstationBehaviorRouter) shouldUseAgentRunExecutor(request workerexecution.WorkstationExecutionRequest) bool {
	if router.AgentRunExecutor == nil || router.RuntimeConfig == nil {
		return false
	}

	workstationDef, ok := router.RuntimeConfig.Workstation(request.Dispatch.WorkstationName)
	if !ok || workstationDef == nil || !interfaces.IsAgentRunWorkstationType(workstationDef.Type) {
		return false
	}

	workerName := request.WorkerType
	if workerName == "" {
		workerName = request.Dispatch.WorkerType
	}
	workerDef, ok := router.RuntimeConfig.Worker(workerName)
	if !ok || workerDef == nil {
		return false
	}
	return interfaces.IsAgentWorkerType(workerDef.Type)
}
