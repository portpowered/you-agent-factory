package internal

import (
	"context"
	"errors"
	"github.com/jonboulle/clockwork"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/build"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"strings"
	"sync"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRuntimeSessionLoggerPreservesSelectedBackendAndScope(t *testing.T) {
	t.Parallel()
	baseCore, baseLogs := observer.New(zap.InfoLevel)
	sinkCore, sinkLogs := observer.New(zap.InfoLevel)
	base := zap.New(baseCore).With(zap.String("backend", "selected-process"))
	sink := &runtimeLoggerSinkStub{logger: zap.New(sinkCore).With(zap.String("backend", "selected-sink"))}
	for _, scenario := range []struct {
		name    string
		sink    factory.RuntimeLogSink
		logs    *observer.ObservedLogs
		backend string
	}{
		{name: "process", logs: baseLogs, backend: "selected-process"},
		{name: "sink", sink: sink, logs: sinkLogs, backend: "selected-sink"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			selected := runtimeSessionBaseLogger(base, scenario.sink)
			first := newSessionLogger(selected, "session-one", "/folder-one", "/factory-one")
			peer := newSessionLogger(selected, "session-two", "/folder-two", "/factory-two")
			first.Info("opening first")
			peer.Info("opening peer")
			selected.Info("unscoped backend")
			entries := scenario.logs.All()
			if len(entries) != 3 {
				t.Fatalf("selected backend received %d entries, want 3", len(entries))
			}
			for i, session := range []string{"one", "two"} {
				fields := entries[i].ContextMap()
				if fields["backend"] != scenario.backend || fields["session_id"] != "session-"+session ||
					fields["folder_path"] != "/folder-"+session || fields["factory_dir"] != "/factory-"+session {
					t.Fatalf("session log fields = %#v", fields)
				}
			}
			fields := entries[2].ContextMap()
			if fields["backend"] != scenario.backend || len(fields) != 1 {
				t.Fatalf("session fields leaked to selected backend: %#v", fields)
			}
		})
	}
}

type runtimeLoggerSinkStub struct {
	runtimeSinkStub
	logger *zap.Logger
}

func (sink *runtimeLoggerSinkStub) Logger() *zap.Logger { return sink.logger }

func TestRuntimeSinkOpeningFailsClosedWithoutInjectedOwners(t *testing.T) {
	t.Parallel()
	if _, _, err := openRuntimeLogScope(nil, zap.NewNop(), RuntimeFileLoggingPolicyEnabled, "/logs", factory.RuntimeLogStorageConfig{}, "session", "/folder", "/factory", "runtime-1"); err == nil || !strings.Contains(err.Error(), "owner is required") {
		t.Fatalf("log opening error = %v, want required owner", err)
	}
	if _, err := openRuntimeMetricsScope(nil, RuntimeMetricsPolicyEnabled, "/metrics", factory.RuntimeMetricsStorageConfig{}, "session", "runtime-1", "/folder", "/factory"); err == nil || !strings.Contains(err.Error(), "owner is required") {
		t.Fatalf("metrics opening error = %v, want required owner", err)
	}
}

func TestRuntimeSinkOpeningDisabledPolicyNeedsNoOwner(t *testing.T) {
	t.Parallel()
	logSink, runtimeID, err := openRuntimeLogScope(nil, zap.NewNop(), RuntimeFileLoggingPolicyDisabled, "", factory.RuntimeLogStorageConfig{}, "session", "/folder", "/factory", "runtime-1")
	if err != nil || logSink != nil || runtimeID != "runtime-1" {
		t.Fatalf("disabled log opening = (%v, %q, %v)", logSink, runtimeID, err)
	}
	metricsSink, err := openRuntimeMetricsScope(nil, RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{}, "session", "runtime-1", "", "")
	if err != nil || metricsSink != nil {
		t.Fatalf("disabled metrics opening = (%v, %v)", metricsSink, err)
	}
}

