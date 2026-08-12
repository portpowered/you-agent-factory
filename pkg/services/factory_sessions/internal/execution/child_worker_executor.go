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
	sessionID             string
	invoke                factory.Service
	records               factory.JavaScriptChildRecordSink
	childValues           factory.JavaScriptChildValues
	observe               workerDispatchObserver
	workingDir            string
	maxAttempts           int
	resourceLeaseAcquirer childResourceLeaseAcquirer
}

// childResourceLease is the execution-owned view of a resource lease. The
// runtime contract still owns the concrete lease; this narrow seam keeps the
// child executor's release behavior directly testable without constructing a
// Factory Runtime value from a Factory Sessions test.
type childResourceLease struct {
	factoryRevision int
	release         func()
}

func (lease *childResourceLease) Release() {
	if lease != nil && lease.release != nil {
		lease.release()
	}
}

type childResourceLeaseAcquirer func(context.Context, factory.ResourceCapacityLeaseRequest) (*childResourceLease, error)

// workerDispatchObserver claims one Workers dispatch identity for the session
// that started it, so the Worker's progress can be routed back to that
// session's own response-event store. The returned function releases the
// claim.
type workerDispatchObserver func(workerDispatchID, sessionID string) func()

func newChildWorkerExecutor(
	sessionID string,
	invoke factory.Service,
	records factory.JavaScriptChildRecordSink,
	childValues factory.JavaScriptChildValues,
	observe workerDispatchObserver,
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
		sessionID:             sessionID,
		invoke:                invoke,
		records:               records,
		childValues:           childValues,
		observe:               observe,
		workingDir:            strings.TrimSpace(workingDir),
		maxAttempts:           attempts,
		resourceLeaseAcquirer: nil,
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
	lease, err := e.acquireResourceLease(ctx, req)
	if err != nil {
		return factory.JavaScriptChildExecutionResult{}, err
	}
	if lease != nil {
		req.FactoryRevision = lease.factoryRevision
		defer lease.Release()
	}

	dispatchID, childIndex := e.childDispatchIdentity(req)
	base, err := e.openChild(req, dispatchID, childIndex)
	if err != nil {
		return factory.JavaScriptChildExecutionResult{}, err
	}

	// The Workers dispatch identity is scoped to this session. Every durable
	// session of one Factory shares that Factory's Workers pool, and a pool
	// treats a dispatch ID as single-use, so two sessions both running their
	// own "dispatch-1" would collide: the second would be refused outright, and
	// its Worker's progress could not be told apart from the first's. The
	// session's own records keep the unqualified identity, which is the one its
	// customer sees.
	workerDispatchID := e.workerDispatchIdentity(dispatchID)
	releaseWorker := e.observeWorker(workerDispatchID)
	defer releaseWorker()

	invoked, err := e.invoke.InvokeWorker(ctx, factory.InvokeWorkerRequest{
		DispatchID:       workerDispatchID,
		WorkerName:       req.Preset,
		Prompt:           req.Prompt,
		Model:            req.Model,
		ModelProvider:    req.ModelProvider,
		ReasoningEffort:  req.ReasoningEffort,
		ExecutorProvider: req.ExecutorProvider,
		RunnerID:         base.RunnerID,
		OutputSchema:     childOutputSchemaJSON(req.OutputSchema),
		WorkingDirectory: e.workingDir,
		SkipPermissions:  req.SkipPermissions,
		MaxAttempts:      e.maxAttempts,
	})
	if err != nil {
		return e.failedChild(base, req, dispatchID, childIndex, factory.InvokeWorkerResult{Diagnostic: err.Error()})
	}
	base.Attempt = invoked.Attempts
	if invoked.Outcome != factory.InvokeWorkerOutcomeCompleted {
		return e.failedChild(base, req, dispatchID, childIndex, invoked)
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
		ArtifactRef:        base.ArtifactRef,
		ProviderSessionRef: invoked.ProviderSessionRef,
		Request:            req,
	}, nil
}

