package wire

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerswire "github.com/portpowered/infinite-you/pkg/services/workers/wire"
	managedbackend "github.com/portpowered/infinite-you/pkg/wire/internal/managedbackend"
)

// newConfiguredProvidersService always installs the shell-free Antigravity
// print-mode command effect. An injected serviceedges.Edges.AgyPTYHost exists
// only to satisfy the legacy PTY allocator construction port and must never
// suppress the canonical command adapter; the command effect unconditionally
// takes priority over the legacy PTY effect in executionwire's built-in
// dependency selection.
func newConfiguredProvidersService(
	options []providerswire.Option,
	agyRunner platformprocess.CommandRunner,
) (providers.Service, error) {
	options = append(options, providerswire.WithAgyCommandRunner(
		workerswire.NewProviderCommandRunner(agyRunner),
	))
	return providerswire.NewService(options...)
}

type modelsProcessLauncher struct {
	recorder managedChildEnvironmentRecorder
}

func (launcher modelsProcessLauncher) Start(ctx context.Context, spec serviceedges.HostProcessStartSpec) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	launch, err := managedbackend.ResolveManagedBackendLaunch(ctx, spec)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(launch.Command, launch.Args...)
	if env := appendManagedBackendEnvironment(spec.Env, launch.Env); len(env) > 0 {
		cmd.Env = env
	}
	if launch.WorkDir != "" {
		cmd.Dir = launch.WorkDir
	}
	if err := cmd.Start(); err != nil {
		launch.Cleanup()
		return nil, managedbackend.WrapBackendStartFailure(err)
	}
	processID := 0
	if cmd.Process != nil {
		processID = cmd.Process.Pid
	}
	if processID > 0 {
		recorder := launcher.recorder
		if recorder != nil {
			recorder.RecordManagedChildEnvironment(managedChildEnvironmentEvidence{
				Kind:        managedChildEvidenceKind,
				Backend:     boundedManagedBackendID(spec.Backend),
				ProcessID:   processID,
				Phase:       managedChildPhaseStarted,
				Environment: managedEnvironmentFacts(effectiveManagedBackendEnvironment(cmd.Env)),
			})
		}
	}
	managed := &modelsManagedProcess{
		cmd:            cmd,
		healthEndpoint: launch.Endpoint,
		cleanup:        launch.Cleanup,
		finished:       make(chan struct{}),
		processID:      processID,
		backend:        boundedManagedBackendID(spec.Backend),
		recorder:       launcher.recorder,
	}
	go func() {
		waitErr := cmd.Wait()
		managed.recordProcessExit(waitErr)
		managed.cleanupResources()
		managed.mu.Lock()
		managed.waitErr = waitErr
		close(managed.finished)
		managed.mu.Unlock()
	}()
	return managed, nil
}

func appendManagedBackendEnvironment(base, additions []string) []string {
	if len(additions) == 0 {
		return append([]string(nil), base...)
	}
	environment := append([]string(nil), base...)
	if len(environment) == 0 {
		environment = os.Environ()
	}
	for _, addition := range additions {
		key, _, ok := strings.Cut(addition, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		environment = replaceManagedBackendEnvironmentKey(environment, key, addition)
	}
	return environment
}

func replaceManagedBackendEnvironmentKey(environment []string, key, replacement string) []string {
	first := -1
	for index, existing := range environment {
		existingKey, _, hasValue := strings.Cut(existing, "=")
		if !hasValue || !strings.EqualFold(existingKey, key) {
			continue
		}
		if first == -1 {
			first = index
		}
	}
	if first == -1 {
		return append(environment, replacement)
	}
	merged := make([]string, 0, len(environment))
	for index, existing := range environment {
		existingKey, _, hasValue := strings.Cut(existing, "=")
		if hasValue && strings.EqualFold(existingKey, key) {
			if index == first {
				merged = append(merged, replacement)
			}
			continue
		}
		merged = append(merged, existing)
	}
	return merged
}

func effectiveManagedBackendEnvironment(environment []string) []string {
	if environment != nil {
		return environment
	}
	return os.Environ()
}

var managedEnvironmentAllowlist = []string{"PATH", "TEMP", "TMP", "VIBEVOICECPP_LIBRARY"}

func managedEnvironmentFacts(environment []string) []managedEnvironmentFact {
	values := make(map[string]string, len(managedEnvironmentAllowlist))
	present := make(map[string]bool, len(managedEnvironmentAllowlist))
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		for _, allowed := range managedEnvironmentAllowlist {
			if strings.EqualFold(key, allowed) {
				present[allowed] = true
				values[allowed] = value
				break
			}
		}
	}
	facts := make([]managedEnvironmentFact, 0, len(managedEnvironmentAllowlist))
	for _, name := range managedEnvironmentAllowlist {
		fact := managedEnvironmentFact{Name: name, Present: present[name]}
		if fact.Present {
			digest := sha256.Sum256([]byte(values[name]))
			fact.ValueSHA256 = hex.EncodeToString(digest[:])
		}
		facts = append(facts, fact)
	}
	return facts
}