func TestRuntimeLogOpeningRejectsAmbientIdentityFallbacks(t *testing.T) {
	t.Parallel()
	owner := runtimeLogOwnerFunc(func(*zap.Logger, factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error) {
		return &runtimeSinkStub{}, nil
	})
	if _, _, err := openRuntimeLogScope(owner, zap.NewNop(), RuntimeFileLoggingPolicyEnabled, "/logs", factory.RuntimeLogStorageConfig{}, "session", "/folder", "/factory", ""); err == nil || !strings.Contains(err.Error(), "runtime instance ID is required") {
		t.Fatalf("empty runtime ID error = %v", err)
	}
}

func TestRuntimeObservabilityOpeningForwardsValues(t *testing.T) {
	t.Parallel()
	fixture := newRuntimeScopeOpeningFixture()
	openedLog, openedMetrics := openRuntimeScopeFixture(t, fixture)
	defer openedLog.Close()
	defer openedMetrics.Close()
	if fixture.logRequest.RuntimeInstanceID != "runtime-1" || fixture.logRequest.SessionID != "session-1" ||
		fixture.logRequest.RootDirectory != "/logs" || fixture.logRequest.Config.MaxSize != 7 {
		t.Fatalf("log scope request = %#v", fixture.logRequest)
	}
	if fixture.metricsRequest.Scope.SessionID != "session-1" || fixture.metricsRequest.Scope.RuntimeInstanceID != "runtime-1" ||
		fixture.metricsRequest.RootDirectory != "/metrics" || fixture.metricsRequest.Config.MaxAge != 9 {
		t.Fatalf("metrics scope request = %#v", fixture.metricsRequest)
	}
}

func TestRuntimeObservabilityOpeningClosesScopesOnce(t *testing.T) {
	t.Parallel()
	fixture := newRuntimeScopeOpeningFixture()
	openedLog, openedMetrics := openRuntimeScopeFixture(t, fixture)
	if err := openedLog.Close(); err != nil {
		t.Fatalf("first log close: %v", err)
	}
	if err := openedLog.Close(); err != nil {
		t.Fatalf("second log close: %v", err)
	}
	if err := openedMetrics.Close(); err != nil {
		t.Fatalf("first metrics close: %v", err)
	}
	if err := openedMetrics.Close(); err != nil {
		t.Fatalf("second metrics close: %v", err)
	}
	if fixture.logScope.closeCalls != 1 || fixture.metricsScope.closeCalls != 1 {
		t.Fatalf("scope close calls = (%d, %d), want exactly once", fixture.logScope.closeCalls, fixture.metricsScope.closeCalls)
	}
}

func TestRuntimeObservabilityOpeningRetriesFailedClose(t *testing.T) {
	t.Parallel()
	fixture := newRuntimeScopeOpeningFixture()
	closeErr := errors.New("scope remains open")
	fixture.logScope.closeErr = closeErr
	fixture.metricsScope.closeErr = closeErr
	openedLog, openedMetrics := openRuntimeScopeFixture(t, fixture)
	for _, scope := range []interface{ Close() error }{openedLog, openedMetrics} {
		if err := scope.Close(); !errors.Is(err, closeErr) {
			t.Fatalf("failed close = %v, want %v", err, closeErr)
		}
	}
	fixture.logScope.closeErr = nil
	fixture.metricsScope.closeErr = nil
	var callers sync.WaitGroup
	for range 8 {
		callers.Add(1)
		go func() {
			defer callers.Done()
			if err := openedLog.Close(); err != nil {
				t.Errorf("retry log close: %v", err)
			}
			if err := openedMetrics.Close(); err != nil {
				t.Errorf("retry metrics close: %v", err)
			}
		}()
	}
	callers.Wait()
	if fixture.logScope.closeCalls != 2 || fixture.metricsScope.closeCalls != 2 {
		t.Fatalf("close calls = (%d, %d), want one failure and one success per scope", fixture.logScope.closeCalls, fixture.metricsScope.closeCalls)
	}
}

