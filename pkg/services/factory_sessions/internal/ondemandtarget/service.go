// Package ondemandtarget implements the on-demand Factory Sessions
// activation a dynamic-target ACP-style consumer needs: unlike the CLI
// daemon's single, pre-opened project runtime, this activation has no fixed
// runtime to hand out and instead lazily opens exactly one ephemeral,
// non-HTTP-bound runtime per caller-selected Factory target the first time it
// is needed, then keeps it open for later calls against the identity it
// returned. This is private implementation: pkg/services/factory_sessions/wire
// exposes it as a thin construction wrapper (matching how that package already
// wraps runtimeopening.Factory as RuntimeOpeningFactory) so a caller outside
// this service tree never imports this package directly.
//
// Service implements exactly StartAsync, InvokeFactorySession, Cancel, and
// CloseFactorySession -- the narrow, owner-published
// factory_sessions/wire.TargetExecutionService capability -- and nothing
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
	"sync"

	"go.uber.org/zap"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
)

// Service satisfies io.Closer so a process lifecycle plan can register it as
// a NamedResource (see pkg/initializer/lifecycle) whose Close tears down
// every runtime it ever lazily activated.
var _ io.Closer = (*Service)(nil)

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

// invocationRuntimeOpener is the narrow capability this service consumes
// from *runtimeopening.Factory, declared here (rather than consuming the
// concrete type directly) so a test can substitute a fake without
// constructing the full Factory's own large production dependency graph.
// *runtimeopening.Factory already satisfies this interface structurally.
type invocationRuntimeOpener interface {
	OpenInvocationRuntime(
		ctx context.Context,
		request *factorysessions.RuntimeOpeningRequest,
		effects runtimeopening.ExternalEffects,
		logger *zap.Logger,
	) (roles.OpenedInvocationRuntime, error)
}

// Service is a consumer-owned Factory Sessions activation that has no fixed,
// pre-opened Factory Session runtime the way the CLI daemon's
// single-project bootstrap (OpenApplication/Assembly.Complete) does.
// Instead, the first StartFactoryTarget for a given caller-selected Factory
// target and working root lazily opens exactly one ephemeral, non-HTTP-bound
// runtime through the existing invocation-mode Runtime Opening path (the
// same primitive the CLI's one-shot named invocation already uses, per
// runtimeopening.Factory.OpenInvocationRuntime), starts its lifecycle, and
// keeps it open so every later call against the returned identity reuses
// that exact runtime instead of starting a second one.
//
// Every opened invocation-mode runtime privately shares the same constant
// internal session identity (factorysessions.DefaultSessionID) -- calling
// StartAsync/InvokeFactorySession/Cancel/CloseFactorySession against that
// raw identity across more than one concurrently-opened runtime would
// collide, so this service substitutes its own generated identity for the
// one returned to callers on Start, and translates it back to the correct
// cached runtime (and the runtime's own DefaultSessionID) on every later
// call. Callers must treat the returned identity as opaque.
type Service struct {
	factory    invocationRuntimeOpener
	effects    runtimeopening.ExternalEffects
	resolve    RuntimeResolver
	generateID factorysessions.SessionIDGenerator
	logger     *zap.Logger

	mu       sync.Mutex
	runtimes map[string]*activatedRuntime
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

// New constructs the on-demand activation over the given *runtimeopening.Factory.
// Construction alone performs no I/O and opens no runtime.
func New(
	factory *runtimeopening.Factory,
	effects runtimeopening.ExternalEffects,
	resolve RuntimeResolver,
	generateID factorysessions.SessionIDGenerator,
	logger *zap.Logger,
) (*Service, error) {
	if factory == nil {
		return nil, errors.New("construct on-demand Factory target activation: Runtime Opening factory is required")
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
		factory:           factory,
		effects:           effects,
		resolve:           resolve,
		generateID:        generateID,
		logger:            logger,
		runtimes:          make(map[string]*activatedRuntime),
		startsByRequestID: make(map[string]string),
		pendingStarts:     make(map[string]*pendingStart),
	}, nil
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

	opened, err := s.factory.OpenInvocationRuntime(ctx, &config, s.effects, s.logger)
	if err != nil {
		s.logger.Error("failed to open on-demand Factory target runtime",
			zap.String("factoryTargetId", factoryTargetID))
		return nil, fmt.Errorf("open Factory target runtime: %w", err)
	}
	runContext, cancel := context.WithCancel(context.WithoutCancel(ctx))
	active := &activatedRuntime{opened: opened, runContext: runContext, cancel: cancel}
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
		return nil, errors.Join(fmt.Errorf("activate Factory target runtime: %w", err), active.close(ctx))
	}
	s.logger.Info("activated on-demand Factory target runtime", zap.String("factoryTargetId", factoryTargetID))
	return active, nil
}

