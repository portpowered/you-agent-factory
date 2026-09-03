// Package ondemandtarget implements the on-demand Factory Sessions
// activation a dynamic-target ACP-style consumer needs: unlike the CLI
// daemon's single, pre-opened project runtime, this activation has no fixed
// runtime to hand out and instead lazily opens exactly one ephemeral,
// non-HTTP-bound runtime per caller-selected Factory target the first time it
// is needed, then keeps it open for later calls against the identity it
// returned. This is private implementation: pkg/services/factory_sessions/wire
// exposes it as a thin construction wrapper so a caller outside this service
// tree never imports this package directly.
//
// Service implements exactly StartAsync, InvokeFactorySession, Cancel, and
// CloseFactorySession -- the narrow, owner-published
// factory_sessions.TargetExecutionService capability -- and nothing
// more. Earlier iterations of this package embedded the full 30+ method
// public factorysessions.Service interface as a permanently-nil value solely
// so this type could be handed to a caller-owned adapter's constructor,
// which then required the literal Service type; every unimplemented method
// panicked if ever reached. That made this type a partial, panic-capable
// stand-in for the full aggregate root -- a real production risk if any
// future composition ever called one of the unimplemented methods. This type
// is now -- and is asserted to be, see the wire package's own var _
// TargetExecutionService assertion -- a complete, non-panicking
// implementation of exactly the capability it claims to support.
package ondemandtarget

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"

	"go.uber.org/zap"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
)

// Service satisfies io.Closer so a process lifecycle plan can register it as
// a NamedResource (see pkg/initializer/lifecycle) whose Close tears down
// every runtime it ever lazily activated.
var _ io.Closer = (*Service)(nil)

var errCancellationIncomplete = errors.New("on-demand Factory target cancellation did not complete")

// RuntimeResolver turns one canonical Factory target identity and editor
// working root into the concrete Runtime Opening request that activating a
// live, project-bound Factory Sessions runtime for that exact target needs.
// The concrete implementation (catalog + named-Factory cross-root
// resolution, operator defaults) is owned by the composing consumer; this
// package stays free of transport-specific target vocabulary.
type RuntimeResolver func(
	ctx context.Context,
	factoryTargetID string,
	workingRoot string,
) (factorysessions.RuntimeOpeningRequest, error)

// Service is Factory Sessions' own on-demand target-execution activation
// (published to peers as factorysessions.TargetExecutionService) that has no
// fixed, pre-opened Factory Session runtime the way the CLI daemon's
// single-project bootstrap (OpenApplication/Assembly.Complete) does.
// Instead, the first StartAsync for a given caller-selected Factory
// target and working root lazily opens exactly one ephemeral, non-HTTP-bound
// runtime through the existing invocation-mode Runtime Opening path (the
// same primitive the CLI's one-shot named invocation already uses), starts
// its lifecycle, and
// keeps it open so every later call against the returned identity reuses
// that exact runtime instead of starting a second one.
//
// Each activation allocates its opaque Factory Session identity before opening
// the runtime and carries that identity through the complete opening path.
// Concurrent activations therefore remain disjoint in both the public map and
// the process-scoped Definition/runtime routers. Callers must treat the
// returned identity as opaque.
type Service struct {
	opening    runtimeopening.InvocationRuntimeOpening
	resolve    RuntimeResolver
	generateID factorysessions.SessionIDGenerator
	logger     *zap.Logger

	mu       sync.Mutex
	runtimes map[string]*activatedRuntime
	// controls serializes lifecycle control across every activation generation
	// that has used one opaque caller-facing session ID. Cancel may replace an
	// activation under that ID; CloseFactorySession must then either run before
	// that replacement or close the replacement, never report success while it
	// remains live.
	controls map[string]*activationControl
	// startsByRequestID indexes a non-blank StartRequest.RequestID onto the
	// wrapper identity StartAsync opened for it, so a retried start for the
	// same stable request identity (this service's caller uses a key derived
	// from the session and episode, not the admitted Turn's own ID, so it is
	// stable across a retry -- see startFactorySessionForEpisode's own doc
	// comment) converges on the exact runtime already opened for it instead
	// of opening a second one. This is what makes StartAsync idempotent under
	// a stable per-episode key even when a caller's own post-start
	// bookkeeping (for example chatsessions.Service.RecordPendingFactorySession)
	// fails after a successful open: the caller's retry passes the same
	// RequestID and observes the same SessionID back.
	startsByRequestID map[string]string
	// pendingStarts indexes a non-blank StartRequest.RequestID that is
	// currently being started onto the in-flight pendingStart a genuinely
	// concurrent second StartAsync call for the exact same RequestID must
	// join rather than independently resolving and opening its own second
	// runtime. Reserving this entry happens under s.mu before any I/O, so at
	// most one goroutine ever performs the resolve/open work for one
	// RequestID no matter how many callers race in with it.
	pendingStarts map[string]*pendingStart
}

