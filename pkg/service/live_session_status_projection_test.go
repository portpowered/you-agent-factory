package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/api"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"go.uber.org/zap"
)

func TestFactoryService_GetFactorySession_ReflectsPausedLifecycleControlStatus(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}

	waitForSessionFactoryState(
		t,
		harness.svc,
		defaultFactorySessionID,
		interfaces.FactoryStatePaused,
		time.Second,
		"live session paused",
	)

	session, err := harness.svc.GetFactorySession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetFactorySession: %v", err)
	}
	if session.Runtime.LifecycleControlStatus == nil {
		t.Fatal("lifecycleControlStatus = nil, want PAUSED")
	}
	if *session.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("lifecycleControlStatus = %q, want PAUSED", *session.Runtime.LifecycleControlStatus)
	}
	if session.Runtime.Progress.FactoryState != string(factoryapi.FactorySessionDurableLifecycleStatusPaused) {
		t.Fatalf("progress.factoryState = %q, want PAUSED", session.Runtime.Progress.FactoryState)
	}
}

func TestFactoryService_GetFactorySession_ReflectsResumedLifecycleControlStatus(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}
	if _, err := harness.svc.ResumeLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("ResumeLiveFactorySession: %v", err)
	}

	waitForSessionFactoryState(
		t,
		harness.svc,
		defaultFactorySessionID,
		interfaces.FactoryStateRunning,
		time.Second,
		"live session resumed",
	)

	session, err := harness.svc.GetFactorySession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetFactorySession: %v", err)
	}
	if session.Runtime.LifecycleControlStatus == nil {
		t.Fatal("lifecycleControlStatus = nil, want RUNNING")
	}
	if *session.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("lifecycleControlStatus = %q, want RUNNING", *session.Runtime.LifecycleControlStatus)
	}
}

func TestFactoryService_GetFactorySession_RepeatPauseDoesNotChangeLifecycleControlStatus(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}
	before, err := harness.svc.GetFactorySession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetFactorySession before repeat pause: %v", err)
	}

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("repeat PauseLiveFactorySession: %v", err)
	}
	after, err := harness.svc.GetFactorySession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetFactorySession after repeat pause: %v", err)
	}
	if before.Runtime.LifecycleControlStatus == nil || after.Runtime.LifecycleControlStatus == nil {
		t.Fatalf("lifecycleControlStatus missing: before=%#v after=%#v", before.Runtime.LifecycleControlStatus, after.Runtime.LifecycleControlStatus)
	}
	if *before.Runtime.LifecycleControlStatus != *after.Runtime.LifecycleControlStatus {
		t.Fatalf("lifecycleControlStatus changed after no-op pause: before=%q after=%q",
			*before.Runtime.LifecycleControlStatus, *after.Runtime.LifecycleControlStatus)
	}
}

func TestFactoryService_GetEngineStateSnapshotForSession_ReflectsLifecycleControlStatus(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}

	snapshot, err := harness.svc.GetEngineStateSnapshotForSession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshotForSession: %v", err)
	}
	if snapshot.LifecycleControlStatus != string(factoryapi.FactorySessionDurableLifecycleStatusPaused) {
		t.Fatalf("lifecycleControlStatus = %q, want PAUSED", snapshot.LifecycleControlStatus)
	}
}

func TestGetStatusBySessionId_ReflectsPausedLifecycleControlStatus(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	if _, err := harness.svc.PauseLiveFactorySession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySessionLifecycleControlRequest{},
	); err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}

	server := newLiveSessionStatusTestServer(t, harness.svc)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/factory-sessions/"+defaultFactorySessionID+"/status", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want 200", resp.StatusCode)
	}

	var payload factoryapi.StatusResponse
	if err := decodeJSONResponse(resp, &payload); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if payload.LifecycleControlStatus == nil {
		t.Fatal("lifecycleControlStatus = nil, want PAUSED")
	}
	if *payload.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("lifecycleControlStatus = %q, want PAUSED", *payload.LifecycleControlStatus)
	}
}

func TestGetFactorySession_MissingLiveSessionReturnsNotFound(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	_, err := harness.svc.GetFactorySession(context.Background(), "live-session-missing-001")
	if err == nil {
		t.Fatal("GetFactorySession = nil, want not found")
	}
	if !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		t.Fatalf("error = %v, want ErrFactorySessionNotFound", err)
	}
}

func newLiveSessionStatusTestServer(t *testing.T, svc *FactoryService) *httptest.Server {
	t.Helper()
	return httptest.NewServer(api.NewServer(svc, 0, zap.NewNop()).Handler())
}

func decodeJSONResponse(resp *http.Response, target any) error {
	decoder := json.NewDecoder(resp.Body)
	return decoder.Decode(target)
}
