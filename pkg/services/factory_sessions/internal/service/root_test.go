package service

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	sessioninvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	"go.uber.org/zap"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/cursors"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livechange"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
	legacyservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/stream"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestNewRootRetainsLiveChangeCoordinator(t *testing.T) {
	t.Parallel()

	coordinator := livechange.NewCoordinator()
	root, err := newRootForTest(coordinator)
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewRoot() returned nil root")
	}
	if root.liveChangeCoordinator != coordinator {
		t.Fatalf("live-change coordinator = %T, want the injected coordinator %T", root.liveChangeCoordinator, coordinator)
	}
}

func TestApplicationOperationsRejectMissingSelectedSession(t *testing.T) {
	root, err := newRootForTest(livechange.NewCoordinator())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := root.ApplicationDiagnostics(""); err == nil {
		t.Fatal("application diagnostics accepted an empty session ID")
	}
	if _, err := root.ApplicationReady("missing"); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("application readiness error = %v", err)
	}
	if _, err := root.ApplicationCleanSnapshot(ctx, "missing"); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("clean snapshot error = %v", err)
	}
	if _, err := root.SessionPresentation("missing"); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("session presentation error = %v", err)
	}
	if _, err := root.ApplicationReplayMetadataWarnings("missing"); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("replay warnings error = %v", err)
	}
	if _, err := root.ApplicationResumeRecoveryMetadata("missing"); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("resume metadata error = %v", err)
	}
	if err := root.RunApplicationTransport(ctx, "missing", nil); err == nil {
		t.Fatal("application transport accepted nil HTTP handler")
	}
	if err := root.RunApplicationTransport(ctx, "missing", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("application transport error = %v", err)
	}
	if err := root.StopApplicationRuntime(ctx, "missing"); err != nil {
		t.Fatalf("stopping an absent application should be idempotent: %v", err)
	}
	if err := root.StopApplicationOrderly(ctx, "missing"); err != nil {
		t.Fatalf("orderly stop of absent application should be idempotent: %v", err)
	}
	if result := root.ApplicationControlWaitToComplete("missing", factoryruntime.WaitToCompleteRequest{}); result != (factoryruntime.WaitToCompleteResult{}) {
		t.Fatalf("absent wait control = %+v", result)
	}
	var nilRoot *Root
	if _, err := nilRoot.ApplicationDiagnostics("missing"); err == nil {
		t.Fatal("nil root accepted application diagnostics")
	}
}

func TestLiveControlRequiresSelectedSession(t *testing.T) {
	root, err := newRootForTest(livechange.NewCoordinator())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var nilRoot *Root
	if _, err := nilRoot.Control(ctx, factorysessions.SessionControlRequest{}); err == nil {
		t.Fatal("nil root accepted control")
	}
	if _, err := root.Control(ctx, factorysessions.SessionControlRequest{Mode: factorysessions.SessionOperationModeLive}); err == nil {
		t.Fatal("live control accepted an empty session ID")
	}
	if _, err := root.Control(ctx, factorysessions.SessionControlRequest{Mode: factorysessions.SessionOperationModeLive, SessionID: "missing"}); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("missing live session control error = %v", err)
	}
}

func TestNewRootClassifiesMissingAndRejectedAssembly(t *testing.T) {
	t.Parallel()
	var typedNil *legacyservice.Assembly
	cases := []struct {
		name     string
		assembly roles.RuntimeAssembly
		want     string
	}{
		{"nil", nil, "construct Factory Sessions: runtime assembly is required"},
		{"typed nil", typedNil, "construct Factory Sessions: runtime assembly implementation rejected"},
		{"foreign", &factorySessionsConstructionStub{}, "construct Factory Sessions: runtime assembly implementation rejected"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root, err := (runtimeOpeningFixture{Assembly: test.assembly, LiveChangeCoordinator: livechange.NewCoordinator()}).newFactory()
			if root != nil || err == nil || err.Error() != test.want {
				t.Fatalf("NewRoot = (%v, %v), want nil and %q", root, err, test.want)
			}
		})
	}
}