type modelHostProcessLauncherAdapter struct {
	next interface {
		Start(context.Context, serviceedges.HostProcessStartSpec) (interface {
			HealthEndpoint() string
			Wait() error
			Stop(context.Context) error
		}, error)
	}
}

func adaptModelHostProcessLauncher(next interface {
	Start(context.Context, serviceedges.HostProcessStartSpec) (interface {
		HealthEndpoint() string
		Wait() error
		Stop(context.Context) error
	}, error)
}) modelswire.HostProcessLauncher {
	if isNilModelEdgeDependency(next) {
		return nil
	}
	return modelHostProcessLauncherAdapter{next: next}
}

func (adapter modelHostProcessLauncherAdapter) Start(
	ctx context.Context,
	spec modelswire.HostProcessStartSpec,
) (modelswire.HostManagedProcess, error) {
	process, err := adapter.next.Start(ctx, serviceedges.HostProcessStartSpec{
		Command:        spec.Command,
		Args:           spec.Args,
		Env:            spec.Env,
		WorkDir:        spec.WorkDir,
		HealthEndpoint: spec.HealthEndpoint,
		Backend:        spec.Backend,
		ModelPath:      spec.ModelPath,
		ModelFiles:     append([]string(nil), spec.ModelFiles...),
		BackendFiles:   append([]string(nil), spec.BackendFiles...),
	})
	if err != nil || process == nil {
		return modelswire.HostManagedProcess(process), err
	}
	return modelswire.HostManagedProcess(process), nil
}

type modelHostClockAdapter struct {
	next interface {
		Now() time.Time
		NewTimer(time.Duration) interface {
			C() <-chan time.Time
			Stop() bool
		}
	}
}

func adaptModelHostClock(next interface {
	Now() time.Time
	NewTimer(time.Duration) interface {
		C() <-chan time.Time
		Stop() bool
	}
}) modelswire.HostClock {
	if isNilModelEdgeDependency(next) {
		return nil
	}
	return modelHostClockAdapter{next: next}
}

func (adapter modelHostClockAdapter) Now() time.Time { return adapter.next.Now() }

func (adapter modelHostClockAdapter) NewTimer(duration time.Duration) modelswire.HostTimer {
	return modelswire.HostTimer(adapter.next.NewTimer(duration))
}

func adaptModelRuntimeTempFile(next serviceedges.RuntimeCreateTempFile) modelswire.RuntimeCreateTempFile {
	if next == nil {
		return nil
	}
	return func(dir, pattern string) (modelswire.RuntimeTempFile, error) {
		file, err := next(dir, pattern)
		return modelswire.RuntimeTempFile(file), err
	}
}

type modelsPullMetricsAdapter struct {
	next interface {
		RecordModelPullMetric(serviceedges.PullMetric)
	}
}

func adaptModelsPullMetricsRecorder(next interface {
	RecordModelPullMetric(serviceedges.PullMetric)
}) modelswire.PullMetricsRecorder {
	if next == nil {
		return nil
	}
	return modelsPullMetricsAdapter{next: next}
}

