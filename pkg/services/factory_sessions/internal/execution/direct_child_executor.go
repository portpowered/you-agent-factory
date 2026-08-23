package factorysessionexecution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// directChildExecutor runs one JavaScript workflow child through the injected
// Workers Execute boundary for the one composition that has no Factory Runtime
// behind it: the standalone `you run script.js` path, which opens a JavaScript
// execution service directly and never builds a runtime, Worker Sessions
// service, or event ledger.
//
// This is not a fallback and nothing selects it at dispatch time. The two
// executors are chosen once, at construction, by which composition is being
// built -- childWorkerExecutor where a runtime exists, this where none does --
// so a child can never take a second route out of the converged one.
//
// It performs bounded detached Execute operations; Workers owns provider retry
// within each operation, while the JavaScript child policy owns the outer
// schema-mismatch retry allowance. Retry and the dispatch/Worker-Session
// association do not become a private Sessions implementation in a composition
// with no Worker Sessions.
// It does still write the queued/running/terminal dispatch records itself,
// because nothing else here can. Children run through this executor are
// invisible to a client: there is no Factory whose tool call they could be
// content inside.
type directChildExecutor struct {
	sessionID         string
	execute           childExecuteService
	records           factory.JavaScriptChildRecordSink
	childValues       factory.JavaScriptChildValues
	workingDir        string
	maxAttempts       int
	maxWorkerDuration time.Duration
}

// legacyDirectChildExecution is a test/in-package compatibility bridge for
// callers that still construct JavaScriptRuntimeService with the old
// InvocationExecutor shape. Wire's standalone production path supplies the
// Workers Execute capability directly; P6-C can delete this bridge with the
// remaining invocation compatibility declarations.
type legacyDirectChildExecution struct {
	invocation workers.InvocationExecutor
}

func (e legacyDirectChildExecution) Execute(
	ctx context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	result, err := e.invocation.Execute(ctx, workers.InvocationInput{
		Request: workers.ProviderInferenceRequest{
			Dispatch:         request.Input.Dispatch,
			WorkerType:       request.Target.WorkerType,
			RunnerID:         request.Target.RunnerID,
			ExecutorProvider: request.Target.ExecutorProvider,
			UserMessage:      request.Target.Prompt.UserMessage,
			OutputSchema:     request.Target.Prompt.OutputSchema,
			Model:            request.Target.Model.Name,
			ModelProvider:    request.Target.Model.Provider,
			ReasoningEffort:  request.Target.Model.ReasoningEffort,
			WorkingDirectory: request.Target.Environment.WorkingDirectory,
			SkipPermissions:  request.Target.Permissions.SkipPermissions,
		},
		Attempt: request.Attempt.Number,
	})
	output := workers.ProposedOutput{}
	if strings.TrimSpace(result.Response.Content) != "" {
		output.Primary = []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: result.Response.Content,
		}}
	}
	if err == nil {
		return workers.ExecuteResult{
			Correlation:  request.Correlation,
			Outcome:      workers.ExecutionOutcomeAccepted,
			Output:       output,
			Continuation: result.Continuation,
		}, nil
	}
	failureType := workers.WorkFailureTypeUnknown
	failureFamily := workers.WorkFailureFamilyTerminal
	if result.FailureMetadata != nil {
		failureType = result.FailureMetadata.Type
		failureFamily = result.FailureMetadata.Family
	}
	message := err.Error()
	if result.FailureDetail != nil && strings.TrimSpace(result.FailureDetail.Message) != "" {
		message = result.FailureDetail.Message
	}
	retryHint := false
	if result.FailureDecision != nil {
		retryHint = result.FailureDecision.Retryable
	}
	return workers.ExecuteResult{
		Correlation: request.Correlation,
		Outcome:     workers.ExecutionOutcomeFailed,
		Failure: &workers.ExecutionFailure{
			Type:      failureType,
			Family:    failureFamily,
			Message:   message,
			RetryHint: retryHint,
			Detail:    result.FailureDetail,
		},
		Continuation: result.Continuation,
	}, err
}