func TestRuntimeObservabilityOpeningRetainsPartialScopesWithRetryableClose(t *testing.T) {
	t.Parallel()
	openingErr, releaseErr := errors.New("partial opening"), errors.New("release failed")
	logScope := &runtimeSinkStub{closeErr: releaseErr}
	metricsScope := &runtimeMetricsSinkStub{closeErr: releaseErr}
	logOwner := runtimeLogOwnerFunc(func(*zap.Logger, factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error) {
		return logScope, openingErr
	})
	metricsOwner := runtimeMetricsOwnerFunc(func(factory.RuntimeMetricsScopeRequest) (factory.RuntimeMetricsSink, error) {
		return metricsScope, openingErr
	})
	logSink, runtimeID, err := openRuntimeLogScope(logOwner, zap.NewNop(), RuntimeFileLoggingPolicyEnabled,
		"/logs", factory.RuntimeLogStorageConfig{}, "session", "/folder", "/factory", "runtime")
	if logSink == nil || runtimeID != "runtime" || !errors.Is(err, openingErr) {
		t.Fatalf("partial log opening = (%v, %q, %v)", logSink, runtimeID, err)
	}
	metricsSink, err := openRuntimeMetricsScope(metricsOwner, RuntimeMetricsPolicyEnabled,
		"/metrics", factory.RuntimeMetricsStorageConfig{}, "session", "runtime", "/folder", "/factory")
	if metricsSink == nil || !errors.Is(err, openingErr) {
		t.Fatalf("partial metrics opening = (%v, %v)", metricsSink, err)
	}
	for _, sink := range []interface{ Close() error }{logSink, metricsSink} {
		if err := sink.Close(); !errors.Is(err, releaseErr) {
			t.Fatalf("failed release = %v", err)
		}
	}
	logScope.closeErr, metricsScope.closeErr = nil, nil
	for _, sink := range []interface{ Close() error }{logSink, metricsSink} {
		if err := errors.Join(sink.Close(), sink.Close()); err != nil {
			t.Fatalf("release retry = %v", err)
		}
	}
	if logScope.closeCalls != 2 || metricsScope.closeCalls != 2 {
		t.Fatalf("release calls = %d/%d, want failure then one success each", logScope.closeCalls, metricsScope.closeCalls)
	}
}

type runtimeScopeOpeningFixture struct {
	logOwner       runtimeLogOwnerFunc
	metricsOwner   runtimeMetricsOwnerFunc
	logRequest     *factory.RuntimeLogScopeRequest
	metricsRequest *factory.RuntimeMetricsScopeRequest
	logScope       *runtimeSinkStub
	metricsScope   *runtimeMetricsSinkStub
}

func newRuntimeScopeOpeningFixture() runtimeScopeOpeningFixture {
	fixture := runtimeScopeOpeningFixture{
		logRequest:     &factory.RuntimeLogScopeRequest{},
		metricsRequest: &factory.RuntimeMetricsScopeRequest{},
		logScope:       &runtimeSinkStub{},
		metricsScope:   &runtimeMetricsSinkStub{},
	}
	fixture.logOwner = func(_ *zap.Logger, request factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error) {
		*fixture.logRequest = request
		return fixture.logScope, nil
	}
	fixture.metricsOwner = func(request factory.RuntimeMetricsScopeRequest) (factory.RuntimeMetricsSink, error) {
		*fixture.metricsRequest = request
		return fixture.metricsScope, nil
	}
	return fixture
}

func openRuntimeScopeFixture(t *testing.T, fixture runtimeScopeOpeningFixture) (factory.RuntimeLogSink, factory.RuntimeMetricsSink) {
	t.Helper()
	logSink, _, err := openRuntimeLogScope(
		fixture.logOwner, zap.NewNop(), RuntimeFileLoggingPolicyEnabled, "/logs", factory.RuntimeLogStorageConfig{MaxSize: 7},
		"session-1", "/folder", "/factory", "runtime-1",
	)
	if err != nil {
		t.Fatalf("openRuntimeLogScope(): %v", err)
	}
	metricsSink, err := openRuntimeMetricsScope(
		fixture.metricsOwner, RuntimeMetricsPolicyEnabled, "/metrics", factory.RuntimeMetricsStorageConfig{MaxAge: 9},
		"session-1", "runtime-1", "/folder", "/factory",
	)
	if err != nil {
		t.Fatalf("openRuntimeMetricsScope(): %v", err)
	}
	return logSink, metricsSink
}

