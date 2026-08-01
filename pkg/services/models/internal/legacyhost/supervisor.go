package modelhost

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
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
)

const (
	DefaultReadinessTimeout      = 30 * time.Second
	DefaultHealthCheckInterval   = 100 * time.Millisecond
	DefaultHealthCheckPath       = "/health"
	supervisedHealthEndpointFlag = "--health-endpoint"
)

// ProcessStartSpec describes one supervised model-host subprocess.
type ProcessStartSpec struct {
	Command        string
	Args           []string
	Env            []string
	WorkDir        string
	HealthEndpoint string
}

// ManagedProcess is the process handle owned by the model host.
type ManagedProcess interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}

// ProcessLauncher starts one supervised model-host subprocess.
type ProcessLauncher interface {
	Start(context.Context, ProcessStartSpec) (ManagedProcess, error)
}

// HTTPDoer is the exact external HTTP effect used by model-host readiness
// probes.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Timer is the retry wait effect used by model-host readiness polling.
type Timer interface {
	C() <-chan time.Time
	Stop() bool
}

// Clock supplies deadlines and retry timers for model-host readiness.
type Clock interface {
	Now() time.Time
	NewTimer(time.Duration) Timer
}

// HealthChecker probes model-server readiness through a health endpoint.
type HealthChecker interface {
	Check(ctx context.Context, healthEndpoint string) error
}

// ServerStartBuilder resolves one supervised process launch spec from installed assets.
type ServerStartBuilder func(identity Identity, inspection CacheInspection, worker *models.RuntimeWorker) (ProcessStartSpec, error)

type supervisorSettings struct {
	ReadinessTimeout    time.Duration
	HealthCheckInterval time.Duration
	HealthCheckPath     string
	ProcessLauncher     ProcessLauncher
	HealthChecker       HealthChecker
	Clock               Clock
	ServerStartBuilder  ServerStartBuilder
	Diagnostics         Diagnostics
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
	failureClass FailureClass
	failureErr   error
	endpoint     string
	process      ManagedProcess
	loadDone     chan struct{}
	cfg          supervisorSettings
	identity     Identity
}

func requiresSupervisedBackend(identity Identity) bool {
	return canonicalBackendName(identity.Backend) == "LLAMACPP"
}

func canonicalBackendName(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func (r *supervisedRuntime) readinessOverlay(identity Identity, base ReadinessSnapshot) ReadinessSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch r.state {
	case supervisedStateReady:
		return ReadinessSnapshot{
			Identity:       identity,
			ReadinessState: managedruntime.ReadinessStateReady,
			LifecycleState: managedruntime.LifecycleStateLoaded,
			FailureClass:   FailureClassNone,
			Diagnostics: mergeDiagnostics(identity, managedruntime.ReadinessStateReady, managedruntime.LifecycleStateLoaded, map[string]string{
				"endpoint": r.endpoint,
			}),
		}
	case supervisedStateLoading:
		return ReadinessSnapshot{
			Identity:       identity,
			ReadinessState: managedruntime.ReadinessStateLoading,
			LifecycleState: managedruntime.LifecycleStateLoading,
			FailureClass:   FailureClassLoadingTimeout,
			Diagnostics:    managedDiagnostics(identity, managedruntime.ReadinessStateLoading, managedruntime.LifecycleStateLoading),
		}
	case supervisedStateFailed:
		readiness := ReadinessStateForFailureClass(r.failureClass)
		lifecycle := managedruntime.LifecycleStateLoaded
		if r.failureClass == FailureClassLoadingTimeout {
			lifecycle = managedruntime.LifecycleStateLoading
		}
		if r.failureClass == FailureClassMissingAssets {
			lifecycle = managedruntime.LifecycleStateNotInstalled
		}
		return ReadinessSnapshot{
			Identity:       identity,
			ReadinessState: readiness,
			LifecycleState: lifecycle,
			FailureClass:   r.failureClass,
			Diagnostics:    managedDiagnostics(identity, readiness, lifecycle),
		}
	default:
		if base.ReadinessState == managedruntime.ReadinessStateReady {
			return ReadinessSnapshot{
				Identity:       identity,
				ReadinessState: managedruntime.ReadinessStateLoading,
				LifecycleState: managedruntime.LifecycleStateLoading,
				FailureClass:   FailureClassLoadingTimeout,
				Diagnostics:    managedDiagnostics(identity, managedruntime.ReadinessStateLoading, managedruntime.LifecycleStateLoading),
			}
		}
		return base
	}
}

