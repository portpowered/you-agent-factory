package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type WorkerExecution interface {
	Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)
}

type childExecuteService = WorkerExecution

// childTimeoutContextFactory is the package-private timeout boundary for one
// child attempt. Production uses childTimeoutContext, while package tests can
// release a controlled deadline without changing the production timeout policy.
type childTimeoutContextFactory func(context.Context, time.Duration) (context.Context, context.CancelFunc)

func childTimeoutContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

type childWorkerProgressPublisher func(string, workers.ProgressFragment)

type childWorkerExecutionBinding struct {
	execute               childExecuteService
	attemptStarter        childWorkerAttemptStarter
	resourceLeaseAcquirer childResourceLeaseAcquirer
	runtimeID             string
	generationID          string
	providerOverride      providers.Service
	mockWorkers           *workers.MockWorkersConfig
	commandRunnerOverride platformprocess.CommandRunner
	progressPublisher     workers.ProgressPublisher
	publish               childWorkerProgressPublisher
}

type childWorkerAttemptStarter func(
	context.Context,
	workers.ExecuteRequest,
) (func(context.Context, workers.ExecuteResult, error) error, error)

// SetWorkerInvoker attaches the Runtime capability used by durable live-change
// control. Child execution deliberately does not use this broad capability;
// it receives the narrow Workers Execute binding below.
func (s *JavaScriptRuntimeService) SetWorkerInvoker(runtime factory.Service) {
	if s == nil {
		return
	}
	s.invokerMu.Lock()
	s.workerInvokerService = runtime
	s.invokerMu.Unlock()
}

// SetDirectWorkerExecution attaches the already-composed Workers Execute
// operation to the standalone JavaScript composition. The standalone path has
// no Factory Runtime to contribute identity or capacity, but it still enters
// Workers through the same detached request/result boundary.
func (s *JavaScriptRuntimeService) SetDirectWorkerExecution(
	execution interface {
		Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)
	},
) {
	if s == nil {
		return
	}
	s.invokerMu.Lock()
	s.directChildExecution = execution
	s.invokerMu.Unlock()
}

// SetWorkerExecution attaches the already-composed Workers Execute operation
// to the durable child path. The binding is request-scoped at the Workers
// boundary: Sessions supplies detached identity and policy values, while
// Workers retains runner/provider ownership and terminal normalization.
func (s *JavaScriptRuntimeService) SetWorkerExecution(
	execution interface {
		Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)
	},
	admission factory.ResourceCapacityLeaseAdmission,
	runtimeID string,
	generationID string,
	providerOverride providers.Service,
	mockWorkers *workers.MockWorkersConfig,
	commandRunnerOverride platformprocess.CommandRunner,
) {
	if s == nil {
		return
	}
	binding := &childWorkerExecutionBinding{
		execute:               execution,
		runtimeID:             strings.TrimSpace(runtimeID),
		generationID:          strings.TrimSpace(generationID),
		providerOverride:      providerOverride,
		mockWorkers:           mockWorkers.Clone(),
		commandRunnerOverride: commandRunnerOverride,
	}
	if admission != nil {
		binding.resourceLeaseAcquirer = func(
			ctx context.Context,
			request factory.ResourceCapacityLeaseRequest,
		) (*childResourceLease, error) {
			lease, err := admission.AcquireResourceCapacityLease(ctx, request)
			if err != nil || lease == nil {
				return nil, err
			}
			return &childResourceLease{
				factoryRevision: lease.FactoryRevision,
				release:         lease.Release,
			}, nil
		}
	}
	// The publisher is intentionally installed on the request rather than on
	// the process Workers root. That keeps each child response stream scoped to
	// its owning durable session and prevents one child from observing another's
	// progress.
	binding.publish = func(workerDispatchID string, fragment workers.ProgressFragment) {
		if strings.TrimSpace(fragment.DispatchID) == "" {
			fragment.DispatchID = workerDispatchID
		}
		if strings.TrimSpace(fragment.Correlation.DispatchID) == "" {
			fragment.Correlation.DispatchID = workerDispatchID
		}
		s.invokerMu.RLock()
		progressPublisher := binding.progressPublisher
		s.invokerMu.RUnlock()
		if progressPublisher != nil {
			progressPublisher(fragment)
			return
		}
		s.PublishWorkerProgress(fragment)
	}
	s.invokerMu.Lock()
	if execution == nil {
		s.workerExecution = nil
	} else {
		s.workerExecution = binding
	}
	s.invokerMu.Unlock()
}

