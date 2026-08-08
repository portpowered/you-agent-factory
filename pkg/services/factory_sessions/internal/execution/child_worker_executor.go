package factorysessionexecution

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// WorkerInvokerResolver resolves the Factory Runtime that owns one session's
// Worker Sessions service and canonical event ledger.
//
// It is late-bound rather than a constructor argument because of ordering, not
// availability: the live-session registry that can resolve a session lives on
// the Factory Sessions service, and this execution service is a dependency of
// that service. Factory Runtime already solves the identical problem the same
// way with Root.BindActiveService.
//
// It is an alias rather than a defined type on purpose: the durable-execution
// wrapper and the runtime opener both reach this binder through structural
// interface assertions, and a defined type would make those method sets
// disagree with an identically-shaped declaration made anywhere else.
type WorkerInvokerResolver = func(sessionID string) factory.Service

// BindWorkerInvoker attaches the resolver JavaScript children invoke their
// Workers through. Binding nothing leaves children unable to run, which is
// deliberate: there is no second execution route to fall back to.
func (s *JavaScriptRuntimeService) BindWorkerInvoker(resolve WorkerInvokerResolver) {
	if s == nil {
		return
	}
	s.invokerMu.Lock()
	s.resolveWorkerInvoker = resolve
	s.invokerMu.Unlock()
}

// workerInvokerBound reports whether this service was composed with a Factory
// Runtime behind it, which is what makes its children Workers.
func (s *JavaScriptRuntimeService) workerInvokerBound() bool {
	if s == nil {
		return false
	}
	s.invokerMu.RLock()
	defer s.invokerMu.RUnlock()
	return s.resolveWorkerInvoker != nil
}

func (s *JavaScriptRuntimeService) workerInvoker(sessionID string) factory.Service {
	if s == nil {
		return nil
	}
	s.invokerMu.RLock()
	resolve := s.resolveWorkerInvoker
	s.invokerMu.RUnlock()
	if resolve == nil {
		return nil
	}
	return resolve(sessionID)
}

// childWorkerExecutor runs one JavaScript workflow child as an ordinary Worker.
//
// It holds no executor, no provider, and no retry loop of its own. Everything a
// Worker needs -- identity, the dispatch/Worker Session association that makes
// it visible, the publication window its output streams through, controls, and
// retry -- belongs to Worker Sessions, reached through the runtime's InvokeWorker
// operation. What remains here is translation: a child spec in, a child result
// out.
type childWorkerExecutor struct {
	sessionID   string
	invoke      factory.Service
	records     factory.JavaScriptChildRecordSink
	childValues factory.JavaScriptChildValues
	workingDir  string
	maxAttempts int
}

func newChildWorkerExecutor(
	sessionID string,
	invoke factory.Service,
	records factory.JavaScriptChildRecordSink,
	childValues factory.JavaScriptChildValues,
	maxRetries int,
	workingDir string,
) *childWorkerExecutor {
	// MaxRetries counts retries after the first attempt; Worker Sessions bounds
	// total attempts, so the budget is one greater.
	attempts := maxRetries + 1
	if attempts < 1 {
		attempts = 1
	}
	return &childWorkerExecutor{
		sessionID:   sessionID,
		invoke:      invoke,
		records:     records,
		childValues: childValues,
		workingDir:  strings.TrimSpace(workingDir),
		maxAttempts: attempts,
	}
}

