package wire

import (
	"context"
	"errors"
	"fmt"
	"sync"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

// FactoryTargetRuntimeResolver turns one canonical Factory target identity
// and editor working root into the concrete Runtime Opening request that
// activating a live, project-bound Factory Sessions runtime for that exact
// target needs. The concrete implementation (catalog + named-Factory
// cross-root resolution, operator defaults) is owned by the composing
// consumer; this package stays free of transport-specific target vocabulary.
type FactoryTargetRuntimeResolver func(
	ctx context.Context,
	factoryTargetID string,
	workingRoot string,
) (factorysessions.RuntimeOpeningRequest, error)

// OnDemandFactoryTargetService is a consumer-owned Factory Sessions
// activation that has no fixed, pre-opened Factory Session runtime the way
// the CLI daemon's single-project bootstrap (OpenApplication/Assembly.Complete)
// does. Instead, the first StartFactoryTarget for a given caller-selected
// Factory target and working root lazily opens exactly one ephemeral,
// non-HTTP-bound runtime through the existing invocation-mode Runtime
// Opening path (the same primitive the CLI's one-shot named invocation
// already uses, per runtimeopening.Factory.OpenInvocationRuntime), starts
// its lifecycle, and keeps it open so every later call against the returned
// identity reuses that exact runtime instead of starting a second one.
//
// Every opened invocation-mode runtime privately shares the same constant
// internal session identity (factorysessions.DefaultSessionID) -- calling
// StartAsync/InvokeFactorySession/Cancel/CloseFactorySession against that
// raw identity across more than one concurrently-opened runtime would
// collide, so this service substitutes its own generated identity for the
// one returned to callers on Start, and translates it back to the correct
// cached runtime (and the runtime's own DefaultSessionID) on every later
// call. Callers must treat the returned identity as opaque.
type OnDemandFactoryTargetService struct {
	factory    *RuntimeOpeningFactory
	effects    RuntimeOpeningExternalEffects
	resolve    FactoryTargetRuntimeResolver
	generateID factorysessions.SessionIDGenerator
	logger     *zap.Logger

	mu       sync.Mutex
	runtimes map[string]*activatedFactoryRuntime
}

// NewOnDemandFactoryTargetService constructs the on-demand activation.
// Construction alone performs no I/O and opens no runtime.
func NewOnDemandFactoryTargetService(
	factory *RuntimeOpeningFactory,
	effects RuntimeOpeningExternalEffects,
	resolve FactoryTargetRuntimeResolver,
	generateID factorysessions.SessionIDGenerator,
	logger *zap.Logger,
) (*OnDemandFactoryTargetService, error) {
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
	return &OnDemandFactoryTargetService{
		factory:    factory,
		effects:    effects,
		resolve:    resolve,
		generateID: generateID,
		logger:     logger,
		runtimes:   make(map[string]*activatedFactoryRuntime),
	}, nil
}

// activatedFactoryRuntime is one lazily-opened, lifecycle-started invocation
// runtime kept alive across calls. runContext is intentionally detached from
// any one caller request's context (context.WithoutCancel) so an
// asynchronous StartAsync dispatch it starts keeps running after the
// initiating ACP request returns; only this type's own close cancels it.
type activatedFactoryRuntime struct {
	opened     OpenedInvocationRuntime
	runContext context.Context
	cancel     context.CancelFunc
	stopWorker factorysessions.RuntimeStop
}

func (s *OnDemandFactoryTargetService) openActivatedRuntime(
	ctx context.Context,
	config factorysessions.RuntimeOpeningRequest,
) (*activatedFactoryRuntime, error) {
	opened, err := s.factory.OpenInvocationRuntime(ctx, &config, s.effects, s.logger)
	if err != nil {
		return nil, fmt.Errorf("open Factory target runtime: %w", err)
	}
	runContext, cancel := context.WithCancel(context.WithoutCancel(ctx))
	active := &activatedFactoryRuntime{opened: opened, runContext: runContext, cancel: cancel}
	err = opened.Lifecycle.StartLifecycle(ctx, runContext)
	if err == nil {
		active.stopWorker, err = opened.Lifecycle.StartWorkerLifecycle(ctx)
	}
	if err == nil {
		err = opened.Lifecycle.CompleteStartup(ctx)
	}
	if err != nil {
		return nil, errors.Join(fmt.Errorf("activate Factory target runtime: %w", err), active.close(ctx))
	}
	return active, nil
}

func (a *activatedFactoryRuntime) close(ctx context.Context) error {
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

// StartFactoryTarget opens (and keeps open) exactly one Factory target
// runtime for this request's Source.FactoryID and Args["workingRoot"], then
// dispatches the requested content on it.
//
// This dispatches through InvokeFactorySession, the same synchronous,
// non-JavaScript-workflow-specific call the CLI's own one-shot named
// invocation already uses successfully, rather than StartAsync/Source:
// StartAsync's Source vocabulary (FACTORY_ID/FACTORY_INLINE/WORKFLOW_FILE/
// WORKFLOW_NAME/INLINE_WORKFLOW) only resolves named JavaScript workflow
// factories -- confirmed by reading
// internal/services/orchestration/javascript/source/lookup.go, whose
// FACTORY_ID case rejects any target that "is not a JavaScript workflow
// factory" -- so it cannot start an ordinary packaged Factory the way this
// service's already-resolved, already-opened runtime needs. Since
// s.resolve/openActivatedRuntime already picked and opened the exact target
// directory, this call needs no further source selection at all. The
// returned InvocationResult is the real, unmodified published invocation
// outcome -- terminal status and ordered primary-result text included --
// except SessionID, which this service substitutes with its own generated
// identity (never the opened runtime's shared internal constant session
// identity) so a later InvokeFactoryTarget/CancelFactoryTarget/
// CloseFactoryTarget call against the returned identity resolves back to
// this exact runtime.
func (s *OnDemandFactoryTargetService) StartFactoryTarget(
	ctx context.Context,
	request factorysessions.StartRequest,
) (factorysessions.InvocationResult, error) {
	workingRoot, _ := request.Args["workingRoot"].(string)
	config, err := s.resolve(ctx, request.Source.FactoryID, workingRoot)
	if err != nil {
		return factorysessions.InvocationResult{}, err
	}
	active, err := s.openActivatedRuntime(ctx, config)
	if err != nil {
		return factorysessions.InvocationResult{}, err
	}

	content, _ := request.Args["content"].([]work.WorkContentPart)
	requestID := request.RequestID
	sourceKind := factorysessions.InvocationInputSourceKindText
	result, err := active.opened.Sessions.InvokeFactorySession(ctx, factorysessions.DefaultSessionID, factorysessions.InvocationRequest{
		Content:         content,
		ContentProvided: true,
		RequestID:       &requestID,
		SourceKind:      &sourceKind,
	})
	if err != nil {
		return factorysessions.InvocationResult{}, errors.Join(err, active.close(ctx))
	}

	wrapperID := s.generateID()
	s.mu.Lock()
	s.runtimes[wrapperID] = active
	s.mu.Unlock()
	result.SessionID = wrapperID
	return result, nil
}

func (s *OnDemandFactoryTargetService) lookup(sessionID string) (*activatedFactoryRuntime, error) {
	s.mu.Lock()
	active, ok := s.runtimes[sessionID]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, sessionID)
	}
	return active, nil
}

