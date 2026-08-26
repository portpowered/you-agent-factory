// Package invocation implements the Workers provider-invocation boundary.
package invocation

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Executor adapts the Providers root to one Worker invocation attempt. The
// adapter translates only the detached Worker request and result values at the
// boundary; provider selection, prerequisite checks, execution, and failure
// normalization remain owned by Providers.
type Executor struct {
	provider providers.Service
}

// NewExecutor constructs the concrete Workers invocation adapter selected by
// Wire and same-owner Workers constituents.
func NewExecutor(provider providers.Service) workers.InvocationExecutor {
	if provider == nil {
		return nil
	}
	return &Executor{provider: provider}
}

// NewProviderExecutor constructs the concrete adapter without narrowing its
// return type. It is useful in same-owner tests that exercise adapter details.
func NewProviderExecutor(provider providers.Service) *Executor {
	return &Executor{provider: provider}
}

// NewRunnerExecutor adapts an already selected Workers runner without
// reintroducing a provider client interface. The runner is still invoked for
// one request-scoped attempt; provider-backed runners reach Providers through
// their owner-side implementation.
func NewRunnerExecutor(runner workers.Runner) workers.InvocationExecutor {
	if runner == nil {
		return nil
	}
	return &runnerExecutor{runner: runner}
}

type runnerExecutor struct {
	runner workers.Runner
}

func (e *runnerExecutor) Execute(
	ctx context.Context,
	input workers.InvocationInput,
) (workers.InvocationResult, error) {
	attempt := input.Attempt
	if attempt < 1 {
		attempt = 1
	}
	if e == nil || e.runner == nil {
		err := workers.NewProviderError(
			workers.WorkFailureTypeMisconfigured,
			"runner execution requires a runner",
			nil,
		)
		return failedInvocationResult(attempt, err), err
	}
	result, err := e.runner.Execute(ctx, input.Request)
	if err != nil {
		return failedInvocationResult(attempt, err), err
	}
	response := workers.InferenceResponse{
		Content:        result.Content,
		Outcome:        result.Outcome,
		ProposedOutput: cloneProposedOutput(result.ProposedOutput),
		Continuation:   cloneContinuation(result.Continuation),
		Diagnostics:    workers.CloneWorkDiagnostics(result.Diagnostics),
	}
	return workers.InvocationResult{
		Response:     response,
		Attempt:      attempt,
		Continuation: cloneContinuation(response.Continuation),
		Diagnostics:  workers.SafeWorkDiagnosticsFromWorkDiagnostics(response.Diagnostics),
	}, nil
}

func cloneProposedOutput(output *workers.ProposedOutput) *workers.ProposedOutput {
	if output == nil {
		return nil
	}
	clone := output.Clone()
	return &clone
}

func (e *Executor) Execute(
	ctx context.Context,
	input workers.InvocationInput,
) (workers.InvocationResult, error) {
	attempt := input.Attempt
	if attempt < 1 {
		attempt = 1
	}
	if err := ctx.Err(); err != nil {
		return failedInvocationResult(attempt, err), err
	}
	if e == nil || e.provider == nil {
		err := workers.NewProviderError(
			workers.WorkFailureTypeMisconfigured,
			"provider execution requires a provider",
			nil,
		)
		return failedInvocationResult(attempt, err), err
	}

	request, err := providersRequest(input.Request)
	if err != nil {
		return failedInvocationResult(attempt, err), err
	}
	result, err := e.executeRequest(ctx, input.Request, request)
	if err != nil {
		normalized := normalizeProvidersFailure(err, input.Request.Model, request.Provider.String())
		return failedInvocationResult(attempt, normalized), normalized
	}
	response := workers.InferenceResponse{
		Content:      result.Content,
		Outcome:      workers.WorkOutcome(result.Outcome),
		Continuation: continuationFromSessionRef(result.SessionRef),
		Diagnostics:  workersDiagnostics(result.Diagnostics, result.SessionRef, input.Request.Model, request.Provider.String()),
	}
	return workers.InvocationResult{
		Response:     response,
		Attempt:      attempt,
		Continuation: cloneContinuation(response.Continuation),
		Diagnostics:  workers.SafeWorkDiagnosticsFromWorkDiagnostics(response.Diagnostics),
	}, nil
}

func (e *Executor) executeRequest(
	ctx context.Context,
	input workers.ProviderInferenceRequest,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	if input.Continuation != nil {
		return e.executeContinuationReference(ctx, input.Continuation, request)
	}
	return e.executeFresh(ctx, request)
}

