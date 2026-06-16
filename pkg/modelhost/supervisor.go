package modelhost

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	defaultReadinessTimeout      = 30 * time.Second
	defaultHealthCheckInterval   = 100 * time.Millisecond
	defaultHealthCheckPath       = "/health"
	supervisedHealthEndpointFlag = "--health-endpoint"
)

// ProcessStartSpec describes one supervised local model-server process.
type ProcessStartSpec struct {
	Command        string
	Args           []string
	Env            []string
	WorkDir        string
	HealthEndpoint string
}

// ManagedProcess is a supervised model-server subprocess owned by the model host.
type ManagedProcess interface {
	HealthEndpoint() string
	Wait() error
	Stop(ctx context.Context) error
}

// ProcessLauncher starts a supervised model-server process.
type ProcessLauncher interface {
	Start(ctx context.Context, spec ProcessStartSpec) (ManagedProcess, error)
}

// HealthChecker probes model-server readiness through a health endpoint.
type HealthChecker interface {
	Check(ctx context.Context, healthEndpoint string) error
}

// ServerStartBuilder resolves one supervised process launch spec from installed assets.
type ServerStartBuilder func(identity Identity, inspection CacheInspection, worker *interfaces.WorkerConfig) (ProcessStartSpec, error)

// SupervisorConfig configures supervised llama.cpp-backed runtime loading.
type SupervisorConfig struct {
	ReadinessTimeout     time.Duration
	HealthCheckInterval  time.Duration
	HealthCheckPath      string
	ProcessLauncher      ProcessLauncher
	HealthChecker        HealthChecker
	ServerStartBuilder   ServerStartBuilder
	Diagnostics          Diagnostics
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
	cfg          SupervisorConfig
	identity     Identity
}

func defaultSupervisorConfig() SupervisorConfig {
	return SupervisorConfig{
		ReadinessTimeout:    defaultReadinessTimeout,
		HealthCheckInterval: defaultHealthCheckInterval,
		HealthCheckPath:     defaultHealthCheckPath,
		ProcessLauncher:     execProcessLauncher{},
		HealthChecker:       HTTPHealthChecker{Path: defaultHealthCheckPath},
		ServerStartBuilder:  defaultLlamaCppServerStartBuilder,
	}
}

func normalizeSupervisorConfig(cfg SupervisorConfig) SupervisorConfig {
	defaults := defaultSupervisorConfig()
	if cfg.ReadinessTimeout <= 0 {
		cfg.ReadinessTimeout = defaults.ReadinessTimeout
	}
	if cfg.HealthCheckInterval <= 0 {
		cfg.HealthCheckInterval = defaults.HealthCheckInterval
	}
	if strings.TrimSpace(cfg.HealthCheckPath) == "" {
		cfg.HealthCheckPath = defaults.HealthCheckPath
	}
	if cfg.ProcessLauncher == nil {
		cfg.ProcessLauncher = defaults.ProcessLauncher
	}
	if cfg.HealthChecker == nil {
		cfg.HealthChecker = defaults.HealthChecker
	}
	if cfg.ServerStartBuilder == nil {
		cfg.ServerStartBuilder = defaults.ServerStartBuilder
	}
	return cfg
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
			ReadinessState: factoryapi.ManagedRuntimeReadinessStateREADY,
			LifecycleState: factoryapi.ManagedRuntimeLifecycleStateLOADED,
			FailureClass:   FailureClassNone,
			Diagnostics:    mergeDiagnostics(identity, factoryapi.ManagedRuntimeReadinessStateREADY, factoryapi.ManagedRuntimeLifecycleStateLOADED, map[string]string{
				"endpoint": r.endpoint,
			}),
		}
	case supervisedStateLoading:
		return ReadinessSnapshot{
			Identity:       identity,
			ReadinessState: factoryapi.ManagedRuntimeReadinessStateLOADING,
			LifecycleState: factoryapi.ManagedRuntimeLifecycleStateLOADING,
			FailureClass:   FailureClassLoadingTimeout,
			Diagnostics:    managedDiagnostics(identity, factoryapi.ManagedRuntimeReadinessStateLOADING, factoryapi.ManagedRuntimeLifecycleStateLOADING),
		}
	case supervisedStateFailed:
		readiness := ReadinessStateForFailureClass(r.failureClass)
		lifecycle := factoryapi.ManagedRuntimeLifecycleStateLOADED
		if r.failureClass == FailureClassLoadingTimeout {
			lifecycle = factoryapi.ManagedRuntimeLifecycleStateLOADING
		}
		if r.failureClass == FailureClassMissingAssets {
			lifecycle = factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED
		}
		return ReadinessSnapshot{
			Identity:       identity,
			ReadinessState: readiness,
			LifecycleState: lifecycle,
			FailureClass:   r.failureClass,
			Diagnostics:    managedDiagnostics(identity, readiness, lifecycle),
		}
	default:
		if base.ReadinessState == factoryapi.ManagedRuntimeReadinessStateREADY {
			return ReadinessSnapshot{
				Identity:       identity,
				ReadinessState: factoryapi.ManagedRuntimeReadinessStateLOADING,
				LifecycleState: factoryapi.ManagedRuntimeLifecycleStateLOADING,
				FailureClass:   FailureClassLoadingTimeout,
				Diagnostics:    managedDiagnostics(identity, factoryapi.ManagedRuntimeReadinessStateLOADING, factoryapi.ManagedRuntimeLifecycleStateLOADING),
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

	deadline := time.Now().Add(r.cfg.ReadinessTimeout)
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
		if time.Now().After(deadline) {
			_ = process.Stop(context.Background())
			return r.markFailed(identity, FailureClassLoadingTimeout, ErrLoadingTimeout)
		}
		timer := time.NewTimer(r.cfg.HealthCheckInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = process.Stop(context.Background())
			return r.markFailed(identity, FailureClassCancelled, cancelError(ctx.Err()))
		case <-timer.C:
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
			return r.failureOutcome()
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
