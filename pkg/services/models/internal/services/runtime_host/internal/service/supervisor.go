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
)

const (
	DefaultReadinessTimeout      = 30 * time.Second
	DefaultHealthCheckInterval   = 100 * time.Millisecond
	DefaultHealthCheckPath       = "/health"
	supervisedHealthEndpointFlag = "--health-endpoint"
)

type supervisorSettings struct {
	ReadinessTimeout    time.Duration
	HealthCheckInterval time.Duration
	HealthCheckPath     string
	ProcessLauncher     models.HostProcessLauncher
	HealthChecker       healthChecker
	Clock               models.HostClock
	ServerStartBuilder  func(
		supervisedIdentity,
		cacheInspection,
		*models.RuntimeWorker,
	) (models.HostProcessStartSpec, error)
	Diagnostics hostDiagnostics
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
	process      models.HostManagedProcess
	loadDone     chan struct{}
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
		if r.endpoint != "" {
			snapshot.Diagnostics["endpoint"] = r.endpoint
		}
		return snapshot
	case supervisedStateLoading:
		snapshot := base.Clone()
		snapshot.Scope = scope
		snapshot.ModelName = modelName
		snapshot.ReadinessState = models.ReadinessStateLoading
		snapshot.LifecycleState = models.LifecycleStateLoading
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
		return snapshot
	default:
		return base
	}
}

func (r *supervisedRuntime) ensureReady(
	ctx context.Context,
	identity supervisedIdentity,
	spec models.HostProcessStartSpec,
) error {
	if err := ctx.Err(); err != nil {
		return cancelHostError(err)
	}

	if err := r.awaitExistingLoad(ctx); err != nil {
		if errors.Is(err, errReadyAlready) {
			return nil
		}
		return err
	}

	r.mu.Lock()
	if r.state == supervisedStateReady {
		r.mu.Unlock()
		return nil
	}
	if r.state == supervisedStateFailed {
		err := r.failureOutcomeLocked()
		r.mu.Unlock()
		return err
	}
	r.identity = identity
	r.state = supervisedStateLoading
	r.failureClass = hostFailureClassNone
	r.failureErr = nil
	r.loadDone = make(chan struct{})
	loadDone := r.loadDone
	r.mu.Unlock()
	defer close(loadDone)

	r.cfg.Diagnostics.logLoadStarted(identity)

	process, err := r.cfg.ProcessLauncher.Start(ctx, spec)
	if err != nil {
		return r.markFailed(
			identity,
			hostFailureClassProcessCrash,
			fmt.Errorf("%w: %v", models.ErrHostProcessCrash, err),
		)
	}

	deadline := r.cfg.Clock.Now().Add(r.cfg.ReadinessTimeout)
	for {
		if err := ctx.Err(); err != nil {
			_ = process.Stop(context.Background())
			return r.markFailed(identity, hostFailureClassCancelled, cancelHostError(err))
		}
		if checkErr := r.cfg.HealthChecker.Check(ctx, process.HealthEndpoint()); checkErr == nil {
			r.mu.Lock()
			r.state = supervisedStateReady
			r.endpoint = process.HealthEndpoint()
			r.process = process
			r.failureClass = hostFailureClassNone
			r.failureErr = nil
			r.mu.Unlock()
			r.cfg.Diagnostics.logLoadReady(identity)
			go r.watchProcessExit(identity, process)
			return nil
		}
		if r.cfg.Clock.Now().After(deadline) {
			_ = process.Stop(context.Background())
			return r.markFailed(identity, hostFailureClassLoadingTimeout, models.ErrHostLoadingTimeout)
		}
		timer := r.cfg.Clock.NewTimer(r.cfg.HealthCheckInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = process.Stop(context.Background())
			return r.markFailed(identity, hostFailureClassCancelled, cancelHostError(ctx.Err()))
		case <-timer.C():
		}
	}
}

var errReadyAlready = errors.New("model host runtime already ready")

func (r *supervisedRuntime) awaitExistingLoad(ctx context.Context) error {
	r.mu.Lock()
	switch r.state {
	case supervisedStateReady:
		r.mu.Unlock()
		return errReadyAlready
	case supervisedStateLoading:
		loadDone := r.loadDone
		r.mu.Unlock()
		return r.waitForLoad(ctx, loadDone)
	case supervisedStateFailed:
		err := r.failureOutcomeLocked()
		r.mu.Unlock()
		return err
	default:
		r.mu.Unlock()
		return nil
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
		default:
			return models.ErrHostRuntimeNotReady
		}
	case <-ctx.Done():
		return cancelHostError(ctx.Err())
	}
}

func (r *supervisedRuntime) markFailed(
	identity supervisedIdentity,
	class hostFailureClass,
	err error,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = supervisedStateFailed
	r.failureClass = class
	r.failureErr = err
	r.endpoint = ""
	r.process = nil
	r.cfg.Diagnostics.logLoadFailed(identity, class, err)
	return r.failureOutcomeLocked()
}

func (r *supervisedRuntime) failureOutcomeLocked() error {
	if r.failureErr == nil {
		return models.ErrHostRuntimeNotReady
	}
	return r.failureErr
}

func (r *supervisedRuntime) watchProcessExit(identity supervisedIdentity, process models.HostManagedProcess) {
	waitErr := process.Wait()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.process != process || r.state != supervisedStateReady {
		return
	}
	r.state = supervisedStateFailed
	r.failureClass = hostFailureClassProcessCrash
	if waitErr != nil {
		r.failureErr = fmt.Errorf("%w: %v", models.ErrHostProcessCrash, waitErr)
	} else {
		r.failureErr = models.ErrHostProcessCrash
	}
	r.endpoint = ""
	r.process = nil
	r.cfg.Diagnostics.logProcessCrash(identity, r.failureErr)
}

func (r *supervisedRuntime) isReady() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state == supervisedStateReady
}

// HTTPHealthChecker probes readiness through HTTP GET on a health endpoint.
type HTTPHealthChecker struct {
	Client models.HostHTTPDoer
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
