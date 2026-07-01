package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/controlplane"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/factorysessions/service"
	"go.uber.org/zap"
)

type openTestHost struct {
	targets         []factorysessions.Target
	discoverErr     error
	scaffoldErr     error
	openSessionID   string
	openErr         error
	requireSession  *factorysessions.LiveSession
	requireSessionE error
	sessionIDs      []string
	sessions        map[string]*factorysessions.LiveSession
	projectionErr   error
}

func (h *openTestHost) DiscoverTargets(_ string) ([]factorysessions.Target, error) {
	if h.discoverErr != nil {
		return nil, h.discoverErr
	}
	return h.targets, nil
}

func (h *openTestHost) InitializeFactoryScaffold(_ string) error {
	return h.scaffoldErr
}

func (h *openTestHost) OpenLiveSessionForTarget(_ context.Context, _ factorysessions.Target) (string, error) {
	if h.openErr != nil {
		return "", h.openErr
	}
	return h.openSessionID, nil
}

func (h *openTestHost) RequireSession(_ string) (*factorysessions.LiveSession, error) {
	if h.requireSessionE != nil {
		return nil, h.requireSessionE
	}
	return h.requireSession, nil
}

func (h *openTestHost) ListLiveSessionIDs() []string {
	return h.sessionIDs
}

func (h *openTestHost) GetLiveSession(sessionID string) *factorysessions.LiveSession {
	if h.sessions == nil {
		return h.requireSession
	}
	return h.sessions[sessionID]
}

func (h *openTestHost) BuildSessionProjectionContext(
	_ context.Context,
	session *factorysessions.LiveSession,
) (factorysessions.ProjectionContext, error) {
	if h.projectionErr != nil {
		return factorysessions.ProjectionContext{}, h.projectionErr
	}
	return factorysessions.ProjectionContext{Session: session}, nil
}

func (h *openTestHost) ResolveSyncPreflightTarget(_ string) (controlplane.SyncPreflightTarget, error) {
	return controlplane.SyncPreflightTarget{Session: h.requireSession}, nil
}

func (h *openTestHost) BackendScopeID() string {
	return "runtime-test"
}

func (h *openTestHost) StreamGenerationID(_ *factorysessions.LiveSession) string {
	return "runtime-test::sess-1"
}

func (h *openTestHost) LiveSessionEvents(_ *factorysessions.LiveSession) []factoryapi.FactoryEvent {
	return nil
}

func (h *openTestHost) SessionFactory(_ string) (factory.Factory, error) {
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

func (h *openTestHost) ResponseStreams(*factorysessions.LiveSession) *factorysessions.SessionResponseStreamSet {
	return nil
}

func (h *openTestHost) NewResponseStream() *factorysessions.SessionResponseStream {
	return factorysessions.NewSessionResponseStream()
}

func (h *openTestHost) CloseResponseStreams(*factorysessions.LiveSession) {}

func (h *openTestHost) CloseResponseStreamDispatch(*factorysessions.LiveSession, string) bool {
	return false
}

func (h *openTestHost) JavaScriptCheckpointStore(*factorysessions.LiveSession) *factorysessions.JavaScriptCheckpointStore {
	return nil
}

func (h *openTestHost) ObserveResponseStreamPublished(*factorysessions.LiveSession, string, responsestream.Event) {
}

func (h *openTestHost) ObserveResponseStreamCompaction(
	*factorysessions.LiveSession,
	string,
	string,
	responsestream.CompactionSummary,
) {
}

func (h *openTestHost) ObserveResponseStreamDegraded(
	*factorysessions.LiveSession,
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
		requireSession: &factorysessions.LiveSession{
			ID: "sess-1",
			SessionState: factorysessions.SessionState{
				FactoryDir: "/tmp/factory",
				FolderPath: "/tmp",
			},
			Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		},
	}
	gateway := factorysessionservice.New(host)

	result, err := gateway.OpenFactorySessionFromFolder(context.Background(), "/tmp", nil, false, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder: %v", err)
	}
	if result.SessionID != "sess-1" {
		t.Fatalf("session id = %q, want sess-1", result.SessionID)
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
	gateway := factorysessionservice.New(host)

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
	gateway := factorysessionservice.New(host)

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

	gateway := factorysessionservice.New(&openTestHost{})
	validateOnly := true
	initNewFactory := true
	_, err := gateway.OpenFactorySession(context.Background(), factoryapi.OpenFactorySessionRequest{
		FolderPath:     "/tmp",
		ValidateOnly:     &validateOnly,
		InitNewFactory:   &initNewFactory,
	})
	if err == nil || !strings.Contains(err.Error(), "initNewFactory cannot be combined with validateOnly") {
		t.Fatalf("OpenFactorySession error = %v, want initNewFactory/validateOnly conflict", err)
	}
}

func TestService_OpenFactorySession_MapsOpenedSessionSummary(t *testing.T) {
	t.Parallel()

	host := &openTestHost{
		targets: []factorysessions.Target{{
			Ref:        factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
			FactoryDir: "/tmp/factory",
			FolderPath: "/tmp",
			Project:    "demo",
		}},
		openSessionID: "sess-1",
		requireSession: &factorysessions.LiveSession{
			ID: "sess-1",
			SessionState: factorysessions.SessionState{
				FactoryDir: "/tmp/factory",
				FolderPath: "/tmp",
			},
			Project: "demo",
			Target:  factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		},
	}
	gateway := factorysessionservice.New(host)

	response, err := gateway.OpenFactorySession(context.Background(), factoryapi.OpenFactorySessionRequest{
		FolderPath: "/tmp",
	})
	if err != nil {
		t.Fatalf("OpenFactorySession: %v", err)
	}
	if response.Session == nil || response.Session.Id != "sess-1" {
		t.Fatalf("response session = %#v, want sess-1", response.Session)
	}
}
