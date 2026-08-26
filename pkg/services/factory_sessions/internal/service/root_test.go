package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

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
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
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
	if root.DetachedOperations() == nil {
		t.Fatal("NewRoot() did not publish detached operations")
	}
	if root.liveChangeCoordinator != coordinator {
		t.Fatalf("live-change coordinator = %T, want the injected coordinator %T", root.liveChangeCoordinator, coordinator)
	}
}

func TestNewRootRejectsMissingRequiredDependencies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*rootTestInputs)
	}{
		{name: "session result projection", mutate: func(in *rootTestInputs) { in.sessionResultProjection = nil }},
		{name: "response event ID generator", mutate: func(in *rootTestInputs) { in.eventIDs = nil }},
		{name: "session ID generator", mutate: func(in *rootTestInputs) { in.sessionIDs = nil }},
		{name: "home directory resolver", mutate: func(in *rootTestInputs) { in.resolveHome = nil }},
		{name: "directory inspection", mutate: func(in *rootTestInputs) { in.directoryInspection = nil }},
		{name: "named path resolver", mutate: func(in *rootTestInputs) { in.namedPaths = nil }},
		{name: "invocation input reader", mutate: func(in *rootTestInputs) { in.invocationInputFiles = nil }},
		{name: "initial Work reader", mutate: func(in *rootTestInputs) { in.initialWorkFiles = nil }},
		{name: "identity service", mutate: func(in *rootTestInputs) { in.identity = nil }},
		{name: "response-stream service", mutate: func(in *rootTestInputs) { in.responseStreams = nil }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			inputs := validRootInputs(livechange.NewCoordinator())
			test.mutate(&inputs)
			root, err := inputs.call()
			if root != nil || err == nil {
				t.Fatalf("NewRoot() = (%#v, %v), want nil root and dependency error", root, err)
			}
		})
	}
}

func TestRootForRuntimeRejectsMissingLiveChangeCoordinator(t *testing.T) {
	t.Parallel()

	root, err := newRootForTest(nil)
	if root != nil || err == nil {
		t.Fatalf("NewRoot() with missing live-change coordinator = (%#v, %v), want nil root and error", root, err)
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
	return NewRoot(
		in.newJavaScriptCheckpointStore,
		in.sessionResultProjection,
		in.interpolation,
		in.invocationWorkTypes,
		in.ttsObservability,
		in.eventIDs,
		in.sessionIDs,
		in.resolveHome,
		in.directoryInspection,
		in.namedPaths,
		in.invocationInputFiles,
		in.initialWorkFiles,
		in.identity,
		in.responseStreams,
		in.clock,
		in.liveChangeCoordinator,
		in.recordedSessionInventory,
	)
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

func (rootTestIdentityService) Discover(context.Context, identity.DiscoverRequest) ([]factorysessions.Target, error) {
	return nil, nil
}

func (rootTestIdentityService) ResolveFolder(string) (string, error) { return "", nil }

func (rootTestIdentityService) Select([]factorysessions.Target, *factorysessions.TargetRef) (*factorysessions.Target, error) {
	return nil, nil
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