func TestNewRootRetainsAssemblyDuringConstruction(t *testing.T) {
	t.Parallel()
	inputs := validRootInputs(livechange.NewCoordinator())
	assembly, err := inputs.callAssembly()
	if err != nil {
		t.Fatal(err)
	}
	root, err := (runtimeOpeningFixture{Assembly: assembly, LiveChangeCoordinator: inputs.liveChangeCoordinator}).newFactory()
	if err != nil {
		t.Fatal(err)
	}
	if any(root.Assembly) != any(assembly) || root.factorySessionsRuntimeAssembly != assembly || root.liveChangeCoordinator != inputs.liveChangeCoordinator {
		t.Fatal("root did not retain its fixed construction roles")
	}
}

func TestRootForRuntimeRejectsMissingLiveChangeCoordinator(t *testing.T) {
	t.Parallel()

	root, err := (runtimeOpeningFixture{Assembly: &legacyservice.Assembly{}}).newFactory()
	if root != nil || err == nil || err.Error() != "construct Factory Sessions: live-change coordinator is required" {
		t.Fatalf("NewRoot() with missing live-change coordinator = (%#v, %v), want nil root and stable error", root, err)
	}
}

func TestRootListSessionsProjectsRecordedHistoryWithoutDetachedOwner(t *testing.T) {
	t.Parallel()

	inventory := &rootRecordedSessionInventory{result: recordings.RecordedSessionInventoryResult{
		Sessions: []recordings.RecordedSessionSummary{
			{FactorySessionID: "session-history", ArtifactReference: "2026/08/24/session-history.jsonl", Format: recordings.RecordedSessionFormatV2JSONL},
		},
	}}
	inputs := validRootInputs(livechange.NewCoordinator())
	inputs.resolveHome = func() (string, error) { return "operator-home", nil }
	inputs.recordedSessionInventory = inventory
	root, err := inputs.call()
	if err != nil {
		t.Fatalf("NewRoot: %v", err)
	}

	result, err := root.ListSessions(context.Background(), factorysessions.ListSessionsRequest{
		Scope: factorysessions.SessionListScopeHistory,
	})
	if err != nil {
		t.Fatalf("ListSessions history: %v", err)
	}
	if inventory.request.RecordingRoot != filepath.Join("operator-home", ".you-agent-factory", "recordings") {
		t.Fatalf("recording root = %q, want canonical recording root", inventory.request.RecordingRoot)
	}
	if len(result.RecordedSessions) != 1 || result.RecordedSessions[0].SessionID != "session-history" || result.RecordedSessions[0].Source != factorysessions.RecordedSessionListSourceHistory {
		t.Fatalf("recorded projection = %#v, want one explicit history row", result.RecordedSessions)
	}
}

func newRootForTest(coordinator factorysessioncontracts.LiveChangeCoordinator) (*Root, error) {
	return validRootInputs(coordinator).call()
}

type rootTestInputs struct {
	newJavaScriptCheckpointStore factoryruntime.JavaScriptCheckpointStoreFactory
	sessionResultProjection      factoryruntime.SessionResultProjectionOperation
	interpolation                factorydefinitions.InvocationInterpolationService
	invocationWorkTypes          factorydefinitions.InvocationWorkTypeService
	ttsObservability             factorydefinitions.TTSObservabilityService
	eventIDs                     factorysessions.ResponseEventIDGenerator
	sessionIDs                   factorysessions.SessionIDGenerator
	resolveHome                  factorysessions.HomeDirectoryResolver
	directoryInspection          roles.DirectoryInspection
	namedPaths                   factorydefinitions.NamedPathResolver
	invocationInputFiles         fileeffects.InvocationInputReader
	initialWorkFiles             fileeffects.InitialWorkReader
	identity                     identity.Service
	responseStreams              responsestreamservice.Service
	clock                        factoryruntime.Clock
	liveChangeCoordinator        factorysessioncontracts.LiveChangeCoordinator
	recordedSessionInventory     recordings.RecordedSessionInventory
}