// pendingStart is the in-flight state a reserved-but-not-yet-published
// StartAsync call publishes for other goroutines racing in with the same
// RequestID to join. done is closed exactly once, by whichever goroutine
// reserved the entry, after result/err are set; every joining goroutine
// waits on done and then reads the identical result/err.
type pendingStart struct {
	done   chan struct{}
	result factorysessions.AsyncStartResult
	err    error
}

// activationControl is deliberately separate from activatedRuntime: Cancel
// replaces the latter while CloseFactorySession must retain one lock spanning
// both generations for the same opaque session ID.
type activationControl struct {
	mu                   sync.Mutex
	capturedTurnControls map[capturedTurnControlKey]factoryruntime.TerminateResult
}

// capturedTurnControlKey identifies one committed parent control independently
// of the replaceable on-demand runtime activation it first reached.
type capturedTurnControlKey struct {
	turnID    string
	controlID string
	action    factoryruntime.WorkerSessionControlAction
}

// New constructs the on-demand activation over the given Factory Sessions
// invocation-opening capability. Construction alone performs no I/O and
// opens no runtime.
func New(
	opening runtimeopening.InvocationRuntimeOpening,
	resolve RuntimeResolver,
	generateID factorysessions.SessionIDGenerator,
	logger *zap.Logger,
) (*Service, error) {
	if missingInvocationRuntimeOpening(opening) {
		return nil, errors.New("construct on-demand Factory target activation: invocation runtime opening is required")
	}
	if resolve == nil {
		return nil, errors.New("construct on-demand Factory target activation: Factory target runtime resolver is required")
	}
	if generateID == nil {
		return nil, errors.New("construct on-demand Factory target activation: session ID generator is required")
	}
	if logger == nil {
		return nil, errors.New("construct on-demand Factory target activation: logger is required")
	}
	return &Service{
		opening:           opening,
		resolve:           resolve,
		generateID:        generateID,
		logger:            logger,
		runtimes:          make(map[string]*activatedRuntime),
		controls:          make(map[string]*activationControl),
		startsByRequestID: make(map[string]string),
		pendingStarts:     make(map[string]*pendingStart),
	}, nil
}

