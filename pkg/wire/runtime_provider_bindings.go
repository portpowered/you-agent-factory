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
	managedchild "github.com/portpowered/infinite-you/pkg/platform/process/managedchild"
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
	recorder      managedChildEnvironmentRecorder
	resolveLaunch func(context.Context, serviceedges.HostProcessStartSpec) (managedbackend.ManagedBackendLaunch, error)
	startProcess  func(context.Context, managedchild.Spec) (*managedchild.Process, error)
}

func (launcher modelsProcessLauncher) Start(ctx context.Context, spec serviceedges.HostProcessStartSpec) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	resolveLaunch := launcher.resolveLaunch
	if resolveLaunch == nil {
		resolveLaunch = managedbackend.ResolveManagedBackendLaunch
	}
	launch, err := resolveLaunch(ctx, spec)
	if err != nil {
		return nil, err
	}
	environment := appendManagedBackendEnvironment(spec.Env, launch.Env)
	startProcess := launcher.startProcess
	if startProcess == nil {
		startProcess = managedchild.Start
	}
	child, err := startProcess(ctx, managedchild.Spec{
		Command: launch.Command, Args: append([]string(nil), launch.Args...),
		Env: environment, WorkDir: launch.WorkDir,
	})
	if err == nil && child == nil {
		err = errors.New("managed child starter returned a nil process")
	}
	if err != nil {
		var cleanupErr error
		if launch.Cleanup != nil {
			cleanupErr = launch.Cleanup()
		}
		return nil, managedbackend.WrapBackendStartFailureWithCleanup(err, cleanupErr)
	}
	processID := child.PID()
	if processID > 0 {
		recorder := launcher.recorder
		if recorder != nil {
			recorder.RecordManagedChildEnvironment(managedChildEnvironmentEvidence{
				Kind:        managedChildEvidenceKind,
				Backend:     boundedManagedBackendID(spec.Backend),
				ProcessID:   processID,
				Phase:       managedChildPhaseStarted,
				Environment: managedEnvironmentFacts(effectiveManagedBackendEnvironment(environment)),
			})
		}
	}
	managed := &modelsManagedProcess{
		child:          child,
		healthEndpoint: launch.Endpoint,
		cleanup:        launch.Cleanup,
		finished:       make(chan struct{}),
		processID:      processID,
		backend:        boundedManagedBackendID(spec.Backend),
		recorder:       launcher.recorder,
	}
	go func() {
		waitErr := child.Wait()
		childSnapshot, snapshotReady := child.Snapshot()
		diagnostic := managedProcessDiagnostic(waitErr, childSnapshot, snapshotReady)
		managed.recordProcessExit(diagnostic)
		managed.cleanupResources()
		managed.mu.Lock()
		managed.waitErr = waitErr
		managed.diagnostic = diagnostic
		managed.diagnosticReady = snapshotReady
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
	configuration := spec.Configuration.Clone()
	process, err := adapter.next.Start(ctx, serviceedges.HostProcessStartSpec{
		Command:        spec.Command,
		Args:           spec.Args,
		Env:            spec.Env,
		WorkDir:        spec.WorkDir,
		HealthEndpoint: spec.HealthEndpoint,
		Backend:        configuration.Backend,
		ModelPath:      configuration.ModelPath,
		MMProjPath:     configuration.MMProjPath,
		ModelFiles:     append([]string(nil), configuration.ModelFiles...),
		BackendFiles:   append([]string(nil), configuration.BackendFiles...),
	})
	if err != nil || process == nil {
		return modelswire.HostManagedProcess(process), err
	}
	return modelHostManagedProcessAdapter{next: process}, nil
}

// modelHostManagedProcessAdapter preserves the base Models lifecycle methods
// while translating the optional edge-owned diagnostic capability. Older edge
// doubles remain valid because the type assertion is deliberately optional.
type modelHostManagedProcessAdapter struct {
	next interface {
		HealthEndpoint() string
		Wait() error
		Stop(context.Context) error
	}
}

