package service_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/service"
)

func TestEnsureModelHostReadinessTimeoutReturnsTypedFailure(t *testing.T) {
	t.Parallel()

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(healthServer.Close)

	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "readiness-timeout")
	ref := openScope(t, scopes, cacheDirectory, supervisedRuntimeConfig())
	launcher := &fakeProcessLauncher{
		newProcess: func(spec modelseffects.HostProcessStartSpec) *fakeManagedProcess {
			return newFakeManagedProcess(healthServer.URL, nil)
		},
	}
	service := internalservice.NewWithSupervisorTestConfig(
		scopes,
		mustAssetsService(t, scopes),
		launcher,
		http.DefaultClient,
		realHostClock{},
		nil,
		nil,
		internalservice.SupervisorTestConfig{
			ReadinessTimeout:    75 * time.Millisecond,
			HealthCheckInterval: 10 * time.Millisecond,
		},
	)

	_, err := service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if !errors.Is(err, models.ErrHostLoadingTimeout) {
		t.Fatalf("error = %v, want ErrHostLoadingTimeout", err)
	}

	inspected, err := service.InspectModelHost(context.Background(), models.InspectModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("InspectModelHost: %v", err)
	}
	if inspected.Host.ReadinessState != models.ReadinessStateLoading {
		t.Fatalf("readiness = %s, want LOADING after timeout", inspected.Host.ReadinessState)
	}
	if inspected.Host.Diagnostics["failureClass"] != "loading_timeout" {
		t.Fatalf("failureClass = %q, want loading_timeout", inspected.Host.Diagnostics["failureClass"])
	}
}