func missingInvocationRuntimeOpening(opening runtimeopening.InvocationRuntimeOpening) bool {
	if opening == nil {
		return true
	}
	value := reflect.ValueOf(opening)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// activatedRuntime is one lazily-opened, lifecycle-started invocation
// runtime kept alive across calls. runContext is intentionally detached from
// any one caller request's context (context.WithoutCancel) so an
// asynchronous StartAsync dispatch it starts keeps running after the
// initiating ACP request returns; only this type's own close cancels it.
type activatedRuntime struct {
	opened     roles.OpenedInvocationRuntime
	runContext context.Context
	cancel     context.CancelFunc
	stopWorker factorysessions.RuntimeStop

	mu         sync.Mutex
	invocation *activeInvocation
	closing    bool
	closed     bool

	closeMu          sync.Mutex
	sessionClosed    bool
	workerStopped    bool
	lifecycleStopped bool
	runtimeWaited    bool
	artifactsClosed  bool

	factoryTargetID string
	config          factorysessions.RuntimeOpeningRequest
}

func (a *activatedRuntime) factorySessionID() string {
	if a != nil {
		if sessionID := strings.TrimSpace(a.config.FactorySession.FactorySessionID); sessionID != "" {
			return sessionID
		}
	}
	return factorysessions.DefaultSessionID
}

// activeInvocation is the one synchronous Factory invocation currently
// executing in an activated runtime. The Chat Session admission boundary
// serializes prompts for an episode, but keeping the cancellation handle here
// makes the owner-published TargetExecutionService capable of interrupting
// the exact request context that reached the provider without exposing the
// runtime's private lifecycle internals to ACP.
type activeInvocation struct {
	cancel context.CancelFunc
	done   chan struct{}

	cancelRequested bool
	cancelOutcome   chan bool
	cancelOnce      sync.Once
}

func (s *Service) openActivatedRuntime(
	ctx context.Context,
	factoryTargetID string,
	config factorysessions.RuntimeOpeningRequest,
) (*activatedRuntime, error) {
	// Structured logs here carry only the Factory target identity (a
	// catalog reference, not private topology) and bounded outcome
	// classifications -- never workingRoot (the caller's editor filesystem
	// path) and never a raw error via zap.Error, since an underlying
	// runtime-opening or lifecycle failure can embed a resolved filesystem
	// path in its own message text.
	s.logger.Info("activating on-demand Factory target runtime",
		zap.String("factoryTargetId", factoryTargetID))

	opened, err := s.opening.OpenInvocationRuntime(ctx, &config)
	if err != nil {
		s.logger.Error("failed to open on-demand Factory target runtime",
			zap.String("factoryTargetId", factoryTargetID))
		return nil, fmt.Errorf("open Factory target runtime: %w", err)
	}
	runContext, cancel := context.WithCancel(context.WithoutCancel(ctx))
	active := &activatedRuntime{
		opened:          opened,
		runContext:      runContext,
		cancel:          cancel,
		factoryTargetID: factoryTargetID,
		config:          config,
	}
	err = opened.Lifecycle.StartLifecycle(ctx, runContext)
	if err == nil {
		active.stopWorker, err = opened.Lifecycle.StartWorkerLifecycle(ctx)
	}
	if err == nil {
		err = opened.Lifecycle.CompleteStartup(ctx)
	}
	if err != nil {
		s.logger.Error("failed to activate on-demand Factory target runtime",
			zap.String("factoryTargetId", factoryTargetID))
		_, closeErr := active.close(ctx)
		return nil, errors.Join(fmt.Errorf("activate Factory target runtime: %w", err), closeErr)
	}
	s.logger.Info("activated on-demand Factory target runtime", zap.String("factoryTargetId", factoryTargetID))
	return active, nil
}

// close serializes cleanup attempts and retains the runtime after a failure
// so its captured owner can retry the same target. It returns the interrupted
// invocation, if any; the caller resolves that invocation only after its
// complete control operation (including a cancel replacement) succeeds.
func (a *activatedRuntime) close(ctx context.Context) (*activeInvocation, error) {
	a.closeMu.Lock()
	defer a.closeMu.Unlock()

	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil, nil
	}
	a.closing = true
	a.mu.Unlock()

	// Cleanup is synchronous, so it must keep the caller's cancellation and
	// deadline as well. A timed-out control leaves this activation mapped and
	// retryable; a later retry resumes only cleanup phases that did not finish.
	invocation, cancelErr := a.cancelInvocation(ctx)
	result := errors.Join(cancelErr, a.cleanup(ctx))
	if result == nil {
		a.mu.Lock()
		a.closed = true
		a.mu.Unlock()
	}
	return invocation, result
}

// cleanup retries only the runtime cleanup steps that have not yet
// succeeded. Repeating a failed close must not replay already-successful
// lifecycle effects or silently convert the original failure into success.
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func (a *activatedRuntime) cleanup(ctx context.Context) error {
	var result error
	var closeSession func(context.Context, string) error
	if a.opened.LiveControl != nil {
		closeSession = a.opened.LiveControl.CloseFactorySession
	} else if a.opened.Sessions != nil {
		if closer, ok := a.opened.Sessions.(interface {
			CloseFactorySession(context.Context, string) error
		}); ok {
			closeSession = closer.CloseFactorySession
		}
	}
	if closeSession != nil && !a.sessionClosed {
		if err := closeSession(ctx, a.factorySessionID()); err != nil &&
			!errors.Is(err, factorysessions.ErrSessionNotFound) {
			result = errors.Join(result, err)
		} else {
			a.sessionClosed = true
		}
	}
	if a.stopWorker != nil && !a.workerStopped {
		if err := a.stopWorker(ctx); err != nil {
			result = errors.Join(result, err)
		} else {
			a.workerStopped = true
		}
	}
	if a.opened.Lifecycle != nil && !a.lifecycleStopped {
		if err := a.opened.Lifecycle.StopLifecycle(ctx); err != nil {
			result = errors.Join(result, err)
		} else {
			a.lifecycleStopped = true
		}
	}
	if a.cancel != nil {
		a.cancel()
	}
	if a.opened.Lifecycle != nil && !a.runtimeWaited {
		if err := a.opened.Lifecycle.WaitForRuntime(ctx); err != nil && !errors.Is(err, context.Canceled) {
			result = errors.Join(result, err)
		} else {
			a.runtimeWaited = true
		}
	}
	if a.opened.CloseArtifacts != nil && !a.artifactsClosed {
		if err := a.opened.CloseArtifacts(); err != nil {
			result = errors.Join(result, err)
		} else {
			a.artifactsClosed = true
		}
	}
	return result
}