func validRootInputs(coordinator factorysessioncontracts.LiveChangeCoordinator) rootTestInputs {
	var namedPaths factorydefinitions.NamedPathResolver = rootTestNamedPathResolver{}
	var identityService identity.Service = rootTestIdentityService{}
	var responseStreams responsestreamservice.Service = rootTestResponseStreams{}

	return rootTestInputs{
		sessionResultProjection: factoryruntime.NewSessionResultProjectionOperation(),
		eventIDs:                factorysessions.ResponseEventIDGenerator(func() string { return "response-event" }),
		sessionIDs:              factorysessions.SessionIDGenerator(func() string { return "session" }),
		resolveHome:             factorysessions.HomeDirectoryResolver(func() (string, error) { return "", nil }),
		directoryInspection:     filesystem.Local{},
		namedPaths:              namedPaths,
		invocationInputFiles:    fileeffects.InvocationInputReader(func(string) ([]byte, error) { return nil, nil }),
		initialWorkFiles:        fileeffects.InitialWorkReader(func(string) ([]byte, error) { return nil, nil }),
		identity:                identityService,
		responseStreams:         responseStreams,
		clock:                   rootTestClock{},
		liveChangeCoordinator:   coordinator,
	}
}

func (in rootTestInputs) call() (*Root, error) {
	assembly, err := in.callAssembly()
	if err != nil {
		return nil, err
	}
	return (runtimeOpeningFixture{Assembly: assembly, LiveChangeCoordinator: in.liveChangeCoordinator}).newFactory()
}

func (in rootTestInputs) callAssembly() (roles.RuntimeAssembly, error) {

	registry := sessionregistry.New()
	responses, err := in.responseStreams.NewStreamRegistry(in.clock)
	if err != nil {
		return nil, err
	}
	state := sessionruntime.NewWithResponseService(registry, responses, nil, in.clock, in.eventIDs, in.sessionIDs, in.responseStreams)
	streams := stream.NewManagerWithResponseService(state, sessionruntime.NewResponseStreamObserver(nil), responses, in.responseStreams)
	return legacyservice.NewAssembly(
		legacyservice.NewWithLiveChangeCoordinator(legacyservice.SessionServiceHost(state, nil, nil, nil, "", in.identity, in.clock, nil, in.newJavaScriptCheckpointStore, nil), streams, nil, in.sessionResultProjection, in.responseStreams, in.liveChangeCoordinator, legacyservice.NewRecordedHistory(in.resolveHome, in.recordedSessionInventory), nil, nil, nil, nil),
		registry, state, streams, sessioninvocation.NewSessionOwner(legacyservice.NewInvocationAuthority(state, platformclock.Real{}, nil), legacyservice.NewScopeControl(state, nil, zap.NewNop()), nil, nil, in.interpolation, in.invocationWorkTypes, in.invocationInputFiles, nil), legacyservice.NewScopeControl(state, nil, zap.NewNop()), legacyservice.NewScopeActivation(state),
		in.newJavaScriptCheckpointStore,
		in.sessionResultProjection,
		in.eventIDs,
		in.sessionIDs,
		in.resolveHome,
		in.directoryInspection,
		in.namedPaths,
		in.initialWorkFiles,
		in.identity,
		in.responseStreams,
		legacyservice.NewRecordedHistory(in.resolveHome, in.recordedSessionInventory),
		legacyservice.SessionServiceHost(state, nil, nil, nil, "", in.identity, in.clock, nil, in.newJavaScriptCheckpointStore, nil),
		legacyservice.NewNamedFactoryActivator(state),
		legacyservice.NewKeyedDefinitionActivationGateway(state, in.clock),
		nil, nil, nil,
		nil,
		nil, nil,
	), nil
}

