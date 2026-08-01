package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
	"go.uber.org/zap"
)

var serviceTestClock = platformclock.Real{}

func newServiceTestResponseStream() *responsestream.SessionResponseStream {
	return responsestream.NewSessionResponseStream(serviceTestClock)
}

func newServiceTestGateway(host factorysessionservice.LegacyHost) *factorysessionservice.Service {
	registry := responsestream.NewRegistry(newServiceTestResponseStream, serviceTestClock)
	return factorysessionservice.New(host, registry)
}

type openTestHost struct {
	targets         []factorysessions.Target
	discoverErr     error
	scaffoldErr     error
	openSessionID   string
	openErr         error
	requireSession  *livesession.LiveSession
	requireSessionE error
	sessionIDs      []string
	sessions        map[string]*livesession.LiveSession
	projectionErr   error
	selectCalls     int
}

func (h *openTestHost) DiscoverTargets(_ string) ([]factorysessions.Target, error) {
	if h.discoverErr != nil {
		return nil, h.discoverErr
	}
	return h.targets, nil
}

func (h *openTestHost) SelectTarget(targets []factorysessions.Target, ref *factorysessions.TargetRef) (*factorysessions.Target, error) {
	h.selectCalls++
	return logicaltarget.Select(targets, ref)
}

func (h *openTestHost) InitializeFactoryScaffold(_ string) error {
	return h.scaffoldErr
}

func (h *openTestHost) ValidateInitNewFactoryNestedDir(string) error { return nil }

func (h *openTestHost) ResolveSessionFolder(folder string) (string, error) { return folder, nil }

func (h *openTestHost) OpenLiveSessionForTarget(_ context.Context, _ factorysessions.Target) (string, error) {
	if h.openErr != nil {
		return "", h.openErr
	}
	return h.openSessionID, nil
}

func (h *openTestHost) RequireSession(_ string) (*livesession.LiveSession, error) {
	if h.requireSessionE != nil {
		return nil, h.requireSessionE
	}
	return h.requireSession, nil
}

func (h *openTestHost) ListLiveSessionIDs() []string {
	return h.sessionIDs
}

func (h *openTestHost) GetLiveSession(sessionID string) *livesession.LiveSession {
	if h.sessions == nil {
		return h.requireSession
	}
	return h.sessions[sessionID]
}

func (h *openTestHost) BuildSessionProjectionContext(
	_ context.Context,
	session *livesession.LiveSession,
) (factorysessions.ProjectionContext, error) {
	if h.projectionErr != nil {
		return factorysessions.ProjectionContext{}, h.projectionErr
	}
	return factorysessions.ProjectionContext{
		Session: &factorysessions.ScopedLiveSessionSummary{
			ID: livesession.CanonicalID(session), FactoryDir: session.FactoryDir,
			FolderPath: session.FolderPath, Project: session.Project,
			IsDefault: session.IsDefault, Target: session.Target,
		},
		FactorySessionID: livesession.CanonicalID(session),
	}, nil
}

func (h *openTestHost) ResolveSyncPreflightTarget(_ string, _ *interfaces.FactorySessionLogicalResolveHint) (controlplane.SyncPreflightTarget, error) {
	return controlplane.SyncPreflightTarget{Session: h.requireSession}, nil
}

func (h *openTestHost) BackendScopeID() string {
	return "runtime-test"
}

func (h *openTestHost) LogicalSessionKeyID(session *livesession.LiveSession) string {
	return controlplane.LogicalSessionKeyID(session)
}

func (h *openTestHost) StreamGenerationID(_ *livesession.LiveSession) string {
	return "runtime-test::sess-1"
}

func (h *openTestHost) LiveSessionEvents(_ *livesession.LiveSession) []interfaces.FactoryEvent {
	return nil
}

func (h *openTestHost) SessionFactory(_ string) (factoryruntime.Service, error) {
	return nil, h.requireSessionE
}

func (h *openTestHost) StopLiveSession(_ string) error {
	return h.requireSessionE
}

