package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

const (
	DefaultReadinessTimeout      = 30 * time.Second
	DefaultHealthCheckInterval   = 100 * time.Millisecond
	DefaultHealthCheckPath       = "/health"
	supervisedHealthEndpointFlag = "--health-endpoint"
)

type supervisorSettings struct {
	ReadinessTimeout     time.Duration
	HealthCheckInterval  time.Duration
	HealthCheckPath      string
	ProcessLauncher      modelseffects.HostProcessLauncher
	HealthChecker        healthChecker
	ProtocolNegotiator   modelseffects.HostProtocolNegotiator
	CompatibilityChecker modelseffects.HostCompatibilityChecker
	ResolveSymlinks      modelseffects.HostResolveSymlinks
	Platform             models.AssetHostPlatform
	Clock                modelseffects.HostClock
	ServerStartBuilder   func(
		supervisedIdentity,
		cacheInspection,
		*models.RuntimeWorker,
	) (modelseffects.HostProcessStartSpec, error)
	Diagnostics               hostDiagnostics
	afterLoadStateObservation func()
	onProcessFailure          func()
}

type healthChecker interface {
	Check(ctx context.Context, healthEndpoint string) error
}

type supervisedState string

const (
	supervisedStateAbsent  supervisedState = "absent"
	supervisedStateLoading supervisedState = "loading"
	supervisedStateReady   supervisedState = "ready"
	supervisedStateFailed  supervisedState = "failed"
)

type supervisedRuntime struct {
	mu           sync.Mutex
	state        supervisedState
	failureClass hostFailureClass
	failureErr   error
	endpoint     string
	process      modelseffects.HostManagedProcess
	loadDone     chan struct{}
	loadCancel   context.CancelFunc
	cfg          supervisorSettings
	identity     supervisedIdentity
}

func (r *supervisedRuntime) hostSnapshotOverlay(
	scope models.RuntimeScopeRef,
	modelName string,
	base models.ModelHostSnapshot,
) models.ModelHostSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch r.state {
	case supervisedStateReady:
		snapshot := base.Clone()
		snapshot.Scope = scope
		snapshot.ModelName = modelName
		snapshot.ReadinessState = models.ReadinessStateReady
		snapshot.LifecycleState = models.LifecycleStateLoaded
		if r.endpoint != "" && !requiresPinnedGRPCBackend(r.identity.Backend) {
			snapshot.Diagnostics["endpoint"] = r.endpoint
		}
		if requiresPinnedGRPCBackend(r.identity.Backend) {
			delete(snapshot.Diagnostics, "cachePath")
		}
		return snapshot
	case supervisedStateLoading:
		snapshot := base.Clone()
		snapshot.Scope = scope
		snapshot.ModelName = modelName
		snapshot.ReadinessState = models.ReadinessStateLoading
		snapshot.LifecycleState = models.LifecycleStateLoading
		if requiresPinnedGRPCBackend(r.identity.Backend) {
			delete(snapshot.Diagnostics, "cachePath")
		}
		return snapshot
	case supervisedStateFailed:
		snapshot := base.Clone()
		snapshot.Scope = scope
		snapshot.ModelName = modelName
		switch r.failureClass {
		case hostFailureClassLoadingTimeout:
			snapshot.ReadinessState = models.ReadinessStateLoading
			snapshot.LifecycleState = models.LifecycleStateLoading
		case hostFailureClassProcessCrash, hostFailureClassCancelled:
			snapshot.ReadinessState = models.ReadinessStateFailed
			snapshot.LifecycleState = models.LifecycleStateLoaded
		default:
			snapshot.ReadinessState = models.ReadinessStateFailed
			snapshot.LifecycleState = models.LifecycleStateLoaded
		}
		if r.failureClass != hostFailureClassNone {
			snapshot.Diagnostics["failureClass"] = string(r.failureClass)
		}
		if requiresPinnedGRPCBackend(r.identity.Backend) {
			delete(snapshot.Diagnostics, "cachePath")
		}
		return snapshot
	default:
		return base
	}
}

func (r *supervisedRuntime) ensureReady(
	ctx context.Context,
	identity supervisedIdentity,
	spec modelseffects.HostProcessStartSpec,
) error {
	if err := ctx.Err(); err != nil {
		return cancelHostError(err)
	}
	loadCtx, loadCancel := context.WithCancel(ctx)
	loadDone, waitDone, alreadyReady := r.beginLoad(identity, loadCancel)
	if alreadyReady {
		loadCancel()
		r.notifyAfterLoadStateObservation()
		return nil
	}
	if waitDone != nil {
		loadCancel()
		r.notifyAfterLoadStateObservation()
		return r.waitForLoad(ctx, waitDone)
	}
	r.notifyAfterLoadStateObservation()
	defer r.finishLoad(loadDone, loadCancel)
	return r.startLoad(loadCtx, loadDone, identity, spec)
}

