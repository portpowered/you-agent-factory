package factorysessionexecution

import (
	"context"
	"fmt"
	"strings"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
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
// It performs exactly one detached Execute operation; Workers owns any
// provider retry within that operation. Retry and the dispatch/Worker-Session
// association do not become a private Sessions implementation in a
// composition with no Worker Sessions.
// It does still write the queued/running/terminal dispatch records itself,
// because nothing else here can. Children run through this executor are
// invisible to a client: there is no Factory whose tool call they could be
// content inside.
type directChildExecutor struct {
	sessionID   string
	execute     childExecuteService
	records     factory.JavaScriptChildRecordSink
	childValues factory.JavaScriptChildValues
	workingDir  string
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
) *directChildExecutor {
	return &directChildExecutor{
		sessionID:   sessionID,
		execute:     execution,
		records:     records,
		childValues: childValues,
		workingDir:  strings.TrimSpace(workingDir),
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
		SkipPermissions: req.SkipPermissions,
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

	executeRequest := e.executeRequest(req, base, dispatchID, runnerID)
	result, err := e.execute.Execute(ctx, executeRequest)
	if err != nil || !childExecutionSucceeded(result.Outcome) {
		return e.failedChild(base, req, dispatchID, childIndex, result, err)
	}

	base.Attempt = executeRequest.Attempt.Number
	providerName, providerSessionRef := childProviderSession(result)
	output := childWorkerOutputFromExecute(req, result)
	completed := base
	completed.Status = factory.JavaScriptChildDispatchStatusCompleted
	completed.Provider = providerName
	completed.ProviderSessionRef = providerSessionRef
	completed.Output = e.childValues.CloneOutputMap(output)
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
		ArtifactRef:        artifactRef,
		ProviderSessionRef: providerSessionRef,
		Request:            req,
	}, nil
}

func (e *directChildExecutor) executeRequest(
	req factory.JavaScriptChildExecutionRequest,
	base factory.JavaScriptChildDispatchRecord,
	dispatchID string,
	runnerID string,
) workers.ExecuteRequest {
	requestID := firstNonBlank(strings.TrimSpace(e.sessionID), "standalone-child")
	traceID := dispatchID
	return workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: requestID,
			RuntimeID:        "standalone-runtime-" + requestID,
			GenerationID:     "standalone-generation-" + requestID,
			DispatchID:       dispatchID,
			AttemptID:        dispatchID + "/attempt/1",
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
			Permissions: workers.PermissionPolicy{SkipPermissions: req.SkipPermissions},
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
		Attempt: workers.AttemptContext{Number: 1},
	}
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
	diagnostic := ""
	if result.Failure != nil {
		diagnostic = strings.TrimSpace(result.Failure.Message)
	}
	if diagnostic == "" && executeErr != nil {
		diagnostic = strings.TrimSpace(executeErr.Error())
	}
	if diagnostic == "" {
		diagnostic = "Provider execution failed."
	}
	providerName, providerSessionRef := childProviderSession(result)
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
				Message: diagnostic,
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
			return newDirectChildExecutor(
				childSessionID,
				execution,
				records,
				s.childValues,
				s.projectRoot,
			)
		}
		// Compatibility is retained for in-package callers that have not yet
		// moved to the standalone Workers binding. It is not part of the Wire
		// production path and is the P6-C retirement survivor.
		if s.directChildInvocation == nil {
			return missingChildExecutor{sessionID: childSessionID}
		}
		return newDirectChildExecutor(
			childSessionID,
			legacyDirectChildExecution{invocation: s.directChildInvocation},
			records,
			s.childValues,
			s.projectRoot,
		)
	}
	return hooks
}