func (e *Executor) executeContinuationReference(
	ctx context.Context,
	reference *workers.ProviderContinuationRef,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	continued, err := e.provider.ContinueReference(ctx, providers.ContinueReferenceRequest{
		Reference: reference.Clone(),
		Attempt:   request,
	})
	if err == nil && continued.Outcome == providers.ContinuationOutcomeUnsupported {
		converted, convertErr := continued.Reference.ToSessionRef()
		if convertErr != nil {
			err = convertErr
		} else {
			err = unsupportedProviderContinuationError(converted)
		}
	}
	return continued.Result, err
}

func (e *Executor) executeFresh(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	identity, err := e.provider.ResolveIdentity(ctx, providers.ResolveIdentityRequest{Identity: request.Provider.String()})
	if err != nil {
		return providers.ExecuteResult{}, err
	}
	request.Provider = identity.ID
	if err := e.provider.ValidatePrerequisites(ctx, providers.ValidatePrerequisitesRequest{ID: request.Provider}); err != nil {
		return providers.ExecuteResult{}, err
	}
	return e.provider.Execute(ctx, request)
}

func providersRequest(request workers.ProviderInferenceRequest) (providers.ExecuteRequest, error) {
	provider := ""
	if request.Continuation != nil {
		provider = strings.TrimSpace(request.Continuation.Provider)
	}
	if provider == "" {
		provider = strings.TrimSpace(request.ModelProvider)
	}
	if provider == "" {
		provider = strings.TrimSpace(request.RunnerID)
	}
	if provider == "" {
		provider = strings.TrimSpace(request.ExecutorProvider)
	}
	runnerID := strings.TrimSpace(request.RunnerID)
	if runnerID == "" {
		runnerID = provider
	}
	attemptID := strings.TrimSpace(request.Correlation.AttemptID)
	if attemptID == "" {
		attemptID = strings.TrimSpace(request.Dispatch.DispatchID)
	}
	dispatchID := strings.TrimSpace(request.Correlation.DispatchID)
	if dispatchID == "" {
		dispatchID = strings.TrimSpace(request.Dispatch.DispatchID)
	}
	requestID := strings.TrimSpace(request.Correlation.RequestID)
	if requestID == "" {
		requestID = strings.TrimSpace(request.Dispatch.Execution.RequestID)
	}
	traceID := strings.TrimSpace(request.Correlation.TraceID)
	if traceID == "" {
		traceID = strings.TrimSpace(request.Dispatch.Execution.TraceID)
	}
	workIDs := append([]string(nil), request.Dispatch.Execution.WorkIDs...)
	return providers.ExecuteRequest{
		Provider:  providers.ID(provider),
		AttemptID: attemptID,
		Correlation: providers.ExecuteCorrelation{
			FactorySessionID: request.Correlation.FactorySessionID,
			RuntimeID:        request.Correlation.RuntimeID,
			GenerationID:     request.Correlation.GenerationID,
			DispatchID:       dispatchID,
			AttemptID:        attemptID,
			RequestID:        requestID,
			TraceID:          traceID,
			ReplayKey:        request.Dispatch.Execution.ReplayKey,
			WorkIDs:          workIDs,
		},
		WorkerType:      request.WorkerType,
		WorkstationName: request.WorkstationType,
		RunnerID:        runnerID,
		ProjectID:       request.ProjectID,
		//TODO: we should likely pull out logic that is largely bounded to petri tokens away from this.
		TransitionID:                request.Dispatch.TransitionID,
		InputBindings:               cloneStringSliceMap(request.Dispatch.InputBindings),
		SessionID:                   request.SessionID,
		Model:                       request.Model,
		ModelOperation:              request.ModelOperation,
		ModelBindings:               providerModelOperationBindings(request.ModelBindings),
		ReasoningEffort:             request.ReasoningEffort,
		ModelLocality:               request.ModelLocality,
		SkipPermissions:             request.SkipPermissions,
		PrintTimeout:                request.PrintTimeout,
		SystemPrompt:                request.SystemPrompt,
		UserMessage:                 request.UserMessage,
		InputTokens:                 append([]any(nil), request.InputTokens...),
		OutputSchema:                request.OutputSchema,
		ToolExecutionMode:           string(request.ToolExecutionMode),
		RequiredCapabilities:        runnerCapabilityNames(request.RequiredOptionalCapabilities),
		Command:                     request.Command,
		Args:                        append([]string(nil), request.Args...),
		FactoryDirectory:            request.FactoryDirectory,
		OutputContract:              request.OutputContract,
		OutputFormat:                request.OutputFormat,
		StopToken:                   request.StopToken,
		DecisionEnvelope:            request.DecisionEnvelope,
		GoalRoutingDecisionEnvelope: request.GoalRoutingDecisionEnvelope,
		WorkingDirectory:            request.WorkingDirectory,
		Worktree:                    request.Worktree,
		EnvVars:                     cloneStringMap(request.EnvVars),
		ProcessEnvironment:          append([]string(nil), request.ProcessEnvironment...),
		ExecutionLogger:             request.ExecutionLogger,
		ProcessLifecycleObserver:    request.ProcessLifecycleObserver,
	}, nil
}