func (r *supervisedRuntime) beginLoad(
	identity supervisedIdentity,
	loadCancel context.CancelFunc,
) (loadDone, waitDone chan struct{}, alreadyReady bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch r.state {
	case supervisedStateReady:
		return nil, nil, true
	case supervisedStateLoading:
		return nil, r.loadDone, false
	case supervisedStateFailed:
		// A failed process is a completed attempt, not a permanent slot
		// decision. Reset the attempt so a later invocation can recover after a
		// crash or startup/readiness failure.
	}
	r.identity = identity
	r.state = supervisedStateLoading
	r.failureClass = hostFailureClassNone
	r.failureErr = nil
	r.loadDone = make(chan struct{})
	r.loadCancel = loadCancel
	return r.loadDone, nil, false
}

func (r *supervisedRuntime) startLoad(
	ctx context.Context,
	loadDone chan struct{},
	identity supervisedIdentity,
	spec modelseffects.HostProcessStartSpec,
) error {
	r.cfg.Diagnostics.logLoadStarted(identity)

	process, err := r.cfg.ProcessLauncher.Start(ctx, spec)
	if err != nil {
		if process != nil {
			_ = process.Stop(context.Background())
		}
		return r.markFailed(
			loadDone,
			identity,
			hostFailureClassProcessCrash,
			fmt.Errorf("%w: %v", models.ErrHostProcessCrash, err),
		)
	}
	if process == nil {
		return r.markFailed(
			loadDone,
			identity,
			hostFailureClassProcessCrash,
			models.ErrHostProcessCrash,
		)
	}
	processExit := make(chan error, 1)
	go func() {
		processExit <- process.Wait()
	}()
	if !r.setProcess(loadDone, process) {
		_ = process.Stop(context.Background())
		return loadCancelledOutcome()
	}
	if err := r.waitForReadiness(ctx, loadDone, identity, spec, process, processExit); err != nil {
		return err
	}
	if !r.markReady(loadDone, identity, process, processExit) {
		_ = process.Stop(context.Background())
		return loadCancelledOutcome()
	}
	return nil
}

func (r *supervisedRuntime) setProcess(
	loadDone chan struct{},
	process modelseffects.HostManagedProcess,
) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state != supervisedStateLoading || r.loadDone != loadDone {
		return false
	}
	r.process = process
	r.endpoint = process.HealthEndpoint()
	return true
}

func (r *supervisedRuntime) waitForReadiness(
	ctx context.Context,
	loadDone chan struct{},
	identity supervisedIdentity,
	spec modelseffects.HostProcessStartSpec,
	process modelseffects.HostManagedProcess,
	processExit <-chan error,
) error {
	deadline := r.cfg.Clock.Now().Add(r.cfg.ReadinessTimeout)
	var lastReadinessErr error
	for {
		if !r.isCurrentLoad(loadDone, process) {
			_ = process.Stop(context.Background())
			return loadCancelledOutcome()
		}
		if waitErr, exited := processExitResult(processExit); exited {
			return r.markFailed(loadDone, identity, hostFailureClassProcessCrash, processExitError(waitErr))
		}
		if err := ctx.Err(); err != nil {
			_ = process.Stop(context.Background())
			return r.markFailed(loadDone, identity, hostFailureClassCancelled, cancelHostError(err))
		}
		ready, checkErr := r.checkReadiness(ctx, identity, spec, process)
		lastReadinessErr = checkErr
		if errors.Is(checkErr, models.ErrHostProtocolIncompatible) {
			_ = process.Stop(context.Background())
			return r.markFailed(loadDone, identity, hostFailureClassProtocol, checkErr)
		}
		if ready {
			if waitErr, exited := processExitResult(processExit); exited {
				return r.markFailed(loadDone, identity, hostFailureClassProcessCrash, processExitError(waitErr))
			}
			return nil
		}
		if r.cfg.Clock.Now().After(deadline) {
			_ = process.Stop(context.Background())
			if errors.Is(lastReadinessErr, models.ErrHostUnsupportedPlatform) {
				return r.markFailed(loadDone, identity, hostFailureClassUnsupportedPlatform, lastReadinessErr)
			}
			return r.markFailed(loadDone, identity, hostFailureClassLoadingTimeout, models.ErrHostLoadingTimeout)
		}
		if err := r.waitForReadinessInterval(ctx, loadDone, identity, process, processExit); err != nil {
			return err
		}
	}
}

