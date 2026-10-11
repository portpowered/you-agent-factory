package wire

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/internal/testutil/checkpointfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/factoryruntimefixtures"
	execution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	events "github.com/portpowered/infinite-you/pkg/services/events"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	identitycontract "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	responsecontract "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/testing/eventsstub"
)

func TestNewRuntimeAssemblyRejectsMissingRequiredDependencies(t *testing.T) {
	t.Parallel()

	valid := validNewServiceInputs()
	tests := []struct {
		name   string
		mutate func(*newServiceInputs)
	}{
		{name: "session result projection", mutate: func(in *newServiceInputs) { in.sessionResultProjection = nil }},
		{name: "response event ID generator", mutate: func(in *newServiceInputs) { in.eventIDs = nil }},
		{name: "session ID generator", mutate: func(in *newServiceInputs) { in.sessionIDs = nil }},
		{name: "home directory resolver", mutate: func(in *newServiceInputs) { in.resolveHome = nil }},
		{name: "directory inspection", mutate: func(in *newServiceInputs) { in.directoryInspection = nil }},
		{name: "named path resolver", mutate: func(in *newServiceInputs) { in.namedPaths = nil }},
		{name: "initial Work reader", mutate: func(in *newServiceInputs) { in.initialWorkFiles = nil }},
		{name: "symlink resolver", mutate: func(in *newServiceInputs) { in.resolveSymlinks = nil }},
		{name: "identity service", mutate: func(in *newServiceInputs) { in.omitIdentity = true }},
		{name: "response-stream service", mutate: func(in *newServiceInputs) { in.omitResponses = true }},
		{name: "events root", mutate: func(in *newServiceInputs) { in.eventsService = nil }},
		{name: "clock", mutate: func(in *newServiceInputs) { in.clock = nil }},
		{name: "live-change coordinator", mutate: func(in *newServiceInputs) { in.liveChangeCoordinator = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inputs := valid
			test.mutate(&inputs)
			assembly, err := inputs.callNewRuntimeAssembly()
			if err == nil {
				t.Fatalf("NewRuntimeAssembly() error = nil, want missing %s dependency", test.name)
			}
			if assembly != nil {
				t.Fatalf("NewRuntimeAssembly() = %#v, want nil assembly", assembly)
			}
		})
	}
}

func TestNewServiceFromAssemblyConstructsPublishedRoot(t *testing.T) {
	t.Parallel()

	inputs := validNewServiceInputs()
	assembly, err := inputs.callNewRuntimeAssembly()
	if err != nil {
		t.Fatalf("NewRuntimeAssembly() error = %v", err)
	}
	service, err := newRootWithAssembly(assembly, inputs.liveChangeCoordinator)
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewRoot() returned nil service")
	}
	var root factorysessions.Service = service
	var liveControl factorysessions.LiveControlService = service
	if any(liveControl) != any(root) {
		t.Fatalf("LiveControlService = %T, want the same authoritative Service instance %T", liveControl, root)
	}
	var deletion factorysessions.LiveDeletionService = service
	if any(deletion) != any(root) {
		t.Fatalf("LiveDeletionService = %T, want the same authoritative Service instance %T", deletion, root)
	}
}

func TestNewServiceFromAssemblyRetainsOneRuntimeAssemblyOnThePublishedRoot(t *testing.T) {
	t.Parallel()

	inputs := validNewServiceInputs()
	assembly, err := inputs.callNewRuntimeAssembly()
	if err != nil {
		t.Fatalf("NewRuntimeAssembly() error = %v", err)
	}
	service, err := newRootWithAssembly(assembly, inputs.liveChangeCoordinator)
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}
	var retained RuntimeAssembly = service
	if any(retained) != any(service) {
		t.Fatalf("runtime assembly = %T(%[1]v), want the same published process root %T", retained, service)
	}
}

