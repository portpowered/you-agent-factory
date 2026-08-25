package executor

import (
	"context"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// ProviderInvocationExecutor executes one Worker that has no authored
// workstation behind it: every fact it needs -- prompt, model, provider,
// reasoning effort, output schema -- arrives on the execution request itself
// rather than being rendered from a workstation definition.
//
// This is the executor behind workers.ProviderInvocationRoute, and it is what
// lets a JavaScript workflow child be an ordinary Worker. A child agent.run
// already holds a literal prompt, so the workstation executor's prompt
// rendering, worktree preparation, and colour parsing have nothing to act on;
// routing a child through them would require inventing a workstation
// definition that does not exist. Instead the child names this route, and
// Worker Sessions supervises it through exactly the same lifecycle, publication
// window, controls, and retry as any Petri Worker.
//
// It performs no retry of its own. Attempt budgeting belongs to Worker
// Sessions, which owns the session the attempts run under.
type ProviderInvocationExecutor struct {
	invocation workerexecution.InvocationExecutor
}

var _ WorkstationRequestExecutor = (*ProviderInvocationExecutor)(nil)

// NewProviderInvocationExecutor constructs the direct-inference executor from
// the Workers-owned invocation boundary. A nil invocation boundary yields a nil
// executor so composition can treat "no provider invocation available" as an
// absent route rather than a route that fails at dispatch time.
func NewProviderInvocationExecutor(invocation workerexecution.InvocationExecutor) *ProviderInvocationExecutor {
	if invocation == nil {
		return nil
	}
	return &ProviderInvocationExecutor{invocation: invocation}
}

// Execute performs one provider attempt for request and reports its outcome as
// an ordinary WorkResult, so callers above this point cannot tell a
// workstation-backed Worker from a provider-invocation one.
func (e *ProviderInvocationExecutor) Execute(
	ctx context.Context,
	request workerexecution.WorkstationExecutionRequest,
) (workerexecution.WorkResult, error) {
	if e == nil || e.invocation == nil {
		return workerexecution.WorkResult{
			DispatchID:   request.Dispatch.DispatchID,
			TransitionID: request.Dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        "provider invocation executor unavailable",
		}, nil
	}
	if request.OutputSchema != "" {
		if schemaErr := validateOutputSchema([]byte(request.OutputSchema)); schemaErr != nil {
			return outputSchemaConfigurationFailure(request, schemaErr), nil
		}
	}

	result, err := e.invocation.Execute(ctx, workerexecution.InvocationInput{
		Request: providerInvocationRequest(request),
		Attempt: 1,
	})
	if err != nil {
		return providerInvocationFailure(request, result), err
	}
	// Diagnostics are deliberately not carried across. The invocation boundary
	// has already narrowed them to the dashboard-safe projection, and
	// WorkResult holds the wider internal form; there is no widening
	// conversion because widening would mean re-inventing detail that was
	// dropped on purpose.
	outcome := workerexecution.OutcomeAccepted
	switch result.Response.Outcome {
	case workerexecution.OutcomeContinue,
		workerexecution.OutcomeRejected,
		workerexecution.OutcomeFailed:
		outcome = result.Response.Outcome
	}
	return attachStructuredResult(request, workerexecution.WorkResult{
		DispatchID:   request.Dispatch.DispatchID,
		TransitionID: request.Dispatch.TransitionID,
		Outcome:      outcome,
		Output:       result.Response.Content,
		Continuation: cloneContinuation(result.Continuation),
	}), nil
}

// providerInvocationRequest maps the execution request onto a provider
// inference request without consulting any worker or workstation definition.
// The absence of that lookup is the whole point of this route: the caller has
// already resolved every selection this Worker needs.
func providerInvocationRequest(
	request workerexecution.WorkstationExecutionRequest,
) workerexecution.ProviderInferenceRequest {
	request = workerexecution.CloneWorkstationExecutionRequest(request)
	runnerID := strings.TrimSpace(request.RunnerID)
	if runnerID == "" {
		// Provider invocation receives a detached request whose runner/provider
		// selection was already resolved by the caller. ModelProvider is the
		// final provider-bound fallback for older request fixtures.
		runnerID = strings.TrimSpace(request.ModelProvider)
	}
	return workerexecution.ProviderInferenceRequest{
		Dispatch:                           work.CloneWorkDispatch(request.Dispatch),
		WorkerType:                         request.WorkerType,
		WorkstationType:                    request.WorkstationType,
		RunnerID:                           runnerID,
		ExecutorProvider:                   strings.TrimSpace(request.ExecutorProvider),
		ProjectID:                          request.ProjectID,
		InputTokens:                        request.InputTokens,
		ModelOperation:                     request.ModelOperation,
		ModelBindings:                      workerexecution.CloneResolvedModelOperationBindings(request.ModelBindings),
		SystemPrompt:                       request.SystemPrompt,
		UserMessage:                        request.UserMessage,
		PromptRedaction:                    request.PromptRedaction.Clone(),
		OutputSchema:                       request.OutputSchema,
		EnvVars:                            cloneEnvVars(request.EnvVars),
		ProcessEnvironment:                 append([]string(nil), request.ProcessEnvironment...),
		ProcessLifecycleObserver:           request.ProcessLifecycleObserver,
		Worktree:                           request.Worktree,
		WorkingDirectory:                   request.WorkingDirectory,
		Model:                              request.Model,
		ModelProvider:                      request.ModelProvider,
		ReasoningEffort:                    request.ReasoningEffort,
		Continuation:                       cloneContinuation(request.Continuation),
		SkipPermissions:                    request.SkipPermissions,
		DeclaredSecretInvocationParameters: append([]string(nil), request.DeclaredSecretInvocationParameters...),
	}
}

// providerInvocationFailure preserves the Workers-owned failure classification
// on the result, which is what Worker Sessions consults to decide whether the
// attempt is worth retrying. Dropping FailureMetadata here would silently make
// every provider failure terminal.
func providerInvocationFailure(
	request workerexecution.WorkstationExecutionRequest,
	result workerexecution.InvocationResult,
) workerexecution.WorkResult {
	failure := workerexecution.WorkResult{
		DispatchID:      request.Dispatch.DispatchID,
		TransitionID:    request.Dispatch.TransitionID,
		Outcome:         workerexecution.OutcomeFailed,
		FailureMetadata: result.FailureMetadata,
		FailureDetail:   workerexecution.CloneFailureDetail(result.FailureDetail),
		Continuation:    cloneContinuation(result.Continuation),
	}
	if result.FailureDetail != nil {
		failure.Error = result.FailureDetail.Message
	}
	if strings.TrimSpace(failure.Error) == "" {
		failure.Error = "Provider execution failed."
	}
	return failure
}
