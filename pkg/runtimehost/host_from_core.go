package runtimehost

import (
	"context"
	"errors"
	"fmt"
	"sync"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// HostShell is the pre-factorysave Host assembly product for wire composition.
type HostShell struct {
	Host *Host
}

// NewHostFromCore wraps a built core in the runtime/session host returned to
// transports and compatibility callers.
func NewHostFromCore(core *Core) *Host {
	if core == nil {
		return nil
	}
	host := &Host{
		core:             core,
		factoryRootDir:   core.FactoryRootDir(),
		sessions:         core.Sessions(),
		hostedWorkers:    core.HostedWorkers(),
		policy:           CoordinatorPolicyFromConfig(core.cfg),
		startupBundle:    core.StartupBundle(),
		cfg:              core.cfg,
		modelAssets:      core.modelAssets,
		modelService:     core.ModelService(),
		baseLogger:       core.BaseLogger(),
		logger:           core.Logger(),
		clock:            core.Clock(),
		runtimeBuild:     core.RuntimeBuild(),
		workersScheduler: core.WorkersScheduler(),
		durableExecution: core.DurableExecution(),
	}
	host.coordinator = newCoordinator(host)
	host.definitions = newFactoryDefinitionService(host)
	return host
}

// ComposeCollaboratorSnapshot reports initialized collaborators for equivalence tests.
func (h *Host) ComposeCollaboratorSnapshot() ComposeCollaboratorSnapshot {
	if h == nil {
		return ComposeCollaboratorSnapshot{}
	}
	snapshot := ComposeCollaboratorSnapshot{
		ModelServiceInitialized: h.modelService != nil,
		FactorySaveInitialized:  h.factorySave != nil,
		DefinitionsInitialized:  h.definitions != nil,
	}
	if h.core != nil {
		coreSnapshot := h.core.ComposeCollaboratorSnapshot()
		coreSnapshot.ModelServiceInitialized = snapshot.ModelServiceInitialized
		coreSnapshot.FactorySaveInitialized = snapshot.FactorySaveInitialized
		coreSnapshot.DefinitionsInitialized = snapshot.DefinitionsInitialized
		return coreSnapshot
	}
	bundle := h.currentRuntimeBundle()
	snapshot.SessionsInitialized = h.sessions != nil
	snapshot.RuntimeBuildInitialized = h.runtimeBuild != nil
	snapshot.WorkersSchedulerInitialized = h.workersScheduler != nil
	snapshot.ModelAssetsInitialized = h.modelAssets != nil
	snapshot.HostedWorkersLoggerReady = h.hostedWorkers.Logger != nil
	if bundle != nil {
		snapshot.BundleModelResources = bundle.ModelResources != nil
		snapshot.BundleLocalModels = bundle.LocalModels != nil
	}
	return snapshot
}

// ApplicationRuntime is the graph-owned runtime lifecycle coordinator. It
// exposes construction-complete runtime, worker/watcher, and foreground
// transport edges without letting a transport recompose or start sidecars.
type ApplicationRuntime struct {
	host *Host

	mu             sync.Mutex
	runCtx         context.Context
	cancelRun      context.CancelFunc
	current        *liveRuntimeHandle
	workerCancel   context.CancelFunc
	workerSidecars sync.WaitGroup
}

// NewApplicationRuntime constructs inert lifecycle state around one host.
func NewApplicationRuntime(host *Host) (*ApplicationRuntime, error) {
	if host == nil {
		return nil, errors.New("construct application runtime: host is required")
	}
	return &ApplicationRuntime{host: host}, nil
}

// StartRuntime starts only the default runtime. Worker/watcher sidecars and
// transports are started through their separate graph lifecycle handles.
func (r *ApplicationRuntime) StartRuntime(ctx context.Context) error {
	if r == nil || r.host == nil {
		return errors.New("start application runtime: coordinator is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	if r.runCtx != nil {
		r.mu.Unlock()
		return errors.New("start application runtime: already started")
	}
	r.runCtx, r.cancelRun = context.WithCancel(ctx)
	r.mu.Unlock()
	r.host.startTime = r.host.clock.Now()
	serviceMode := runtimeModeOrDefault(r.host.cfg.RuntimeMode) == interfaces.RuntimeModeService
	if err := r.host.prepareRunInputs(ctx, serviceMode); err != nil {
		_ = r.StopRuntime(context.Background())
		return err
	}
	current, err := r.host.StartDefaultRuntime(ctx, r.runCtx, false)
	if err != nil {
		_ = r.StopRuntime(context.Background())
		return err
	}
	r.mu.Lock()
	r.current = current
	r.mu.Unlock()
	return nil
}

// StartWorkers activates the watcher, metrics observer, and worker scheduler
// sidecars for the runtime already started by StartRuntime.
func (r *ApplicationRuntime) StartWorkers(ctx context.Context) error {
	if r == nil || r.host == nil {
		return errors.New("start application workers: coordinator is required")
	}
	r.mu.Lock()
	if r.workerCancel != nil {
		r.mu.Unlock()
		return errors.New("start application workers: already started")
	}
	current := r.current
	workerCtx, cancel := context.WithCancel(ctx)
	r.workerCancel = cancel
	r.mu.Unlock()
	if current == nil || current.Bundle == nil {
		cancel()
		return errors.New("start application workers: runtime is not started")
	}
	serviceMode := runtimeModeOrDefault(r.host.cfg.RuntimeMode) == interfaces.RuntimeModeService
	if serviceMode {
		if err := r.host.StartLiveRuntimeSidecars(workerCtx, current); err != nil {
			cancel()
			return err
		}
		return nil
	}
	r.host.startListenerSidecar(workerCtx, &r.workerSidecars, current.Bundle.Listener, r.host.logger)
	return nil
}

// StopWorkers stops and joins every worker/watcher sidecar owned by the graph.
func (r *ApplicationRuntime) StopWorkers(context.Context) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	cancel, current := r.workerCancel, r.current
	r.workerCancel = nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if current != nil {
		r.host.StopLiveRuntimeSidecars(current)
	}
	r.workerSidecars.Wait()
	return nil
}

// RunTransport owns only the API/CLI foreground edge and wait behavior. It
// assumes initializer already started graph-owned runtime and worker sidecars.
func (r *ApplicationRuntime) RunTransport(ctx context.Context, surface apisurface.APISurface) error {
	if r == nil || r.host == nil {
		return errors.New("run application transport: coordinator is required")
	}
	transportCtx, cancel := context.WithCancel(ctx)
	var transport sync.WaitGroup
	defer func() {
		cancel()
		transport.Wait()
	}()
	r.host.startAPIServerSidecar(transportCtx, &transport, surface)
	r.mu.Lock()
	current := r.current
	r.mu.Unlock()
	serviceMode := runtimeModeOrDefault(r.host.cfg.RuntimeMode) == interfaces.RuntimeModeService
	if err := r.host.waitForServiceModeStartupWorkReadability(ctx, serviceMode); err != nil {
		return r.host.failServiceModeStartup(current, err)
	}
	if err := r.host.submitServiceModeWorkFile(ctx, current, serviceMode); err != nil {
		return err
	}
	r.host.logServiceStartup()
	if err := r.waitForRuntime(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("factory run: %w", err)
	}
	return nil
}

func (r *ApplicationRuntime) waitForRuntime(ctx context.Context) error {
	for {
		current := r.host.currentLiveRuntime()
		if current == nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case apiErr := <-r.host.apiServerExit:
			if apiErr == nil {
				return errors.New("API server stopped unexpectedly")
			}
			return fmt.Errorf("API server failed: %w", apiErr)
		case <-current.RunDone:
		}
		if r.host.currentLiveRuntime() != current {
			continue
		}
		if runtimeModeOrDefault(r.host.cfg.RuntimeMode) == interfaces.RuntimeModeService && r.host.sessions != nil && r.host.sessions.Count() == 0 {
			continue
		}
		return current.Result()
	}
}

// StopRuntime stops the active runtime and every additional live session.
func (r *ApplicationRuntime) StopRuntime(context.Context) error {
	if r == nil || r.host == nil {
		return nil
	}
	r.mu.Lock()
	cancel := r.cancelRun
	r.cancelRun = nil
	r.runCtx = nil
	r.current = nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	current := r.host.currentLiveRuntime()
	var result error
	if err := r.host.StopLiveRuntime(current); err != nil && !errors.Is(err, context.Canceled) {
		result = err
	}
	if err := r.host.ShutdownOtherLiveSessions(current); err != nil {
		result = errors.Join(result, err)
	}
	r.host.clearRunState()
	return result
}
