package service_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/service"
)

func TestManagedLocalAIPreflightRejectsUnsupportedHostBeforeProcessStart(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		options   runtimehost.Options
		wantErr   error
		wantClass models.HostFailureClass
	}{
		{
			name: "missing platform",
			options: runtimehost.Options{
				CompatibilityChecker: &testCompatibilityChecker{},
				ProtocolNegotiator:   &testProtocolNegotiator{},
			},
			wantErr:   models.ErrHostUnsupportedPlatform,
			wantClass: models.HostFailureClassUnsupportedPlatform,
		},
		{
			name: "unsupported accelerator policy",
			options: runtimehost.Options{
				Platform:             managedHostPlatform(),
				CompatibilityChecker: &testCompatibilityChecker{err: errors.New("accelerator unavailable")},
				ProtocolNegotiator:   &testProtocolNegotiator{},
			},
			wantErr:   models.ErrHostUnsupportedPlatform,
			wantClass: models.HostFailureClassUnsupportedPlatform,
		},
		{
			name: "missing pinned protocol effect",
			options: runtimehost.Options{
				Platform:             managedHostPlatform(),
				CompatibilityChecker: &testCompatibilityChecker{},
			},
			wantErr:   models.ErrHostProtocolIncompatible,
			wantClass: models.HostFailureClassProtocol,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			cacheDirectory := t.TempDir()
			writeCacheFixture(t, cacheDirectory, true)
			scopes := newScopes(t, "story-003-preflight-"+test.name)
			ref := openScope(t, scopes, cacheDirectory, managedLocalAIConfig(models.LoadPolicyOnDemand))
			launcher := &recordingProcessLauncher{}
			host := internalservice.NewWithHostTestConfig(
				scopes,
				mustAssetsService(t, scopes),
				launcher,
				http.DefaultClient,
				realHostClock{},
				nil,
				nil,
				internalservice.SupervisorTestConfig{},
				internalservice.HostPolicyTestConfig{},
				test.options,
			)

			_, err := host.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
				Scope: ref,
				Name:  managedModelName,
			})
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("EnsureModelHost error = %v, want %v", err, test.wantErr)
			}
			var readinessErr *models.HostReadinessError
			if !errors.As(err, &readinessErr) {
				t.Fatalf("EnsureModelHost error = %T, want HostReadinessError", err)
			}
			if readinessErr.Snapshot.FailureClass != test.wantClass {
				t.Fatalf("failure class = %q, want %q", readinessErr.Snapshot.FailureClass, test.wantClass)
			}
			if launcher.starts != 0 {
				t.Fatalf("process starts = %d, want 0", launcher.starts)
			}
		})
	}
}

func TestManagedLocalAIUsesPinnedProtocolAndKeepsPrivateRuntimeDetailsPrivate(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "story-003-protocol")
	ref := openScope(t, scopes, cacheDirectory, managedLocalAIConfig(models.LoadPolicyOnDemand))
	launcher := &controlledProcessLauncher{}
	protocol := &testProtocolNegotiator{}
	host := internalservice.NewWithHostTestConfig(
		scopes,
		mustAssetsService(t, scopes),
		launcher,
		http.DefaultClient,
		realHostClock{},
		nil,
		nil,
		internalservice.SupervisorTestConfig{},
		internalservice.HostPolicyTestConfig{},
		runtimehost.Options{
			Platform:             managedHostPlatform(),
			CompatibilityChecker: &testCompatibilityChecker{},
			ProtocolNegotiator:   protocol,
		},
	)
	t.Cleanup(func() { _ = internalservice.ShutdownHost(context.Background(), host) })

	result, err := host.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: ref,
		Name:  managedModelName,
	})
	if err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}
	assertManagedHostReady(t, result)
	assertManagedProcess(t, launcher)
	assertManagedProtocolCall(t, protocol)
}

func assertManagedHostReady(t *testing.T, result models.EnsureModelHostResult) {
	t.Helper()
	if result.Outcome != models.HostEnsureBecameReady || result.Host.ReadinessState != models.ReadinessStateReady {
		t.Fatalf("EnsureModelHost result = %#v, want newly ready host", result)
	}
	if result.Host.Diagnostics["endpoint"] != "" || result.Host.Diagnostics["cachePath"] != "" {
		t.Fatalf("public host diagnostics = %#v, want no endpoint or cache path", result.Host.Diagnostics)
	}
}