// beginInvocation registers the per-request cancellation context before the
// Factory Sessions call starts. A target has one active Chat turn at a time;
// treating a second concurrent call as an error avoids silently assigning a
// cancel notification to the wrong invocation if that invariant is violated.
func (a *activatedRuntime) beginInvocation(ctx context.Context) (context.Context, *activeInvocation, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closing {
		return nil, nil, errors.New("on-demand Factory target is closing")
	}
	if a.invocation != nil {
		return nil, nil, errors.New("on-demand Factory target already has an active invocation")
	}
	invocationContext, cancel := context.WithCancel(ctx)
	invocation := &activeInvocation{
		cancel:        cancel,
		done:          make(chan struct{}),
		cancelOutcome: make(chan bool, 1),
	}
	a.invocation = invocation
	return invocationContext, invocation, nil
}

// beginCancellation prevents another invocation from starting while Cancel
// closes this runtime. A retry after a failed close remains actionable even
// though its original invocation has already stopped; only a normally
// completed, still-running runtime is a true no-op.
func (a *activatedRuntime) beginCancellation() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.invocation == nil && !a.closing {
		return false
	}
	a.closing = true
	if a.invocation != nil {
		a.invocation.cancelRequested = true
		a.invocation.cancel()
	}
	return true
}

// finishInvocation clears a completed invocation and waits for the owning
// Cancel or Close operation to report whether all of its cleanup succeeded.
// A raw provider context cancellation is not itself a truthful ACP canceled
// result when the owner control failed and remains retryable.
func (a *activatedRuntime) finishInvocation(invocation *activeInvocation) (bool, bool) {
	a.mu.Lock()
	if a.invocation == invocation {
		a.invocation = nil
	}
	cancelRequested := invocation.cancelRequested
	close(invocation.done)
	a.mu.Unlock()
	invocation.cancel()
	if !cancelRequested {
		return false, false
	}
	return <-invocation.cancelOutcome, true
}

// cancelInvocation signals the exact active invocation and waits for its
// Factory Sessions call to terminalize. Waiting keeps the Chat control fence
// closed until the captured prompt has observed cancellation, so a later
// prompt cannot inherit an in-flight provider operation. It reports false
// when a normally completed turn has already cleared the active invocation.
func (a *activatedRuntime) cancelInvocation(ctx context.Context) (*activeInvocation, error) {
	a.mu.Lock()
	invocation := a.invocation
	if invocation == nil {
		a.mu.Unlock()
		return nil, nil
	}
	invocation.cancelRequested = true
	invocation.cancel()
	done := invocation.done
	a.mu.Unlock()

	select {
	case <-done:
		return invocation, nil
	case <-ctx.Done():
		return invocation, ctx.Err()
	}
}

func (a *activatedRuntime) resolveCancellation(invocation *activeInvocation, succeeded bool) {
	if invocation == nil {
		return
	}
	invocation.cancelOnce.Do(func() {
		invocation.cancelOutcome <- succeeded
	})
}

// StartAsync opens (and keeps open) exactly one Factory target runtime for
// this request's Source.FactoryID and Args["workingRoot"], but dispatches no
// content: this on-demand activation has no fixed pre-opened runtime the way
// the CLI daemon's StartAsync semantics assume, and StartAsync's own Source
// vocabulary (FACTORY_ID/FACTORY_INLINE/WORKFLOW_FILE/WORKFLOW_NAME/
// INLINE_WORKFLOW) only resolves named JavaScript workflow factories --
// confirmed by reading
// internal/services/orchestration/javascript/source/lookup.go, whose
// FACTORY_ID case rejects any target that "is not a JavaScript workflow
// factory" -- so it cannot itself dispatch an ordinary packaged Factory the
// way this service's caller needs. This method's own job is narrower and
// honest about it: open the runtime and report it RUNNING with no
// fabricated terminal outcome; a caller that wants the first turn's actual
// published outcome (including text) makes a separate, immediate
// InvokeFactorySession call against the returned SessionID -- the same
// synchronous, non-JavaScript-workflow-specific call every later turn
// already uses (see the owner-published TargetExecutionService this Service
// satisfies, in ../../target_execution_contract.go). The returned SessionID
// is this service's own generated
// identity (never the opened runtime's shared internal constant session
// identity), so a later InvokeFactorySession/Cancel/CloseFactorySession call
// against it resolves back to this exact runtime.
func (s *Service) StartAsync(
	ctx context.Context,
	request factorysessions.StartRequest,
) (factorysessions.AsyncStartResult, error) {
	if request.RequestID == "" {
		return s.startNewActivation(ctx, request)
	}

	result, err, joined := s.reserveOrJoinStart(request.RequestID)
	if joined {
		if err == nil {
			s.logger.Info("on-demand Factory target start converged onto an existing activation for this request")
		}
		return result, err
	}

	result, err = s.startNewActivation(ctx, request)
	s.finishPendingStart(request.RequestID, result, err)
	return result, err
}