func providerModelOperationBindings(values []workers.ResolvedModelOperationBinding) []providers.ResolvedModelOperationBinding {
	if values == nil {
		return nil
	}
	converted := make([]providers.ResolvedModelOperationBinding, len(values))
	for index, value := range values {
		converted[index] = providers.ResolvedModelOperationBinding{
			Slot:    value.Slot,
			Source:  string(value.Source),
			Content: work.CloneWorkContentParts(value.Content),
		}
	}
	return converted
}

func normalizeProvidersFailure(err error, model, provider string) error {
	var continuation providers.ContinuationFailure
	if errors.As(err, &continuation) {
		normalized := workers.NewProviderError(
			workers.WorkFailureTypePermanentBadRequest,
			"provider session continuation was rejected",
			err,
		)
		normalized.ProviderContinuationFailureKind = continuation.Kind
		normalized.Continuation = continuationFromSessionRef(&continuation.Reference)
		normalized.Diagnostics = workersDiagnostics(nil, &continuation.Reference, model, provider)
		return normalized
	}
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		return err
	}
	normalized := workers.NewProviderError(
		workersFailureType(failure),
		failure.Message,
		err,
	)
	normalized.ProviderFailureKind = failure.Kind
	normalized.Continuation = continuationFromSessionRef(failure.SessionRef)
	normalized.Diagnostics = workersDiagnostics(failure.Diagnostics, failure.SessionRef, model, provider)
	return normalized
}

func unsupportedProviderContinuationError(reference providers.SessionRef) error {
	normalized := workers.NewProviderError(
		workers.WorkFailureTypePermanentBadRequest,
		"provider session continuation is unsupported",
		nil,
	)
	normalized.ProviderContinuationOutcome = providers.ContinuationOutcomeUnsupported
	normalized.Continuation = continuationFromSessionRef(&reference)
	return normalized
}

func workersFailureType(failure providers.ExecuteFailure) workers.WorkFailureType {
	if failure.Kind == providers.ExecuteFailureKindUnknown &&
		failure.Diagnostics != nil &&
		failure.Diagnostics.Metadata[providers.ExecuteDiagnosticMetadataUnrecognizedProviderRefusal] == "true" {
		return workers.WorkFailureTypePermanentBadRequest
	}
	switch failure.Kind {
	case providers.ExecuteFailureKindCanceled:
		return workers.WorkFailureTypeUnknown
	case providers.ExecuteFailureKindTimeout:
		return workers.WorkFailureTypeTimeout
	case providers.ExecuteFailureKindAuthentication:
		return workers.WorkFailureTypeAuthFailure
	case providers.ExecuteFailureKindInvalidRequest,
		providers.ExecuteFailureKindCapabilityMismatch:
		return workers.WorkFailureTypePermanentBadRequest
	case providers.ExecuteFailureKindThrottled:
		return workers.WorkFailureTypeThrottled
	case providers.ExecuteFailureKindMisconfigured:
		return workers.WorkFailureTypeMisconfigured
	case providers.ExecuteFailureKindDependency:
		return workers.WorkFailureTypeUnknown
	case providers.ExecuteFailureKindUnknown, providers.ExecuteFailureKindSessionNotFound:
		return workers.WorkFailureTypeUnknown
	default:
		return workers.WorkFailureTypeUnknown
	}
}

func continuationFromSessionRef(reference *providers.SessionRef) *workers.ProviderContinuationRef {
	if reference == nil {
		return nil
	}
	continuation := reference.ContinuationRef()
	return &continuation
}

