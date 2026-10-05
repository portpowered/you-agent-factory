package internal

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"go.uber.org/zap"
)

func TestRuntimeSinkOpeningFailsClosedWithoutInjectedOwners(t *testing.T) {
	t.Parallel()
	if _, _, err := openRuntimeLogScope(nil, RuntimeFileLoggingPolicyEnabled, "/logs", factory.RuntimeLogStorageConfig{}, "session", "/folder", "/factory", "runtime-1"); err == nil || !strings.Contains(err.Error(), "owner is required") {
		t.Fatalf("log opening error = %v, want required owner", err)
	}
	if _, err := openRuntimeMetricsScope(nil, RuntimeMetricsPolicyEnabled, "/metrics", factory.RuntimeMetricsStorageConfig{}, "session", "runtime-1", "/folder", "/factory"); err == nil || !strings.Contains(err.Error(), "owner is required") {
		t.Fatalf("metrics opening error = %v, want required owner", err)
	}
}

func TestRuntimeSinkOpeningDisabledPolicyNeedsNoOwner(t *testing.T) {
	t.Parallel()
	logSink, runtimeID, err := openRuntimeLogScope(nil, RuntimeFileLoggingPolicyDisabled, "", factory.RuntimeLogStorageConfig{}, "session", "/folder", "/factory", "runtime-1")
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
	owner := runtimeLogOwnerFunc(func(factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error) {
		return &runtimeSinkStub{}, nil
	})
	if _, _, err := openRuntimeLogScope(owner, RuntimeFileLoggingPolicyEnabled, "/logs", factory.RuntimeLogStorageConfig{}, "session", "/folder", "/factory", ""); err == nil || !strings.Contains(err.Error(), "runtime instance ID is required") {
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
	fixture.logOwner = func(request factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error) {
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
		fixture.logOwner, RuntimeFileLoggingPolicyEnabled, "/logs", factory.RuntimeLogStorageConfig{MaxSize: 7},
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

type runtimeLogOwnerFunc func(factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error)

func (owner runtimeLogOwnerFunc) Open(request factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error) {
	return owner(request)
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