func (h *openTestHost) ObserveLiveLifecycleControl(
	_ string,
	_ factorysessionexecution.LifecycleControlKind,
	_ factorysessionexecution.ControlRequest,
	_ factorysessionexecution.LifecycleControlOutcome,
	_ factorysessionexecution.LifecycleStatus,
	_ error,
) {
}

func (h *openTestHost) DurableExecution() factorysessionexecution.Service {
	return nil
}

func (h *openTestHost) ResponseStreams(*livesession.LiveSession) *responsestream.StreamSet {
	return nil
}

func (h *openTestHost) NewResponseStream() *responsestream.SessionResponseStream {
	return newServiceTestResponseStream()
}

func (h *openTestHost) CloseResponseStreams(*livesession.LiveSession) {}

func (h *openTestHost) CloseResponseStreamDispatch(*livesession.LiveSession, string) bool {
	return false
}

func (h *openTestHost) JavaScriptCheckpointStore(*livesession.LiveSession) factoryruntime.JavaScriptCheckpointStore {
	return nil
}

func (h *openTestHost) ObserveResponseStreamPublished(*livesession.LiveSession, string, responsestream.Event) {
}

func (h *openTestHost) ObserveResponseStreamCompaction(
	*livesession.LiveSession,
	string,
	string,
	responsestream.CompactionSummary,
) {
}

func (h *openTestHost) ObserveResponseStreamDegraded(
	*livesession.LiveSession,
	string,
	string,
	string,
	*zap.Logger,
	error,
) {
}

func TestService_OpenFactorySessionFromFolder_AutoOpensSingleTarget(t *testing.T) {
	t.Parallel()

	host := &openTestHost{
		targets: []factorysessions.Target{{
			Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
			FactoryDir: "/tmp/factory",
			FolderPath: "/tmp",
		}},
		openSessionID: "sess-1",
		requireSession: &livesession.LiveSession{
			ID: "sess-1",
			SessionState: livesession.SessionState{
				FactoryDir: "/tmp/factory",
				FolderPath: "/tmp",
			},
			Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		},
	}
	gateway := newServiceTestGateway(host)

	result, err := gateway.OpenFactorySessionFromFolder(context.Background(), "/tmp", nil, false, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder: %v", err)
	}
	if result.SessionID != "sess-1" {
		t.Fatalf("session id = %q, want sess-1", result.SessionID)
	}
	if host.selectCalls != 1 {
		t.Fatalf("identity target selections = %d, want 1", host.selectCalls)
	}
}

func TestService_OpenFactorySessionFromFolder_ReturnsTargetPickerMetadata(t *testing.T) {
	t.Parallel()

	host := &openTestHost{
		targets: []factorysessions.Target{
			{Ref: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}, Label: "default"},
			{Ref: factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "beta"}, Label: "beta"},
		},
	}
	gateway := newServiceTestGateway(host)

	result, err := gateway.OpenFactorySessionFromFolder(context.Background(), "/tmp", nil, false, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder: %v", err)
	}
	if result.SessionID != "" {
		t.Fatalf("session id = %q, want empty", result.SessionID)
	}
	if len(result.Targets) != 2 {
		t.Fatalf("targets = %d, want 2", len(result.Targets))
	}
}

func TestService_OpenFactorySessionFromFolder_ValidateOnlyReturnsTargetsWithoutOpening(t *testing.T) {
	t.Parallel()

	host := &openTestHost{
		targets: []factorysessions.Target{
			{Ref: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}, Label: "default"},
			{Ref: factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "beta"}, Label: "beta"},
		},
		openErr: errors.New("open should not run"),
	}
	gateway := newServiceTestGateway(host)

	result, err := gateway.OpenFactorySessionFromFolder(context.Background(), "/tmp", nil, true, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder: %v", err)
	}
	if len(result.Targets) != 2 {
		t.Fatalf("targets = %d, want 2", len(result.Targets))
	}
}