func assertManagedProcess(t *testing.T, launcher *controlledProcessLauncher) {
	t.Helper()
	if launcher.startCount() != 1 {
		t.Fatalf("process starts = %d, want 1", launcher.startCount())
	}
	spec := launcher.spec(0)
	if spec.Command != "fake-localai" {
		t.Fatalf("process command = %q, want fake-localai", spec.Command)
	}
	if !containsArg(spec.Args, "serve") || !containsArg(spec.Args, "--cache-path") {
		t.Fatalf("process args = %#v, want serve and cache path", spec.Args)
	}
	if containsArg(spec.Args, "--grpc-endpoint") || containsArg(spec.Args, "grpc://127.0.0.1:50051") {
		t.Fatalf("process args = %#v, want endpoint consumed by host adapter", spec.Args)
	}
}

func assertManagedProtocolCall(t *testing.T, protocol *testProtocolNegotiator) {
	t.Helper()
	call := protocol.call()
	if call.endpoint != "grpc://127.0.0.1:50051" || call.request.ProtocolVersion != modelseffects.PinnedHostProtocolVersion {
		t.Fatalf("protocol call = %#v, want pinned endpoint/version", call)
	}
	if call.request.Backend != "localai-llamacpp" || call.request.Platform != managedHostPlatform() {
		t.Fatalf("protocol request = %#v, want resolved backend/platform", call.request)
	}
}

func TestManagedLocalAICrashRevokesCapacityAndRecoversWithFreshProcess(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "story-003-crash-recovery")
	cfg := managedLocalAIConfig(models.LoadPolicyOnDemand)
	cfg.Resources[0].Capacity = 1
	ref := openScope(t, scopes, cacheDirectory, cfg)
	launcher := &controlledProcessLauncher{}
	protocol := &testProtocolNegotiator{}
	failureObserved := make(chan struct{})
	var failureOnce sync.Once
	host, err := internalservice.NewWiredWithSupervisorConfig(
		scopes,
		mustAssetsService(t, scopes),
		launcher,
		http.DefaultClient,
		realHostClock{},
		nil,
		nil,
		internalservice.SupervisorTestConfig{
			OnProcessFailure: func() {
				failureOnce.Do(func() { close(failureObserved) })
			},
		},
		internalservice.HostPolicyTestConfig{},
		runtimehost.Options{
			Platform:             managedHostPlatform(),
			CompatibilityChecker: &testCompatibilityChecker{},
			ProtocolNegotiator:   protocol,
		},
	)
	if err != nil {
		t.Fatalf("NewWiredWithSupervisorConfig: %v", err)
	}
	t.Cleanup(func() { _ = internalservice.ShutdownHost(context.Background(), host) })

	if _, err := host.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: ref,
		Name:  managedModelName,
	}); err != nil {
		t.Fatalf("first EnsureModelHost: %v", err)
	}
	first := launcher.process(0)
	lease, err := host.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope: ref, Name: managedModelName, Holder: "worker-a",
	})
	if err != nil {
		t.Fatalf("AcquireModelLease before crash: %v", err)
	}

	first.crash(errors.New("backend exited"))
	select {
	case <-failureObserved:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for process-failure lease revocation")
	}
	gotLease, getErr := host.GetModelLease(context.Background(), models.GetModelLeaseRequest{
		Scope: ref, Lease: lease.Lease.Lease,
	})
	if !errors.Is(getErr, models.ErrHostLeaseExpired) || gotLease.Lease.Status != models.ModelLeaseStatusExpired {
		t.Fatalf("GetModelLease after crash = %#v, %v, want expired lease", gotLease, getErr)
	}

	if _, err := host.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: ref,
		Name:  managedModelName,
	}); err != nil {
		t.Fatalf("recovery EnsureModelHost: %v", err)
	}
	if launcher.startCount() != 2 {
		t.Fatalf("process starts after recovery = %d, want 2", launcher.startCount())
	}
	if _, err := host.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope: ref, Name: managedModelName, Holder: "worker-b",
	}); err != nil {
		t.Fatalf("AcquireModelLease after recovery: %v", err)
	}
}