func newDirectChildExecutor(
	sessionID string,
	execution childExecuteService,
	records factory.JavaScriptChildRecordSink,
	childValues factory.JavaScriptChildValues,
	workingDir string,
	maxRetries int,
) *directChildExecutor {
	attempts := maxRetries + 1
	if attempts < 1 {
		attempts = 1
	}
	return &directChildExecutor{
		sessionID:   sessionID,
		execute:     execution,
		records:     records,
		childValues: childValues,
		workingDir:  strings.TrimSpace(workingDir),
		maxAttempts: attempts,
	}
}

// missingChildExecutor is returned when a live child composition was opened
// without its required Workers Execute capability. Keeping this as an
// explicit executor makes the wiring failure immediate and attributable to
// the child session instead of constructing a nil-capability executor that
// can leave a durable session waiting forever.
type missingChildExecutor struct {
	sessionID string
}

func (e missingChildExecutor) Execute(
	ctx context.Context,
	_ factory.JavaScriptChildExecutionRequest,
) (factory.JavaScriptChildExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return factory.JavaScriptChildExecutionResult{}, err
	}
	return factory.JavaScriptChildExecutionResult{}, fmt.Errorf(
		"child session %q cannot execute: Workers Execute capability is required",
		strings.TrimSpace(e.sessionID),
	)
}

func (e *directChildExecutor) Execute(
	ctx context.Context,
	req factory.JavaScriptChildExecutionRequest,
) (factory.JavaScriptChildExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return factory.JavaScriptChildExecutionResult{}, err
	}
	if e == nil || e.execute == nil {
		return factory.JavaScriptChildExecutionResult{}, NewValidationError(
			"runtime.childExecutorMode",
			"the Workers Execute capability is required for live child execution",
		)
	}
	skipPermissions := effectiveChildSkipPermissions(req)

	dispatchID, childIndex := e.childDispatchIdentity(req)
	runnerID, err := childRunnerID(req.ExecutorProvider, req.ModelProvider)
	if err != nil {
		return factory.JavaScriptChildExecutionResult{}, err
	}
	if runnerID == "" {
		runnerID = firstNonBlank(req.Command, req.ModelProvider)
	}
	artifactID := e.records.NextChildArtifactID()
	artifactRef := factory.FormatArtifactURI(e.sessionID, artifactID)

	base := factory.JavaScriptChildDispatchRecord{
		DispatchID:      dispatchID,
		ChildIndex:      childIndex,
		Attempt:         1,
		Label:           req.Label,
		PromptDigest:    e.childValues.TextDigest(req.Prompt),
		Preset:          req.Preset,
		ModelProvider:   req.ModelProvider,
		Model:           req.Model,
		ReasoningEffort: req.ReasoningEffort,
		ResourceID:      req.ResourceID,
		FactoryRevision: req.FactoryRevision,
		SkipPermissions: skipPermissions,
		Command:         req.Command,
		Sandbox:         req.Sandbox,
		SchemaDigest:    e.childValues.SchemaDigest(req.OutputSchema),
		RunnerID:        runnerID,
		ExecutionMode:   factory.JavaScriptChildExecutionModeLive,
		ArtifactRef:     artifactRef,
	}

	// QUEUED and RUNNING are appended here because this composition has no
	// Worker Session to emit them. The converged path deliberately does not:
	// there, they are the Worker Session's own lifecycle records.
	e.records.AppendChildDispatch(base, factory.JavaScriptChildDispatchStatusQueued)
	e.records.AppendChildDispatch(base, factory.JavaScriptChildDispatchStatusRunning)

	for attemptNumber := 1; attemptNumber <= e.maxAttempts; attemptNumber++ {
		executeRequest := e.executeRequest(req, base, dispatchID, runnerID, attemptNumber)
		base.Attempt = executeRequest.Attempt.Number
		result, err := executeChildAttempt(ctx, e.execute, executeRequest)
		result = normalizeChildStructuredResult(req, result)
		if childExecutionShouldRetry(ctx, result, err, attemptNumber, e.maxAttempts) {
			continue
		}
		if err != nil || !childExecutionSucceeded(result.Outcome) {
			return e.failedChild(base, req, dispatchID, childIndex, result, err)
		}

		providerName, providerSessionRef := childProviderSession(result)
		output, schemaValidated := childWorkerOutputFromExecute(req, result)
		completed := base
		completed.Status = factory.JavaScriptChildDispatchStatusCompleted
		completed.Provider = providerName
		completed.ProviderSessionRef = providerSessionRef
		completed.Output = e.childValues.CloneOutputMap(output)
		completed.SchemaValidated = schemaValidated
		e.records.Append(factory.JavaScriptRuntimeRecord{
			Kind:          factory.JavaScriptRecordKindChildDispatch,
			ChildDispatch: &completed,
		})
		return factory.JavaScriptChildExecutionResult{
			DispatchID:         dispatchID,
			ChildIndex:         childIndex,
			Status:             factory.JavaScriptChildDispatchStatusCompleted,
			ExecutionMode:      factory.JavaScriptChildExecutionModeLive,
			Output:             output,
			SchemaValidated:    schemaValidated,
			SchemaDigest:       base.SchemaDigest,
			ArtifactRef:        artifactRef,
			ProviderSessionRef: providerSessionRef,
			Request:            req,
		}, nil
	}
	return factory.JavaScriptChildExecutionResult{}, fmt.Errorf("javascript child execution exhausted its attempt budget")
}