func TestService_OpenFactorySession_RejectsValidateOnlyWithInitNewFactory(t *testing.T) {
	t.Parallel()

	gateway := newServiceTestGateway(&openTestHost{})
	validateOnly := true
	initNewFactory := true
	_, err := gateway.OpenFactorySession(context.Background(), factorysessions.OpenRequest{
		FolderPath:     "/tmp",
		ValidateOnly:   validateOnly,
		InitNewFactory: initNewFactory,
	})
	if err == nil || !strings.Contains(err.Error(), "initNewFactory cannot be combined with validateOnly") {
		t.Fatalf("OpenFactorySession error = %v, want initNewFactory/validateOnly conflict", err)
	}
}

func TestService_OpenFactorySession_ReturnsOpenedSessionIdentity(t *testing.T) {
	t.Parallel()

	host := &openTestHost{
		targets: []factorysessions.Target{{
			Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
			FactoryDir: "/tmp/factory",
			FolderPath: "/tmp",
			Project:    "demo",
		}},
		openSessionID: "sess-1",
		requireSession: &livesession.LiveSession{
			ID: "sess-1",
			SessionState: livesession.SessionState{
				FactoryDir: "/tmp/factory",
				FolderPath: "/tmp",
			},
			Project: "demo",
			Target:  factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		},
	}
	gateway := newServiceTestGateway(host)

	result, err := gateway.OpenFactorySession(context.Background(), factorysessions.OpenRequest{
		FolderPath: "/tmp",
	})
	if err != nil {
		t.Fatalf("OpenFactorySession: %v", err)
	}
	if result == nil || result.SessionID != "sess-1" {
		t.Fatalf("open result = %#v, want sess-1", result)
	}
	if result.Session == nil || result.Session.ID != "sess-1" || result.Session.Project != "demo" {
		t.Fatalf("open session = %#v, want owner-projected sess-1 summary", result.Session)
	}
}

func TestService_StartRoutesLiveOpenThroughTheRoot(t *testing.T) {
	t.Parallel()

	host := &openTestHost{
		targets: []factorysessions.Target{{
			Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
			FactoryDir: "/tmp/factory",
			FolderPath: "/tmp",
		}},
		openSessionID: "sess-root-start",
	}
	gateway := newServiceTestGateway(host)

	started, err := gateway.Start(context.Background(), factorysessions.StartRequest{
		Mode: factorysessions.StartModeLive,
		Live: &factorysessions.OpenRequest{FolderPath: "/tmp"},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if started.SessionID != "sess-root-start" || started.Status != factorysessions.LifecycleStatusRunning ||
		started.Mode != factorysessions.StartModeLive || started.Live == nil {
		t.Fatalf("Start() = %#v, want live root result", started)
	}
}

func TestService_StartPreservesLiveValidationWithoutCreatingSession(t *testing.T) {
	t.Parallel()

	host := &openTestHost{
		targets: []factorysessions.Target{{
			Ref: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		}},
		openErr: errors.New("live open should not run for validation"),
	}
	gateway := newServiceTestGateway(host)

	started, err := gateway.Start(context.Background(), factorysessions.StartRequest{
		Mode: factorysessions.StartModeLive,
		Live: &factorysessions.OpenRequest{FolderPath: "/tmp", ValidateOnly: true},
	})
	if err != nil {
		t.Fatalf("Start() validation error = %v", err)
	}
	if started.SessionID != "" || started.Status != "" || started.Live == nil {
		t.Fatalf("Start() validation = %#v, want target-only result without live status", started)
	}
}

func TestService_OpenFactorySession_LeavesSummaryAbsentWhenSessionCannotResolve(t *testing.T) {
	t.Parallel()

	host := &openTestHost{
		targets: []factorysessions.Target{{
			Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
			FactoryDir: "/tmp/factory",
		}},
		openSessionID:   "sess-1",
		requireSessionE: errors.New("session missing"),
	}
	gateway := newServiceTestGateway(host)

	result, err := gateway.OpenFactorySession(context.Background(), factorysessions.OpenRequest{
		FolderPath: "/tmp",
	})
	if err != nil {
		t.Fatalf("OpenFactorySession: %v", err)
	}
	if result == nil || result.SessionID != "sess-1" {
		t.Fatalf("open result = %#v, want sess-1", result)
	}
	if result.Session != nil {
		t.Fatalf("open session = %#v, want nil for unresolved session", result.Session)
	}
}