// SetWorkerProgressPublisher attaches the runtime-owned progress bridge to
// the already-bound child Execute operation. Runtime construction creates the
// bridge while it binds the session-owned Worker Sessions service, which is
// later than the durable execution service itself. Keeping this as a narrow
// optional bind preserves the existing Execute seam for standalone, replay,
// and test compositions.
func (s *JavaScriptRuntimeService) SetWorkerProgressPublisher(
	publisher workers.ProgressPublisher,
) {
	if s == nil {
		return
	}
	s.invokerMu.Lock()
	if s.workerExecution != nil {
		s.workerExecution.progressPublisher = publisher
	}
	s.invokerMu.Unlock()
}

// SetWorkerAttemptStarter attaches the Runtime-owned Worker Session opening
// boundary to the direct child Execute route. Runtime remains responsible for
// admission and execution; the returned completion callback only commits the
// durable Worker Session observation after Execute returns.
func (s *JavaScriptRuntimeService) SetWorkerAttemptStarter(
	starter func(context.Context, workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) error, error),
) {
	if s == nil {
		return
	}
	s.invokerMu.Lock()
	if s.workerExecution != nil {
		s.workerExecution.attemptStarter = starter
	}
	s.invokerMu.Unlock()
}

func (s *JavaScriptRuntimeService) workerInvoker() factory.Service {
	if s == nil {
		return nil
	}
	s.invokerMu.RLock()
	runtime := s.workerInvokerService
	s.invokerMu.RUnlock()
	return runtime
}

func (s *JavaScriptRuntimeService) workerExecutionBound() bool {
	return s.workerExecutionBinding() != nil
}

func (s *JavaScriptRuntimeService) workerExecutionBinding() *childWorkerExecutionBinding {
	if s == nil {
		return nil
	}
	s.invokerMu.RLock()
	binding := s.workerExecution
	s.invokerMu.RUnlock()
	if binding == nil || binding.execute == nil {
		return nil
	}
	return binding
}

func (s *JavaScriptRuntimeService) directWorkerExecution() childExecuteService {
	if s == nil {
		return nil
	}
	s.invokerMu.RLock()
	execution := s.directChildExecution
	s.invokerMu.RUnlock()
	return execution
}