// reserveOrJoinStart reports (result, err, true) when requestID either
// already has a completed, still-tracked activation (a genuine retry after
// an earlier StartAsync call fully returned) or is currently being started by
// another goroutine (a truly concurrent call, which blocks on that
// goroutine's own pendingStart.done and returns its identical outcome
// instead of independently resolving and opening a second runtime). It
// reports (_, _, false) with a fresh pendingStarts[requestID] entry reserved
// under s.mu when this call is the one that must actually perform the start;
// the caller must then call finishPendingStart(requestID, ...) exactly once.
func (s *Service) reserveOrJoinStart(requestID string) (factorysessions.AsyncStartResult, error, bool) {
	s.mu.Lock()
	if wrapperID, ok := s.startsByRequestID[requestID]; ok {
		if _, exists := s.runtimes[wrapperID]; exists {
			s.mu.Unlock()
			return factorysessions.AsyncStartResult{SessionID: wrapperID, Status: string(factorysessions.LifecycleStatusRunning)}, nil, true
		}
	}
	if pending, ok := s.pendingStarts[requestID]; ok {
		s.mu.Unlock()
		<-pending.done
		return pending.result, pending.err, true
	}
	s.pendingStarts[requestID] = &pendingStart{done: make(chan struct{})}
	s.mu.Unlock()
	return factorysessions.AsyncStartResult{}, nil, false
}

// finishPendingStart records the outcome of a reserved start for requestID
// (see reserveOrJoinStart) and wakes every goroutine that joined it while it
// was in flight. It is a no-op if the reservation was somehow already
// cleared, which should not happen since only this call ever removes it.
func (s *Service) finishPendingStart(requestID string, result factorysessions.AsyncStartResult, err error) {
	s.mu.Lock()
	pending := s.pendingStarts[requestID]
	delete(s.pendingStarts, requestID)
	s.mu.Unlock()
	if pending == nil {
		return
	}
	pending.result = result
	pending.err = err
	close(pending.done)
}

// startNewActivation resolves and opens exactly one fresh Factory target
// runtime for request, then publishes it under a validated generated
// identity. Callers with a non-blank RequestID must route through
// reserveOrJoinStart/finishPendingStart so a genuinely concurrent second
// caller for the same RequestID never reaches this method independently.
func (s *Service) startNewActivation(
	ctx context.Context,
	request factorysessions.StartRequest,
) (factorysessions.AsyncStartResult, error) {
	wrapperID, err := s.nextActivationID()
	if err != nil {
		return factorysessions.AsyncStartResult{}, err
	}
	workingRoot, _ := request.Args["workingRoot"].(string)
	config, err := s.resolve(ctx, request.Source.FactoryID, workingRoot)
	if err != nil {
		return factorysessions.AsyncStartResult{}, err
	}
	config.FactorySession.FactorySessionID = wrapperID
	active, err := s.openActivatedRuntime(ctx, request.Source.FactoryID, config)
	if err != nil {
		return factorysessions.AsyncStartResult{}, err
	}

	err = s.publishActivation(request.RequestID, wrapperID, active)
	if err != nil {
		_, _ = active.close(context.WithoutCancel(ctx))
		return factorysessions.AsyncStartResult{}, err
	}
	s.logger.Info("on-demand Factory target runtime ready",
		zap.String("factoryTargetId", request.Source.FactoryID))
	return factorysessions.AsyncStartResult{
		SessionID: wrapperID,
		Status:    string(factorysessions.LifecycleStatusRunning),
	}, nil
}