func (e *directChildExecutor) executeRequest(
	req factory.JavaScriptChildExecutionRequest,
	base factory.JavaScriptChildDispatchRecord,
	dispatchID string,
	runnerID string,
	attemptNumber int,
) workers.ExecuteRequest {
	requestID := firstNonBlank(strings.TrimSpace(e.sessionID), "standalone-child")
	traceID := dispatchID
	return workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: requestID,
			RuntimeID:        "standalone-runtime-" + requestID,
			GenerationID:     "standalone-generation-" + requestID,
			DispatchID:       dispatchID,
			AttemptID:        fmt.Sprintf("%s/attempt/%d", dispatchID, attemptNumber),
			RequestID:        requestID,
			TraceID:          traceID,
		},
		Target: workers.ExecutionTarget{
			WorkerName:       req.Preset,
			WorkerType:       req.Preset,
			WorkstationName:  workers.ProviderInvocationRoute,
			RunnerID:         runnerID,
			ExecutorProvider: strings.TrimSpace(req.ExecutorProvider),
			Command:          strings.TrimSpace(req.Command),
			FactoryDirectory: e.workingDir,
			Provider: workers.ProviderReference{
				ID:    strings.TrimSpace(req.ModelProvider),
				Alias: strings.TrimSpace(req.ModelProvider),
			},
			Model: workers.ModelReference{
				Name:            strings.TrimSpace(req.Model),
				Provider:        strings.TrimSpace(req.ModelProvider),
				ReasoningEffort: strings.TrimSpace(req.ReasoningEffort),
			},
			Prompt: workers.PromptPolicy{
				UserMessage:  req.Prompt,
				OutputSchema: childOutputSchemaJSON(req.OutputSchema),
			},
			Environment: workers.EnvironmentPolicy{
				WorkingDirectory:    e.workingDir,
				WorkingDirectorySet: e.workingDir != "",
			},
			Workspace: workers.WorkspacePolicy{
				WorkingDirectory: e.workingDir,
				FactoryDirectory: e.workingDir,
			},
			Permissions: workers.PermissionPolicy{SkipPermissions: effectiveChildSkipPermissions(req)},
			Timeout:     childAttemptTimeout(req, runnerID, e.maxWorkerDuration),
		},
		Input: workers.ExecutionInput{
			Dispatch: work.WorkDispatch{
				DispatchID:      dispatchID,
				WorkerType:      strings.TrimSpace(req.Label),
				WorkstationName: workers.ProviderInvocationRoute,
				Execution: work.ExecutionMetadata{
					RequestID: requestID,
					TraceID:   traceID,
				},
			},
			WorkflowContext: &workers.Context{
				FactoryDirectory: e.workingDir,
				WorkDirectory:    e.workingDir,
				SessionID:        requestID,
			},
		},
		Attempt: workers.AttemptContext{Number: attemptNumber},
	}
}