func TestManagedLocalAIRetentionUsesResolvedLoadPolicyAndInjectedClock(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name         string
		loadPolicy   models.LoadPolicy
		wantTimer    bool
		wantStopIdle bool
	}{
		{name: "on demand unloads", loadPolicy: models.LoadPolicyOnDemand, wantTimer: true, wantStopIdle: true},
		{name: "keep warm remains resident", loadPolicy: models.LoadPolicyKeepWarm, wantTimer: false, wantStopIdle: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			clock := newDeterministicHostClock()
			cacheDirectory := t.TempDir()
			writeCacheFixture(t, cacheDirectory, true)
			scopes := newScopes(t, "story-003-retention-"+test.name)
			cfg := managedLocalAIConfig(test.loadPolicy)
			ref := openScope(t, scopes, cacheDirectory, cfg)
			launcher := &controlledProcessLauncher{}
			host, err := internalservice.NewWiredWithSupervisorConfig(
				scopes,
				mustAssetsService(t, scopes),
				launcher,
				http.DefaultClient,
				clock,
				nil,
				nil,
				internalservice.SupervisorTestConfig{},
				internalservice.HostPolicyTestConfig{IdleUnloadAfter: time.Hour},
				runtimehost.Options{
					Platform:             managedHostPlatform(),
					CompatibilityChecker: &testCompatibilityChecker{},
					ProtocolNegotiator:   &testProtocolNegotiator{},
				},
			)
			if err != nil {
				t.Fatalf("NewWiredWithSupervisorConfig: %v", err)
			}
			t.Cleanup(func() { _ = internalservice.ShutdownHost(context.Background(), host) })

			if _, err := host.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
				Scope: ref,
				Name:  managedModelName,
			}); err != nil {
				t.Fatalf("EnsureModelHost: %v", err)
			}
			process := launcher.process(0)
			lease, err := host.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
				Scope: ref, Name: managedModelName, Holder: "worker-a",
			})
			if err != nil {
				t.Fatalf("AcquireModelLease: %v", err)
			}
			if _, err := host.ReleaseModelLease(context.Background(), models.ReleaseModelLeaseRequest{
				Scope: ref, Lease: lease.Lease.Lease,
			}); err != nil {
				t.Fatalf("ReleaseModelLease: %v", err)
			}
			if clock.timerCount() != boolToInt(test.wantTimer) {
				t.Fatalf("injected timer count = %d, want %d", clock.timerCount(), boolToInt(test.wantTimer))
			}
			if test.wantTimer {
				clock.fireAll()
				select {
				case <-process.stopped:
				case <-time.After(2 * time.Second):
					t.Fatal("timed out waiting for injected idle-unload timer")
				}
			} else {
				select {
				case <-process.stopped:
					t.Fatal("KEEP_WARM process stopped after final lease")
				default:
				}
			}
		})
	}
}

const managedModelName = "OMNIVOICE_Q4_K_M"

func managedLocalAIConfig(loadPolicy models.LoadPolicy) models.RuntimeConfig {
	cfg := supervisedRuntimeConfig()
	cfg.Resources[0].Backend = "localai-llamacpp"
	cfg.Resources[0].LoadPolicy = string(loadPolicy)
	cfg.Workers[0].Command = "fake-localai"
	cfg.Workers[0].Args = []string{"--grpc-endpoint", "grpc://127.0.0.1:50051"}
	return cfg
}

func managedHostPlatform() models.AssetHostPlatform {
	return models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

type testCompatibilityChecker struct {
	mu       sync.Mutex
	calls    int
	err      error
	requests []modelseffects.HostCompatibilityRequest
}

func (checker *testCompatibilityChecker) Check(
	_ context.Context,
	request modelseffects.HostCompatibilityRequest,
) error {
	checker.mu.Lock()
	checker.calls++
	checker.requests = append(checker.requests, request)
	checker.mu.Unlock()
	return checker.err
}

type protocolCall struct {
	endpoint string
	request  modelseffects.HostProtocolNegotiationRequest
}

type testProtocolNegotiator struct {
	mu     sync.Mutex
	calls  []protocolCall
	result modelseffects.HostProtocolNegotiationResult
	err    error
}

func (negotiator *testProtocolNegotiator) Negotiate(
	_ context.Context,
	endpoint string,
	request modelseffects.HostProtocolNegotiationRequest,
) (modelseffects.HostProtocolNegotiationResult, error) {
	negotiator.mu.Lock()
	negotiator.calls = append(negotiator.calls, protocolCall{endpoint: endpoint, request: request})
	result := negotiator.result
	negotiator.mu.Unlock()
	if negotiator.err != nil {
		return modelseffects.HostProtocolNegotiationResult{}, negotiator.err
	}
	if result.ProtocolVersion == "" {
		result.ProtocolVersion = modelseffects.PinnedHostProtocolVersion
	}
	if result.Backend == "" {
		result.Backend = request.Backend
	}
	if !result.Ready {
		result.Ready = true
	}
	return result, nil
}

func (negotiator *testProtocolNegotiator) call() protocolCall {
	negotiator.mu.Lock()
	defer negotiator.mu.Unlock()
	if len(negotiator.calls) == 0 {
		return protocolCall{}
	}
	return negotiator.calls[0]
}

type controlledProcessLauncher struct {
	mu        sync.Mutex
	starts    int
	specs     []modelseffects.HostProcessStartSpec
	processes []*controlledManagedProcess
}

func (launcher *controlledProcessLauncher) Start(
	_ context.Context,
	spec modelseffects.HostProcessStartSpec,
) (modelseffects.HostManagedProcess, error) {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	launcher.starts++
	process := newControlledManagedProcess(spec.HealthEndpoint)
	spec.Args = append([]string(nil), spec.Args...)
	launcher.specs = append(launcher.specs, spec)
	launcher.processes = append(launcher.processes, process)
	return process, nil
}

func (launcher *controlledProcessLauncher) startCount() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.starts
}