// lockControl acquires the lifecycle-generation lock for sessionID. A caller
// holds it from lookup through every close, replacement, and map update, so a
// concurrent close cannot observe and silently leave a Cancel replacement.
func (s *Service) lockControl(sessionID string) (*activationControl, bool) {
	for {
		s.mu.Lock()
		control, ok := s.controls[sessionID]
		s.mu.Unlock()
		if !ok {
			return nil, false
		}
		control.mu.Lock()
		s.mu.Lock()
		current := s.controls[sessionID]
		s.mu.Unlock()
		if current == control {
			return control, true
		}
		// A prior close evicted this generation while this caller waited. Do
		// not let the stale lock govern a newly reused opaque identity.
		control.mu.Unlock()
	}
}

func (s *Service) unlockControl(control *activationControl) {
	control.mu.Unlock()
}

// releaseControl removes the now-unused per-session control only when it is
// still the generation lock this caller acquired and no activation remains.
func (s *Service) releaseControl(sessionID string, control *activationControl) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.controls[sessionID] == control && s.runtimes[sessionID] == nil {
		delete(s.controls, sessionID)
	}
}

func (s *Service) lookup(sessionID string) (*activatedRuntime, error) {
	s.mu.Lock()
	active, ok := s.runtimes[sessionID]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, sessionID)
	}
	return active, nil
}

// InvokeFactorySession synchronously invokes the exact runtime a prior
// StartAsync call opened for sessionID.
func (s *Service) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.InvocationRequest,
) (factorysessions.InvocationResult, error) {
	active, err := s.lookup(sessionID)
	if err != nil {
		return factorysessions.InvocationResult{}, err
	}
	invocationContext, invocation, err := active.beginInvocation(ctx)
	if err != nil {
		return factorysessions.InvocationResult{}, err
	}
	result, err := s.invokeOnActivatedRuntime(invocationContext, active, request)
	canceled, cancellationRequested := active.finishInvocation(invocation)
	if canceled {
		if request.RequestID != nil && result.RequestID == "" {
			result.RequestID = *request.RequestID
		}
		result.Status = factorysessions.InvocationTerminalStatusCanceled
		result.ErrorCode = string(factorysessions.InvocationErrorCodeCanceled)
		result.Message = ""
		result.PrimaryResult = nil
		result.SessionID = sessionID
		s.logger.Info("on-demand Factory target invoke canceled")
		return result, nil
	}
	if cancellationRequested {
		s.logger.Error("on-demand Factory target cancellation did not complete")
		return result, errCancellationIncomplete
	}
	if err != nil {
		s.logger.Error("on-demand Factory target invoke failed")
		return result, err
	}
	result.SessionID = sessionID
	s.logger.Info("on-demand Factory target invoke completed", zap.String("status", string(result.Status)))
	return result, nil
}

// SubscribeFactoryResponseEvents subscribes to the exact runtime and explicit
// Factory Session identity a prior StartAsync call opened for req.SessionID.
func (s *Service) SubscribeFactoryResponseEvents(
	ctx context.Context,
	req factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	active, err := s.lookup(req.SessionID)
	if err != nil {
		return nil, err
	}
	req.SessionID = active.factorySessionID()
	return active.opened.Sessions.SubscribeFactoryResponseEvents(ctx, req)
}

// SubscribeFactoryEventsForSession exposes the canonical Factory Event
// history/live stream for the exact on-demand runtime identified by sessionID.
// The wrapper identity is also the opened runtime's Factory Session identity,
// so no caller can accidentally observe another activated target.
func (s *Service) SubscribeFactoryEventsForSession(
	ctx context.Context,
	sessionID string,
	reconnect *factorydefinitions.FactoryEventReconnectCursor,
) (*factorydefinitions.FactoryEventStream, error) {
	active, err := s.lookup(sessionID)
	if err != nil {
		return nil, err
	}
	return active.opened.Sessions.SubscribeFactoryEventsForSession(ctx, active.factorySessionID(), reconnect)
}