func effectiveChildSkipPermissions(req factory.JavaScriptChildExecutionRequest) bool {
	if req.Permissions != "" {
		return req.Permissions == factory.JavaScriptChildPermissionSkipPermissions
	}
	return req.SkipPermissions
}

func childRunnerID(executorProvider, modelProvider string) (string, error) {
	executorProvider = strings.TrimSpace(executorProvider)
	modelProvider = strings.TrimSpace(modelProvider)
	if strings.EqualFold(executorProvider, workers.ExecutorProviderACP) {
		if modelProvider == "" {
			return "", fmt.Errorf("executorProvider ACP requires modelProvider to name an ACP integration")
		}
		return modelProvider, nil
	}
	if executorProvider != "" && !strings.EqualFold(executorProvider, "SCRIPT_WRAP") {
		return executorProvider, nil
	}
	if modelProvider != "" {
		return modelProvider, nil
	}
	// The legacy Factory Runtime selection resolved an otherwise unspecified
	// provider child to the Codex runner. Preserve that public default before
	// the detached Workers request is validated; an injected provider edge may
	// intentionally have no catalog identity of its own.
	return workers.RunnerIDCodex, nil
}

// failedChild preserves the Workers-owned failure classification on the record,
// which is what dispatch inspection reads to explain why a child failed.
func (e *directChildExecutor) failedChild(
	base factory.JavaScriptChildDispatchRecord,
	req factory.JavaScriptChildExecutionRequest,
	dispatchID string,
	childIndex int,
	result workers.ExecuteResult,
	executeErr error,
) (factory.JavaScriptChildExecutionResult, error) {
	result = normalizeChildExecuteFailure(result, executeErr)
	diagnostic := childFailureDiagnostic(result, executeErr, req)
	providerName, providerSessionRef := childProviderSession(result)
	if providerName == "" {
		providerName = childProviderName(result)
	}
	if providerName == "" {
		providerName = canonicalChildProvider(req.ModelProvider)
	}
	failed := base
	failed.Status = factory.JavaScriptChildDispatchStatusFailed
	failed.Provider = providerName
	failed.ProviderSessionRef = providerSessionRef
	if result.Failure != nil {
		failed.FailureClassification = result.Failure.Type
		failed.FailureDetail = workers.CloneFailureDetail(result.Failure.Detail)
		if failed.FailureDetail == nil {
			failed.FailureDetail = &workers.FailureDetail{
				Reason:  result.Failure.Type,
				Message: childFailureMessage(result, executeErr),
			}
		}
		retryable := result.Failure.RetryHint
		failed.Retryable = &retryable
	}
	e.records.Append(factory.JavaScriptRuntimeRecord{
		Kind:          factory.JavaScriptRecordKindChildDispatch,
		ChildDispatch: &failed,
	})
	return factory.JavaScriptChildExecutionResult{
		DispatchID:         dispatchID,
		ChildIndex:         childIndex,
		Status:             factory.JavaScriptChildDispatchStatusFailed,
		ExecutionMode:      factory.JavaScriptChildExecutionModeLive,
		Diagnostic:         diagnostic,
		ProviderSessionRef: providerSessionRef,
		Request:            req,
	}, fmt.Errorf("%s", diagnostic)
}