func (r *supervisedRuntime) ensureReady(ctx context.Context, identity Identity, spec ProcessStartSpec) error {
	if err := ctx.Err(); err != nil {
		return cancelError(err)
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
	r.failureClass = FailureClassNone
	r.failureErr = nil
	r.loadDone = make(chan struct{})
	loadDone := r.loadDone
	r.mu.Unlock()
	defer close(loadDone)

	r.cfg.Diagnostics.logLoadStarted(identity)

	process, err := r.cfg.ProcessLauncher.Start(ctx, spec)
	if err != nil {
		return r.markFailed(identity, FailureClassProcessCrash, fmt.Errorf("%w: %v", ErrProcessCrash, err))
	}

	deadline := r.cfg.Clock.Now().Add(r.cfg.ReadinessTimeout)
	for {
		if err := ctx.Err(); err != nil {
			_ = process.Stop(context.Background())
			return r.markFailed(identity, FailureClassCancelled, cancelError(err))
		}
		if checkErr := r.cfg.HealthChecker.Check(ctx, process.HealthEndpoint()); checkErr == nil {
			r.mu.Lock()
			r.state = supervisedStateReady
			r.endpoint = process.HealthEndpoint()
			r.process = process
			r.failureClass = FailureClassNone
			r.failureErr = nil
			r.mu.Unlock()
			r.cfg.Diagnostics.logLoadReady(identity)
			go r.watchProcessExit(identity, process)
			return nil
		}
		if r.cfg.Clock.Now().After(deadline) {
			_ = process.Stop(context.Background())
			return r.markFailed(identity, FailureClassLoadingTimeout, ErrLoadingTimeout)
		}
		timer := r.cfg.Clock.NewTimer(r.cfg.HealthCheckInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = process.Stop(context.Background())
			return r.markFailed(identity, FailureClassCancelled, cancelError(ctx.Err()))
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
			return ErrRuntimeNotReady
		}
	case <-ctx.Done():
		return cancelError(ctx.Err())
	}
}

func (r *supervisedRuntime) markFailed(identity Identity, class FailureClass, err error) error {
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

func (r *supervisedRuntime) failureOutcome() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.failureOutcomeLocked()
}

func (r *supervisedRuntime) failureOutcomeLocked() error {
	if r.failureErr == nil {
		return &ReadinessError{
			Snapshot: ReadinessSnapshot{FailureClass: r.failureClass},
			Cause:    ErrRuntimeNotReady,
		}
	}
	return &ReadinessError{
		Snapshot: ReadinessSnapshot{
			FailureClass: r.failureClass,
		},
		Cause: r.failureErr,
	}
}

func (r *supervisedRuntime) watchProcessExit(identity Identity, process ManagedProcess) {
	waitErr := process.Wait()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.process != process || r.state != supervisedStateReady {
		return
	}
	r.state = supervisedStateFailed
	r.failureClass = FailureClassProcessCrash
	if waitErr != nil {
		r.failureErr = fmt.Errorf("%w: %v", ErrProcessCrash, waitErr)
	} else {
		r.failureErr = ErrProcessCrash
	}
	r.endpoint = ""
	r.process = nil
	r.cfg.Diagnostics.logProcessCrash(identity, r.failureErr)
}

func (r *supervisedRuntime) stop(ctx context.Context) error {
	r.mu.Lock()
	process := r.process
	r.process = nil
	r.endpoint = ""
	r.state = supervisedStateAbsent
	r.failureClass = FailureClassNone
	r.failureErr = nil
	r.mu.Unlock()

	if process == nil {
		return nil
	}
	return process.Stop(ctx)
}

func (r *supervisedRuntime) endpointValue() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.endpoint
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

func (r *supervisedRuntime) isReady() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state == supervisedStateReady
}

func (r *supervisedRuntime) isLoading() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state == supervisedStateLoading
}

// HTTPHealthChecker probes readiness through HTTP GET on a health endpoint.
type HTTPHealthChecker struct {
	Client HTTPDoer
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
	if strings.Contains(trimmed, "://") && (strings.HasSuffix(trimmed, "/health") || strings.Contains(trimmed, "/health?")) {
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

func DefaultServerStartBuilder(
	identity Identity,
	inspection CacheInspection,
	worker *models.RuntimeWorker,
) (ProcessStartSpec, error) {
	if worker == nil {
		return ProcessStartSpec{}, fmt.Errorf("local model worker is required for supervised backend %q", identity.Backend)
	}
	command := strings.TrimSpace(worker.Command)
	if command == "" {
		command = localmodels.DefaultOmniVoiceCommand
	}
	healthEndpoint, args, err := supervisedHealthEndpointAndArgs(worker.Args)
	if err != nil {
		return ProcessStartSpec{}, err
	}
	if strings.TrimSpace(inspection.CachePath) == "" {
		return ProcessStartSpec{}, fmt.Errorf("%w: cache path is required for supervised runtime %q", ErrMissingAssets, identity.Name)
	}
	args = append([]string{"serve"}, args...)
	args = append(args, "--cache-path", inspection.CachePath)
	return ProcessStartSpec{
		Command:        command,
		Args:           args,
		HealthEndpoint: healthEndpoint,
	}, nil
}

func supervisedHealthEndpointAndArgs(workerArgs []string) (string, []string, error) {
	args := append([]string(nil), workerArgs...)
	for i := 0; i < len(args); i++ {
		if args[i] != supervisedHealthEndpointFlag {
			continue
		}
		if i+1 >= len(args) {
			return "", nil, fmt.Errorf("flag %q requires a value", supervisedHealthEndpointFlag)
		}
		endpoint := strings.TrimSpace(args[i+1])
		remaining := append(append([]string(nil), args[:i]...), args[i+2:]...)
		if endpoint == "" {
			return "", nil, fmt.Errorf("flag %q requires a non-empty value", supervisedHealthEndpointFlag)
		}
		return endpoint, remaining, nil
	}
	return "", args, fmt.Errorf("supervised llama.cpp runtime requires worker arg %q", supervisedHealthEndpointFlag)
}

func workerDeclaresSupervisedHealthEndpoint(worker *models.RuntimeWorker) bool {
	if worker == nil {
		return false
	}
	_, _, err := supervisedHealthEndpointAndArgs(worker.Args)
	return err == nil
}