// Cancel cancels the exact runtime a prior StartAsync call opened for
// sessionID. A live Factory's provider work is owned by its runtime lifecycle,
// not the durable session control surface, so a real cancellation closes the
// captured runtime and immediately replaces it under the same opaque target
// identity. A later Chat turn therefore starts from a clean live runtime
// without ever being routed to the canceled provider operation.
func (s *Service) Cancel(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	control, ok := s.lockControl(sessionID)
	if !ok {
		return factorysessions.LifecycleControlResult{}, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, sessionID)
	}
	defer s.unlockControl(control)

	active, err := s.lookup(sessionID)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	if err := controlCapturedTurn(ctx, control, active, request, factoryruntime.WorkerSessionControlActionCancel); err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	if !active.beginCancellation() {
		s.logger.Info("on-demand Factory target cancel completed", zap.String("outcome", string(factorysessions.LifecycleControlOutcomeNoOp)))
		return factorysessions.LifecycleControlResult{
			SessionID: sessionID,
			Operation: factorysessions.LifecycleControlCancel,
			Outcome:   factorysessions.LifecycleControlOutcomeNoOp,
			Status:    factorysessions.LifecycleStatusRunning,
		}, nil
	}
	invocation, closeErr := active.close(ctx)
	if closeErr != nil {
		active.resolveCancellation(invocation, false)
		s.logger.Error("on-demand Factory target cancel failed")
		return factorysessions.LifecycleControlResult{}, closeErr
	}
	replacement, err := s.openActivatedRuntime(ctx, active.factoryTargetID, active.config)
	if err != nil {
		active.resolveCancellation(invocation, false)
		s.logger.Error("on-demand Factory target reset failed")
		return factorysessions.LifecycleControlResult{}, err
	}
	if !s.replaceActivation(sessionID, active, replacement) {
		_, _ = replacement.close(context.WithoutCancel(ctx))
		active.resolveCancellation(invocation, false)
		return factorysessions.LifecycleControlResult{}, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, sessionID)
	}
	active.resolveCancellation(invocation, true)
	s.logger.Info("on-demand Factory target cancel completed", zap.String("outcome", string(factorysessions.LifecycleControlOutcomeAccepted)))
	return factorysessions.LifecycleControlResult{
		SessionID: sessionID,
		Operation: factorysessions.LifecycleControlCancel,
		Outcome:   factorysessions.LifecycleControlOutcomeAccepted,
		Status:    factorysessions.LifecycleStatusRunning,
	}, nil
}

// controlCapturedTurn forwards a committed ACP turn control to the exact
// Factory Runtime that was active when this target control began. It runs
// before invocation-context cancellation and cleanup so Factory Runtime
// remains the authority that fans the requested action through Worker
// Sessions. The result is retained by the target generation lock, rather than
// its replaceable activation, so a retry can never bind the same control to a
// later turn.
func controlCapturedTurn(
	ctx context.Context,
	control *activationControl,
	active *activatedRuntime,
	request factorysessions.ControlRequest,
	action factoryruntime.WorkerSessionControlAction,
) error {
	turnID := strings.TrimSpace(request.TurnID)
	if turnID == "" {
		return nil
	}
	controlID := strings.TrimSpace(request.RequestID)
	if controlID == "" {
		return errors.New("control captured Factory turn: control request id is required")
	}
	key := capturedTurnControlKey{turnID: turnID, controlID: controlID, action: action}
	if _, ok := control.capturedTurnControls[key]; ok {
		return nil
	}
	if active == nil || active.opened.Lifecycle == nil {
		return errors.New("control captured Factory turn: runtime lifecycle is required")
	}
	hosted := active.opened.Lifecycle.CurrentRuntimeBundle()
	if hosted == nil || hosted.RuntimeService() == nil {
		return errors.New("control captured Factory turn: Factory Runtime is required")
	}
	result, err := hosted.RuntimeService().ControlTerminate(
		context.WithoutCancel(ctx),
		factoryruntime.TerminateRequest{
			Reason:              request.Reason,
			TurnID:              turnID,
			ControlID:           controlID,
			WorkerSessionAction: action,
		},
	)
	if err != nil {
		return fmt.Errorf("control captured Factory turn: %w", err)
	}
	if control.capturedTurnControls == nil {
		control.capturedTurnControls = make(map[capturedTurnControlKey]factoryruntime.TerminateResult)
	}
	control.capturedTurnControls[key] = result
	return nil
}