func (e *directChildExecutor) childDispatchIdentity(
	req factory.JavaScriptChildExecutionRequest,
) (string, int) {
	if req.ReservedIdentity != nil {
		return req.ReservedIdentity.DispatchID, req.ReservedIdentity.ChildIndex
	}
	return e.records.NextChildDispatchIdentity()
}
func (s *JavaScriptRuntimeService) childExecutorHooks(mode, sessionID string) factory.JavaScriptRuntimeHooks {
	hooks := factory.JavaScriptRuntimeHooks{
		OnRecord: func(record factory.JavaScriptRuntimeRecord) {
			s.applyRunningRuntimeRecord(sessionID, record)
		},
	}
	if mode != ChildExecutorModeLive {
		return hooks
	}
	hooks.NewChildExecutor = func(childSessionID string, records factory.JavaScriptChildRecordSink, policy factory.JavaScriptPolicy) factory.JavaScriptChildExecutor {
		// Which executor serves a session is decided by which composition built
		// this service, not by anything on the request. A runtime-backed session
		// invokes its children as Workers through the already-composed Execute
		// capability; the standalone `you run script.js` composition builds no
		// runtime and reaches the same Execute capability directly.
		if binding := s.workerExecutionBinding(); binding != nil {
			executor := newChildWorkerExecutor(
				childSessionID,
				binding.execute,
				records,
				s.childValues,
				s.observeWorkerDispatch,
				s.projectRoot,
				policy.MaxRetries,
			)
			executor.maxWorkerDuration = childWorkerDurationFromPolicy(policy)
			executor.resourceLeaseAcquirer = binding.resourceLeaseAcquirer
			executor.runtimeID = binding.runtimeID
			executor.generationID = binding.generationID
			executor.providerOverride = binding.providerOverride
			executor.mockWorkers = binding.mockWorkers.Clone()
			executor.commandRunnerOverride = binding.commandRunnerOverride
			executor.attemptStarter = binding.attemptStarter
			executor.publish = binding.publish
			return executor
		}
		if execution := s.directWorkerExecution(); execution != nil {
			executor := newDirectChildExecutor(
				childSessionID,
				execution,
				records,
				s.childValues,
				s.projectRoot,
				policy.MaxRetries,
			)
			executor.maxWorkerDuration = childWorkerDurationFromPolicy(policy)
			return executor
		}
		// Compatibility is retained for in-package callers that have not yet
		// moved to the standalone Workers binding. It is not part of the Wire
		// production path and is the P6-C retirement survivor.
		if s.directChildInvocation == nil {
			return missingChildExecutor{sessionID: childSessionID}
		}
		executor := newDirectChildExecutor(
			childSessionID,
			legacyDirectChildExecution{invocation: s.directChildInvocation},
			records,
			s.childValues,
			s.projectRoot,
			policy.MaxRetries,
		)
		executor.maxWorkerDuration = childWorkerDurationFromPolicy(policy)
		return executor
	}
	return hooks
}

// childWorkerProgressBridge keeps provider-owned terminal fragments from
// racing the canonical Execute result. Provider runners may publish a
// STREAM_COMPLETED/STREAM_FAILED fragment before Execute returns, while a
// plain provider, model, harness, cancellation, or timeout may publish none.
// Buffering that one fragment lets the child publish exactly one final
// response outcome after the retry budget and Execute result are known.
type childWorkerProgressBridge struct {
	mu         sync.Mutex
	publish    childWorkerProgressPublisher
	dispatchID string
	pending    *workers.ProgressFragment
	terminal   bool
}

func newChildWorkerProgressBridge(
	publish childWorkerProgressPublisher,
	dispatchID string,
) *childWorkerProgressBridge {
	return &childWorkerProgressBridge{
		publish:    publish,
		dispatchID: dispatchID,
	}
}

func (b *childWorkerProgressBridge) publishProgress(fragment workers.ProgressFragment) {
	if b == nil || b.publish == nil {
		return
	}
	fragment = b.normalize(fragment)
	b.mu.Lock()
	if b.terminal {
		b.mu.Unlock()
		return
	}
	if isChildTerminalProgress(fragment) {
		b.pending = &fragment
		b.mu.Unlock()
		return
	}
	publish := b.publish
	dispatchID := b.dispatchID
	b.mu.Unlock()
	publish(dispatchID, fragment)
}

func (b *childWorkerProgressBridge) resetAttempt() {
	if b == nil {
		return
	}
	b.mu.Lock()
	if b.terminal {
		b.mu.Unlock()
		return
	}
	b.pending = nil
	b.mu.Unlock()
}