func (e *childWorkerExecutor) acquireResourceLease(
	ctx context.Context,
	req factory.JavaScriptChildExecutionRequest,
) (*childResourceLease, error) {
	resourceID := strings.TrimSpace(req.ResourceID)
	if resourceID == "" {
		return nil, nil
	}
	request := factory.ResourceCapacityLeaseRequest{ResourceID: resourceID}
	if e.resourceLeaseAcquirer != nil {
		return e.resourceLeaseAcquirer(ctx, request)
	}
	admission, ok := e.invoke.(factory.ResourceCapacityLeaseAdmission)
	if !ok {
		return nil, fmt.Errorf(
			"javascript child resource %q requires Factory Runtime resource lease admission",
			resourceID,
		)
	}
	lease, err := admission.AcquireResourceCapacityLease(ctx, request)
	if err != nil {
		return nil, err
	}
	if lease == nil {
		return nil, nil
	}
	return &childResourceLease{
		factoryRevision: lease.FactoryRevision,
		release:         lease.Release,
	}, nil
}

// openChild resolves everything the session records about one child before its
// Worker runs, and commits the queued and running dispatch facts.
//
// The runner is resolved here, by the caller, because that is the whole premise
// of the provider-invocation route: no workstation definition exists downstream
// to resolve it from. A child names its runner through executorProvider, or
// falls back to the command it asked for and then to its model provider, which
// is how a workflow written as agent.run({modelProvider: "codex"}) selects one.
func (e *childWorkerExecutor) openChild(
	req factory.JavaScriptChildExecutionRequest,
	dispatchID string,
	childIndex int,
) (factory.JavaScriptChildDispatchRecord, error) {
	runnerID, err := workers.RunnerIdentityForWorker(req.ExecutorProvider, req.ModelProvider)
	if err != nil {
		return factory.JavaScriptChildDispatchRecord{}, err
	}
	if runnerID == "" {
		runnerID = firstNonBlank(req.Command, req.ModelProvider)
	}
	artifactID := e.records.NextChildArtifactID()

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
		ResourceID:      req.ResourceID,
		FactoryRevision: req.FactoryRevision,
		SkipPermissions: req.SkipPermissions,
		Command:         req.Command,
		Sandbox:         req.Sandbox,
		SchemaDigest:    e.childValues.SchemaDigest(req.OutputSchema),
		ExecutionMode:   factory.JavaScriptChildExecutionModeLive,
		ArtifactRef:     factory.FormatArtifactURI(e.sessionID, artifactID),
	}

	// These are the durable session's own dispatch-projection records, not
	// Worker Session lifecycle records: they are what the session's progress
	// counts and dispatch inspection read, and nothing else writes them.
	e.records.AppendChildDispatch(base, factory.JavaScriptChildDispatchStatusQueued)
	e.records.AppendChildDispatch(base, factory.JavaScriptChildDispatchStatusRunning)
	return base, nil
}

func (e *childWorkerExecutor) failedChild(
	base factory.JavaScriptChildDispatchRecord,
	req factory.JavaScriptChildExecutionRequest,
	dispatchID string,
	childIndex int,
	invoked factory.InvokeWorkerResult,
) (factory.JavaScriptChildExecutionResult, error) {
	diagnostic := strings.TrimSpace(invoked.Diagnostic)
	if diagnostic == "" {
		diagnostic = "Provider execution failed."
	}
	providerSessionRef := invoked.ProviderSessionRef
	failed := base
	failed.Status = factory.JavaScriptChildDispatchStatusFailed
	// The provider travels with its session reference or not at all. A
	// reference on its own is rejected when the session's runtime facts are
	// mapped to canonical events, which fails the whole execution rather than
	// the child -- so a failed Worker would surface as an internal error
	// instead of a failed session.
	failed.Provider = invoked.Provider
	failed.ProviderSessionRef = providerSessionRef
	failed.Retryable = invoked.Retryable
	if reason := strings.TrimSpace(invoked.FailureReason); reason != "" {
		failed.FailureClassification = workers.WorkFailureType(reason)
		failed.FailureDetail = &workers.FailureDetail{
			Reason:  workers.WorkFailureType(reason),
			Message: diagnostic,
		}
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

// workerDispatchIdentity qualifies one child's dispatch identity with the
// session that owns it. A session with no identity of its own -- which only a
// test composes -- keeps the child identity unchanged.
func (e *childWorkerExecutor) workerDispatchIdentity(dispatchID string) string {
	sessionID := strings.TrimSpace(e.sessionID)
	if sessionID == "" {
		return dispatchID
	}
	return sessionID + "/" + dispatchID
}

func (e *childWorkerExecutor) observeWorker(workerDispatchID string) func() {
	if e.observe == nil {
		return func() {}
	}
	return e.observe(workerDispatchID, e.sessionID)
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
