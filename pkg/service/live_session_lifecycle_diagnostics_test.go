package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestObserveLiveLifecycleControl_LogsAcceptedPauseWithoutSensitiveFields(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	harness.svc.logger = zap.New(core)
	defer harness.stop(t)

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{
			RequestId: strPtr("pause-req-001"),
			Reason:    strPtr("operator pause with secret /Users/me/prompt.txt"),
		},
	); err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}

	entry := findLifecycleControlLog(t, observed, "factory session lifecycle control")
	assertLogField(t, entry, "session_id", defaultFactorySessionID)
	assertLogField(t, entry, "operation", "PAUSE")
	assertLogField(t, entry, "outcome", "ACCEPTED")
	assertLogField(t, entry, "lifecycle_control_status", "PAUSED")
	assertLogField(t, entry, "request_id", "pause-req-001")
	assertLogDoesNotContain(t, entry, "prompt")
	assertLogDoesNotContain(t, entry, "/Users/")
}

func TestObserveLiveLifecycleControl_LogsNoOpRepeatPause(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	harness.svc.logger = zap.New(core)
	defer harness.stop(t)

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("initial PauseLiveFactorySession: %v", err)
	}
	observed.TakeAll()

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("repeat PauseLiveFactorySession: %v", err)
	}

	entry := findLifecycleControlLog(t, observed, "factory session lifecycle control")
	assertLogField(t, entry, "outcome", "NO_OP")
}

func TestObserveLiveLifecycleControl_LogsInvalidStateResume(t *testing.T) {
	core, observed := observer.New(zap.WarnLevel)
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	harness.svc.logger = zap.New(core)
	defer harness.stop(t)

	_, err := harness.svc.ResumeLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	)
	if err == nil {
		t.Fatal("ResumeLiveFactorySession = nil, want invalid state")
	}

	entry := findLifecycleControlLog(t, observed, "factory session lifecycle control rejected")
	assertLogField(t, entry, "operation", "RESUME")
	assertLogField(t, entry, "outcome", string(factorysessionexecution.LifecycleControlOutcomeInvalidState))
}

func TestObserveLiveLifecycleControl_LogsNotFound(t *testing.T) {
	core, observed := observer.New(zap.WarnLevel)
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	harness.svc.logger = zap.New(core)
	defer harness.stop(t)

	_, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		"live-session-missing-001",
		factoryapi.FactorySessionLifecycleControlRequest{},
	)
	if err == nil {
		t.Fatal("PauseLiveFactorySession = nil, want not found")
	}
	if !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		t.Fatalf("error = %v, want ErrFactorySessionNotFound", err)
	}

	entry := findLifecycleControlLog(t, observed, "factory session lifecycle control rejected")
	assertLogField(t, entry, "session_id", "live-session-missing-001")
	assertLogField(t, entry, "outcome", "NOT_FOUND")
}

func TestObserveLiveLifecycleControl_EmitsAcceptedPauseMetric(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	session := harness.svc.sessionByID(defaultFactorySessionID)
	if session == nil || liveSessionHandle(session) == nil || liveSessionHandle(session).runtime == nil || liveSessionHandle(session).runtime.metricsSink == nil {
		t.Fatal("live session runtime metrics sink is required")
	}
	metricsPath := liveSessionHandle(session).runtime.metricsSink.Path()

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}

	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricNameAndValue(record, runtimeMetricLifecycleControl, 1) &&
			metricRecordString(record, "outcome") == "ACCEPTED" &&
			metricRecordString(record, "reason") == "PAUSE"
	}, "accepted pause lifecycle control")
}

func findLifecycleControlLog(t *testing.T, observed *observer.ObservedLogs, message string) observer.LoggedEntry {
	t.Helper()
	for _, entry := range observed.All() {
		if entry.Message == message {
			return entry
		}
	}
	t.Fatalf("lifecycle control log %q not found in %#v", message, observed.All())
	return observer.LoggedEntry{}
}

func assertLogField(t *testing.T, entry observer.LoggedEntry, key, want string) {
	t.Helper()
	for _, field := range entry.Context {
		if field.Key != key {
			continue
		}
		if field.String == want {
			return
		}
		t.Fatalf("log field %q = %q, want %q", key, field.String, want)
	}
	t.Fatalf("log field %q missing from %#v", key, entry.Context)
}

func assertLogDoesNotContain(t *testing.T, entry observer.LoggedEntry, fragment string) {
	t.Helper()
	for _, field := range entry.Context {
		if strings.Contains(field.String, fragment) {
			t.Fatalf("log field %q leaked %q: %q", field.Key, fragment, field.String)
		}
	}
}
