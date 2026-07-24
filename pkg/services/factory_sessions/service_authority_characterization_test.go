package factorysessions_test

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// peerRootServiceFake is a peer-owned fake of the published Factory Sessions
// root Service. It intentionally imports only the Sessions root package (plus
// approved peer roots already present in root signatures) and never imports
// factory_sessions/internal.
type peerRootServiceFake struct {
	peerExecutionStub
	sessions map[string]factorysessions.SessionProjection
}

func newPeerRootServiceFake() *peerRootServiceFake {
	return &peerRootServiceFake{
		sessions: make(map[string]factorysessions.SessionProjection),
	}
}

var _ factorysessions.Service = (*peerRootServiceFake)(nil)

func (fake *peerRootServiceFake) ForRuntime(factorysessions.RuntimeBinding) (factorysessions.Service, error) {
	return fake, nil
}

func (fake *peerRootServiceFake) OpenFactorySession(context.Context, factorysessions.OpenRequest) (*factorysessions.OpenResult, error) {
	return &factorysessions.OpenResult{SessionID: factorysessions.DefaultSessionID}, nil
}

func (fake *peerRootServiceFake) OpenFactorySessionFromFolder(context.Context, string, *factorysessions.TargetRef, bool, bool) (*factorysessions.OpenResult, error) {
	return &factorysessions.OpenResult{SessionID: factorysessions.DefaultSessionID}, nil
}

func (fake *peerRootServiceFake) ListFactorySessions(context.Context) ([]factorysessions.ReadProjection, error) {
	return nil, nil
}

func (fake *peerRootServiceFake) GetFactorySession(_ context.Context, sessionID string) (factorysessions.SessionProjection, error) {
	if projection, ok := fake.sessions[sessionID]; ok {
		return projection, nil
	}
	return factorysessions.SessionProjection{}, factorysessions.ErrSessionNotFound
}

func (fake *peerRootServiceFake) GetFactorySessionSyncPreflight(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor, *factorydefinitions.FactorySessionLogicalResolveHint) (factorysessions.SyncPreflightResult, error) {
	return factorysessions.SyncPreflightResult{}, factorysessions.ErrSessionNotFound
}

func (fake *peerRootServiceFake) GetFactorySessionResult(context.Context, string) (factoryruntime.LiveSessionResult, error) {
	return factoryruntime.LiveSessionResult{}, factorysessions.ErrSessionNotFound
}

func (fake *peerRootServiceFake) GetFactorySessionPartialResult(context.Context, string) (factoryruntime.PartialSessionResult, error) {
	return factoryruntime.PartialSessionResult{}, factorysessions.ErrSessionNotFound
}

func (fake *peerRootServiceFake) SubscribeFactoryResponseEvents(context.Context, factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error) {
	return nil, factorysessions.ErrSessionNotFound
}

func (fake *peerRootServiceFake) SubscribeFactoryEventsForSession(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) (*factorydefinitions.FactoryEventStream, error) {
	return nil, factorysessions.ErrSessionNotFound
}

func (fake *peerRootServiceFake) ProbeFactoryEventsForSession(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) error {
	return factorysessions.ErrSessionNotFound
}

func (fake *peerRootServiceFake) ReadDurableFactorySessionEventStream(context.Context, string, factorysessions.EventReconnectRequest) (*factorydefinitions.FactoryEventStream, error) {
	return nil, factorysessions.ErrDurableSessionNotFound
}

func (fake *peerRootServiceFake) ProbeDurableFactorySessionEvents(context.Context, string, factorysessions.EventReconnectRequest) error {
	return factorysessions.ErrDurableSessionNotFound
}

func (fake *peerRootServiceFake) GetEngineStateSnapshotForSession(context.Context, string) (*factoryruntime.StateSnapshot, error) {
	return nil, factorysessions.ErrSessionNotFound
}

func (fake *peerRootServiceFake) PauseLiveFactorySession(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrSessionNotFound
}

func (fake *peerRootServiceFake) ResumeLiveFactorySession(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrSessionNotFound
}

func (fake *peerRootServiceFake) CloseFactorySession(context.Context, string) error {
	return factorysessions.ErrSessionNotFound
}