// childWorkerExecutor runs one JavaScript workflow child as an ordinary Worker.
//
// It holds no executor, provider, runner registry, or Runtime root. Everything
// a Worker needs is translated into one detached ExecuteRequest and handed to
// the already-composed Workers operation. What remains here is the durable
// session projection: a child spec in, a child result out.
type childWorkerExecutor struct {
	sessionID             string
	execute               childExecuteService
	records               factory.JavaScriptChildRecordSink
	childValues           factory.JavaScriptChildValues
	observe               workerDispatchObserver
	publish               childWorkerProgressPublisher
	workingDir            string
	runtimeID             string
	generationID          string
	providerOverride      providers.Service
	mockWorkers           *workers.MockWorkersConfig
	commandRunnerOverride platformprocess.CommandRunner
	attemptStarter        childWorkerAttemptStarter
	maxAttempts           int
	resourceLeaseAcquirer childResourceLeaseAcquirer
	maxWorkerDuration     time.Duration
	timeoutContextFactory childTimeoutContextFactory
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
	execution childExecuteService,
	records factory.JavaScriptChildRecordSink,
	childValues factory.JavaScriptChildValues,
	observe workerDispatchObserver,
	workingDir string,
	maxRetries int,
) *childWorkerExecutor {
	attempts := maxRetries + 1
	if attempts < 1 {
		attempts = 1
	}
	return &childWorkerExecutor{
		sessionID:             sessionID,
		execute:               execution,
		records:               records,
		childValues:           childValues,
		observe:               observe,
		workingDir:            strings.TrimSpace(workingDir),
		maxAttempts:           attempts,
		resourceLeaseAcquirer: nil,
		timeoutContextFactory: childTimeoutContext,
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
	if e == nil || e.execute == nil {
		return factory.JavaScriptChildExecutionResult{}, fmt.Errorf(
			"javascript child execution requires the Workers Execute capability",
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

	var progress *childWorkerProgressBridge
	if e.publish != nil {
		progress = newChildWorkerProgressBridge(e.publish, workerDispatchID)
	}
	var completeAttempt func(context.Context, workers.ExecuteResult, error) error
	for attemptNumber := 1; attemptNumber <= e.maxAttempts; attemptNumber++ {
		request := e.executeRequest(req, base, workerDispatchID, attemptNumber, progress)
		base.Attempt = request.Attempt.Number
		var preStartResult workers.ExecuteResult
		if attemptNumber == 1 {
			completeAttempt, preStartResult, err = e.beginChildWorkerAttempt(ctx, request, progress)
			if err != nil {
				return e.failedChild(base, req, dispatchID, childIndex, preStartResult, err)
			}
		}
		invoked, err := e.executeChildAttempt(ctx, request)
		invoked = normalizeChildStructuredResult(req, invoked)
		if childExecutionShouldRetry(ctx, invoked, err, attemptNumber, e.maxAttempts) {
			if progress != nil {
				progress.resetAttempt()
			}
			continue
		}
		finishChildWorkerAttempt(completeAttempt, progress, invoked, err)
		if err != nil || !childExecutionSucceeded(invoked.Outcome) {
			return e.failedChild(base, req, dispatchID, childIndex, invoked, err)
		}
		return e.completedChild(base, req, dispatchID, childIndex, invoked)
	}
	return factory.JavaScriptChildExecutionResult{}, fmt.Errorf("javascript child execution exhausted its attempt budget")
}

func (e *childWorkerExecutor) executeChildAttempt(
	ctx context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	timeoutContextFactory := e.timeoutContextFactory
	if timeoutContextFactory == nil {
		timeoutContextFactory = childTimeoutContext
	}
	return executeChildAttemptWithFactory(ctx, timeoutContextFactory, e.execute, request)
}

func (e *childWorkerExecutor) beginChildWorkerAttempt(
	ctx context.Context,
	request workers.ExecuteRequest,
	progress *childWorkerProgressBridge,
) (func(context.Context, workers.ExecuteResult, error) error, workers.ExecuteResult, error) {
	if e == nil || e.attemptStarter == nil {
		return nil, workers.ExecuteResult{}, nil
	}
	complete, err := e.attemptStarter(context.WithoutCancel(ctx), request)
	if err == nil {
		return complete, workers.ExecuteResult{}, nil
	}
	result := failedChildWorkerExecuteResult(request, err)
	// A producer may have opened its lifecycle window before discovering a
	// preparation failure. If it returned a completion handle alongside that
	// error, close the window before returning the child error; dropping the
	// handle recreates the response-bridge wait with no terminal record.
	if complete != nil {
		_ = complete(context.Background(), result, err)
	}
	if progress != nil {
		progress.publishTerminal(result, err)
	}
	return nil, result, err
}

func failedChildWorkerExecuteResult(
	request workers.ExecuteRequest,
	err error,
) workers.ExecuteResult {
	return workers.ExecuteResult{
		Correlation: request.Correlation,
		Outcome:     workers.ExecutionOutcomeFailed,
		Failure: &workers.ExecutionFailure{
			Type:    workers.WorkFailureTypeUnknown,
			Family:  workers.WorkFailureFamilyTerminal,
			Message: err.Error(),
		},
	}
}

// executeChildAttempt keeps the JavaScript child boundary bounded even when a
// test, legacy adapter, or provider effect returns after its context has been
// canceled. The buffered result channel lets that late operation finish
// without retaining the child caller, while the caller publishes the timeout
// result exactly once at the deadline.
func executeChildAttempt(
	ctx context.Context,
	execute childExecuteService,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	return executeChildAttemptWithFactory(ctx, childTimeoutContext, execute, request)
}

func executeChildAttemptWithFactory(
	ctx context.Context,
	timeoutContextFactory childTimeoutContextFactory,
	execute childExecuteService,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if request.Target.Timeout <= 0 {
		return execute.Execute(ctx, request)
	}
	if timeoutContextFactory == nil {
		timeoutContextFactory = childTimeoutContext
	}
	attemptCtx, cancel := timeoutContextFactory(ctx, request.Target.Timeout)
	defer cancel()

	type completion struct {
		result workers.ExecuteResult
		err    error
	}
	completed := make(chan completion, 1)
	go func() {
		result, err := execute.Execute(attemptCtx, request)
		completed <- completion{result: result, err: err}
	}()

	for {
		select {
		case outcome := <-completed:
			// If the deadline won the race with a late successful result, the
			// timeout remains authoritative. A caller-owned deadline or
			// cancellation remains the caller's terminal outcome.
			if timedOutChildAttempt(attemptCtx, ctx, request.Target.Timeout) {
				return childTimeoutExecuteResult(request), context.DeadlineExceeded
			}
			return outcome.result, outcome.err
		case <-attemptCtx.Done():
			if timedOutChildAttempt(attemptCtx, ctx, request.Target.Timeout) {
				return childTimeoutExecuteResult(request), context.DeadlineExceeded
			}
			return workers.ExecuteResult{Correlation: request.Correlation}, attemptCtx.Err()
		}
	}
}

func timedOutChildAttempt(attemptCtx, parentCtx context.Context, timeout time.Duration) bool {
	return timeout > 0 &&
		errors.Is(attemptCtx.Err(), context.DeadlineExceeded) &&
		(parentCtx == nil || parentCtx.Err() == nil)
}

func childTimeoutExecuteResult(request workers.ExecuteRequest) workers.ExecuteResult {
	provider := childProviderForExecuteRequest(request)
	timeout := request.Target.Timeout
	message := "child execution timed out"
	if timeout > 0 {
		message = fmt.Sprintf("child execution timed out after %s", timeout)
	}
	return workers.ExecuteResult{
		Correlation: request.Correlation,
		Outcome:     workers.ExecutionOutcomeFailed,
		Failure: &workers.ExecutionFailure{
			Type:                workers.WorkFailureTypeTimeout,
			Family:              workers.WorkFailureFamilyTerminal,
			Message:             message,
			RetryHint:           false,
			ProviderFailureKind: providers.ExecuteFailureKindTimeout,
			Detail: &workers.FailureDetail{
				Reason:  workers.WorkFailureTypeTimeout,
				Message: message,
			},
		},
		Diagnostics: &workers.SafeDiagnostics{
			Provider: &workers.SafeProviderDiagnostic{Provider: provider},
		},
	}
}

func childProviderForExecuteRequest(request workers.ExecuteRequest) string {
	provider := firstNonBlank(
		request.Target.Model.Provider,
		request.Target.Provider.ID,
		request.Target.Provider.Alias,
	)
	if provider == "" &&
		!strings.EqualFold(strings.TrimSpace(request.Target.ExecutorProvider), workers.ExecutorProviderACP) &&
		!strings.EqualFold(strings.TrimSpace(request.Target.ExecutorProvider), "SCRIPT_WRAP") {
		provider = request.Target.RunnerID
	}
	return canonicalChildProvider(provider)
}

func childWorkerDurationFromPolicy(policy factory.JavaScriptPolicy) time.Duration {
	if policy.MaxWorkerDurationMs == nil || *policy.MaxWorkerDurationMs <= 0 {
		return 0
	}
	return time.Duration(*policy.MaxWorkerDurationMs) * time.Millisecond
}

func childAttemptTimeout(
	request factory.JavaScriptChildExecutionRequest,
	runnerID string,
	configured time.Duration,
) time.Duration {
	provider := ""
	if !strings.EqualFold(strings.TrimSpace(request.ExecutorProvider), workers.ExecutorProviderACP) {
		provider = firstNonBlank(request.ModelProvider, runnerID)
	}
	providerDefault := providers.ID(provider).NativeAttemptTimeout()
	if configured <= 0 {
		return providerDefault
	}
	if providerDefault <= 0 || configured < providerDefault {
		return configured
	}
	return providerDefault
}

func finishChildWorkerAttempt(
	complete func(context.Context, workers.ExecuteResult, error) error,
	progress *childWorkerProgressBridge,
	result workers.ExecuteResult,
	err error,
) {
	if complete != nil {
		_ = complete(context.Background(), result, err)
	}
	if progress != nil {
		progress.publishTerminal(result, err)
	}
}

func childExecutionShouldRetry(
	ctx context.Context,
	result workers.ExecuteResult,
	executeErr error,
	attemptNumber int,
	maxAttempts int,
) bool {
	if attemptNumber >= maxAttempts || ctx.Err() != nil || executeErr != nil {
		return false
	}
	if result.Outcome != workers.ExecutionOutcomeFailed || result.Failure == nil {
		return false
	}
	// A schema mismatch is a child-contract failure, not a provider outage.
	// It must consume only the JavaScript child's existing attempt allowance;
	// it must not opt into the unrelated Workers provider retry budget.
	if result.Failure.Type == workers.WorkFailureTypeStructuredOutputSchemaViolation {
		return true
	}
	return result.Failure.RetryHint
}

func (e *childWorkerExecutor) completedChild(
	base factory.JavaScriptChildDispatchRecord,
	req factory.JavaScriptChildExecutionRequest,
	dispatchID string,
	childIndex int,
	result workers.ExecuteResult,
) (factory.JavaScriptChildExecutionResult, error) {
	provider, providerSessionRef := childProviderSession(result)
	completed := base
	completed.Status = factory.JavaScriptChildDispatchStatusCompleted
	completed.Provider = provider
	completed.ProviderSessionRef = providerSessionRef
	output, schemaValidated := childWorkerOutputFromExecute(req, result)
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
		ArtifactRef:        base.ArtifactRef,
		ProviderSessionRef: providerSessionRef,
		Request:            req,
	}, nil
}

func (e *childWorkerExecutor) executeRequest(
	req factory.JavaScriptChildExecutionRequest,
	base factory.JavaScriptChildDispatchRecord,
	workerDispatchID string,
	attemptNumber int,
	progressBridge *childWorkerProgressBridge,
) workers.ExecuteRequest {
	requestID := strings.TrimSpace(e.sessionID)
	traceID := workerDispatchID
	dispatch := work.WorkDispatch{
		DispatchID:      workerDispatchID,
		WorkerType:      req.Preset,
		WorkstationName: workers.ProviderInvocationRoute,
		Execution: work.ExecutionMetadata{
			RequestID: requestID,
			TraceID:   traceID,
		},
	}
	progress := workers.ProgressPublisher(nil)
	if progressBridge != nil {
		progress = progressBridge.publishProgress
	}
	skipPermissions := effectiveChildSkipPermissions(req)
	attemptID := fmt.Sprintf("%s/attempt/%d", workerDispatchID, attemptNumber)
	return workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: requestID,
			RuntimeID:        firstNonBlank(e.runtimeID, "runtime-"+requestID),
			GenerationID:     firstNonBlank(e.generationID, "generation-"+requestID),
			DispatchID:       workerDispatchID,
			AttemptID:        attemptID,
			RequestID:        requestID,
			TraceID:          traceID,
		},
		Target: workers.ExecutionTarget{
			WorkerName:       req.Preset,
			WorkerType:       req.Preset,
			WorkstationName:  workers.ProviderInvocationRoute,
			RunnerID:         base.RunnerID,
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
				WorkingDirectorySet: strings.TrimSpace(e.workingDir) != "",
			},
			Workspace: workers.WorkspacePolicy{
				WorkingDirectory: e.workingDir,
				FactoryDirectory: e.workingDir,
			},
			Permissions: workers.PermissionPolicy{SkipPermissions: skipPermissions},
			Timeout:     childAttemptTimeout(req, base.RunnerID, e.maxWorkerDuration),
		},
		Input: workers.ExecutionInput{
			Dispatch:              dispatch,
			MockWorkers:           e.mockWorkers.Clone(),
			ProviderOverride:      e.providerOverride,
			CommandRunnerOverride: e.commandRunnerOverride,
			WorkflowContext:       &workers.Context{FactoryDirectory: e.workingDir, WorkDirectory: e.workingDir, SessionID: requestID},
			ProgressPublisher:     progress,
		},
		Attempt: workers.AttemptContext{Number: attemptNumber},
	}
}

