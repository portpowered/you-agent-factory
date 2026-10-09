package service

import (
	"context"
	"errors"
	"fmt"
	canonicaldurable "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/canonical/durable"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
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
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/modelinvocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
	legacyservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/stream"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
	"github.com/portpowered/infinite-you/pkg/services/models"
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

func newRootForTest(coordinator factorysessioncontracts.LiveChangeCoordinator) (*runtimeOpeningTestRoot, error) {
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

func (in rootTestInputs) call() (*runtimeOpeningTestRoot, error) {
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

type selectedModelRecord struct {
	runtimebinding.RuntimeInstance
	generation string
	directory  string
}

func (r selectedModelRecord) StreamGeneration() string { return r.generation }
func (r selectedModelRecord) Directory() string        { return r.directory }

func TestSelectedModelFactsKeepCapturedGeneration(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"first", "second", "third", "fourth"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			scope, err := (models.RuntimeScopeRef{}).Parse("scope-" + id)
			if err != nil {
				t.Fatal(err)
			}
			bound := &runtimebinding.SessionState{ModelsScope: scope,
				ModelInvocation: modelinvocation.RuntimeModelInvocation{RuntimeID: "runtime-" + id},
				Instance:        selectedModelRecord{generation: "generation-" + id, directory: "/" + id},
			}
			expected := modelinvocation.RuntimeModelInvocation{
				FactorySessionID: id, Scope: scope, RuntimeID: "runtime-" + id,
				GenerationID: "generation-" + id, FactoryDirectory: "/" + id, WorkingDirectory: "/" + id,
			}
			captured := selectedModelFacts(bound, id)
			if captured != expected {
				t.Fatalf("selected facts = %+v, want %+v", captured, expected)
			}
			// Later registration changes do not mutate captured facts. A fresh
			// presentation reads the replacement generation from its acquired instance.
			bound.Instance = selectedModelRecord{generation: "replacement", directory: "/replacement"}
			bound.ModelInvocation.FactorySessionID = "other"
			replacement := selectedModelFacts(bound, id)
			if replacement.GenerationID != "replacement" || replacement.FactoryDirectory != "/replacement" || replacement.FactorySessionID != id {
				t.Fatalf("replacement facts = %+v", replacement)
			}
			if captured != expected {
				t.Fatalf("captured facts changed after replacement: %+v", captured)
			}
		})
	}
}

// These isolated Root witnesses inject operations and a durable owner directly;
// no RuntimeOpening or composed process participates in their forwarding proof.
func TestStartUsesInjectedOperationsAndPreservesTypedFailure(t *testing.T) {
	for _, failing := range []bool{false, true} {
		name := "success"
		if failing {
			name = "typed failure"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			request := factorysessions.SessionStartRequest{FolderPath: "/selected", Correlation: factorysessions.SessionOperationCorrelation{RequestID: "selected-request"}}
			cause := &os.PathError{Op: "open", Path: "/selected", Err: os.ErrPermission}
			var outcomeErr error
			if failing {
				outcomeErr = cause
			}
			ctx := t.Context()
			calls := 0
			startResult := factorysessions.SessionStartResult{SessionID: "opened-selected"}
			replay := &factorysessions.HistoricalReplayInspection{}
			inspectionResult := HistoricalApplicationInspection{Replay: replay}
			start := func(gotCtx context.Context, got factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
				calls++
				if gotCtx != ctx || !reflect.DeepEqual(got, request) {
					t.Fatal("start changed selected context/request")
				}
				return startResult, outcomeErr
			}
			inspect := func(gotCtx context.Context, got factorysessions.SessionStartRequest) (HistoricalApplicationInspection, bool, error) {
				calls++
				if gotCtx != ctx || !reflect.DeepEqual(got, request) {
					t.Fatal("inspection changed selected context/request")
				}
				return inspectionResult, true, outcomeErr
			}
			root, err := NewRoot(&legacyservice.Assembly{}, nil, start, livechange.NewCoordinator(), inspect,
				nil, nil, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 0 {
				t.Fatal("construction invoked opening operations")
			}
			assertInjectedOpeningOutcomes(t, root, ctx, request, startResult, replay, outcomeErr, cause, failing)
			if calls != 2 {
				t.Fatalf("operation calls = %d, want 2", calls)
			}
		})
	}
}

type rootDurableStartStub struct {
	durableexecution.Service
	request               factorysessions.StartRequest
	ctx                   context.Context
	failure               error
	syncCalls, asyncCalls int
}