func (fake *peerRootServiceFake) PauseDurableFactorySession(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *peerRootServiceFake) ResumeDurableFactorySession(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *peerRootServiceFake) CancelDurableFactorySession(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *peerRootServiceFake) TerminateDurableFactorySession(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *peerRootServiceFake) ApproveDurableFactorySession(context.Context, string, factorysessions.ApproveRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *peerRootServiceFake) RetryDurableFactorySessionDispatch(context.Context, string, factorysessions.RetryDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *peerRootServiceFake) InterruptDurableFactorySessionDispatch(context.Context, string, factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}

// peerExecutionStub satisfies the durable ExecutionService methods embedded in
// the singular root Service so a peer can compile against one aggregate authority.
type peerExecutionStub struct{}

func (peerExecutionStub) StartAsync(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
	return factorysessions.AsyncStartResult{}, factorysessions.ErrDurableSessionNotFound
}
func (peerExecutionStub) StartSync(context.Context, factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
	return factorysessions.SyncStartResult{}, factorysessions.ErrDurableSessionNotFound
}
func (peerExecutionStub) ResumeInterruptedSession(context.Context, string, factorysessions.ResumeSessionRequest) (factorysessions.AsyncStartResult, error) {
	return factorysessions.AsyncStartResult{}, factorysessions.ErrDurableSessionNotFound
}
func (peerExecutionStub) GetSession(context.Context, string) (factorysessions.SessionReadResult, error) {
	return factorysessions.SessionReadResult{}, factorysessions.ErrDurableSessionNotFound
}
func (peerExecutionStub) Pause(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}
func (peerExecutionStub) Resume(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}
func (peerExecutionStub) Cancel(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}
func (peerExecutionStub) Terminate(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}
func (peerExecutionStub) Approve(context.Context, string, factorysessions.ApproveRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}
func (peerExecutionStub) RetryDispatch(context.Context, string, factorysessions.RetryDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}
func (peerExecutionStub) InterruptDispatch(context.Context, string, factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
}
func (peerExecutionStub) GetResult(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
	return factorysessions.ResultReadResult{}, factorysessions.ErrDurableSessionNotFound
}
func (peerExecutionStub) ListDispatches(context.Context, string) (factorysessions.ListDispatchesResult, error) {
	return factorysessions.ListDispatchesResult{}, nil
}
func (peerExecutionStub) QueryDispatches(context.Context, factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error) {
	return factorysessions.ListDispatchesResult{}, nil
}
func (peerExecutionStub) GetDispatch(context.Context, string, string) (factorysessions.DispatchDetail, error) {
	return factorysessions.DispatchDetail{}, factorysessions.ErrDispatchNotFound
}
func (peerExecutionStub) ListArtifacts(context.Context, string) (factorysessions.ListArtifactsResult, error) {
	return factorysessions.ListArtifactsResult{}, nil
}
func (peerExecutionStub) GetArtifact(context.Context, string, string) (factorysessions.ArtifactDetail, error) {
	return factorysessions.ArtifactDetail{}, factorysessions.ErrArtifactNotFound
}
func (peerExecutionStub) ReadEvents(context.Context, string, factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error) {
	return factorysessions.EventReadResult{}, factorysessions.ErrDurableSessionNotFound
}
func (peerExecutionStub) ListSessions(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
	return factorysessions.ListSessionsResult{}, nil
}

func TestSingularRootServiceAuthority_PeerFakeReadNotFound(t *testing.T) {
	t.Parallel()

	var service factorysessions.Service = newPeerRootServiceFake()
	ctx := context.Background()

	projection, err := service.GetFactorySession(ctx, "missing-session")
	if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("GetFactorySession error = %v, want ErrSessionNotFound", err)
	}
	if projection.Context.FactorySessionID != "" || projection.Runtime.Status != "" {
		t.Fatalf("GetFactorySession projection = %#v, want empty identity on not-found", projection)
	}

	listed, err := service.ListFactorySessions(ctx)
	if err != nil {
		t.Fatalf("ListFactorySessions error = %v, want nil", err)
	}
	if len(listed) != 0 {
		t.Fatalf("ListFactorySessions len = %d, want empty list", len(listed))
	}

	opened, err := service.OpenFactorySession(ctx, factorysessions.OpenRequest{FolderPath: "/factories/demo"})
	if err != nil {
		t.Fatalf("OpenFactorySession error = %v, want nil", err)
	}
	if opened == nil || opened.SessionID == "" {
		t.Fatalf("OpenFactorySession result = %#v, want reachable open path through singular root", opened)
	}
}