func TestEnsureModelHostPostStartCrashSurfacesFailedReadinessOnInspect(t *testing.T) {
	t.Parallel()

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	exitCh := make(chan error, 1)
	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "post-start-crash")
	ref := openScope(t, scopes, cacheDirectory, supervisedRuntimeConfig())
	launcher := &fakeProcessLauncher{
		newProcess: func(spec modelseffects.HostProcessStartSpec) *fakeManagedProcess {
			return newFakeManagedProcess(healthServer.URL, exitCh)
		},
	}
	service := newTestRuntimeHostWithScopesAndClock(t, scopes, launcher, realHostClock{})

	_, err := service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}
	exitCh <- errors.New("unexpected exit")

	deadline := time.Now().Add(2 * time.Second)
	for {
		inspected, inspectErr := service.InspectModelHost(context.Background(), models.InspectModelHostRequest{
			Scope: ref,
			Name:  "OMNIVOICE_Q4_K_M",
		})
		if inspectErr != nil {
			t.Fatalf("InspectModelHost: %v", inspectErr)
		}
		if inspected.Host.ReadinessState == models.ReadinessStateFailed {
			if inspected.Host.Diagnostics["failureClass"] != "process_crash" {
				t.Fatalf("failureClass = %q, want process_crash", inspected.Host.Diagnostics["failureClass"])
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for crash readiness; last state = %s", inspected.Host.ReadinessState)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestEnsureModelHostCancellationStopsManagedProcess(t *testing.T) {
	t.Parallel()

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(healthServer.Close)

	var stopCount atomic.Int32
	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "cancellation")
	ref := openScope(t, scopes, cacheDirectory, supervisedRuntimeConfig())
	launcher := &fakeProcessLauncher{
		newProcess: func(spec modelseffects.HostProcessStartSpec) *fakeManagedProcess {
			process := newFakeManagedProcess(healthServer.URL, nil)
			process.stopFn = func() error {
				stopCount.Add(1)
				return process.defaultStop()
			}
			return process
		},
	}
	service := internalservice.NewWithSupervisorTestConfig(
		scopes,
		mustAssetsService(t, scopes),
		launcher,
		http.DefaultClient,
		realHostClock{},
		nil,
		nil,
		internalservice.SupervisorTestConfig{
			ReadinessTimeout:    time.Second,
			HealthCheckInterval: 25 * time.Millisecond,
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := service.EnsureModelHost(ctx, models.EnsureModelHostRequest{
			Scope: ref,
			Name:  "OMNIVOICE_Q4_K_M",
		})
		errCh <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()

	err := <-errCh
	if !errors.Is(err, models.ErrHostCancelled) {
		t.Fatalf("error = %v, want ErrHostCancelled", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want underlying context.Canceled", err)
	}
	if stopCount.Load() != 1 {
		t.Fatalf("stop count = %d, want 1", stopCount.Load())
	}
}

func TestEnsureModelHostDiagnosticsReadinessTimeoutEmitsFailureLogAndMetric(t *testing.T) {
	t.Parallel()

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(healthServer.Close)

	logger := &capturingDiagnosticsLogger{}
	metrics := &capturingMetricsRecorder{}
	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "timeout-diagnostics")
	ref := openScope(t, scopes, cacheDirectory, supervisedRuntimeConfig())
	launcher := &fakeProcessLauncher{
		newProcess: func(spec modelseffects.HostProcessStartSpec) *fakeManagedProcess {
			return newFakeManagedProcess(healthServer.URL, nil)
		},
	}
	service := internalservice.NewWithSupervisorTestConfig(
		scopes,
		mustAssetsService(t, scopes),
		launcher,
		http.DefaultClient,
		realHostClock{},
		logger,
		metrics,
		internalservice.SupervisorTestConfig{
			ReadinessTimeout:    75 * time.Millisecond,
			HealthCheckInterval: 10 * time.Millisecond,
		},
	)

	_, err := service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err == nil {
		t.Fatal("expected readiness timeout failure")
	}
	if !errors.Is(err, models.ErrHostLoadingTimeout) {
		t.Fatalf("error = %v, want ErrHostLoadingTimeout", err)
	}

	entry, ok := logger.findWarn("model host load failed")
	if !ok {
		t.Fatal("expected model host load failed warning log")
	}
	if entry.fields["managed_runtime_identity"] != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("identity = %q, want OMNIVOICE_Q4_K_M", entry.fields["managed_runtime_identity"])
	}
	if entry.fields["backend"] != "LLAMACPP" {
		t.Fatalf("backend = %q, want LLAMACPP", entry.fields["backend"])
	}
	if entry.fields["failure_class"] != "loading_timeout" {
		t.Fatalf("failure_class = %q, want loading_timeout", entry.fields["failure_class"])
	}
	if !metrics.contains("model_host.load.failure", map[string]string{
		"managed_runtime_identity": "OMNIVOICE_Q4_K_M",
		"failure_class":            "loading_timeout",
	}) {
		t.Fatalf("metrics = %#v, want load failure metric", metrics.metrics)
	}
	if !metrics.contains("model_host.readiness.timeout", map[string]string{
		"managed_runtime_identity": "OMNIVOICE_Q4_K_M",
	}) {
		t.Fatalf("metrics = %#v, want readiness timeout metric", metrics.metrics)
	}
}

func TestEnsureModelHostDiagnosticsProcessCrashEmitsFailureLogAndMetric(t *testing.T) {
	t.Parallel()

	exitCh := make(chan error, 1)
	logger := &capturingDiagnosticsLogger{}
	metrics := &capturingMetricsRecorder{}
	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "crash-diagnostics")
	ref := openScope(t, scopes, cacheDirectory, supervisedRuntimeConfig())
	launcher := &fakeProcessLauncher{
		newProcess: func(spec modelseffects.HostProcessStartSpec) *fakeManagedProcess {
			return newFakeManagedProcess("http://127.0.0.1:1", exitCh)
		},
	}
	service := internalservice.NewWithSupervisorTestConfig(
		scopes,
		mustAssetsService(t, scopes),
		launcher,
		http.DefaultClient,
		realHostClock{},
		logger,
		metrics,
		internalservice.SupervisorTestConfig{
			HealthChecker: alwaysHealthyChecker{},
		},
	)

	_, err := service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}
	exitCh <- errors.New("server exited")

	deadline := time.Now().Add(2 * time.Second)
	for {
		entry, ok := logger.findWarn("model host process crashed")
		if ok {
			if entry.fields["failure_class"] != "process_crash" {
				t.Fatalf("failure_class = %q, want process_crash", entry.fields["failure_class"])
			}
			if !metrics.contains("model_host.process.crash", map[string]string{
				"managed_runtime_identity": "OMNIVOICE_Q4_K_M",
			}) {
				if time.Now().After(deadline) {
					t.Fatalf("metrics = %#v, want process crash metric", metrics.metrics)
				}
				time.Sleep(10 * time.Millisecond)
				continue
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for process crash diagnostics")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type capturingDiagnosticsLogger struct {
	mu   sync.Mutex
	logs []diagnosticLogEntry
}

type diagnosticLogEntry struct {
	level  string
	msg    string
	fields map[string]string
}

func (l *capturingDiagnosticsLogger) Info(msg string, fields map[string]string) {
	l.record("info", msg, fields)
}

func (l *capturingDiagnosticsLogger) Warn(msg string, fields map[string]string) {
	l.record("warn", msg, fields)
}

func (l *capturingDiagnosticsLogger) record(level, msg string, fields map[string]string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cloned := make(map[string]string, len(fields))
	for key, value := range fields {
		cloned[key] = value
	}
	l.logs = append(l.logs, diagnosticLogEntry{
		level:  level,
		msg:    msg,
		fields: cloned,
	})
}

func (l *capturingDiagnosticsLogger) findWarn(msg string) (diagnosticLogEntry, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, entry := range l.logs {
		if entry.level == "warn" && entry.msg == msg {
			return entry, true
		}
	}
	return diagnosticLogEntry{}, false
}

type capturingMetricsRecorder struct {
	mu      sync.Mutex
	metrics []recordedMetric
}

type recordedMetric struct {
	name   string
	labels map[string]string
}

func (r *capturingMetricsRecorder) RecordMetric(name string, labels map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cloned := make(map[string]string, len(labels))
	for key, value := range labels {
		cloned[key] = value
	}
	r.metrics = append(r.metrics, recordedMetric{
		name:   name,
		labels: cloned,
	})
}

func (r *capturingMetricsRecorder) contains(name string, wantLabels map[string]string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, metric := range r.metrics {
		if metric.name != name {
			continue
		}
		if labelsMatch(metric.labels, wantLabels) {
			return true
		}
	}
	return false
}

func labelsMatch(got, want map[string]string) bool {
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}

type alwaysHealthyChecker struct{}

func (alwaysHealthyChecker) Check(context.Context, string) error {
	return nil
}