// Execute invokes one child Worker and records its terminal dispatch fact.
//
// The queued/running/terminal dispatch records written here are the durable
// session's own projection, which is why they survive the convergence: the
// Worker Session's lifecycle records live on the Worker topic and feed the
// transport, not the session's dispatch counts. What did not survive is the
// retry loop that used to wrap them.
func (e *childWorkerExecutor) Execute(
	ctx context.Context,
	req factory.JavaScriptChildExecutionRequest,
) (factory.JavaScriptChildExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return factory.JavaScriptChildExecutionResult{}, err
	}
	if e == nil || e.invoke == nil {
		return factory.JavaScriptChildExecutionResult{}, fmt.Errorf(
			"javascript child execution requires a Factory Runtime worker invoker",
		)
	}

	dispatchID, childIndex := e.childDispatchIdentity(req)
	// The runner is resolved here, by the caller, because that is the whole
	// premise of the provider-invocation route: no workstation definition exists
	// downstream to resolve it from. A child names its runner through
	// executorProvider, or falls back to the command it asked for and then to
	// its model provider, which is how a workflow written as
	// agent.run({modelProvider: "codex"}) selects one.
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
		RunnerID:        runnerID,
		DispatchID:      dispatchID,
		ChildIndex:      childIndex,
		Attempt:         1,
		Label:           req.Label,
		PromptDigest:    e.childValues.TextDigest(req.Prompt),
		Preset:          req.Preset,
		ModelProvider:   req.ModelProvider,
		Model:           req.Model,
		ReasoningEffort: req.ReasoningEffort,
		SkipPermissions: req.SkipPermissions,
		Command:         req.Command,
		Sandbox:         req.Sandbox,
		SchemaDigest:    e.childValues.SchemaDigest(req.OutputSchema),
		ExecutionMode:   factory.JavaScriptChildExecutionModeLive,
		ArtifactRef:     artifactRef,
	}

	// These are the durable session's own dispatch-projection records, not
	// Worker Session lifecycle records: they are what the session's progress
	// counts and dispatch inspection read, and nothing else writes them.
	e.records.AppendChildDispatch(base, factory.JavaScriptChildDispatchStatusQueued)
	e.records.AppendChildDispatch(base, factory.JavaScriptChildDispatchStatusRunning)

	invoked, err := e.invoke.InvokeWorker(ctx, factory.InvokeWorkerRequest{
		DispatchID:       dispatchID,
		Label:            req.Label,
		Prompt:           req.Prompt,
		Model:            req.Model,
		ModelProvider:    req.ModelProvider,
		ReasoningEffort:  req.ReasoningEffort,
		ExecutorProvider: req.ExecutorProvider,
		RunnerID:         runnerID,
		OutputSchema:     childOutputSchemaJSON(req.OutputSchema),
		WorkingDirectory: e.workingDir,
		SkipPermissions:  req.SkipPermissions,
		MaxAttempts:      e.maxAttempts,
	})
	if err != nil {
		return e.failedChild(base, req, dispatchID, childIndex, "", err.Error())
	}
	base.Attempt = invoked.Attempts
	if invoked.Outcome != factory.InvokeWorkerOutcomeCompleted {
		return e.failedChild(base, req, dispatchID, childIndex, invoked.ProviderSessionRef, invoked.Diagnostic)
	}

	output := childWorkerOutput(req, invoked.Output)
	completed := base
	completed.Status = factory.JavaScriptChildDispatchStatusCompleted
	completed.Provider = invoked.Provider
	completed.ProviderSessionRef = invoked.ProviderSessionRef
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
		ProviderSessionRef: invoked.ProviderSessionRef,
		Request:            req,
	}, nil
}

func (e *childWorkerExecutor) failedChild(
	base factory.JavaScriptChildDispatchRecord,
	req factory.JavaScriptChildExecutionRequest,
	dispatchID string,
	childIndex int,
	providerSessionRef string,
	diagnostic string,
) (factory.JavaScriptChildExecutionResult, error) {
	if strings.TrimSpace(diagnostic) == "" {
		diagnostic = "Provider execution failed."
	}
	failed := base
	failed.Status = factory.JavaScriptChildDispatchStatusFailed
	failed.ProviderSessionRef = providerSessionRef
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

func (e *childWorkerExecutor) childDispatchIdentity(
	req factory.JavaScriptChildExecutionRequest,
) (string, int) {
	if req.ReservedIdentity != nil {
		return req.ReservedIdentity.DispatchID, req.ReservedIdentity.ChildIndex
	}
	return e.records.NextChildDispatchIdentity()
}

func childOutputSchemaJSON(schema map[string]any) string {
	if schema == nil {
		return ""
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func childWorkerOutput(req factory.JavaScriptChildExecutionRequest, content string) map[string]any {
	trimmed := strings.TrimSpace(content)
	if req.OutputSchema != nil && trimmed != "" {
		var decoded map[string]any
		if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
			return decoded
		}
	}
	return map[string]any{
		"text":            trimmed,
		"subject":         req.ArgsSubject,
		"schemaValidated": req.OutputSchema != nil,
	}
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