func (a *activatedRuntime) close(ctx context.Context) error {
	cleanupContext := context.WithoutCancel(ctx)
	var result error
	if a.opened.Sessions != nil {
		if err := a.opened.Sessions.CloseFactorySession(cleanupContext, factorysessions.DefaultSessionID); err != nil &&
			!errors.Is(err, factorysessions.ErrSessionNotFound) {
			result = errors.Join(result, err)
		}
	}
	if a.stopWorker != nil {
		result = errors.Join(result, a.stopWorker(cleanupContext))
	}
	if a.opened.Lifecycle != nil {
		result = errors.Join(result, a.opened.Lifecycle.StopLifecycle(cleanupContext))
	}
	if a.cancel != nil {
		a.cancel()
	}
	if a.opened.Lifecycle != nil {
		if err := a.opened.Lifecycle.WaitForRuntime(cleanupContext); err != nil && !errors.Is(err, context.Canceled) {
			result = errors.Join(result, err)
		}
	}
	if a.opened.CloseArtifacts != nil {
		result = errors.Join(result, a.opened.CloseArtifacts())
	}
	return result
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
// already uses (see chat_sessions/internal/factorysessionsshim.Shim, whose
// unmodified StartFactoryTarget/InvokeFactoryTarget map directly onto these
// two methods). The returned SessionID is this service's own generated
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
	workingRoot, _ := request.Args["workingRoot"].(string)
	config, err := s.resolve(ctx, request.Source.FactoryID, workingRoot)
	if err != nil {
		return factorysessions.AsyncStartResult{}, err
	}
	active, err := s.openActivatedRuntime(ctx, request.Source.FactoryID, config)
	if err != nil {
		return factorysessions.AsyncStartResult{}, err
	}

	wrapperID, err := s.publishActivation(request.RequestID, active)
	if err != nil {
		_ = active.close(context.WithoutCancel(ctx))
		return factorysessions.AsyncStartResult{}, err
	}
	s.logger.Info("on-demand Factory target runtime ready",
		zap.String("factoryTargetId", request.Source.FactoryID))
	return factorysessions.AsyncStartResult{
		SessionID: wrapperID,
		Status:    string(factorysessions.LifecycleStatusRunning),
	}, nil
}

// publishActivation validates and reserves a non-blank, non-colliding
// generated wrapper identity for active, then publishes it into s.runtimes
// (and, if requestID is non-blank, s.startsByRequestID). A blank identity or
// one that collides with an already-tracked identity fails without mutating
// either map, so a bad value from generateID can never overwrite -- and
// thereby strand -- an existing activation; the caller is responsible for
// closing active in that case.
func (s *Service) publishActivation(requestID string, active *activatedRuntime) (string, error) {
	wrapperID := s.generateID()
	s.mu.Lock()
	defer s.mu.Unlock()
	if wrapperID == "" {
		return "", errors.New("on-demand Factory target activation: generated session identity was blank")
	}
	if _, exists := s.runtimes[wrapperID]; exists {
		return "", fmt.Errorf("on-demand Factory target activation: generated session identity %q collided with an existing activation", wrapperID)
	}
	s.runtimes[wrapperID] = active
	if requestID != "" {
		s.startsByRequestID[requestID] = wrapperID
	}
	return wrapperID, nil
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
	result, err := active.opened.Sessions.InvokeFactorySession(ctx, factorysessions.DefaultSessionID, request)
	if err != nil {
		s.logger.Error("on-demand Factory target invoke failed")
		return result, err
	}
	result.SessionID = sessionID
	s.logger.Info("on-demand Factory target invoke completed", zap.String("status", string(result.Status)))
	return result, nil
}

// Cancel cancels the exact runtime a prior StartAsync call opened for
// sessionID.
func (s *Service) Cancel(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	active, err := s.lookup(sessionID)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return active.opened.Sessions.Cancel(ctx, factorysessions.DefaultSessionID, request)
}

// CloseFactorySession tears down and evicts the exact runtime a prior
// StartAsync call opened for sessionID. Closing an unknown or already-closed
// identity is a no-op success, matching the idempotent close semantics
// factorysessions.Service.CloseFactorySession already documents.
func (s *Service) CloseFactorySession(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	active, ok := s.runtimes[sessionID]
	if ok {
		delete(s.runtimes, sessionID)
	}
	s.mu.Unlock()
	if !ok {
		return nil
	}
	err := active.close(ctx)
	if err != nil {
		s.logger.Error("on-demand Factory target close failed")
	} else {
		s.logger.Info("on-demand Factory target closed")
	}
	return err
}

// Close tears down and evicts every runtime this Service has opened and not
// yet closed, aggregating every individual close failure into one returned
// error rather than stopping at the first. It satisfies io.Closer so a
// process lifecycle plan can register this Service as a reachable,
// deterministic unwind step for every runtime it ever lazily activated,
// instead of leaving them open for the life of the process; construction
// alone opens no runtime, so calling Close before any StartAsync call is a
// no-op success. Close is idempotent: a runtime evicted by this call or by
// an earlier CloseFactorySession call is never closed twice.
func (s *Service) Close() error {
	s.mu.Lock()
	active := make([]*activatedRuntime, 0, len(s.runtimes))
	for sessionID, runtime := range s.runtimes {
		active = append(active, runtime)
		delete(s.runtimes, sessionID)
	}
	s.mu.Unlock()

	var result error
	for _, runtime := range active {
		if err := runtime.close(context.WithoutCancel(context.Background())); err != nil {
			result = errors.Join(result, err)
		}
	}
	if result != nil {
		s.logger.Error("on-demand Factory target close-all failed", zap.Int("runtimeCount", len(active)))
	} else if len(active) > 0 {
		s.logger.Info("on-demand Factory target close-all completed", zap.Int("runtimeCount", len(active)))
	}
	return result
}