// InvokeFactoryTarget synchronously invokes the exact runtime a prior
// StartFactoryTarget call opened for sessionID.
func (s *OnDemandFactoryTargetService) InvokeFactoryTarget(
	ctx context.Context,
	sessionID string,
	request factorysessions.InvocationRequest,
) (factorysessions.InvocationResult, error) {
	active, err := s.lookup(sessionID)
	if err != nil {
		return factorysessions.InvocationResult{}, err
	}
	return active.opened.Sessions.InvokeFactorySession(ctx, factorysessions.DefaultSessionID, request)
}

// CancelFactoryTarget cancels the exact runtime a prior StartFactoryTarget
// call opened for sessionID.
func (s *OnDemandFactoryTargetService) CancelFactoryTarget(
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

// CloseFactoryTarget tears down and evicts the exact runtime a prior
// StartFactoryTarget call opened for sessionID. Closing an unknown or
// already-closed identity is a no-op success, matching the idempotent close
// semantics factorysessions.Service.CloseFactorySession already documents.
func (s *OnDemandFactoryTargetService) CloseFactoryTarget(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	active, ok := s.runtimes[sessionID]
	if ok {
		delete(s.runtimes, sessionID)
	}
	s.mu.Unlock()
	if !ok {
		return nil
	}
	return active.close(ctx)
}
