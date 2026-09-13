package service_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
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
	var process *fakeManagedProcess
	launcher := &fakeProcessLauncher{
		newProcess: func(spec modelseffects.HostProcessStartSpec) *fakeManagedProcess {
			process = newFakeManagedProcess(healthServer.URL, nil)
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
	if process == nil {
		t.Fatal("readiness timeout did not start a managed process")
	}
	select {
	case <-process.stopCh:
	default:
		t.Fatal("readiness timeout left managed process active")
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
	processStarted := make(chan struct{})
	var processStartedOnce sync.Once
	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "cancellation")
	ref := openScope(t, scopes, cacheDirectory, supervisedRuntimeConfig())
	launcher := &fakeProcessLauncher{
		newProcess: func(spec modelseffects.HostProcessStartSpec) *fakeManagedProcess {
			processStartedOnce.Do(func() { close(processStarted) })
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
	// Observe the injected launcher instead of guessing when startup has begun;
	// the timeout only bounds a broken startup so the regression cannot hang.
	select {
	case <-processStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("managed process did not start before cancellation")
	}
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
	if entry.fields["failure_class"] != "TIMED_OUT" {
		t.Fatalf("failure_class = %q, want TIMED_OUT", entry.fields["failure_class"])
	}
	if _, present := entry.fields["exit_class"]; present {
		t.Fatalf("absent diagnostic source emitted process fields: %#v", entry.fields)
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
			if entry.fields["failure_class"] != "PROCESS_EXITED" {
				t.Fatalf("failure_class = %q, want PROCESS_EXITED", entry.fields["failure_class"])
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

func TestEnsureAndStopModelHostEmitCorrelatedBoundedLifecycleEvidence(t *testing.T) {
	t.Parallel()

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	logger := &capturingDiagnosticsLogger{}
	sink := &runtimeEvidenceSink{}
	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "success-diagnostics")
	ref := openScope(t, scopes, cacheDirectory, supervisedRuntimeConfig())
	var process *fakeManagedProcess
	launcher := &fakeProcessLauncher{
		newProcess: func(spec modelseffects.HostProcessStartSpec) *fakeManagedProcess {
			process = newFakeManagedProcess(healthServer.URL, nil)
			return process
		},
	}
	host := internalservice.NewWithHostTestConfig(
		scopes,
		mustAssetsService(t, scopes),
		launcher,
		http.DefaultClient,
		realHostClock{},
		logger,
		nil,
		internalservice.SupervisorTestConfig{},
		internalservice.HostPolicyTestConfig{},
		runtimehost.Options{
			RuntimeEvidence: modelseffects.NewOrderedRuntimeEvidenceRecorder(sink),
		},
	)
	ctx := modelseffects.WithRuntimeCorrelation(context.Background(), "models-invocation-42")
	request := models.EnsureModelHostRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"}
	if _, err := host.EnsureModelHost(ctx, request); err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}
	if _, err := host.StopModelHost(ctx, models.StopModelHostRequest{
		Scope: ref, Name: "OMNIVOICE_Q4_K_M",
	}); err != nil {
		t.Fatalf("StopModelHost: %v", err)
	}
	assertControlledProcessStopped(t, process)
	assertCorrelatedHostDiagnostics(t, logger.snapshot())
	assertBackendStartEvidence(t, sink.snapshot())
}

func assertControlledProcessStopped(t *testing.T, process *fakeManagedProcess) {
	t.Helper()
	select {
	case <-process.stopCh:
	default:
		t.Fatal("controlled process was not stopped")
	}
}

func assertCorrelatedHostDiagnostics(t *testing.T, entries []diagnosticLogEntry) {
	t.Helper()
	wantMessages := []string{
		"model host load started",
		"model host load ready",
		"model host unload",
		"model host stopped",
	}
	if len(entries) != len(wantMessages) {
		t.Fatalf("diagnostic entries = %#v, want %d lifecycle entries", entries, len(wantMessages))
	}
	for index, entry := range entries {
		assertHostDiagnosticEntry(t, index, entry, wantMessages[index])
	}
}

func assertHostDiagnosticEntry(t *testing.T, index int, entry diagnosticLogEntry, wantMessage string) {
	t.Helper()
	if entry.msg != wantMessage {
		t.Fatalf("diagnostic[%d] message = %q, want %q", index, entry.msg, wantMessage)
	}
	if entry.fields["managed_runtime_identity"] != "OMNIVOICE_Q4_K_M" ||
		entry.fields["backend"] != "LLAMACPP" ||
		entry.fields["revision"] != "rev-test" ||
		entry.fields["correlation_id"] != "models-invocation-42" ||
		entry.fields["duration_millis"] == "" {
		t.Fatalf("diagnostic[%d] fields = %#v, want bounded correlated identity", index, entry.fields)
	}
	if entry.fields["stage"] == "" || entry.fields["outcome"] == "" {
		t.Fatalf("diagnostic[%d] fields = %#v, want stage and outcome", index, entry.fields)
	}
}

func assertBackendStartEvidence(t *testing.T, records []modelseffects.RuntimeEvidenceRecord) {
	t.Helper()
	if len(records) != 1 || records[0].Sequence != 1 ||
		records[0].Stage != modelseffects.RuntimeStageBackendStart ||
		records[0].Outcome != modelseffects.RuntimeEvidenceOutcomeCompleted {
		t.Fatalf("success runtime evidence = %#v, want one completed backend-start record", records)
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

func (l *capturingDiagnosticsLogger) snapshot() []diagnosticLogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	entries := make([]diagnosticLogEntry, len(l.logs))
	copy(entries, l.logs)
	return entries
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

func TestManagedLocalAIPreflightRecordsProtocolStageEvidence(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "runtime-evidence-preflight")
	ref := openScope(t, scopes, cacheDirectory, managedLocalAIConfig(models.LoadPolicyOnDemand))
	sink := &runtimeEvidenceSink{}
	host := internalservice.NewWithHostTestConfig(
		scopes,
		mustAssetsService(t, scopes),
		&recordingProcessLauncher{},
		http.DefaultClient,
		realHostClock{},
		nil,
		nil,
		internalservice.SupervisorTestConfig{},
		internalservice.HostPolicyTestConfig{},
		runtimehost.Options{
			Platform:             managedHostPlatform(),
			CompatibilityChecker: &testCompatibilityChecker{},
			RuntimeEvidence:      modelseffects.NewOrderedRuntimeEvidenceRecorder(sink),
		},
	)

	_, err := host.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: ref,
		Name:  managedModelName,
	})
	if !errors.Is(err, models.ErrHostProtocolIncompatible) {
		t.Fatalf("EnsureModelHost error = %v, want protocol incompatibility", err)
	}
	stage, class, ok := modelseffects.ClassifyRuntimeFailure(err)
	if !ok || stage != modelseffects.RuntimeStageProtocolLoad ||
		class != modelseffects.RuntimeFailureProtocolIncompatible {
		t.Fatalf("runtime classification = (%q, %q, %t), want PROTOCOL_LOAD/PROTOCOL_INCOMPATIBLE", stage, class, ok)
	}
	assertProtocolStageEvidence(t, sink.snapshot())
}

func assertProtocolStageEvidence(t *testing.T, records []modelseffects.RuntimeEvidenceRecord) {
	t.Helper()
	if len(records) != 1 {
		t.Fatalf("runtime evidence records = %d, want one host stage", len(records))
	}
	record := records[0]
	if record.Sequence != 1 || record.Kind != modelseffects.RuntimeEvidenceKindStage ||
		record.Stage != modelseffects.RuntimeStageProtocolLoad ||
		record.Class != modelseffects.RuntimeFailureProtocolIncompatible ||
		record.Outcome != modelseffects.RuntimeEvidenceOutcomeFailed {
		t.Fatalf("runtime evidence record = %#v, want bounded protocol-load failure", record)
	}
}

type runtimeEvidenceSink struct {
	mu      sync.Mutex
	records []modelseffects.RuntimeEvidenceRecord
}

func (sink *runtimeEvidenceSink) RecordRuntimeEvidence(
	record modelseffects.RuntimeEvidenceRecord,
) {
	if sink == nil {
		return
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	sink.records = append(sink.records, record)
}

func (sink *runtimeEvidenceSink) snapshot() []modelseffects.RuntimeEvidenceRecord {
	if sink == nil {
		return nil
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	return append([]modelseffects.RuntimeEvidenceRecord(nil), sink.records...)
}

func runtimeDiagnosticSnapshot(causeCode string) modelseffects.HostProcessDiagnosticSnapshot {
	return modelseffects.HostProcessDiagnosticSnapshot{
		ExitClass:     "NONZERO_EXIT",
		ExitCode:      17,
		ExitCodeKnown: true,
		Stdout: modelseffects.HostProcessStreamDiagnostic{
			Bytes: 98304, SHA256: strings.Repeat("a", 64), Truncated: true,
		},
		Stderr: modelseffects.HostProcessStreamDiagnostic{
			Bytes: 2048, SHA256: strings.Repeat("b", 64), Truncated: false,
		},
		CauseCode:            causeCode,
		CauseMessage:         "PRIVATE_BACKEND_DIAGNOSTIC_SENTINEL",
		CauseMessageRedacted: false,
	}
}

func assertManagedProcessDiagnosticFields(
	t *testing.T,
	fields map[string]string,
	wantCause string,
) {
	t.Helper()
	want := map[string]string{
		"exit_class":             "NONZERO_EXIT",
		"exit_code":              "17",
		"exit_code_known":        "true",
		"stdout_bytes":           "98304",
		"stdout_sha256":          strings.Repeat("a", 64),
		"stdout_truncated":       "true",
		"stderr_bytes":           "2048",
		"stderr_sha256":          strings.Repeat("b", 64),
		"stderr_truncated":       "false",
		"cause_code":             wantCause,
		"cause_message":          causeMessageForTest(wantCause),
		"cause_message_redacted": "false",
	}
	for key, value := range want {
		if fields[key] != value {
			t.Fatalf("diagnostic field %q = %q, want %q; fields = %#v", key, fields[key], value, fields)
		}
	}
	if strings.Contains(fields["cause_message"], "PRIVATE_BACKEND_DIAGNOSTIC_SENTINEL") {
		t.Fatalf("diagnostic fields leaked cause sentinel: %#v", fields)
	}
}

func causeMessageForTest(code string) string {
	switch code {
	case "CANCELLED":
		return "backend operation cancelled"
	case "RPC_REJECTED":
		return "backend RPC request rejected"
	case "TIMEOUT":
		return "backend operation timed out"
	case "PROCESS_EXITED":
		return "managed backend process exited"
	default:
		return ""
	}
}

func assertFailedManagedProcessEvidence(
	t *testing.T,
	records []modelseffects.RuntimeEvidenceRecord,
	wantStage modelseffects.RuntimeStage,
) {
	t.Helper()
	var failed *modelseffects.RuntimeEvidenceRecord
	for index := range records {
		if records[index].Outcome == modelseffects.RuntimeEvidenceOutcomeFailed {
			if failed != nil {
				t.Fatalf("multiple failed evidence records = %#v", records)
			}
			failed = &records[index]
		}
	}
	if failed == nil {
		t.Fatalf("evidence = %#v, want failed managed-process stage", records)
	}
	if failed.Stage != wantStage || failed.ExitClass != "NONZERO_EXIT" ||
		failed.ExitCode != 17 || !failed.ExitCodeKnown ||
		failed.StdoutBytes != 98304 || failed.StderrBytes != 2048 ||
		failed.StdoutSHA256 != strings.Repeat("a", 64) ||
		failed.StderrSHA256 != strings.Repeat("b", 64) {
		t.Fatalf("failed managed-process evidence = %#v, want bounded snapshot", *failed)
	}
	if failed.CauseMessage == "PRIVATE_BACKEND_DIAGNOSTIC_SENTINEL" {
		t.Fatalf("evidence leaked cause sentinel: %#v", *failed)
	}
}

type neverReadyHostChecker struct{}

func (neverReadyHostChecker) Check(context.Context, string) error {
	return models.ErrHostRuntimeNotReady
}

type diagnosticEvidenceSink struct {
	mu          sync.Mutex
	records     []modelseffects.RuntimeEvidenceRecord
	failure     chan struct{}
	failureOnce sync.Once
}

func newDiagnosticEvidenceSink() *diagnosticEvidenceSink {
	return &diagnosticEvidenceSink{failure: make(chan struct{})}
}

func (sink *diagnosticEvidenceSink) RecordRuntimeEvidence(
	record modelseffects.RuntimeEvidenceRecord,
) {
	if sink == nil {
		return
	}
	sink.mu.Lock()
	sink.records = append(sink.records, record)
	sink.mu.Unlock()
	if record.Outcome == modelseffects.RuntimeEvidenceOutcomeFailed {
		sink.failureOnce.Do(func() { close(sink.failure) })
	}
}

func (sink *diagnosticEvidenceSink) awaitFailure(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case <-sink.failure:
	case <-ctx.Done():
		t.Fatal("timed out waiting for failed runtime evidence")
	}
}

func (sink *diagnosticEvidenceSink) snapshot() []modelseffects.RuntimeEvidenceRecord {
	if sink == nil {
		return nil
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	return append([]modelseffects.RuntimeEvidenceRecord(nil), sink.records...)
}

type runtimeDiagnosticLauncher struct {
	mu         sync.Mutex
	newProcess func() *runtimeDiagnosticProcess
	processes  []*runtimeDiagnosticProcess
	started    chan struct{}
	startOnce  sync.Once
}

func (launcher *runtimeDiagnosticLauncher) Start(
	_ context.Context,
	_ modelseffects.HostProcessStartSpec,
) (modelseffects.HostManagedProcess, error) {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	process := launcher.newProcess()
	launcher.processes = append(launcher.processes, process)
	launcher.startOnce.Do(func() {
		if launcher.started != nil {
			close(launcher.started)
		}
	})
	return process, nil
}

func (launcher *runtimeDiagnosticLauncher) process(index int) *runtimeDiagnosticProcess {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.processes[index]
}

func (launcher *runtimeDiagnosticLauncher) startCount() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return len(launcher.processes)
}

type runtimeDiagnosticProcess struct {
	mu                    sync.Mutex
	endpoint              string
	snapshot              modelseffects.HostProcessDiagnosticSnapshot
	diagnosticPanics      bool
	diagnosticUnavailable bool
	diagnosticCalls       int
	ready                 bool
	waitErr               error
	exitCh                chan error
	waited                chan struct{}
	exitOnce              sync.Once
	waitOnce              sync.Once
	stopOnce              sync.Once
	stopCalls             int
}

func newRuntimeDiagnosticProcess(
	snapshot modelseffects.HostProcessDiagnosticSnapshot,
) *runtimeDiagnosticProcess {
	return &runtimeDiagnosticProcess{
		endpoint: "http://127.0.0.1:1",
		snapshot: snapshot,
		exitCh:   make(chan error, 1),
		waited:   make(chan struct{}),
	}
}

func (process *runtimeDiagnosticProcess) HealthEndpoint() string {
	return process.endpoint
}

func (process *runtimeDiagnosticProcess) Wait() error {
	process.waitOnce.Do(func() {
		err := <-process.exitCh
		process.mu.Lock()
		process.waitErr = err
		process.ready = true
		process.mu.Unlock()
		close(process.waited)
	})
	<-process.waited
	process.mu.Lock()
	defer process.mu.Unlock()
	return process.waitErr
}

func (process *runtimeDiagnosticProcess) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	process.stopOnce.Do(func() {
		process.mu.Lock()
		process.stopCalls++
		process.mu.Unlock()
		process.exitOnce.Do(func() { process.exitCh <- errors.New("controlled forced stop") })
	})
	select {
	case <-process.waited:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (process *runtimeDiagnosticProcess) crash(err error) {
	if err == nil {
		err = errors.New("controlled process crash")
	}
	process.exitOnce.Do(func() { process.exitCh <- err })
}

func (process *runtimeDiagnosticProcess) DiagnosticSnapshot() (modelseffects.HostProcessDiagnosticSnapshot, bool) {
	process.mu.Lock()
	process.diagnosticCalls++
	ready := process.ready
	panics := process.diagnosticPanics
	unavailable := process.diagnosticUnavailable
	snapshot := process.snapshot
	process.mu.Unlock()
	if panics {
		panic("controlled diagnostic source failure")
	}
	if !ready || unavailable {
		return modelseffects.HostProcessDiagnosticSnapshot{}, false
	}
	return snapshot, true
}

func (process *runtimeDiagnosticProcess) stopCount() int {
	process.mu.Lock()
	defer process.mu.Unlock()
	return process.stopCalls
}

func (process *runtimeDiagnosticProcess) diagnosticCallCount() int {
	process.mu.Lock()
	defer process.mu.Unlock()
	return process.diagnosticCalls
}

type signalingDiagnosticsLogger struct {
	mu     sync.Mutex
	logs   []diagnosticLogEntry
	events chan diagnosticLogEntry
}

func newSignalingDiagnosticsLogger() *signalingDiagnosticsLogger {
	return &signalingDiagnosticsLogger{events: make(chan diagnosticLogEntry, 32)}
}

func (logger *signalingDiagnosticsLogger) Info(msg string, fields map[string]string) {
	logger.record("info", msg, fields)
}

func (logger *signalingDiagnosticsLogger) Warn(msg string, fields map[string]string) {
	logger.record("warn", msg, fields)
}

func (logger *signalingDiagnosticsLogger) record(level, msg string, fields map[string]string) {
	cloned := make(map[string]string, len(fields))
	for key, value := range fields {
		cloned[key] = value
	}
	entry := diagnosticLogEntry{level: level, msg: msg, fields: cloned}
	logger.mu.Lock()
	logger.logs = append(logger.logs, entry)
	logger.mu.Unlock()
	select {
	case logger.events <- entry:
	default:
	}
}

func (logger *signalingDiagnosticsLogger) awaitMessage(
	t *testing.T,
	wantMessage string,
) diagnosticLogEntry {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		select {
		case entry := <-logger.events:
			if entry.msg == wantMessage {
				return entry
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for diagnostic message %q", wantMessage)
			return diagnosticLogEntry{}
		}
	}
}

func (logger *signalingDiagnosticsLogger) countMessage(wantMessage string) int {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	count := 0
	for _, entry := range logger.logs {
		if entry.msg == wantMessage {
			count++
		}
	}
	return count
}
