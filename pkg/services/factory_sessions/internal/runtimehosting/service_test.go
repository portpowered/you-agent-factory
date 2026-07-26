package runtimehosting

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type lifecycleRuntime struct {
	mu            sync.Mutex
	events        []string
	waitErr       error
	failErr       error
	completeErr   error
	failedBecause error
	hosted        factoryruntime.HostedInstance
}

func (*lifecycleRuntime) StartLifecycle(context.Context, context.Context) error {
	panic("unexpected StartLifecycle")
}

func (*lifecycleRuntime) StartWorkerLifecycle(context.Context) (factorysessions.RuntimeStop, error) {
	panic("unexpected StartWorkerLifecycle")
}

func (runtime *lifecycleRuntime) CompleteStartup(context.Context) error {
	runtime.record("complete")
	return runtime.completeErr
}

func (runtime *lifecycleRuntime) WaitForRuntime(context.Context) error {
	runtime.record("wait")
	return runtime.waitErr
}

func (*lifecycleRuntime) StopLifecycle(context.Context) error {
	panic("unexpected StopLifecycle")
}

func (runtime *lifecycleRuntime) FailStartup(err error) error {
	runtime.record("fail")
	runtime.failedBecause = err
	return runtime.failErr
}

func (runtime *lifecycleRuntime) CurrentRuntimeBundle() factoryruntime.HostedInstance {
	return runtime.hosted
}

func (runtime *lifecycleRuntime) record(event string) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	runtime.events = append(runtime.events, event)
}

func (runtime *lifecycleRuntime) recordedEvents() []string {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return append([]string(nil), runtime.events...)
}