var _ factorysessioncontracts.LiveChangeCoordinator = (*livechange.Service)(nil)

type rootRecordedSessionInventory struct {
	request recordings.RecordedSessionInventoryRequest
	result  recordings.RecordedSessionInventoryResult
}

func (inventory *rootRecordedSessionInventory) ListRecordedSessions(request recordings.RecordedSessionInventoryRequest) (recordings.RecordedSessionInventoryResult, error) {
	inventory.request = request
	return inventory.result, nil
}

type rootTestNamedPathResolver struct{}

func (rootTestNamedPathResolver) ResolveCandidatePaths(string, string, string) (factorydefinitions.NamedFactoryCandidatePaths, error) {
	return factorydefinitions.NamedFactoryCandidatePaths{}, nil
}

func (rootTestNamedPathResolver) ResolveExistingDir(string, string) (string, error) {
	return "", nil
}

func (rootTestNamedPathResolver) RequireDefinitionDir(string) error { return nil }

func (rootTestNamedPathResolver) ResolveCurrentDir(string) (string, error) {
	return "", nil
}

func (rootTestNamedPathResolver) ReadCurrentPointer(string) (string, error) {
	return "", nil
}

func (rootTestNamedPathResolver) WriteCurrentPointer(string, string) error { return nil }

type rootTestIdentityService struct{}

func (rootTestIdentityService) Normalize(context.Context, identity.NormalizeRequest) (identity.ResolvedIdentity, error) {
	return identity.ResolvedIdentity{}, nil
}

func (rootTestIdentityService) NormalizeProvider(context.Context, identity.NormalizeProviderRequest) (identity.ResolvedIdentity, error) {
	return identity.ResolvedIdentity{}, nil
}

func (rootTestIdentityService) Resolve(sessionregistry.Service, string) *livesession.LiveSession {
	return nil
}

func (rootTestIdentityService) ResolveLogical(sessionregistry.Service, string, string) *livesession.LiveSession {
	return nil
}

type rootTestResponseStreams struct{}

func (rootTestResponseStreams) NewEventStore(string, factoryruntime.Clock) (*responseeventstore.SessionResponseEventStore, error) {
	return nil, nil
}

func (rootTestResponseStreams) NewStreamRegistry(clock factoryruntime.Clock) (*responsestream.Registry, error) {
	if clock == nil {
		return nil, errors.New("clock is required")
	}
	return responsestream.NewRegistry(
		func() *responsestream.SessionResponseStream { return responsestream.NewSessionResponseStream(clock) },
		clock,
	), nil
}

func (rootTestResponseStreams) Subscribe(context.Context, *responseeventstore.SessionResponseEventStore, responsestreamservice.SubscriptionRequest) (*responsestreamservice.Cursor, error) {
	return nil, nil
}

func (rootTestResponseStreams) NewCursorTracker(cursors.Store, cursors.StorageIdentity) (*responsestreamservice.Tracker, error) {
	return nil, nil
}

func (rootTestResponseStreams) NewPublisher(*responsestream.SessionResponseStream, responsestream.DiagnosticsObserver) *responsestreamservice.Publisher {
	return nil
}

func (rootTestResponseStreams) Publish(*responseeventstore.SessionResponseEventStore, responseevents.FactoryResponseEvent) (responseevents.FactoryResponseEvent, error) {
	return responseevents.FactoryResponseEvent{}, nil
}

func (rootTestResponseStreams) Complete(*responseeventstore.SessionResponseEventStore) {}

func (rootTestResponseStreams) Close(*responseeventstore.SessionResponseEventStore) {}

var _ identity.Service = rootTestIdentityService{}
var _ responsestreamservice.Service = rootTestResponseStreams{}

type rootTestClock struct{}

func (rootTestClock) Now() time.Time { return time.Unix(0, 0) }