func (b *childWorkerProgressBridge) publishTerminal(
	result workers.ExecuteResult,
	executeErr error,
) {
	if b == nil || b.publish == nil {
		return
	}
	b.mu.Lock()
	if b.terminal {
		b.mu.Unlock()
		return
	}
	b.terminal = true
	pending := b.pending
	b.pending = nil
	b.mu.Unlock()
	if pending != nil && childTerminalProgressMatches(*pending, result, executeErr) {
		b.publish(b.dispatchID, *pending)
		return
	}
	b.publish(b.dispatchID, childTerminalProgressFromResult(b.dispatchID, result, executeErr))
}

func (b *childWorkerProgressBridge) normalize(fragment workers.ProgressFragment) workers.ProgressFragment {
	if strings.TrimSpace(fragment.DispatchID) == "" {
		fragment.DispatchID = b.dispatchID
	}
	if strings.TrimSpace(fragment.Correlation.DispatchID) == "" {
		fragment.Correlation.DispatchID = b.dispatchID
	}
	return fragment
}

func isChildTerminalProgress(fragment workers.ProgressFragment) bool {
	return fragment.Kind == workers.CompletedFragmentKind || fragment.Kind == workers.FailedFragmentKind
}

func childTerminalProgressMatches(
	fragment workers.ProgressFragment,
	result workers.ExecuteResult,
	executeErr error,
) bool {
	switch fragment.Kind {
	case workers.CompletedFragmentKind:
		return executeErr == nil && childExecutionSucceeded(result.Outcome)
	case workers.FailedFragmentKind:
		// A provider's failed fragment can be less specific than the
		// canonical Execute result (for example STREAM_FAILED versus a typed
		// timeout or cancellation). Always synthesize the canonical terminal
		// for unhappy outcomes so the durable response keeps that classification.
		return false
	default:
		return false
	}
}

func childTerminalProgressFromResult(
	dispatchID string,
	result workers.ExecuteResult,
	executeErr error,
) workers.ProgressFragment {
	fragment := workers.ProgressFragment{
		DispatchID:   dispatchID,
		Correlation:  result.Correlation,
		Provider:     childProviderName(result),
		Continuation: (result.Continuation).ClonePtr(),
	}
	if strings.TrimSpace(fragment.Correlation.DispatchID) == "" {
		fragment.Correlation.DispatchID = dispatchID
	}
	fragment.Kind = workers.CompletedFragmentKind
	fragment.Type = "COMPLETED"
	fragment.ExternalEventType = "STREAM_COMPLETED"
	if !childExecutionSucceeded(result.Outcome) || executeErr != nil {
		fragment.Kind = workers.FailedFragmentKind
		fragment.Type = "FAILED"
		fragment.ExternalEventType = "STREAM_FAILED"
		if result.Outcome == workers.ExecutionOutcomeCanceled || errors.Is(executeErr, context.Canceled) {
			fragment.Type = "CANCELED"
		}
		fragment.Payload = childExecutionDiagnostic(result, executeErr)
		if result.Failure != nil {
			fragment.Metadata = map[string]string{
				"work_failure_type": string(result.Failure.Type),
				"retryable":         fmt.Sprintf("%t", result.Failure.RetryHint),
			}
		}
	}
	return fragment
}

func childProviderName(result workers.ExecuteResult) string {
	provider, _ := childProviderSession(result)
	if provider != "" {
		return provider
	}
	if result.Diagnostics != nil && result.Diagnostics.Provider != nil {
		return canonicalChildProvider(result.Diagnostics.Provider.Provider)
	}
	return ""
}

func childExecutionDiagnostic(result workers.ExecuteResult, executeErr error) string {
	return childFailureDiagnostic(result, executeErr, factory.JavaScriptChildExecutionRequest{})
}