func processExitResult(processExit <-chan error) (error, bool) {
	select {
	case waitErr := <-processExit:
		return waitErr, true
	default:
		return nil, false
	}
}

func (r *supervisedRuntime) waitForReadinessInterval(
	ctx context.Context,
	loadDone chan struct{},
	identity supervisedIdentity,
	process modelseffects.HostManagedProcess,
	processExit <-chan error,
) error {
	timer := r.cfg.Clock.NewTimer(r.cfg.HealthCheckInterval)
	select {
	case waitErr := <-processExit:
		timer.Stop()
		return r.markFailed(loadDone, identity, hostFailureClassProcessCrash, processExitError(waitErr))
	case <-ctx.Done():
		timer.Stop()
		_ = process.Stop(context.Background())
		return r.markFailed(loadDone, identity, hostFailureClassCancelled, cancelHostError(ctx.Err()))
	case <-timer.C():
		return nil
	}
}

func (r *supervisedRuntime) markReady(
	loadDone chan struct{},
	identity supervisedIdentity,
	process modelseffects.HostManagedProcess,
	processExit <-chan error,
) bool {
	r.mu.Lock()
	if r.state != supervisedStateLoading || r.loadDone != loadDone || r.process != process {
		r.mu.Unlock()
		return false
	}
	r.state = supervisedStateReady
	r.failureClass = hostFailureClassNone
	r.failureErr = nil
	r.mu.Unlock()
	r.cfg.Diagnostics.logLoadReady(identity)
	go r.watchProcessExit(identity, process, processExit)
	return true
}

func processExitError(waitErr error) error {
	if waitErr == nil {
		return models.ErrHostProcessCrash
	}
	return fmt.Errorf("%w: %v", models.ErrHostProcessCrash, waitErr)
}

func (r *supervisedRuntime) checkReadiness(
	ctx context.Context,
	identity supervisedIdentity,
	spec modelseffects.HostProcessStartSpec,
	process modelseffects.HostManagedProcess,
) (bool, error) {
	if requiresPinnedGRPCBackend(identity.Backend) {
		if r.cfg.ProtocolNegotiator == nil {
			return false, models.ErrHostProtocolIncompatible
		}
		negotiated, err := r.cfg.ProtocolNegotiator.Negotiate(
			ctx,
			process.HealthEndpoint(),
			modelseffects.HostProtocolNegotiationRequest{
				ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
				Backend:         identity.Backend,
				ModelName:       identity.Name,
				Revision:        identity.Revision,
				Platform:        r.cfg.Platform,
				ModelPath:       strings.TrimSpace(spec.ModelPath),
				ModelFiles:      append([]string(nil), spec.ModelFiles...),
			},
		)
		if err != nil {
			return false, err
		}
		if negotiated.ProtocolVersion != modelseffects.PinnedHostProtocolVersion ||
			!sameBackend(negotiated.Backend, identity.Backend) {
			return false, models.ErrHostProtocolIncompatible
		}
		return negotiated.Ready, nil
	}
	if r.cfg.HealthChecker == nil {
		return false, models.ErrHostRuntimeNotReady
	}
	if err := r.cfg.HealthChecker.Check(ctx, process.HealthEndpoint()); err != nil {
		return false, err
	}
	return true, nil
}

func (r *supervisedRuntime) notifyAfterLoadStateObservation() {
	if r.cfg.afterLoadStateObservation != nil {
		r.cfg.afterLoadStateObservation()
	}
}

func (r *supervisedRuntime) waitForLoad(ctx context.Context, loadDone chan struct{}) error {
	select {
	case <-loadDone:
		r.mu.Lock()
		defer r.mu.Unlock()
		switch r.state {
		case supervisedStateReady:
			return nil
		case supervisedStateFailed:
			return r.failureOutcomeLocked()
		case supervisedStateAbsent:
			return loadCancelledOutcome()
		default:
			return models.ErrHostRuntimeNotReady
		}
	case <-ctx.Done():
		return cancelHostError(ctx.Err())
	}
}