type runtimeLogOwnerFunc func(*zap.Logger, factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error)

func (owner runtimeLogOwnerFunc) Open(logger *zap.Logger, request factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error) {
	return owner(logger, request)
}

type runtimeMetricsOwnerFunc func(factory.RuntimeMetricsScopeRequest) (factory.RuntimeMetricsSink, error)

func (owner runtimeMetricsOwnerFunc) Open(request factory.RuntimeMetricsScopeRequest) (factory.RuntimeMetricsSink, error) {
	return owner(request)
}

type runtimeSinkStub struct {
	mu         sync.Mutex
	closeCalls int
	closeErr   error
}

func (*runtimeSinkStub) Logger() *zap.Logger { return zap.NewNop() }
func (*runtimeSinkStub) Artifact() factory.RuntimeLogArtifact {
	return factory.RuntimeLogArtifact{}
}
func (sink *runtimeSinkStub) Close() error {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	sink.closeCalls++
	return sink.closeErr
}

type runtimeMetricsSinkStub struct {
	mu         sync.Mutex
	closeCalls int
	closeErr   error
}

func (*runtimeMetricsSinkStub) Counter(context.Context, string, float64, factory.Fields) error {
	return nil
}
func (*runtimeMetricsSinkStub) Gauge(context.Context, string, float64, factory.Fields) error {
	return nil
}
func (*runtimeMetricsSinkStub) Sample(context.Context, string, float64, string, factory.Fields) error {
	return nil
}
func (sink *runtimeMetricsSinkStub) Close() error {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	sink.closeCalls++
	return sink.closeErr
}
func (*runtimeMetricsSinkStub) Path() string { return "metrics" }
func (*runtimeMetricsSinkStub) Artifact() factory.RuntimeMetricsArtifact {
	return factory.RuntimeMetricsArtifact{}
}

// initialPreparationFake deliberately supplies a replacement-scoped path. The
// initial operation must select its invocation path after preparation.
type initialPreparationFake struct {
	runtimebuild.ExecutionPreparation
}

func (*initialPreparationFake) PrepareExecutionValues(_ context.Context, defaults runtimebuild.BuildDefaults,
	values runtimebuild.SessionBuildValues, _ *zap.Logger, _ providers.Service,
	_ platformprocess.CommandRunner, _ *workers.MockWorkersConfig,
) (runtimebuild.PreparedExecutionValues, error) {
	return runtimebuild.PreparedExecutionValues{PreparedSessionValues: runtimebuild.PreparedSessionValues{
		SessionID: values.SessionID, RecordPath: runtimebuild.SessionScopedRecordPath(defaults.RecordPath, values.SessionID),
	}}, nil
}

func (*assemblyWorldStateOpening) ReplayExecution(*recordings.ReplayArtifact) (
	providers.Service, platformprocess.CommandRunner, []recordings.ReplayHook, recordings.CompletionDeliveryPlanner, error,
) {
	return nil, nil, nil, nil, nil
}

func TestAssemblyInitialRecordingPathsRetainInvocationSelection(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ name, path, want string }{
		{"empty", "", ""},
		{"default", "recording.json", "recording.json"},
		{"explicit", "/recordings/chosen.json", "/recordings/chosen.json"},
		{"placeholder", "/recordings/__factory_session_id__.json", "/recordings/selected.json"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			clock := clockwork.NewFakeClock()
			assembly := &Assembly{preparation: &initialPreparationFake{}, recordingsRuntime: &assemblyWorldStateOpening{}}
			request := factory.RuntimeActivationRequest{FactorySessionID: "selected"}
			request.Inputs.Recordings.RecordPath = test.path
			spec, err := assembly.prepareInitialOpening(t.Context(), request, nil, clock, zap.NewNop(), nil, nil)
			if err != nil || spec.RecordPath != test.want || spec.SessionID != "selected" || spec.Clock != clock {
				t.Fatalf("initial spec = %#v, %v; want path %q and addressed identity/clock", spec, err, test.want)
			}
		})
	}
}