func workersDiagnostics(diagnostics *providers.ExecuteDiagnostics, reference *providers.SessionRef, model, provider string) *workers.WorkDiagnostics {
	if diagnostics == nil && reference == nil && strings.TrimSpace(model) == "" && strings.TrimSpace(provider) == "" {
		return nil
	}
	metadata := map[string]string(nil)
	if diagnostics != nil {
		metadata = cloneStringMap(diagnostics.Metadata)
	}
	providerName := strings.TrimSpace(provider)
	if reference != nil {
		providerName = reference.Provider.String()
	}
	result := &workers.WorkDiagnostics{
		Provider: &workers.ProviderDiagnostic{
			Provider:         providerName,
			Model:            strings.TrimSpace(model),
			ResponseMetadata: metadata,
		},
		Metadata: metadata,
	}
	if diagnostics != nil && diagnostics.Command != nil {
		result.Command = &workers.CommandDiagnostic{
			Command:    diagnostics.Command.Command,
			Args:       append([]string(nil), diagnostics.Command.Args...),
			Env:        cloneStringMap(diagnostics.Command.Env),
			Stdin:      diagnostics.Command.Stdin,
			Stdout:     diagnostics.Command.Stdout,
			Stderr:     diagnostics.Command.Stderr,
			ExitCode:   diagnostics.Command.ExitCode,
			TimedOut:   diagnostics.Command.TimedOut,
			Duration:   time.Duration(diagnostics.Command.DurationMS) * time.Millisecond,
			WorkingDir: diagnostics.Command.WorkingDir,
		}
	}
	if diagnostics != nil && diagnostics.Panic != nil {
		result.Panic = &workers.PanicDiagnostic{
			Message: diagnostics.Panic.Message,
			Stack:   diagnostics.Panic.Stack,
		}
	}
	return result
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneStringSliceMap(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string][]string, len(values))
	for key, items := range values {
		cloned[key] = append([]string(nil), items...)
	}
	return cloned
}

func runnerCapabilityNames(values []workers.RunnerOptionalCapability) []string {
	if len(values) == 0 {
		return nil
	}
	names := make([]string, len(values))
	for index, value := range values {
		names[index] = string(value)
	}
	return names
}

func failedInvocationResult(attempt int, err error) workers.InvocationResult {
	providerErr := workers.NormalizeProviderExecutionError(err)
	if providerErr == nil {
		providerErr = workers.NewProviderError(
			workers.WorkFailureTypeUnknown,
			"Provider execution failed.",
			err,
		)
	}
	metadata := &workers.WorkFailureMetadata{
		Family: providerErr.Family,
		Type:   providerErr.Type,
	}
	decision := workers.FailureDecisionFromMetadata(metadata)
	return workers.InvocationResult{
		Attempt:         attempt,
		Continuation:    cloneContinuation(providerErr.Continuation),
		FailureMetadata: metadata,
		FailureDecision: &decision,
		FailureDetail: &workers.FailureDetail{
			Reason:  providerErr.Type,
			Message: providerFailureMessage(providerErr),
		},
		Diagnostics: workers.SafeWorkDiagnosticsFromWorkDiagnostics(providerErr.Diagnostics),
	}
}

const providerUnrecognizedRefusalMessage = "provider rejected the execution request"

func providerFailureMessage(providerErr *workers.ProviderError) string {
	if providerErr == nil {
		return safeFailureMessage(workers.WorkFailureTypeUnknown)
	}
	if providerErr.Diagnostics != nil && providerErr.Diagnostics.Provider != nil {
		metadata := providerErr.Diagnostics.Provider.ResponseMetadata
		if metadata[providers.ExecuteDiagnosticMetadataUnrecognizedProviderRefusal] == "true" {
			return providerUnrecognizedRefusalMessage
		}
		if metadata[providers.ExecuteDiagnosticMetadataSafeFailureMessage] == "true" {
			if message := strings.TrimSpace(providerErr.Message); message != "" {
				return message
			}
		}
	}
	// Providers normalizes ExecuteFailure.Message before it crosses into
	// Workers. ProviderFailureKind is the marker that this is that safe,
	// provider-owned explanation; hand-built Worker errors still use the stable
	// type allowlist below so arbitrary error text cannot leak into the public
	// invocation result.
	if providerErr.ProviderFailureKind != "" {
		if message := strings.TrimSpace(providerErr.Message); message != "" {
			return message
		}
	}
	return safeFailureMessage(providerErr.Type)
}

func cloneContinuation(reference *workers.ProviderContinuationRef) *workers.ProviderContinuationRef {
	if reference == nil {
		return nil
	}
	clone := reference.Clone()
	return &clone
}

func safeFailureMessage(reason workers.WorkFailureType) string {
	switch reason {
	case workers.WorkFailureTypeAuthFailure:
		return "Provider authentication failed."
	case workers.WorkFailureTypePermanentBadRequest:
		return "Provider rejected the request as invalid."
	case workers.WorkFailureTypeThrottled:
		return "Provider is temporarily unavailable due to usage or capacity limits."
	case workers.WorkFailureTypeInternalServerError:
		return "Provider encountered a temporary server error."
	case workers.WorkFailureTypeTimeout:
		return "Provider request timed out."
	case workers.WorkFailureTypeMisconfigured:
		return "Provider command could not be started."
	case workers.WorkFailureTypeMissingExecutable:
		return "Provider executable could not be found."
	case workers.WorkFailureTypeCommandLineTooLong:
		return "Provider command exceeded the operating system command-line limit."
	default:
		return "Provider execution failed."
	}
}
