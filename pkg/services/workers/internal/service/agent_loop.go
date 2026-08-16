package service

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	workerrecording "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/execution/recording"
	workerexecutor "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
)

// runAgentLoop runs one attempt through the agent-run harness instead of a
// single provider call. Runtime resolves the AGENT_RUN route from the Factory
// definitions it owns and declares it on the detached target, so this path adds
// no Workers-held workstation, pool, or definition state: the loop, its tool
// policy, and its per-turn runner all exist only for this Execute call.
//
// The harness owns provider retry for the turns it drives, matching the
// behavior the workstation router produced before dispatch became detached.
//
func (s *Service) runAgentLoop(
	ctx context.Context,
	request workers.ExecuteRequest,
	identity string,
	runnerRequest workers.RunnerExecutionRequest,
	providerOverride providers.Service,
) (workers.RunnerExecutionResult, error) {
	if s.agentRunHarness == nil {
		return workers.RunnerExecutionResult{}, errMisconfigured("agent-run harness is required")
	}
	return agentrun.ExecuteDetached(
		ctx,
		s.agentRunHarness,
		s.agentLoopRunner(request, identity, providerOverride),
		agentrun.DetachedRequest{
			Attempt:           runnerRequest,
			ProgressPublisher: request.Input.ProgressPublisher,
			ToolPolicy:        request.Target.Tools.AgentToolPolicy,
			WorkingDirectory:  runnerRequest.WorkingDirectory,
		},
	)
}

// agentLoopRunner selects the provider edge one agent loop turns on. It is the
// same edge a single attempt would use, so substituting a process provider
// keeps working for agent-run workstations.
func (s *Service) agentLoopRunner(
	request workers.ExecuteRequest,
	identity string,
	providerOverride providers.Service,
) workers.Runner {
	if providerOverride != nil && identity == runners.AgentIdentity &&
		!usesACPProvider(request.Target.ExecutorProvider) {
		return agentLoopProviderOverrideRunner{
			runner: workerrecording.NewProviderRunner(
				workerexecutor.RunnerFromProvider(providerOverride),
				request.Input.InferenceEventRecorder,
				s.clock,
			),
			decisionEnvelopes: s.decisionEnvelopes,
		}
	}
	return agentLoopRunnerFunc(func(
		turnCtx context.Context,
		attempt workers.RunnerExecutionRequest,
	) (workers.RunnerExecutionResult, error) {
		return s.runners.Execute(turnCtx, runners.ExecuteRequest{
			Identity: identity,
			RequiredCapabilities: runnerResolutionCapabilities(
				attempt.RequiredOptionalCapabilities,
				identity,
			),
			Attempt: attempt,
		})
	})
}

// agentLoopRunnerFunc adapts one closure to the Runner contract the harness
// inferencer consumes.
type agentLoopRunnerFunc func(
	context.Context,
	workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error)

func (run agentLoopRunnerFunc) Execute(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	return run(ctx, request)
}
