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
package ondemandtarget

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.uber.org/zap"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

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
		factory:    factory,
		effects:    effects,
		resolve:    resolve,
		generateID: generateID,
		logger:     logger,
		runtimes:   make(map[string]*activatedRuntime),
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
	workingRoot string,
	config factorysessions.RuntimeOpeningRequest,
) (*activatedRuntime, error) {
	s.logger.Info("activating on-demand Factory target runtime",
		zap.String("factoryTargetId", factoryTargetID), zap.String("workingRoot", workingRoot))

	opened, err := s.factory.OpenInvocationRuntime(ctx, &config, s.effects, s.logger)
	if err != nil {
		s.logger.Error("failed to open on-demand Factory target runtime",
			zap.String("factoryTargetId", factoryTargetID), zap.Error(err))
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
			zap.String("factoryTargetId", factoryTargetID), zap.Error(err))
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
func (s *Service) StartFactoryTarget(
	ctx context.Context,
	request factorysessions.StartRequest,
) (factorysessions.InvocationResult, error) {
	workingRoot, _ := request.Args["workingRoot"].(string)
	config, err := s.resolve(ctx, request.Source.FactoryID, workingRoot)
	if err != nil {
		return factorysessions.InvocationResult{}, err
	}
	active, err := s.openActivatedRuntime(ctx, request.Source.FactoryID, workingRoot, config)
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
		s.logger.Error("on-demand Factory target dispatch failed",
			zap.String("factoryTargetId", request.Source.FactoryID), zap.Error(err))
		return factorysessions.InvocationResult{}, errors.Join(err, active.close(ctx))
	}

	wrapperID := s.generateID()
	s.mu.Lock()
	s.runtimes[wrapperID] = active
	s.mu.Unlock()
	result.SessionID = wrapperID
	s.logger.Info("on-demand Factory target dispatch completed",
		zap.String("factoryTargetId", request.Source.FactoryID), zap.String("status", string(result.Status)))
	return result, nil
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

// InvokeFactoryTarget synchronously invokes the exact runtime a prior
// StartFactoryTarget call opened for sessionID.
func (s *Service) InvokeFactoryTarget(
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
		s.logger.Error("on-demand Factory target invoke failed", zap.Error(err))
		return result, err
	}
	s.logger.Info("on-demand Factory target invoke completed", zap.String("status", string(result.Status)))
	return result, nil
}

// CancelFactoryTarget cancels the exact runtime a prior StartFactoryTarget
// call opened for sessionID.
func (s *Service) CancelFactoryTarget(
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
func (s *Service) CloseFactoryTarget(ctx context.Context, sessionID string) error {
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
		s.logger.Error("on-demand Factory target close failed", zap.Error(err))
	} else {
		s.logger.Info("on-demand Factory target closed")
	}
	return err
}