func childExecutionSucceeded(outcome workers.ExecutionOutcome) bool {
	return outcome == workers.ExecutionOutcomeAccepted || outcome == workers.ExecutionOutcomeContinue
}

func childProviderSession(result workers.ExecuteResult) (string, string) {
	if result.Continuation == nil {
		return "", ""
	}
	session := result.Continuation.SessionMetadata()
	if session == nil {
		return "", ""
	}
	provider := providers.ID(session.Provider).CanonicalSessionProvider()
	if provider == "" {
		provider = strings.TrimSpace(session.Provider)
	}
	return provider, strings.TrimSpace(session.ID)
}

func normalizeChildStructuredResult(
	req factory.JavaScriptChildExecutionRequest,
	result workers.ExecuteResult,
) workers.ExecuteResult {
	if req.OutputSchema == nil || !childExecutionSucceeded(result.Outcome) {
		return result
	}
	if result.StructuredResultPresent {
		if _, ok := result.StructuredResult.(map[string]any); ok {
			return result
		}
		return childStructuredOutputFailure(result, "structured output schema violation: instance $; expected a validated JSON object")
	}
	return childStructuredOutputFailure(result, "structured output schema violation: instance $; validated JSON object is missing")
}

func childStructuredOutputFailure(result workers.ExecuteResult, message string) workers.ExecuteResult {
	result.Outcome = workers.ExecutionOutcomeFailed
	result.StructuredResult = nil
	result.StructuredResultPresent = false
	result.Failure = &workers.ExecutionFailure{
		Type:    workers.WorkFailureTypeStructuredOutputSchemaViolation,
		Family:  workers.WorkFailureFamilyTerminal,
		Message: message,
		Detail:  &workers.FailureDetail{Reason: workers.WorkFailureTypeStructuredOutputSchemaViolation, Message: message},
	}
	return result
}

