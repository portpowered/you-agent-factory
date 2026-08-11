package factorysessionexecution

import (
	"context"
	"strings"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// directChildExecutor runs one JavaScript workflow child straight against the
// provider, for the one composition that has no Factory Runtime behind it: the
// standalone `you run script.js` path, which opens a JavaScript execution
// service directly and never builds a runtime, Worker Sessions service, or
// event ledger.
//
// This is not a fallback and nothing selects it at dispatch time. The two
// executors are chosen once, at construction, by which composition is being
// built -- childWorkerExecutor where a runtime exists, this where none does --
// so a child can never take a second route out of the converged one.
//
// It performs exactly one attempt. Retry and the dispatch/Worker-Session
// association belong to Worker Sessions, and a composition with no Worker
// Sessions gets neither rather than a private re-implementation of them -- the
// duplicated retry loop this replaces is the defect the convergence removed.
// It does still write the queued/running/terminal dispatch records itself,
// because nothing else here can. Children run through this executor are
// invisible to a client: there is no Factory whose tool call they could be
// content inside.
type directChildExecutor struct {
	sessionID   string
	invocation  workers.InvocationExecutor
	records     factory.JavaScriptChildRecordSink
	childValues factory.JavaScriptChildValues
	workingDir  string
}

func newDirectChildExecutor(
	sessionID string,
	invocation workers.InvocationExecutor,
	records factory.JavaScriptChildRecordSink,
	childValues factory.JavaScriptChildValues,
	workingDir string,
) *directChildExecutor {
	return &directChildExecutor{
		sessionID:   sessionID,
		invocation:  invocation,
		records:     records,
		childValues: childValues,
		workingDir:  strings.TrimSpace(workingDir),
	}
}

func (e *directChildExecutor) Execute(
	ctx context.Context,
	req factory.JavaScriptChildExecutionRequest,
) (factory.JavaScriptChildExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return factory.JavaScriptChildExecutionResult{}, err
	}
	if e == nil || e.invocation == nil {
		return factory.JavaScriptChildExecutionResult{}, NewValidationError(
			"runtime.childExecutorMode",
			"worker invocation executor is required for live child execution",
		)
	}

	dispatchID, childIndex := e.childDispatchIdentity(req)
	runnerID, err := workers.RunnerIdentityForWorker(req.ExecutorProvider, req.ModelProvider)
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

	result, err := e.invocation.Execute(ctx, workers.InvocationInput{
		Request: e.inferenceRequest(req, dispatchID, runnerID),
		Attempt: 1,
	})
	providerName, providerSessionRef := directChildProviderFields(result)
	if err != nil {
		return e.failedChild(base, req, dispatchID, childIndex, providerName, providerSessionRef, result, err)
	}

	output := childWorkerOutput(req, result.Response.Content)
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

func (e *directChildExecutor) inferenceRequest(
	req factory.JavaScriptChildExecutionRequest,
	dispatchID string,
	runnerID string,
) workers.ProviderInferenceRequest {
	return workers.ProviderInferenceRequest{
		Dispatch: work.WorkDispatch{
			DispatchID: dispatchID,
			WorkerType: strings.TrimSpace(req.Label),
		},
		WorkerType:       strings.TrimSpace(req.Label),
		RunnerID:         runnerID,
		ExecutorProvider: strings.TrimSpace(req.ExecutorProvider),
		UserMessage:      req.Prompt,
		OutputSchema:     childOutputSchemaJSON(req.OutputSchema),
		Model:            strings.TrimSpace(req.Model),
		ModelProvider:    strings.TrimSpace(req.ModelProvider),
		ReasoningEffort:  strings.TrimSpace(req.ReasoningEffort),
		WorkingDirectory: e.workingDir,
		SkipPermissions:  req.SkipPermissions,
	}
}

func directChildProviderFields(result workers.InvocationResult) (string, string) {
	if result.ProviderSession == nil {
		return "", ""
	}
	name := workers.CanonicalProviderSessionProvider(result.ProviderSession.Provider)
	if name == "" {
		name = strings.TrimSpace(result.ProviderSession.Provider)
	}
	return name, strings.TrimSpace(result.ProviderSession.ID)
}

// failedChild preserves the Workers-owned failure classification on the record,
// which is what dispatch inspection reads to explain why a child failed.
func (e *directChildExecutor) failedChild(
	base factory.JavaScriptChildDispatchRecord,
	req factory.JavaScriptChildExecutionRequest,
	dispatchID string,
	childIndex int,
	providerName string,
	providerSessionRef string,
	result workers.InvocationResult,
	cause error,
) (factory.JavaScriptChildExecutionResult, error) {
	diagnostic := "Provider execution failed."
	if result.FailureDetail != nil && strings.TrimSpace(result.FailureDetail.Message) != "" {
		diagnostic = result.FailureDetail.Message
	}
	failed := base
	failed.Status = factory.JavaScriptChildDispatchStatusFailed
	failed.Provider = providerName
	failed.ProviderSessionRef = providerSessionRef
	failed.FailureDetail = result.FailureDetail
	if result.FailureDetail != nil {
		failed.FailureClassification = result.FailureDetail.Reason
	}
	if result.FailureDecision != nil {
		failed.Retryable = &result.FailureDecision.Retryable
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
	}, cause
}

func (e *directChildExecutor) childDispatchIdentity(
	req factory.JavaScriptChildExecutionRequest,
) (string, int) {
	if req.ReservedIdentity != nil {
		return req.ReservedIdentity.DispatchID, req.ReservedIdentity.ChildIndex
	}
	return e.records.NextChildDispatchIdentity()
}