func (adapter modelHostManagedProcessAdapter) HealthEndpoint() string {
	return adapter.next.HealthEndpoint()
}

func (adapter modelHostManagedProcessAdapter) Wait() error {
	return adapter.next.Wait()
}

func (adapter modelHostManagedProcessAdapter) Stop(ctx context.Context) error {
	return adapter.next.Stop(ctx)
}

func (adapter modelHostManagedProcessAdapter) DiagnosticSnapshot() (
	snapshot modelswire.HostProcessDiagnosticSnapshot,
	ok bool,
) {
	defer func() {
		if recover() != nil {
			snapshot = modelswire.HostProcessDiagnosticSnapshot{}
			ok = false
		}
	}()
	source, ok := adapter.next.(serviceedges.HostManagedProcessDiagnosticSource)
	if !ok || isNilModelEdgeDependency(source) {
		return modelswire.HostProcessDiagnosticSnapshot{}, false
	}
	edgeSnapshot, ready := source.DiagnosticSnapshot()
	if !ready {
		return modelswire.HostProcessDiagnosticSnapshot{}, false
	}
	edgeSnapshot, valid := normalizeManagedProcessDiagnostic(edgeSnapshot)
	if !valid {
		return modelswire.HostProcessDiagnosticSnapshot{}, false
	}
	return modelswire.HostProcessDiagnosticSnapshot{
		ExitClass:            edgeSnapshot.ExitClass,
		ExitCode:             edgeSnapshot.ExitCode,
		ExitCodeKnown:        edgeSnapshot.ExitCodeKnown,
		Stdout:               modelswire.HostProcessStreamDiagnostic(edgeSnapshot.Stdout),
		Stderr:               modelswire.HostProcessStreamDiagnostic(edgeSnapshot.Stderr),
		CauseCode:            edgeSnapshot.CauseCode,
		CauseMessage:         edgeSnapshot.CauseMessage,
		CauseMessageRedacted: edgeSnapshot.CauseMessageRedacted,
	}, true
}

func normalizeManagedProcessDiagnostic(
	snapshot serviceedges.HostProcessDiagnosticSnapshot,
) (serviceedges.HostProcessDiagnosticSnapshot, bool) {
	if strings.TrimSpace(snapshot.ExitClass) == "" {
		return serviceedges.HostProcessDiagnosticSnapshot{}, false
	}
	switch strings.ToUpper(strings.TrimSpace(snapshot.ExitClass)) {
	case managedChildExitClassExited, managedChildExitClassNonzero, managedChildExitClassWaitFailed:
		snapshot.ExitClass = strings.ToUpper(strings.TrimSpace(snapshot.ExitClass))
	default:
		snapshot.ExitClass = "UNKNOWN"
	}
	if !snapshot.ExitCodeKnown || snapshot.ExitCode < 0 {
		snapshot.ExitCode = 0
		snapshot.ExitCodeKnown = false
	}
	snapshot.Stdout.SHA256 = normalizeManagedProcessDigest(snapshot.Stdout.SHA256)
	snapshot.Stderr.SHA256 = normalizeManagedProcessDigest(snapshot.Stderr.SHA256)
	snapshot.CauseCode, snapshot.CauseMessage, snapshot.CauseMessageRedacted =
		normalizeManagedProcessCause(snapshot.CauseCode, snapshot.CauseMessageRedacted)
	return snapshot, true
}

func normalizeManagedProcessDigest(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != sha256.Size*2 {
		return ""
	}
	if _, err := hex.DecodeString(value); err != nil {
		return ""
	}
	return value
}

