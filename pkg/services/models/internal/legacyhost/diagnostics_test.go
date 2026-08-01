package modelhost

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

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
	l.logs = append(l.logs, diagnosticLogEntry{
		level:  level,
		msg:    msg,
		fields: cloneDiagnosticLabels(fields),
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
	r.metrics = append(r.metrics, recordedMetric{
		name:   name,
		labels: cloneDiagnosticLabels(labels),
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

func TestCatalogHost_Diagnostics_ReadinessTimeoutEmitsFailureLogAndMetric(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(healthServer.Close)

	logger := &capturingDiagnosticsLogger{}
	metrics := &capturingMetricsRecorder{}
	loaded := mustLoadedCatalogConfig(t, supervisedCatalogFactoryConfig())
	host := newSupervisedTestHost(t, &fakeProcessLauncher{
		newProcess: func(spec ProcessStartSpec) *fakeManagedProcess {
			return newFakeManagedProcess(healthServer.URL, nil)
		},
	})
	host.diagnostics = Diagnostics{Logger: logger, Metrics: metrics}
	host.supervisor.Diagnostics = host.diagnostics
	host.supervisor.ReadinessTimeout = 75 * time.Millisecond
	host.supervisor.HealthCheckInterval = 10 * time.Millisecond

	_, err := host.AcquireLease(context.Background(), loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{})
	if err == nil {
		t.Fatal("expected readiness timeout failure")
	}
	if !errors.Is(err, ErrLoadingTimeout) {
		t.Fatalf("error = %v, want ErrLoadingTimeout", err)
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
	if entry.fields["failure_class"] != string(FailureClassLoadingTimeout) {
		t.Fatalf("failure_class = %q, want %s", entry.fields["failure_class"], FailureClassLoadingTimeout)
	}
	if !metrics.contains(metricLoadFailure, map[string]string{
		"managed_runtime_identity": "OMNIVOICE_Q4_K_M",
		"failure_class":            string(FailureClassLoadingTimeout),
	}) {
		t.Fatalf("metrics = %#v, want load failure metric", metrics.metrics)
	}
	if !metrics.contains(metricReadinessTimeout, map[string]string{
		"managed_runtime_identity": "OMNIVOICE_Q4_K_M",
	}) {
		t.Fatalf("metrics = %#v, want readiness timeout metric", metrics.metrics)
	}
}

func TestCatalogHost_Diagnostics_ProcessCrashEmitsFailureLogAndMetric(t *testing.T) {
	exitCh := make(chan error, 1)

	logger := &capturingDiagnosticsLogger{}
	metrics := &capturingMetricsRecorder{}
	loaded := mustLoadedCatalogConfig(t, supervisedCatalogFactoryConfig())
	host := newSupervisedTestHost(t, &fakeProcessLauncher{
		newProcess: func(spec ProcessStartSpec) *fakeManagedProcess {
			return newFakeManagedProcess("http://127.0.0.1:1", exitCh)
		},
	})
	host.diagnostics = Diagnostics{Logger: logger, Metrics: metrics}
	host.supervisor.Diagnostics = host.diagnostics
	host.supervisor.HealthChecker = alwaysHealthyChecker{}

	lease, err := host.AcquireLease(context.Background(), loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{})
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	exitCh <- errors.New("server exited")

	deadline := time.Now().Add(2 * time.Second)
	for {
		entry, ok := logger.findWarn("model host process crashed")
		if ok {
			if entry.fields["failure_class"] != string(FailureClassProcessCrash) {
				t.Fatalf("failure_class = %q, want process_crash", entry.fields["failure_class"])
			}
			if !metrics.contains(metricProcessCrash, map[string]string{
				"managed_runtime_identity": "OMNIVOICE_Q4_K_M",
			}) {
				if time.Now().After(deadline) {
					t.Fatalf("metrics = %#v, want process crash metric", metrics.metrics)
				}
				time.Sleep(10 * time.Millisecond)
				continue
			}
			_ = host.ReleaseLease(context.Background(), lease.ID)
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for process crash diagnostics")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type alwaysHealthyChecker struct{}

func (alwaysHealthyChecker) Check(context.Context, string) error {
	return nil
}