// TerminateFactorySession routes a committed close control through the exact
// Factory Runtime before closing the captured target. A blank turn remains a
// generic target close, preserving process-lifecycle cleanup behavior.
func (s *Service) TerminateFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) error {
	control, ok := s.lockControl(sessionID)
	if !ok {
		return nil
	}
	defer s.unlockControl(control)

	active, err := s.lookup(sessionID)
	if errors.Is(err, factorysessions.ErrSessionNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := controlCapturedTurn(ctx, control, active, request, factoryruntime.WorkerSessionControlActionTerminate); err != nil {
		return err
	}
	return s.closeCapturedActivation(ctx, sessionID, control, active)
}

// replaceActivation swaps a closed, canceled runtime for its freshly opened
// successor only when sessionID still refers to that exact captured runtime.
// A concurrent full CloseFactorySession wins by removing the mapping; in that
// case the caller observes ErrSessionNotFound and the successor is closed by
// Cancel before it returns.
func (s *Service) replaceActivation(sessionID string, current, replacement *activatedRuntime) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtimes[sessionID] != current {
		return false
	}
	s.runtimes[sessionID] = replacement
	return true
}

// removeActivation evicts sessionID only if it still resolves to current, so
// a concurrent replacement can never be removed by an older close attempt.
func (s *Service) removeActivation(sessionID string, current *activatedRuntime) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtimes[sessionID] != current {
		return false
	}
	delete(s.runtimes, sessionID)
	return true
}

// CloseFactorySession tears down and evicts the exact runtime a prior
// StartAsync call opened for sessionID. Closing an unknown or already-closed
// identity is a no-op success, matching the idempotent close semantics
// factorysessions.LiveControlService.CloseFactorySession already documents.
func (s *Service) CloseFactorySession(ctx context.Context, sessionID string) error {
	control, ok := s.lockControl(sessionID)
	if !ok {
		return nil
	}
	defer s.unlockControl(control)

	active, err := s.lookup(sessionID)
	if errors.Is(err, factorysessions.ErrSessionNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.closeCapturedActivation(ctx, sessionID, control, active)
}

func (s *Service) closeCapturedActivation(
	ctx context.Context,
	sessionID string,
	control *activationControl,
	active *activatedRuntime,
) error {
	invocation, err := active.close(ctx)
	active.resolveCancellation(invocation, err == nil)
	if err != nil {
		s.logger.Error("on-demand Factory target close failed")
	} else {
		if !s.removeActivation(sessionID, active) {
			return fmt.Errorf("on-demand Factory target close lost ownership of session %q", sessionID)
		}
		s.releaseControl(sessionID, control)
		s.logger.Info("on-demand Factory target closed")
	}
	return err
}

// Close tears down every runtime this Service has opened and evicts only
// those that closed successfully, retaining a failed target for a truthful
// retry rather than losing it behind an idempotent no-op. It aggregates every
// individual close failure into one returned error rather than stopping at
// the first. It satisfies io.Closer so a
// process lifecycle plan can register this Service as a reachable,
// deterministic unwind step for every runtime it ever lazily activated,
// instead of leaving them open for the life of the process; construction
// alone opens no runtime, so calling Close before any StartAsync call is a
// no-op success. Close is idempotent: a runtime evicted by this call or by
// an earlier CloseFactorySession call is never closed twice.
func (s *Service) Close() error {
	s.mu.Lock()
	active := make(map[string]*activatedRuntime, len(s.runtimes))
	for sessionID, runtime := range s.runtimes {
		active[sessionID] = runtime
	}
	s.mu.Unlock()

	var result error
	for sessionID := range active {
		control, ok := s.lockControl(sessionID)
		if !ok {
			continue
		}
		runtime, err := s.lookup(sessionID)
		if errors.Is(err, factorysessions.ErrSessionNotFound) {
			s.releaseControl(sessionID, control)
			s.unlockControl(control)
			continue
		}
		if err != nil {
			result = errors.Join(result, err)
			s.unlockControl(control)
			continue
		}
		invocation, err := runtime.close(context.WithoutCancel(context.Background()))
		runtime.resolveCancellation(invocation, err == nil)
		if err != nil {
			result = errors.Join(result, err)
			s.unlockControl(control)
			continue
		}
		if !s.removeActivation(sessionID, runtime) {
			result = errors.Join(result, fmt.Errorf("on-demand Factory target close-all lost ownership of session %q", sessionID))
			s.unlockControl(control)
			continue
		}
		s.releaseControl(sessionID, control)
		s.unlockControl(control)
	}
	if result != nil {
		s.logger.Error("on-demand Factory target close-all failed", zap.Int("runtimeCount", len(active)))
	} else if len(active) > 0 {
		s.logger.Info("on-demand Factory target close-all completed", zap.Int("runtimeCount", len(active)))
	}
	return result
}