func normalizeManagedProcessCause(code string, redacted bool) (string, string, bool) {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case managedProcessCauseCancelled:
		return managedProcessCauseCancelled, "backend operation cancelled", redacted
	case managedProcessCauseEndpointBind:
		return managedProcessCauseEndpointBind, "backend endpoint bind failed", redacted
	case managedProcessCauseModelLoad:
		return managedProcessCauseModelLoad, "model load failed", redacted
	case managedProcessCauseProcessExited:
		return managedProcessCauseProcessExited, "managed backend process exited", redacted
	case managedProcessCauseProtocolIncompat:
		return managedProcessCauseProtocolIncompat, "backend protocol incompatible", redacted
	case managedProcessCauseRPCRejected:
		return managedProcessCauseRPCRejected, "backend RPC request rejected", redacted
	case managedProcessCauseTimedOut:
		return managedProcessCauseTimedOut, "backend operation timed out", redacted
	default:
		return "", "", false
	}
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
	mu              sync.Mutex
	child           *managedchild.Process
	healthEndpoint  string
	cleanup         func() error
	cleanupOnce     sync.Once
	cleanupErr      error
	processID       int
	backend         string
	recorder        managedChildEnvironmentRecorder
	diagnostic      serviceedges.HostProcessDiagnosticSnapshot
	diagnosticReady bool
	// finished is broadcast to both the supervisor's Wait observer and the
	// application lifecycle closer; a one-shot error channel would let one
	// consumer strand the other during normal teardown.
	finished chan struct{}
	waitErr  error
	stopped  bool
}

func (p *modelsManagedProcess) recordProcessExit(
	diagnostic serviceedges.HostProcessDiagnosticSnapshot,
) {
	if p == nil || p.processID <= 0 || p.recorder == nil {
		return
	}
	p.recorder.RecordManagedChildEnvironment(managedChildEnvironmentEvidence{
		Kind:                 managedChildEvidenceKind,
		Backend:              p.backend,
		ProcessID:            p.processID,
		Phase:                managedChildPhaseExited,
		ExitClass:            diagnostic.ExitClass,
		ExitCode:             diagnostic.ExitCode,
		ExitCodeKnown:        diagnostic.ExitCodeKnown,
		StdoutBytes:          diagnostic.Stdout.Bytes,
		StdoutSHA256:         diagnostic.Stdout.SHA256,
		StdoutTruncated:      diagnostic.Stdout.Truncated,
		StderrBytes:          diagnostic.Stderr.Bytes,
		StderrSHA256:         diagnostic.Stderr.SHA256,
		StderrTruncated:      diagnostic.Stderr.Truncated,
		CauseCode:            diagnostic.CauseCode,
		CauseMessage:         diagnostic.CauseMessage,
		CauseMessageRedacted: diagnostic.CauseMessageRedacted,
	})
}

func managedProcessDiagnostic(
	waitErr error,
	childSnapshot managedchild.Snapshot,
	snapshotReady bool,
) serviceedges.HostProcessDiagnosticSnapshot {
	if !snapshotReady {
		return serviceedges.HostProcessDiagnosticSnapshot{
			ExitClass: managedChildExitClass(waitErr),
		}
	}
	causeCode, causeMessage, causeMessageRedacted := reduceManagedProcessCause(
		waitErr, childSnapshot.Stdout.Tail, childSnapshot.Stderr.Tail,
	)
	return serviceedges.HostProcessDiagnosticSnapshot{
		ExitClass:            string(childSnapshot.ExitClass),
		ExitCode:             childSnapshot.ExitCode,
		ExitCodeKnown:        childSnapshot.ExitCodeKnown,
		Stdout:               serviceedges.HostProcessStreamDiagnostic{Bytes: childSnapshot.Stdout.Bytes, SHA256: childSnapshot.Stdout.SHA256, Truncated: childSnapshot.Stdout.Truncated},
		Stderr:               serviceedges.HostProcessStreamDiagnostic{Bytes: childSnapshot.Stderr.Bytes, SHA256: childSnapshot.Stderr.SHA256, Truncated: childSnapshot.Stderr.Truncated},
		CauseCode:            causeCode,
		CauseMessage:         causeMessage,
		CauseMessageRedacted: causeMessageRedacted,
	}
}