func (s *rootDurableStartStub) StartSync(ctx context.Context, request factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
	s.ctx, s.request = ctx, request
	s.syncCalls++
	return factorysessions.SyncStartResult{AsyncStartResult: factorysessions.AsyncStartResult{SessionID: "sync-selected"}}, s.failure
}
func (s *rootDurableStartStub) StartAsync(ctx context.Context, request factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
	s.ctx, s.request = ctx, request
	s.asyncCalls++
	return factorysessions.AsyncStartResult{SessionID: "async-selected"}, s.failure
}
func TestStartUsesInjectedDurableOwner(t *testing.T) {
	t.Parallel()
	owner := &rootDurableStartStub{failure: &os.PathError{Op: "write", Path: "/selected", Err: os.ErrPermission}}
	root, err := NewRoot(&legacyservice.Assembly{}, owner, nil, livechange.NewCoordinator(), nil,
		nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := factorysessions.StartRequest{WorkerSettings: &factoryruntime.JavaScriptWorkerSettings{}}
	ctx := t.Context()
	syncResult, err := root.StartSync(ctx, request)
	if !errors.Is(err, owner.failure) || syncResult.SessionID != "sync-selected" {
		t.Fatalf("sync = (%+v, %v)", syncResult, err)
	}
	if owner.ctx != ctx || !reflect.DeepEqual(owner.request, request) {
		t.Fatal("sync changed selected request/context")
	}
	asyncResult, err := root.StartAsync(ctx, request)
	if !errors.Is(err, owner.failure) || asyncResult.SessionID != "async-selected" {
		t.Fatalf("async = (%+v, %v)", asyncResult, err)
	}
	if owner.ctx != ctx || !reflect.DeepEqual(owner.request, request) {
		t.Fatal("async changed selected request/context")
	}
	if owner.syncCalls != 1 || owner.asyncCalls != 1 {
		t.Fatalf("durable calls = %d/%d", owner.syncCalls, owner.asyncCalls)
	}
}

type openingCanonicalDurableOwner interface {
	canonicaldurable.Service
}

type openingDurableStartStub struct {
	durableexecution.Service
	openingCanonicalDurableOwner
	request     factorysessions.StartRequest
	ctx         context.Context
	synchronous bool
	calls       int
	failure     error
}

func (s *openingDurableStartStub) StartCanonical(ctx context.Context, request factorysessions.StartRequest, synchronous bool) (durableexecution.CanonicalStartResult, error) {
	s.ctx, s.request, s.synchronous = ctx, request, synchronous
	s.calls++
	if s.failure != nil {
		return durableexecution.CanonicalStartResult{}, s.failure
	}
	async := factorysessions.AsyncStartResult{SessionID: request.RequestID, Status: "running"}
	if synchronous {
		return durableexecution.CanonicalStartResult{Sync: &factorysessions.SyncStartResult{AsyncStartResult: async}}, nil
	}
	return durableexecution.CanonicalStartResult{Async: &async}, nil
}

func TestRuntimeOpeningUsesInjectedDurableStartOwner(t *testing.T) {
	t.Parallel()
	for _, synchronous := range []bool{false, true} {
		t.Run(fmt.Sprint(synchronous), func(t *testing.T) {
			t.Parallel()
			owner := &openingDurableStartStub{}
			// An Assembly without a gateway cannot supply a replacement durable owner.
			opening := &RuntimeOpening{assembly: &legacyservice.Assembly{}, durable: owner}
			request := factorysessions.SessionStartRequest{
				Mode: factorysessions.SessionOperationModeDurable, FolderPath: " /selected ",
				Correlation: factorysessions.SessionOperationCorrelation{RequestID: " selected-request "},
				Synchronous: synchronous, Args: map[string]any{"selected": "original"},
				WorkerSettings: &factoryruntime.JavaScriptWorkerSettings{},
			}
			result, err := opening.Start(t.Context(), request)
			if err != nil || result.SessionID != "selected-request" || result.Status != "running" || result.Mode != request.Mode {
				t.Fatalf("start = (%+v, %v)", result, err)
			}
			if owner.calls != 1 || owner.ctx != t.Context() || owner.synchronous != synchronous || owner.request.ProjectRoot != "/selected" {
				t.Fatalf("durable request = %+v, calls = %d", owner.request, owner.calls)
			}
			if (result.Sync != nil) != synchronous || (result.Async != nil) == synchronous {
				t.Fatalf("mode result = %+v", result)
			}
			owner.request.Args["selected"] = "changed"
			if request.Args["selected"] != "original" || owner.request.WorkerSettings == request.WorkerSettings {
				t.Fatal("durable owner retained mutable caller selections")
			}
			assertDurableOpeningRetryAndValidation(t, opening, owner, request)
		})
	}
}

func assertInjectedOpeningOutcomes(t *testing.T, root *Root, ctx context.Context, request factorysessions.SessionStartRequest, startResult factorysessions.SessionStartResult, replay *factorysessions.HistoricalReplayInspection, outcomeErr error, cause *os.PathError, failing bool) {
	t.Helper()
	got, err := root.Start(ctx, request)
	if !reflect.DeepEqual(got, startResult) || !errors.Is(err, outcomeErr) {
		t.Fatalf("start = (%+v, %v)", got, err)
	}
	if failing {
		var typed *os.PathError
		if !errors.As(err, &typed) || typed != cause {
			t.Fatalf("lost typed cause: %v", err)
		}
	}
	inspected, historical, err := root.InspectHistoricalApplication(ctx, request)
	if inspected.Replay != replay || !historical || !errors.Is(err, outcomeErr) {
		t.Fatalf("inspection = (%+v, %v, %v)", inspected, historical, err)
	}
}

func assertDurableOpeningRetryAndValidation(t *testing.T, opening *RuntimeOpening, owner *openingDurableStartStub, request factorysessions.SessionStartRequest) {
	t.Helper()
	cause := &os.PathError{Op: "write", Path: "/selected", Err: os.ErrPermission}
	owner.failure = cause
	if _, err := opening.Start(t.Context(), request); !errors.Is(err, cause) {
		t.Fatalf("typed cause = %v", err)
	}
	owner.failure = nil
	if _, err := opening.Start(t.Context(), request); err != nil || owner.calls != 3 {
		t.Fatalf("corrected retry = %v, calls = %d", err, owner.calls)
	}
	request.Wait.TimeoutMillis = -1
	var invalid *factorysessions.DetachedRequestError
	if _, err := opening.Start(t.Context(), request); !errors.As(err, &invalid) || invalid.Field != "wait.timeoutMillis" || owner.calls != 3 {
		t.Fatalf("invalid request = %v, calls = %d", err, owner.calls)
	}
}