// normalizeChildExecuteFailure keeps a typed Worker/Provider error typed when a
// child executor returns it as the operation error rather than embedding the
// failure on ExecuteResult. Only an error with no typed Worker detail takes the
// terminal/unknown fallback path.
func normalizeChildExecuteFailure(
	result workers.ExecuteResult,
	executeErr error,
) workers.ExecuteResult {
	if result.Failure != nil || executeErr == nil {
		return result
	}
	if providerErr := workers.NormalizeProviderExecutionError(executeErr); providerErr != nil {
		metadata := workers.WorkFailureMetadataFromProviderError(providerErr)
		decision := workers.FailureDecisionFromMetadata(metadata)
		failureType := workers.WorkFailureTypeUnknown
		family := workers.WorkFailureFamilyTerminal
		if metadata != nil {
			failureType = metadata.Type
			family = metadata.Family
		}
		message := strings.TrimSpace(providerErr.Message)
		if providerErr.ProviderFailureKind == "" {
			message = ""
		}
		if message == "" {
			message = childSafeFailureMessage(failureType)
		}
		result.Failure = &workers.ExecutionFailure{
			Type:                            failureType,
			Family:                          family,
			Message:                         message,
			RetryHint:                       decision.Retryable,
			ProviderFailureKind:             providerErr.ProviderFailureKind,
			ProviderContinuationFailureKind: providerErr.ProviderContinuationFailureKind,
			ProviderContinuationOutcome:     providerErr.ProviderContinuationOutcome,
			Detail:                          &workers.FailureDetail{Reason: failureType, Message: message},
		}
		if result.Diagnostics == nil {
			result.Diagnostics = providerErr.Diagnostics.ToSafeDiagnostics()
		}
		if result.Continuation == nil {
			result.Continuation = (providerErr.Continuation).ClonePtr()
		}
		return result
	}

	message := strings.TrimSpace(executeErr.Error())
	if message == "" {
		message = "Provider execution failed."
	}
	result.Failure = &workers.ExecutionFailure{
		Type:    workers.WorkFailureTypeUnknown,
		Family:  workers.WorkFailureFamilyTerminal,
		Message: message,
		Detail:  &workers.FailureDetail{Reason: workers.WorkFailureTypeUnknown, Message: message},
	}
	return result
}

func childFailureDiagnostic(
	result workers.ExecuteResult,
	executeErr error,
	req factory.JavaScriptChildExecutionRequest,
) string {
	message := childFailureMessage(result, executeErr)
	provider := childProviderName(result)
	if provider == "" {
		provider = canonicalChildProvider(req.ModelProvider)
	}
	if provider == "" && req.ExecutorProvider != "" &&
		!strings.EqualFold(strings.TrimSpace(req.ExecutorProvider), workers.ExecutorProviderACP) &&
		!strings.EqualFold(strings.TrimSpace(req.ExecutorProvider), "SCRIPT_WRAP") {
		provider = canonicalChildProvider(req.ExecutorProvider)
	}
	return formatChildProviderFailure(provider, message)
}

func childFailureMessage(result workers.ExecuteResult, executeErr error) string {
	message := ""
	if result.Failure != nil && result.Failure.Detail != nil {
		message = strings.TrimSpace(result.Failure.Detail.Message)
	}
	if message == "" && result.Failure != nil {
		message = strings.TrimSpace(result.Failure.Message)
	}
	if message == "" && executeErr != nil {
		message = strings.TrimSpace(executeErr.Error())
	}
	if message == "" {
		if result.Outcome == workers.ExecutionOutcomeCanceled {
			message = "Provider execution canceled."
		} else {
			message = "Provider execution failed."
		}
	}
	return message
}

func childSafeFailureMessage(reason workers.WorkFailureType) string {
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
	default:
		return "Provider execution failed."
	}
}

func canonicalChildProvider(provider string) string {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return ""
	}
	canonical := providers.ID(strings.ToLower(provider)).CanonicalSessionProvider()
	if canonical == "" {
		return strings.ToLower(provider)
	}
	return canonical
}

func formatChildProviderFailure(provider, message string) string {
	provider = canonicalChildProvider(provider)
	message = strings.TrimSpace(message)
	if provider == "" || message == "" {
		return message
	}
	display := provider
	if runes := []rune(display); len(runes) > 0 {
		display = strings.ToUpper(string(runes[0])) + string(runes[1:])
	}
	if strings.HasPrefix(strings.ToLower(message), strings.ToLower(display)+":") {
		return message
	}
	return fmt.Sprintf("%s: %s", display, message)
}