const (
	managedProcessCauseCancelled        = "CANCELLED"
	managedProcessCauseEndpointBind     = "ENDPOINT_BIND_FAILED"
	managedProcessCauseModelLoad        = "MODEL_LOAD_FAILED"
	managedProcessCauseProcessExited    = "PROCESS_EXITED"
	managedProcessCauseProtocolIncompat = "PROTOCOL_INCOMPATIBLE"
	managedProcessCauseRPCRejected      = "RPC_REJECTED"
	managedProcessCauseTimedOut         = "TIMEOUT"
)

func reduceManagedProcessCause(waitErr error, stdoutTail, stderrTail []byte) (string, string, bool) {
	output := make([]byte, 0, len(stdoutTail)+len(stderrTail))
	output = append(output, stdoutTail...)
	output = append(output, stderrTail...)
	lower := strings.ToLower(string(output))
	switch {
	case (strings.Contains(lower, "projector") || strings.Contains(lower, "mmproj")) &&
		(strings.Contains(lower, "load") || strings.Contains(lower, "fail") || strings.Contains(lower, "error")):
		return managedProcessCauseModelLoad, "model load failed", false
	case strings.Contains(lower, "protocol") &&
		(strings.Contains(lower, "incompat") || strings.Contains(lower, "reject")):
		return managedProcessCauseProtocolIncompat, "backend protocol incompatible", false
	case strings.Contains(lower, "rpc") &&
		(strings.Contains(lower, "reject") || strings.Contains(lower, "invalid")):
		return managedProcessCauseRPCRejected, "backend RPC request rejected", false
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "timed out"):
		return managedProcessCauseTimedOut, "backend operation timed out", false
	case strings.Contains(lower, "cancel"):
		return managedProcessCauseCancelled, "backend operation cancelled", false
	case strings.Contains(lower, "address already in use") ||
		(strings.Contains(lower, "endpoint") && strings.Contains(lower, "bind")):
		return managedProcessCauseEndpointBind, "backend endpoint bind failed", false
	case strings.Contains(lower, "model") &&
		(strings.Contains(lower, "load") || strings.Contains(lower, "fail")):
		return managedProcessCauseModelLoad, "model load failed", false
	}
	if len(output) > 0 {
		if waitErr != nil {
			return managedProcessCauseProcessExited, "managed backend process exited", true
		}
		return "", "", false
	}
	if waitErr == nil {
		return "", "", false
	}
	return managedProcessCauseProcessExited, "managed backend process exited", false
}

func (p *modelsManagedProcess) DiagnosticSnapshot() (
	serviceedges.HostProcessDiagnosticSnapshot,
	bool,
) {
	if p == nil {
		return serviceedges.HostProcessDiagnosticSnapshot{}, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.diagnosticReady {
		return serviceedges.HostProcessDiagnosticSnapshot{}, false
	}
	return p.diagnostic, true
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
	p.cleanupOnce.Do(func() {
		cleanupErr := p.cleanup()
		p.mu.Lock()
		p.cleanupErr = cleanupErr
		p.mu.Unlock()
	})
}

func (p *modelsManagedProcess) HealthEndpoint() string { return p.healthEndpoint }

func (p *modelsManagedProcess) Wait() error {
	if p == nil || p.finished == nil {
		return nil
	}
	<-p.finished
	p.mu.Lock()
	waitErr, cleanupErr := p.waitErr, p.cleanupErr
	p.mu.Unlock()
	if cleanupErr != nil {
		if waitErr == nil {
			return cleanupErr
		}
		return errors.Join(waitErr, cleanupErr)
	}
	return waitErr
}

func (p *modelsManagedProcess) Stop(ctx context.Context) error {
	if p == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.mu.Lock()
	if p.stopped || p.child == nil {
		p.mu.Unlock()
		return nil
	}
	p.stopped = true
	child := p.child
	p.mu.Unlock()
	if err := child.Stop(ctx); err != nil {
		return err
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