func TestServiceRunHostsAPICompletesStartupAndWaitsForRuntime(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	stopped := make(chan struct{})
	observedBinding := make(chan factorysessions.RuntimeHostBinding, 1)
	host := New(func(
		ctx context.Context,
		request platformhttpserver.StartRequest,
	) error {
		if request.Handler == nil || request.Port != 8123 || !request.AutoPort || request.Logger == nil || request.OnBound == nil {
			return errors.New("unexpected API host input")
		}
		close(started)
		request.OnBound(platformhttpserver.Binding{Port: request.Port})
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	})
	runtime := &lifecycleRuntime{}

	err := host.Run(
		context.Background(),
		http.NewServeMux(),
		runtime,
		zap.NewNop(),
		factorysessions.RuntimeHostRequest{
			RuntimeMode: interfaces.RuntimeModeService,
			WorkFile:    "work.json",
			Port:        8123,
			AutoPort:    true,
		},
		func(binding factorysessions.RuntimeHostBinding) {
			runtime.record("observe")
			observedBinding <- binding
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	select {
	case <-started:
	default:
		t.Fatal("API starter was not called")
	}
	select {
	case <-stopped:
	default:
		t.Fatal("Run returned before the API host joined")
	}
	if got := runtime.recordedEvents(); strings.Join(got, ",") != "complete,observe,wait" {
		t.Fatalf("runtime events = %v, want [complete observe wait]", got)
	}
	if got := <-observedBinding; got.Port != 8123 {
		t.Fatalf("observed binding = %+v, want port 8123", got)
	}
}

func TestServiceRunDoesNotReportReadinessWhenRuntimeStartupFails(t *testing.T) {
	t.Parallel()

	completeErr := errors.New("runtime startup failed")
	observed := false
	host := New(func(ctx context.Context, request platformhttpserver.StartRequest) error {
		request.OnBound(platformhttpserver.Binding{Port: request.Port})
		<-ctx.Done()
		return ctx.Err()
	})
	runtime := &lifecycleRuntime{completeErr: completeErr}

	err := host.Run(
		context.Background(),
		http.NewServeMux(),
		runtime,
		zap.NewNop(),
		factorysessions.RuntimeHostRequest{Port: 8123},
		func(factorysessions.RuntimeHostBinding) { observed = true },
	)
	if !errors.Is(err, completeErr) {
		t.Fatalf("Run() error = %v, want %v", err, completeErr)
	}
	if observed {
		t.Fatal("runtime host reported readiness after startup failure")
	}
	if got := runtime.recordedEvents(); strings.Join(got, ",") != "complete" {
		t.Fatalf("runtime events = %v, want [complete]", got)
	}
}

func TestServiceRunReadinessFailureTransitionsRuntimeStartupFailure(t *testing.T) {
	t.Parallel()

	apiErr := errors.New("listener failed")
	failErr := errors.New("startup transition recorded")
	observed := false
	host := New(func(context.Context, platformhttpserver.StartRequest) error {
		return apiErr
	})
	runtime := &lifecycleRuntime{failErr: failErr}

	err := host.Run(
		context.Background(),
		http.NewServeMux(),
		runtime,
		zap.NewNop(),
		factorysessions.RuntimeHostRequest{
			RuntimeMode: interfaces.RuntimeModeService,
			WorkFile:    "work.json",
			Port:        8123,
		},
		func(factorysessions.RuntimeHostBinding) { observed = true },
	)
	if !errors.Is(err, failErr) {
		t.Fatalf("Run() error = %v, want %v", err, failErr)
	}
	if !errors.Is(runtime.failedBecause, apiErr) {
		t.Fatalf("failure cause = %v, want wrapped %v", runtime.failedBecause, apiErr)
	}
	if got := runtime.recordedEvents(); strings.Join(got, ",") != "fail" {
		t.Fatalf("runtime events = %v, want [fail]", got)
	}
	if observed {
		t.Fatal("runtime host reported readiness after listener startup failure")
	}
}

func TestServiceRunReportsNonCancellationRuntimeFailure(t *testing.T) {
	t.Parallel()

	waitErr := errors.New("runtime stopped")
	runtime := &lifecycleRuntime{waitErr: waitErr}
	err := New(nil).Run(
		context.Background(),
		http.NewServeMux(),
		runtime,
		zap.NewNop(),
		factorysessions.RuntimeHostRequest{},
		nil,
	)
	if !errors.Is(err, waitErr) || !strings.Contains(err.Error(), "factory run") {
		t.Fatalf("Run() error = %v, want wrapped runtime failure", err)
	}
}

func TestServiceRunRejectsMissingHTTPStarterWhenHostingIsRequested(t *testing.T) {
	t.Parallel()

	failErr := errors.New("startup failure recorded")
	runtime := &lifecycleRuntime{failErr: failErr}
	err := New(nil).Run(
		context.Background(), http.NewServeMux(), runtime, zap.NewNop(),
		factorysessions.RuntimeHostRequest{Port: 8123},
		nil,
	)
	if !errors.Is(err, failErr) || runtime.failedBecause == nil ||
		!strings.Contains(runtime.failedBecause.Error(), "HTTP starter is required") {
		t.Fatalf("Run() error = %v, cause = %v, want required HTTP starter", err, runtime.failedBecause)
	}
	if got := runtime.recordedEvents(); strings.Join(got, ",") != "fail" {
		t.Fatalf("runtime events = %v, want [fail]", got)
	}
}

func TestServiceRunLogsHostedRuntimeDiagnostics(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)
	startedAt := time.Date(2026, time.July, 20, 10, 11, 12, 13, time.UTC)
	runtime := &lifecycleRuntime{hosted: hostedRuntime{
		logger: logger,
		diagnostics: factoryruntime.RuntimeLogDiagnostics{
			Path: "/logs/runtime.jsonl", RootDir: "/logs", StartTimeUTC: startedAt,
			MaxSizeMB: 10, MaxBackups: 3, MaxAgeDays: 7, Compress: true,
		},
	}}

	err := New(func(ctx context.Context, request platformhttpserver.StartRequest) error {
		request.OnBound(platformhttpserver.Binding{Port: request.Port + 1})
		<-ctx.Done()
		return ctx.Err()
	}).Run(
		context.Background(),
		http.NewServeMux(),
		runtime,
		zap.NewNop(),
		factorysessions.RuntimeHostRequest{
			Directory: "/factory", RuntimeMode: interfaces.RuntimeModeService,
			MockWorkers: true, Port: 8123,
		},
		nil,
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	entries := observed.FilterMessage("factory started").All()
	if len(entries) != 1 {
		t.Fatalf("factory-started log count = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	for name, want := range map[string]any{
		"dir":              "/factory",
		"runtime_log_path": "/logs/runtime.jsonl",
		"runtime_mode":     string(interfaces.RuntimeModeService),
		"mock-workers":     true,
		"port":             int64(8124),
	} {
		if got := fields[name]; got != want {
			t.Errorf("log field %s = %#v, want %#v", name, got, want)
		}
	}
}

type hostedRuntime struct {
	logger      *zap.Logger
	diagnostics factoryruntime.RuntimeLogDiagnostics
}

func (hostedRuntime) RuntimeService() factoryruntime.Service { return nil }
func (hostedRuntime) Directory() string                      { return "" }
func (hostedRuntime) FolderDirectory() string                { return "" }
func (hostedRuntime) BackendScope() string                   { return "" }
func (hostedRuntime) StartTime() time.Time                   { return time.Time{} }
func (hostedRuntime) LoadedRuntimeConfig() factoryruntime.LoadedConfig {
	return nil
}
func (hostedRuntime) CanonicalEvents() []interfaces.FactoryEvent { return nil }
func (hostedRuntime) AddEventTypeRecorder(func(interfaces.FactoryEventType)) {
}
func (hostedRuntime) StreamGeneration() string                      { return "" }
func (runtime hostedRuntime) RuntimeLogger() *zap.Logger            { return runtime.logger }
func (hostedRuntime) RuntimeMetrics() factoryruntime.MetricsEmitter { return nil }
func (runtime hostedRuntime) RuntimeDiagnostics() factoryruntime.RuntimeLogDiagnostics {
	return runtime.diagnostics
}
func (hostedRuntime) RecordingLedger() recordings.Ledger { return nil }
func (hostedRuntime) CloseArtifacts() error              { return nil }