func TestNewServiceFromAssemblyReturnsDirectRootIdentity(t *testing.T) {
	t.Parallel()

	inputs := validNewServiceInputs()
	assembly, err := inputs.callNewRuntimeAssembly()
	if err != nil {
		t.Fatalf("NewRuntimeAssembly() error = %v", err)
	}
	root, err := newRootWithAssembly(assembly, inputs.liveChangeCoordinator)
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}
	var service factorysessions.Service = root
	if any(service) != any(root) {
		t.Fatal("Service is not the exact same *Root instance")
	}
}

func TestNewServiceFromAssemblyConstructsInertRoot(t *testing.T) {
	t.Parallel()

	directories := &recordingDirectoryInspection{}
	homeCalls := 0
	symlinkCalls := 0
	invocationInputCalls := 0
	initialWorkCalls := 0
	eventIDCalls := 0
	sessionIDCalls := 0
	inputs := validNewServiceInputs()
	inputs.directoryInspection = directories
	inputs.resolveHome = func() (string, error) {
		homeCalls++
		panic("home directory resolved during inert construction")
	}
	inputs.resolveSymlinks = func(path string) (string, error) {
		symlinkCalls++
		panic("symlink resolved during inert construction")
	}
	inputs.invocationInputFiles = fileeffects.InvocationInputReader(func(string) ([]byte, error) {
		invocationInputCalls++
		panic("invocation input read during inert construction")
	})
	inputs.initialWorkFiles = fileeffects.InitialWorkReader(func(string) ([]byte, error) {
		initialWorkCalls++
		panic("initial Work read during inert construction")
	})
	inputs.eventIDs = func() string {
		eventIDCalls++
		return "response-event-id"
	}
	inputs.sessionIDs = func() string {
		sessionIDCalls++
		return "session-id"
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	assembly, err := inputs.callNewRuntimeAssembly()
	if err != nil {
		t.Fatalf("NewRuntimeAssembly() error = %v", err)
	}
	service, err := newRootWithAssembly(assembly, inputs.liveChangeCoordinator)
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewRoot() returned nil service")
	}
	if directories.calls != 0 {
		t.Fatalf("construction inspected filesystem %d times, want no runtime activity", directories.calls)
	}
	if homeCalls != 0 || symlinkCalls != 0 || invocationInputCalls != 0 || initialWorkCalls != 0 {
		t.Fatalf(
			"construction invoked effect stubs (home=%d symlinks=%d invocation input=%d initial Work=%d), want inert construction",
			homeCalls, symlinkCalls, invocationInputCalls, initialWorkCalls,
		)
	}
	if eventIDCalls != 0 || sessionIDCalls != 0 {
		t.Fatalf(
			"construction invoked generators (event IDs=%d session IDs=%d), want inert construction",
			eventIDCalls, sessionIDCalls,
		)
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	if leaked := runtime.NumGoroutine() - baseline; leaked > 4 {
		t.Fatalf("goroutine leak after construction: baseline=%d current=%d delta=%d", baseline, runtime.NumGoroutine(), leaked)
	}
}

type resultProjector struct{}

func (resultProjector) ProjectSessionResults(factoryruntime.SessionResultInput) factoryruntime.SessionResultProjection {
	return factoryruntime.SessionResultProjection{}
}

type newServiceInputs struct {
	omitIdentity, omitResponses  bool
	newJavaScriptCheckpointStore factoryruntime.JavaScriptCheckpointStoreFactory
	sessionResultProjection      factoryruntime.SessionResultProjectionOperation
	interpolation                factorydefinitions.InvocationInterpolationService
	invocationWorkTypes          factorydefinitions.InvocationWorkTypeService
	ttsObservability             factorydefinitions.TTSObservabilityService
	eventIDs                     factorysessions.ResponseEventIDGenerator
	responseEventRetentionLimits *factorysessions.ResponseEventRetentionLimits
	sessionIDs                   factorysessions.SessionIDGenerator
	resolveHome                  factorysessions.HomeDirectoryResolver
	directoryInspection          DirectoryInspection
	namedPaths                   factorydefinitions.NamedPathResolver
	invocationInputFiles         fileeffects.InvocationInputReader
	initialWorkFiles             fileeffects.InitialWorkReader
	resolveSymlinks              factorysessions.LogicalTargetResolveSymlinks
	eventsService                events.Service
	clock                        factoryruntime.Clock
	liveChangeCoordinator        LiveChangeCoordinator
}

func validNewServiceInputs() newServiceInputs {
	eventsService := eventsstub.New()
	return newServiceInputs{
		sessionResultProjection: resultProjector{},
		eventIDs:                func() string { return "response-event-id" },
		sessionIDs:              func() string { return "session-id" },
		resolveHome:             func() (string, error) { return "home", nil },
		directoryInspection:     &recordingDirectoryInspection{},
		namedPaths:              namedPathResolver{},
		invocationInputFiles:    fileeffects.InvocationInputReader(func(string) ([]byte, error) { return nil, nil }),
		initialWorkFiles:        fileeffects.InitialWorkReader(func(string) ([]byte, error) { return nil, nil }),
		resolveSymlinks:         func(path string) (string, error) { return path, nil },
		eventsService:           eventsService,
		clock:                   &recordingClock{},
		liveChangeCoordinator:   NewLiveChangeCoordinator(),
	}
}

func (in newServiceInputs) callNewRuntimeAssembly() (RuntimeAssembly, error) {
	identity, err := NewIdentity(in.resolveSymlinks, in.resolveHome)
	if err != nil {
		return nil, err
	}
	responses, err := NewResponseStreams(in.eventIDs, in.responseEventRetentionLimits, in.eventsService, logging.NoopLogger{})
	if err != nil {
		return nil, err
	}
	registry := NewSessionRegistry()
	responseRegistry, err := NewResponseStreamRegistry(responses, in.clock)
	if err != nil {
		return nil, err
	}
	state := NewSessionState(registry, responseRegistry, in.clock, in.eventIDs, in.sessionIDs, responses)
	streams := NewStreamManager(state, NewStreamObserver(), responseRegistry, responses)
	assemblyIdentity, assemblyResponses := identity, responses
	if in.omitIdentity {
		assemblyIdentity = nil
	}
	if in.omitResponses {
		assemblyResponses = nil
	}

	gateway := NewGateway(NewSessionHost(state, NewScopeControl(state, nil, zap.NewNop()), identity, in.clock, nil, in.newJavaScriptCheckpointStore, zap.NewNop()), streams, nil, in.sessionResultProjection, responses, in.liveChangeCoordinator, nil, nil, nil, NewNamedFactoryActivator(state), NewKeyedDefinitionActivationGateway(state, in.clock))
	return NewRuntimeAssembly(
		gateway,
		registry, state, streams, sessioninvocation.NewSessionOwner(NewInvocationAuthority(state, platformclock.Real{}, nil), NewScopeControl(state, nil, zap.NewNop()), nil, nil, in.interpolation, in.invocationWorkTypes, in.invocationInputFiles, nil), NewScopeControl(state, nil, zap.NewNop()), NewScopeActivation(state),
		in.newJavaScriptCheckpointStore,
		in.sessionResultProjection,
		in.eventIDs,
		in.sessionIDs,
		in.resolveHome,
		in.directoryInspection,
		in.namedPaths,
		in.initialWorkFiles,
		assemblyIdentity,
		assemblyResponses,
		in.clock,
		in.liveChangeCoordinator,
		nil,
		NewSessionHost(state, NewScopeControl(state, nil, zap.NewNop()), identity, in.clock, nil, in.newJavaScriptCheckpointStore, zap.NewNop()),
		NewNamedFactoryActivator(state),
		NewKeyedDefinitionActivationGateway(state, in.clock),
		nil, nil, nil,
		nil,
		nil, nil,
	)
}

type recordingClock struct{ calls int }

func (c *recordingClock) Now() time.Time {
	c.calls++
	return time.Time{}
}

type recordingDirectoryInspection struct{ calls int }

func (d *recordingDirectoryInspection) Stat(string) (fs.FileInfo, error) {
	d.calls++
	return nil, fs.ErrNotExist
}

func (d *recordingDirectoryInspection) ReadDir(string) ([]fs.DirEntry, error) {
	d.calls++
	return nil, nil
}

type namedPathResolver struct{}

func (namedPathResolver) ResolveCandidatePaths(string, string, string) (factorydefinitions.NamedFactoryCandidatePaths, error) {
	return factorydefinitions.NamedFactoryCandidatePaths{}, nil
}
func (namedPathResolver) ResolveExistingDir(string, string) (string, error) { return "", nil }
func (namedPathResolver) RequireDefinitionDir(string) error                 { return nil }
func (namedPathResolver) ResolveCurrentDir(string) (string, error)          { return "", nil }
func (namedPathResolver) ReadCurrentPointer(string) (string, error)         { return "", nil }
func (namedPathResolver) WriteCurrentPointer(string, string) error          { return nil }

var _ DirectoryInspection = (*recordingDirectoryInspection)(nil)

func TestIdentityNormalizesUsingSelectedEffects(t *testing.T) {
	t.Parallel()
	home, canonical := t.TempDir(), t.TempDir()
	var homes, symlinks int
	service, err := NewIdentity(func(path string) (string, error) {
		symlinks++
		if path != filepath.Join(home, "factory") {
			t.Fatalf("symlink input = %q", path)
		}
		return canonical, nil
	}, func() (string, error) { homes++; return home, nil })
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.Normalize(t.Context(), identitycontract.NormalizeRequest{BackendScopeID: "scope", FolderPath: "~/factory", Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Reference.FolderPath != canonical || got.Reference.BackendScopeID != "scope" || got.LogicalSessionKeyID == "" || homes != 1 || symlinks != 1 {
		t.Fatalf("normalized identity = %#v, home=%d symlinks=%d", got, homes, symlinks)
	}
	_, err = service.Normalize(t.Context(), identitycontract.NormalizeRequest{FolderPath: "~/factory", Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}})
	if err == nil {
		t.Fatal("missing backend scope accepted")
	}
}

type selectedResponseClock struct{ now time.Time }

func (c selectedResponseClock) Now() time.Time { return c.now }

func TestResponseStreamsRetainIndependentStoresAtSelectedClocks(t *testing.T) {
	t.Parallel()
	var ids atomic.Uint64
	limits := &factorysessions.ResponseEventRetentionLimits{MaxEvents: 2, MaxBytes: 1 << 20, CompletedRetentionWindow: time.Minute}
	service, err := NewResponseStreams(func() string { return fmt.Sprintf("selected-%d", ids.Add(1)) }, limits, eventsstub.New(), logging.NoopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	firstTime := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	stores := []string{"first", "second"}
	for i, name := range stores {
		now := firstTime.Add(time.Duration(i) * time.Hour)
		store, err := service.NewEventStore(name, selectedResponseClock{now})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { service.Close(store) })
		gotLimits := store.RetentionLimits()
		wantLimits := responseeventstore.RetentionLimits{MaxEvents: 2, MaxBytes: 1 << 20, CompletedRetentionWindow: time.Minute}
		if gotLimits != wantLimits {
			t.Fatalf("retention = %#v, want %#v", gotLimits, wantLimits)
		}
		var last responseevents.FactoryResponseEvent
		for n := range 3 {
			last, err = service.Publish(store, responseevents.FactoryResponseEvent{Kind: responseevents.KindMessage, Phase: responseevents.PhaseDelta, RunID: "run-" + name, Provenance: responseevents.Provenance{Provider: "test", NativeEventType: "delta", Delivery: responseevents.DeliveryNativeStream, Representation: responseevents.RepresentationDelta, Fidelity: responseevents.FidelityLossless}, Payload: json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"` + name + `"}`)})
			if err != nil {
				t.Fatal(err)
			}
			assertSelectedResponseEvent(t, last, name, now, fmt.Sprintf("selected-%d", ids.Load()), int64(n+1))
		}
		service.Complete(store)
		cursor, err := service.Subscribe(t.Context(), store, responsecontract.SubscriptionRequest{AfterSequence: last.Sequence - 1})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(cursor.Detach)
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		got, err := cursor.Next(ctx)
		cancel()
		if err != nil || len(got) != 1 || got[0].EventID != last.EventID || got[0].FactorySessionID != name {
			t.Fatalf("retained suffix = %#v, %v", got, err)
		}
	}
}

func assertSelectedResponseEvent(t *testing.T, event responseevents.FactoryResponseEvent, sessionID string, now time.Time, eventID string, sequence int64) {
	t.Helper()
	if event.FactorySessionID != sessionID || !event.RecordedAt.Equal(now) || event.EventID != eventID || event.Sequence != sequence {
		t.Fatalf("published = %#v, want session %s time %s ID %s sequence %d", event, sessionID, now, eventID, sequence)
	}
}

// Only the allocation boundary is implemented; other calls would expose an
// unexpected construction side effect.
type failingResponseRegistry struct {
	ResponseStreams
	err error
}

func (f failingResponseRegistry) NewStreamRegistry(factoryruntime.Clock) (*responsestream.Registry, error) {
	return nil, f.err
}
func TestResponseStreamRegistryReturnsSelectedAllocationError(t *testing.T) {
	t.Parallel()
	failure := errors.New("selected allocation failed")
	registry, err := NewResponseStreamRegistry(failingResponseRegistry{err: failure}, &recordingClock{})
	if registry != nil || !errors.Is(err, failure) {
		t.Fatalf("allocation = (%v, %v), want nil and selected error", registry, err)
	}
}

// This component witness checks emitted effects, rather than construction identity.
func TestInvocationTelemetryUsesAddressedLoggerAndSelectedMetrics(t *testing.T) {
	t.Parallel()
	registry := NewSessionRegistry()
	state := &SessionState{}
	// The paired state owns selection; each session owns its logger.
	inputs := validNewServiceInputs()
	responses, err := NewResponseStreams(inputs.eventIDs, nil, inputs.eventsService, logging.NoopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	streams, err := NewResponseStreamRegistry(responses, inputs.clock)
	if err != nil {
		t.Fatal(err)
	}
	state = NewSessionState(registry, streams, inputs.clock, inputs.eventIDs, inputs.sessionIDs, responses)
	logs := make(map[string]*observer.ObservedLogs)
	for _, id := range []string{"a", "b"} {
		core, observed := observer.New(zap.InfoLevel)
		logs[id] = observed
		registry.Upsert(&livesession.LiveSession{ID: id, Handle: &runtimebinding.SessionState{Logger: zap.New(core).With(zap.String("owned_scope", id))}}, false)
	}
	fallbackCore, fallback := observer.New(zap.InfoLevel)
	metrics := &invocationTelemetryMetrics{}
	telemetry := NewInvocationTelemetry(state, nil, metrics, zap.New(fallbackCore))
	cfg := &factorydefinitions.FactoryConfig{}
	for _, id := range []string{"a", "b"} {
		telemetry.NormalizationAttempt(cfg, work.InputSourceLabel("text"))
		telemetry.LogInvocationSubmitted(id, work.InputSourceLabel("text"), cfg, work.WorkRequestSubmitResult{RequestID: "request-" + id})
	}
	if len(metrics.records) != 2 || metrics.records[0].Name != sessioninvocation.InvocationMetricNormalizationAttempts {
		t.Fatalf("selected metrics = %#v", metrics.records)
	}
	for _, id := range []string{"a", "b"} {
		entries := logs[id].All()
		if len(entries) != 1 || entries[0].ContextMap()["session_id"] != id || entries[0].ContextMap()["owned_scope"] != id {
			t.Fatalf("scope %s emitted logs = %#v", id, entries)
		}
	}
	if fallback.Len() != 0 {
		t.Fatalf("addressed emissions reached fallback: %v", fallback.All())
	}
	telemetry.LogInvocationSubmitted("missing", work.InputSourceLabel("text"), cfg, work.WorkRequestSubmitResult{})
	if fallback.Len() != 1 {
		t.Fatal("missing addressed logger did not use selected process logger")
	}
}

type invocationTelemetryMetrics struct {
	records []factorysessions.InvocationMetric
}

func (m *invocationTelemetryMetrics) RecordInvocationMetric(metric factorysessions.InvocationMetric) {
	m.records = append(m.records, metric)
}

func TestNewInvocationOwnerRequiresInputReader(t *testing.T) {
	t.Parallel()
	owner, err := NewInvocationOwner(nil, nil, nil, nil, nil, nil, nil, nil)
	if err == nil || owner != nil {
		t.Fatalf("owner=%v error=%v, want missing input reader", owner, err)
	}
}

type processDurableScopeFixture struct {
	project string
	err     error
}

func (f *processDurableScopeFixture) CurrentProjectRoot() string { return f.project }
func (f *processDurableScopeFixture) ResumeRuntimeScope(project string) (execution.ResumeRuntimeScope, error) {
	if project != f.project {
		return execution.ResumeRuntimeScope{}, factorysessions.ErrSessionNotFound
	}
	return execution.ResumeRuntimeScope{}, f.err
}

type processDurableStore struct{ runtimepersist.Store }

func (processDurableStore) Save(string, []byte) error   { return nil }
func (processDurableStore) Load(string) ([]byte, error) { return nil, fs.ErrNotExist }

type processDurableWriter struct {
	recordings.PortableRecordingWriter
}

func TestProcessDurableOwnerPreservesSelectedFactsAndErrors(t *testing.T) {
	t.Parallel()
	failure := errors.New("selected project store failed")
	scope := &processDurableScopeFixture{project: "project-first"}
	roots := []string{}
	stores := func(root string) (roles.RuntimePersistenceStore, error) {
		roots = append(roots, root)
		if root == "project-second" {
			return nil, failure
		}
		return processDurableStore{}, nil
	}
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	owner, err := newProcessDurableFixture(func() (string, error) { return "selected-home", nil }, stores, scope, selectedResponseClock{now})
	if err != nil {
		t.Fatal(err)
	}
	started, err := owner.StartSync(t.Context(), factorysessions.DurableStartRequest{
		RequestID: "selected-start", ProjectRoot: "project-first",
		Source: factorysessions.Source{Kind: factoryruntime.WorkflowSourceKindInlineWorkflow,
			InlineWorkflow: &factorysessions.InlineWorkflowSource{Dialect: "you-workflow-v1", InlineSource: "return {ok:true};"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	read, err := owner.GetSession(t.Context(), started.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if read.Lifecycle == nil || read.Lifecycle.StartedAt == nil || !read.Lifecycle.StartedAt.Equal(now) || read.Status != factorysessions.LifecycleStatusSucceeded {
		t.Fatalf("selected lifecycle = %#v", read)
	}
	scope.project = "project-second"
	probe := owner.(interface {
		HasDurableState(context.Context, string) (bool, error)
	})
	if _, err := probe.HasDurableState(t.Context(), "dur-sess-unopened"); !errors.Is(err, failure) {
		t.Fatalf("read route failure = %v", err)
	}
	if len(roots) < 3 || roots[0] != "selected-home" || roots[len(roots)-1] != "project-second" {
		t.Fatalf("selected store roots = %v", roots)
	}
}

func newProcessDurableFixture(resolve factorysessions.HomeDirectoryResolver, stores RuntimePersistenceStoreFactory, scope ProcessDurableScope, clock factoryruntime.Clock) (durableexecution.Service, error) {
	workflows := factoryruntimefixtures.ScriptedJavaScriptWorkflows{RunFunc: func(context.Context, factoryruntime.JavaScriptRuntimeRequest, factoryruntime.JavaScriptRuntimeHooks) (factoryruntime.JavaScriptRuntimeOutcome, error) {
		return factoryruntime.JavaScriptRuntimeOutcome{OK: true}, nil
	}}
	return NewProcessDurableExecution(resolve, factorysessions.ChildExecutorModeFake, stores, clock,
		platformclock.Real{}, checkpointfixtures.CheckpointSummariesFixture{}, workflows, workflows,
		processDurableWriter{}, func() string { return "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb" }, nil, nil, nil,
		scope, nil, nil, zap.NewNop())
}

func TestProcessDurableOwnerPreservesConstructionFailureCauses(t *testing.T) {
	t.Parallel()
	failure := errors.New("selected home failure")
	result, err := newProcessDurableFixture(func() (string, error) { return "", failure }, nil, nil, platformclock.Real{})
	if result != nil || !errors.Is(err, failure) {
		t.Fatalf("home result=%v error=%v", result, err)
	}
	result, err = newProcessDurableFixture(func() (string, error) { return "home", nil }, func(string) (roles.RuntimePersistenceStore, error) { return nil, failure }, nil, platformclock.Real{})
	var validation *execution.ValidationError
	if result != nil || !errors.As(err, &validation) || validation.Field != "persistence" || !strings.Contains(err.Error(), failure.Error()) {
		t.Fatalf("persistence construction result=%v error=%v", result, err)
	}
}

type gatewayOwnerInvocation struct {
	InvocationService
	sessionID, requestID string
	err                  error
}

func (owner *gatewayOwnerInvocation) InvokeFactorySession(_ context.Context, sessionID string, request factorysessions.InvocationRequest) (factorydefinitions.FactoryInvocationResult, error) {
	owner.sessionID = sessionID
	owner.requestID = ""
	if request.RequestID != nil {
		owner.requestID = *request.RequestID
	}
	return factorydefinitions.FactoryInvocationResult{SessionID: sessionID, RequestID: owner.requestID, WorkID: "selected-work"}, owner.err
}

type gatewayOwnerHistory struct{ err error }

func (history gatewayOwnerHistory) ListSessions(_ context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
	return factorysessions.ListSessionsResult{Scope: request.Scope, RecordedSessions: []factorysessions.RecordedSessionListSummary{{SessionID: "recorded", ArtifactReference: "recorded.jsonl"}}}, history.err
}

type gatewayOwnerDurable struct {
	DurableExecutionService
	request factorysessions.ListSessionsRequest
	err     error
}

func (owner *gatewayOwnerDurable) ListSessions(_ context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
	owner.request = request
	return factorysessions.ListSessionsResult{Scope: request.Scope, DurableSessions: []factorysessions.DurableSessionListSummary{{SessionID: "persisted"}}}, owner.err
}

func newOwnerGatewayFixture(t *testing.T, durable DurableExecutionService, history RecordedHistory, invoker InvocationService, activate NamedFactoryActivator) roles.SessionGateway {
	t.Helper()
	inputs := validNewServiceInputs()
	identity, err := NewIdentity(inputs.resolveSymlinks, inputs.resolveHome)
	if err != nil {
		t.Fatal(err)
	}
	responses, err := NewResponseStreams(inputs.eventIDs, nil, inputs.eventsService, logging.NoopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewResponseStreamRegistry(responses, inputs.clock)
	if err != nil {
		t.Fatal(err)
	}
	state := NewSessionState(NewSessionRegistry(), registry, inputs.clock, inputs.eventIDs, inputs.sessionIDs, responses)
	host := NewSessionHost(state, NewScopeControl(state, nil, zap.NewNop()), identity, inputs.clock, nil, inputs.newJavaScriptCheckpointStore, zap.NewNop())
	streams := NewStreamManager(state, NewStreamObserver(), registry, responses)
	gateway := NewGateway(host, streams, nil, inputs.sessionResultProjection, responses, inputs.liveChangeCoordinator, durable, history, invoker, activate, NewKeyedDefinitionActivationGateway(state, inputs.clock))
	return gateway
}

func TestOwnerGatewayPreservesInjectedInvocationAndActivation(t *testing.T) {
	t.Parallel()
	failure := errors.New("selected invocation failure")
	invoker := &gatewayOwnerInvocation{}
	var activationName string
	activationFailure := errors.New("selected activation failure")
	gateway := newOwnerGatewayFixture(t, nil, nil, invoker, func(_ context.Context, name string) error { activationName = name; return activationFailure })
	for _, id := range []string{"selected", "peer"} {
		requestID := "request-" + id
		result, err := gateway.InvokeFactorySession(t.Context(), id, factorysessions.InvocationRequest{RequestID: &requestID})
		if err != nil || result.SessionID != id || result.RequestID != "request-"+id || result.WorkID != "selected-work" || invoker.sessionID != id || invoker.requestID != result.RequestID {
			t.Fatalf("invocation %s = %+v, %v", id, result, err)
		}
	}
	invoker.err = failure
	if _, err := gateway.InvokeFactorySession(t.Context(), "selected", factorysessions.InvocationRequest{}); !errors.Is(err, failure) {
		t.Fatalf("invocation error = %v", err)
	}
	if err := gateway.ActivateNamedFactory(t.Context(), "owned-factory"); !errors.Is(err, activationFailure) || activationName != "owned-factory" {
		t.Fatalf("activation = %q, %v", activationName, err)
	}
	if _, err := gateway.GetFactorySession(t.Context(), "missing"); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("missing live session = %v", err)
	}
}

func TestOwnerGatewayPreservesInjectedHistoryAndDurableFailures(t *testing.T) {
	t.Parallel()
	durable := &gatewayOwnerDurable{}
	gateway := newOwnerGatewayFixture(t, durable, gatewayOwnerHistory{}, nil, nil)
	request := factorysessions.ListSessionsRequest{Scope: factorysessions.SessionListScopeAll, Filters: factorysessions.SessionListFilters{ProjectBoundary: "selected-project"}}
	result, err := gateway.ListSessions(t.Context(), request)
	if err != nil || len(result.RecordedSessions) != 1 || result.RecordedSessions[0].SessionID != "recorded" || len(result.DurableSessions) != 1 || result.DurableSessions[0].SessionID != "persisted" || durable.request.Filters.ProjectBoundary != "selected-project" {
		t.Fatalf("combined inventory = %+v, %v", result, err)
	}
	durable.err = errors.New("selected persistence read failure")
	if _, err := gateway.ListSessions(t.Context(), request); !errors.Is(err, durable.err) {
		t.Fatalf("durable failure = %v", err)
	}
	failure := errors.New("selected history failure")
	gateway = newOwnerGatewayFixture(t, nil, gatewayOwnerHistory{err: failure}, nil, nil)
	if _, err := gateway.ListSessions(t.Context(), factorysessions.ListSessionsRequest{Scope: factorysessions.SessionListScopeHistory}); !errors.Is(err, failure) {
		t.Fatalf("history failure = %v", err)
	}
}

func TestOwnerGatewayPreservesUnavailableRequiredCollaborators(t *testing.T) {
	t.Parallel()
	if gateway := NewGateway(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil); gateway != nil {
		t.Fatal("unavailable host and streams returned a non-nil gateway capability")
	}
}

// newRootWithAssembly exercises the production constructor with only the roles
// these root publication tests observe; no opening operation runs in this fixture.
func newRootWithAssembly(assembly RuntimeAssembly, liveChangeCoordinator LiveChangeCoordinator) (*Root, error) {
	return NewRoot(assembly, nil, nil, liveChangeCoordinator, nil, nil, nil, nil, nil, nil)
}