func (r *supervisedRuntime) markFailed(
	loadDone chan struct{},
	identity supervisedIdentity,
	class hostFailureClass,
	err error,
) error {
	r.mu.Lock()
	if r.state != supervisedStateLoading || r.loadDone != loadDone {
		r.mu.Unlock()
		return loadCancelledOutcome()
	}
	r.state = supervisedStateFailed
	r.failureClass = class
	r.failureErr = typedHostReadinessFailure(identity, class, err)
	r.endpoint = ""
	r.process = nil
	failure := r.failureOutcomeLocked()
	r.mu.Unlock()
	r.cfg.Diagnostics.logLoadFailed(identity, class, err)
	if r.cfg.onProcessFailure != nil {
		r.cfg.onProcessFailure()
	}
	return failure
}

func (r *supervisedRuntime) finishLoad(loadDone chan struct{}, loadCancel context.CancelFunc) {
	r.mu.Lock()
	if r.loadDone == loadDone {
		r.loadDone = nil
		r.loadCancel = nil
	}
	close(loadDone)
	r.mu.Unlock()
	loadCancel()
}

func (r *supervisedRuntime) isCurrentLoad(
	loadDone chan struct{},
	process modelseffects.HostManagedProcess,
) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state == supervisedStateLoading && r.loadDone == loadDone && r.process == process
}

func loadCancelledOutcome() error {
	return cancelHostError(context.Canceled)
}

func (r *supervisedRuntime) failureOutcomeLocked() error {
	if r.failureErr == nil {
		return models.ErrHostRuntimeNotReady
	}
	return r.failureErr
}

func (r *supervisedRuntime) failureOutcome() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.failureOutcomeLocked()
}

func (r *supervisedRuntime) watchProcessExit(
	identity supervisedIdentity,
	process modelseffects.HostManagedProcess,
	processExit <-chan error,
) {
	waitErr := <-processExit
	r.mu.Lock()
	if r.process != process || r.state != supervisedStateReady {
		r.mu.Unlock()
		return
	}
	r.state = supervisedStateFailed
	r.failureClass = hostFailureClassProcessCrash
	r.failureErr = typedHostReadinessFailure(
		identity,
		hostFailureClassProcessCrash,
		processExitError(waitErr),
	)
	r.endpoint = ""
	r.process = nil
	failureErr := r.failureErr
	r.mu.Unlock()
	r.cfg.Diagnostics.logProcessCrash(identity, failureErr)
	if r.cfg.onProcessFailure != nil {
		r.cfg.onProcessFailure()
	}
}

func (r *supervisedRuntime) isReady() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state == supervisedStateReady
}

func (r *supervisedRuntime) invocationEndpoint() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state != supervisedStateReady {
		return ""
	}
	return strings.TrimSpace(r.endpoint)
}

func (r *supervisedRuntime) isLoading() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state == supervisedStateLoading
}

func (r *supervisedRuntime) isResident() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch r.state {
	case supervisedStateLoading, supervisedStateReady, supervisedStateFailed:
		return true
	default:
		return false
	}
}

func (r *supervisedRuntime) stop(ctx context.Context) error {
	r.mu.Lock()
	process := r.process
	loadDone := r.loadDone
	loadCancel := r.loadCancel
	r.process = nil
	r.endpoint = ""
	r.state = supervisedStateAbsent
	r.failureClass = hostFailureClassNone
	r.failureErr = nil
	r.loadDone = nil
	r.loadCancel = nil
	r.mu.Unlock()

	if loadCancel != nil {
		loadCancel()
	}
	var stopErr error
	if process != nil {
		stopErr = process.Stop(ctx)
	}
	if loadDone != nil {
		<-loadDone
	}
	return stopErr
}

// HTTPHealthChecker probes readiness through HTTP GET on a health endpoint.
type HTTPHealthChecker struct {
	Client modelseffects.HostHTTPDoer
	Path   string
}

func (h HTTPHealthChecker) Check(ctx context.Context, healthEndpoint string) error {
	url := healthEndpointURL(healthEndpoint, h.Path)
	client := h.Client
	if client == nil {
		return fmt.Errorf("model host health HTTP client is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}
	return nil
}

func healthEndpointURL(endpoint string, path string) string {
	trimmed := strings.TrimSpace(endpoint)
	if strings.Contains(trimmed, "://") &&
		(strings.HasSuffix(trimmed, "/health") || strings.Contains(trimmed, "/health?")) {
		return trimmed
	}
	base := strings.TrimRight(trimmed, "/")
	healthPath := strings.TrimSpace(path)
	if healthPath == "" {
		healthPath = DefaultHealthCheckPath
	}
	if !strings.HasPrefix(healthPath, "/") {
		healthPath = "/" + healthPath
	}
	return base + healthPath
}