// isNilModelEdgeDependency preserves Models Wire's typed-nil validation when
// an edge-owned interface is wrapped by a canonical adapter. Without this
// check, a nil pointer would become a non-nil adapter value and fail later at
// invocation rather than during inert construction.
func isNilModelEdgeDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (adapter modelsPullMetricsAdapter) RecordModelPullMetric(metric modelswire.PullMetric) {
	labels := make(map[string]string, len(metric.Labels))
	for key, value := range metric.Labels {
		labels[key] = value
	}
	adapter.next.RecordModelPullMetric(serviceedges.PullMetric{
		Name:   metric.Name,
		Labels: labels,
	})
}

func modelLocalRuntimeHooks(hooks workers.LocalRuntimeHooks) modelswire.LocalRuntimeHooks {
	return modelswire.LocalRuntimeHooks{
		MarkResourceWaitStarted:  hooks.MarkResourceWaitStarted,
		MarkResourceWaitFinished: hooks.MarkResourceWaitFinished,
		MarkLoadRequested:        hooks.MarkLoadRequested,
		MarkLoadFinished:         hooks.MarkLoadFinished,
		MarkLoadReused:           hooks.MarkLoadReused,
	}
}

type modelsManagedProcess struct {
	mu             sync.Mutex
	cmd            *exec.Cmd
	healthEndpoint string
	cleanup        func()
	cleanupOnce    sync.Once
	processID      int
	backend        string
	recorder       managedChildEnvironmentRecorder
	// finished is broadcast to both the supervisor's Wait observer and the
	// application lifecycle closer; a one-shot error channel would let one
	// consumer strand the other during normal teardown.
	finished chan struct{}
	waitErr  error
	stopped  bool
}

func (p *modelsManagedProcess) recordProcessExit(waitErr error) {
	if p == nil || p.processID <= 0 || p.recorder == nil {
		return
	}
	p.recorder.RecordManagedChildEnvironment(managedChildEnvironmentEvidence{
		Kind:      managedChildEvidenceKind,
		Backend:   p.backend,
		ProcessID: p.processID,
		Phase:     managedChildPhaseExited,
		ExitClass: managedChildExitClass(waitErr),
	})
}

func managedChildExitClass(waitErr error) string {
	if waitErr == nil {
		return managedChildExitClassExited
	}
	var exitError *exec.ExitError
	if errors.As(waitErr, &exitError) {
		return managedChildExitClassNonzero
	}
	return managedChildExitClassWaitFailed
}

func boundedManagedBackendID(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "localai-llamacpp":
		return "localai-llamacpp"
	case "localai-vibevoice":
		return "localai-vibevoice"
	case "localai-whisper":
		return "localai-whisper"
	default:
		return "UNKNOWN"
	}
}

func (p *modelsManagedProcess) cleanupResources() {
	if p == nil || p.cleanup == nil {
		return
	}
	p.cleanupOnce.Do(p.cleanup)
}

func (p *modelsManagedProcess) HealthEndpoint() string { return p.healthEndpoint }

func (p *modelsManagedProcess) Wait() error {
	if p == nil || p.finished == nil {
		return nil
	}
	<-p.finished
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.waitErr
}

func (p *modelsManagedProcess) Stop(ctx context.Context) error {
	if p == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.mu.Lock()
	if p.stopped || p.cmd == nil || p.cmd.Process == nil {
		p.mu.Unlock()
		return nil
	}
	p.stopped = true
	command := p.cmd
	p.mu.Unlock()
	if !p.processFinished() {
		if err := command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) && !p.processFinished() {
			return err
		}
	}
	if p.finished == nil {
		return nil
	}
	select {
	case <-p.finished:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *modelsManagedProcess) processFinished() bool {
	if p == nil {
		return true
	}
	if p.finished != nil {
		select {
		case <-p.finished:
			return true
		default:
		}
	}
	return p.cmd != nil && p.cmd.ProcessState != nil
}