func (launcher *controlledProcessLauncher) spec(index int) modelseffects.HostProcessStartSpec {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.specs[index]
}

func (launcher *controlledProcessLauncher) process(index int) *controlledManagedProcess {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.processes[index]
}

type controlledManagedProcess struct {
	endpoint   string
	exitCh     chan error
	stopped    chan struct{}
	stopOnce   sync.Once
	waited     chan struct{}
	waitedOnce sync.Once
}

func newControlledManagedProcess(endpoint string) *controlledManagedProcess {
	return &controlledManagedProcess{
		endpoint: endpoint,
		exitCh:   make(chan error, 1),
		stopped:  make(chan struct{}),
		waited:   make(chan struct{}),
	}
}

func (process *controlledManagedProcess) HealthEndpoint() string {
	return process.endpoint
}

func (process *controlledManagedProcess) Wait() error {
	err := <-process.exitCh
	process.waitedOnce.Do(func() { close(process.waited) })
	return err
}

func (process *controlledManagedProcess) Stop(context.Context) error {
	process.stopOnce.Do(func() {
		process.exitCh <- errors.New("managed process stopped")
		close(process.stopped)
	})
	return nil
}

func (process *controlledManagedProcess) crash(err error) {
	process.exitCh <- err
}

type deterministicHostClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*deterministicHostTimer
}

func newDeterministicHostClock() *deterministicHostClock {
	return &deterministicHostClock{now: time.Unix(0, 0)}
}

func (clock *deterministicHostClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *deterministicHostClock) NewTimer(time.Duration) modelseffects.HostTimer {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	timer := &deterministicHostTimer{clock: clock, channel: make(chan time.Time, 1)}
	clock.timers = append(clock.timers, timer)
	return timer
}

func (clock *deterministicHostClock) timerCount() int {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	count := 0
	for _, timer := range clock.timers {
		if !timer.isStopped() {
			count++
		}
	}
	return count
}

func (clock *deterministicHostClock) fireAll() {
	clock.mu.Lock()
	timers := append([]*deterministicHostTimer(nil), clock.timers...)
	now := clock.now
	clock.mu.Unlock()
	for _, timer := range timers {
		timer.fire(now)
	}
}

type deterministicHostTimer struct {
	mu      sync.Mutex
	clock   *deterministicHostClock
	channel chan time.Time
	stopped bool
}

func (timer *deterministicHostTimer) C() <-chan time.Time {
	return timer.channel
}

func (timer *deterministicHostTimer) Stop() bool {
	timer.mu.Lock()
	defer timer.mu.Unlock()
	if timer.stopped {
		return false
	}
	timer.stopped = true
	return true
}

func (timer *deterministicHostTimer) isStopped() bool {
	timer.mu.Lock()
	defer timer.mu.Unlock()
	return timer.stopped
}

func (timer *deterministicHostTimer) fire(now time.Time) {
	timer.mu.Lock()
	defer timer.mu.Unlock()
	if timer.stopped {
		return
	}
	timer.channel <- now
}

var _ modelseffects.HostProtocolNegotiator = (*testProtocolNegotiator)(nil)