func childWorkerOutputFromExecute(
	req factory.JavaScriptChildExecutionRequest,
	result workers.ExecuteResult,
) (map[string]any, bool) {
	if result.StructuredResultPresent {
		if structured, ok := result.StructuredResult.(map[string]any); ok {
			return structured, req.OutputSchema != nil
		}
	}
	var text strings.Builder
	for _, part := range result.Output.Primary {
		if part.Type.Normalized() == work.WorkContentPartTypeText || part.Type.Normalized() == "" {
			text.WriteString(part.Text)
		}
	}
	return childWorkerOutput(req, text.String()), false
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
	return nil, fmt.Errorf(
		"javascript child resource %q requires injected resource lease admission",
		resourceID,
	)
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
	runnerID, err := childRunnerID(req.ExecutorProvider, req.ModelProvider)
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
		SkipPermissions: effectiveChildSkipPermissions(req),
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
	result workers.ExecuteResult,
	executeErr error,
) (factory.JavaScriptChildExecutionResult, error) {
	result = normalizeChildExecuteFailure(result, executeErr)
	diagnostic := childFailureDiagnostic(result, executeErr, req)
	provider, providerSessionRef := childProviderSession(result)
	if provider == "" {
		provider = childProviderName(result)
	}
	if provider == "" {
		provider = canonicalChildProvider(req.ModelProvider)
	}
	failed := base
	failed.Status = factory.JavaScriptChildDispatchStatusFailed
	// The provider travels with its session reference or not at all. A
	// reference on its own is rejected when the session's runtime facts are
	// mapped to canonical events, which fails the whole execution rather than
	// the child -- so a failed Worker would surface as an internal error
	// instead of a failed session.
	failed.Provider = provider
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
	return map[string]any{
		"text":    trimmed,
		"subject": req.ArgsSubject,
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
