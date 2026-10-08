package service

import (
	"context"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/recordingreplay"
	"reflect"
	"slices"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livechange"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	legacyservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

type factoryDefinitionsConstructionStub struct {
	factorydefinitions.Service
}

func TestBindWorkerScopeRejectsMissingRequiredBinder(t *testing.T) {
	_, err := bindWorkerScope("session-42", struct{}{}, nil, "runtime-1", "generation-1", nil, nil, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "session-42") || !strings.Contains(err.Error(), "binder is required") {
		t.Fatalf("missing scope binder error = %v, want session and required binder", err)
	}
}

// runtimeOpeningFixture supplies controlled direct collaborators to the opening owner.
type runtimeOpeningFixture struct {
	RecordingProjections         recordings.ProjectionService
	LiveChangeCoordinator        factorysessioncontracts.LiveChangeCoordinator
	ProviderSessions             providersessions.Service
	Logger                       *zap.Logger
	FactoryWorkflows             factoryruntime.JavaScriptWorkflowDefinitions
	WorkflowPreview              factoryruntime.WorkflowPreviewOperation
	RuntimeRoot                  FactoryRuntimeRoot
	ResolveClock                 factoryruntime.ClockResolver
	NewSessionLogger             factoryruntime.SessionLoggerFactory
	Clock                        factoryruntime.Clock
	ProviderOverride             ProviderOverrideService
	SubmissionRecorder           recordings.SubmissionRecorder
	DispatchRecorder             recordings.DispatchRecorder
	Validator                    factorydefinitions.Validator
	NamedPaths                   factorydefinitions.NamedPathResolver
	Definitions                  factorydefinitions.Service
	RuntimeRouter                *factorysessions.DefinitionRuntimeRouter
	LoadFactory                  factorydefinitions.LoadedFactoryLoader
	NewLoadedFactory             factorydefinitions.LoadedFactorySourceFactory
	DecodeReplayConfig           factorydefinitions.ReplayRuntimeConfigDecoder
	CaptureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer
	Assembly                     roles.RuntimeAssembly
	DurableOpening               *DurableOpening
	FactoryScaffoldInitializer   factorysessions.FactoryScaffoldInitializer
	EditableFactoryValidator     factorysessions.EditableFactoryValidator
	ProcessRuntimeFactory        roles.ProcessRuntimeFactory
	GenerateSessionID            factorysessions.SessionIDGenerator
	GenerateRuntimeInstanceID    factorysessions.RuntimeInstanceIDGenerator
	ResolveHome                  factorysessions.HomeDirectoryResolver
	ProviderIdentities           factorysessions.ProviderIdentityResolver
	WorkService                  work.Service
	AutomationService            automations.Service
	WebhooksService              webhooks.Service
	ModelService                 models.Service
	RecordingsService            recordings.Service
	RecordingsRuntime            recordings.RuntimeScopeService
	WorkerService                workers.Service
	ProviderCommandRunner        ProviderCommandRunner
	ScriptCommandRunner          ScriptCommandRunner
	EnsureBackendScope           operatorsettings.BackendScopeEnsurer
	InitialActivation            factoryruntime.InitialRuntimeActivationOperation
}

func (fixture runtimeOpeningFixture) newFactory() (*Root, error) {
	return NewRoot(
		fixture.ProviderSessions,
		fixture.Logger,
		nil,
		fixture.FactoryWorkflows,
		fixture.WorkflowPreview,
		fixture.RuntimeRoot,
		fixture.ResolveClock,
		fixture.Clock,
		fixture.ProviderOverride,
		fixture.SubmissionRecorder,
		fixture.DispatchRecorder,
		fixture.Validator,
		fixture.NamedPaths,
		fixture.Definitions,
		fixture.RuntimeRouter,
		NewRuntimeInputLoading(fixture.LoadFactory, fixture.NewLoadedFactory, fixture.DecodeReplayConfig, fixture.RecordingsRuntime, fixture.CaptureLoadedFactorySnapshot, fixture.NewSessionLogger, fixture.Logger),
		fixture.snapshotSelection(),
		fixture.Assembly,
		fixture.DurableOpening,
		fixture.FactoryScaffoldInitializer,
		fixture.EditableFactoryValidator,
		fixture.ProcessRuntimeFactory,
		fixture.GenerateSessionID,
		fixture.GenerateRuntimeInstanceID,
		fixture.ResolveHome,
		fixture.ProviderIdentities,
		fixture.WorkService,
		fixture.AutomationService,
		fixture.WebhooksService,
		fixture.ModelService,
		fixture.RecordingsService,
		fixture.RecordingsRuntime,
		fixture.WorkerService,
		fixture.ProviderCommandRunner,
		fixture.ScriptCommandRunner,
		fixture.EnsureBackendScope,
		fixture.InitialActivation,
		nil,
		recordingreplay.NewBehavior(),
		fixture.LiveChangeCoordinator,
		fixture.RecordingProjections,
	)
}
func TestNewFactoryRemainsInert(t *testing.T) {
	t.Parallel()
	calls := 0
	dependencies := validRuntimeOpeningCollaborators(&calls)
	materializer := &selectedOpeningMaterializer{}
	dependencies.WorkService = work.MaterializationService(materializer)
	factory, err := dependencies.newFactory()
	if err != nil || factory == nil {
		t.Fatalf("NewRoot() = (%v, %v)", factory, err)
	}
	if calls != 0 {
		t.Fatalf("construction invoked %d collaborator functions", calls)
	}
	path, cleanup, err := factory.workService.MaterializeContentURL(t.Context(), "file:///identity.png")
	if err != nil || path != "/tmp/identity.png" || cleanup == nil {
		t.Fatalf("selected Work materialization = (%q, %v, %v)", path, cleanup, err)
	}
	cleanup()
	if materializer.calls != 1 || materializer.input != "file:///identity.png" {
		t.Fatalf("selected materializer calls = %d with %q", materializer.calls, materializer.input)
	}
}

func TestNewFactoryOpensHistoricalReplayWithoutLiveRuntimeCollaborators(t *testing.T) {
	t.Parallel()

	portable := runtimeInputPortableFixture()
	var events []string
	calls := 0
	dependencies := validRuntimeOpeningCollaborators(&calls)
	replayInputs := &historicalReplayInputsRecorder{portable: portable, events: &events}
	recordingsRoot := &recordingsRootConstructionStub{replayInputs: replayInputs}
	dependencies.RecordingsService = recordingsRoot
	dependencies.RecordingsRuntime = recordingsRoot
	dependencies.GenerateRuntimeInstanceID = func() string {
		events = append(events, "runtime-instance-id")
		return "historical-runtime"
	}
	dependencies.NewSessionLogger = func(*zap.Logger, string, string, string) *zap.Logger {
		events = append(events, "session-logger")
		return zap.NewNop()
	}

	factory, err := dependencies.newFactory()
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	opened, historical, err := factory.InspectHistoricalApplication(t.Context(), factorysessions.SessionStartRequest{
		FolderPath:       t.TempDir(),
		RuntimeSelection: &factorysessions.SessionRuntimeSelection{Recording: factorysessions.SessionRecordingSelection{ReplayPath: "recording.json"}},
	})
	if err != nil {
		t.Fatalf("InspectHistoricalApplication() error = %v", err)
	}
	if !historical || opened.Replay == nil {
		t.Fatal("InspectHistoricalApplication() Replay = nil, want inspection-only replay")
	}
	if opened.Close == nil {
		t.Fatal("InspectHistoricalApplication() Close = nil, want historical replay cleanup")
	}
	if !slices.Equal(events, []string{"replay-input", "runtime-instance-id", "session-logger"}) {
		t.Fatalf("historical replay inspection events = %v, want one replay input before inspection resources", events)
	}
	if len(replayInputs.requests) != 1 || replayInputs.requests[0].Path != "recording.json" {
		t.Fatalf("replay input requests = %#v, want recording.json once", replayInputs.requests)
	}
	if calls != 0 {
		t.Fatalf("historical replay invoked %d live runtime collaborator functions, want none", calls)
	}
}

func validRuntimeOpeningCollaborators(calls *int) runtimeOpeningFixture {
	factorySessionsRoot := &legacyservice.Assembly{}
	return runtimeOpeningFixture{
		LiveChangeCoordinator:        livechange.NewCoordinator(),
		InitialActivation:            inertRuntimeOpeningFunction[factoryruntime.InitialRuntimeActivationOperation](calls),
		ProviderSessions:             providerSessionsConstructionStub{},
		Logger:                       zap.NewNop(),
		FactoryWorkflows:             workflowDefinitionsConstructionStub{},
		WorkflowPreview:              workflowPreviewConstructionStub{},
		ResolveClock:                 inertRuntimeOpeningFunction[factoryruntime.ClockResolver](calls),
		NewSessionLogger:             inertRuntimeOpeningFunction[factoryruntime.SessionLoggerFactory](calls),
		Clock:                        openingCoordinatorClock{},
		Validator:                    validatorConstructionStub{},
		NamedPaths:                   namedPathsConstructionStub{},
		Definitions:                  factoryDefinitionsConstructionStub{},
		RuntimeRouter:                &factorysessions.DefinitionRuntimeRouter{},
		LoadFactory:                  inertRuntimeOpeningFunction[factorydefinitions.LoadedFactoryLoader](calls),
		NewLoadedFactory:             inertRuntimeOpeningFunction[factorydefinitions.LoadedFactorySourceFactory](calls),
		DecodeReplayConfig:           inertRuntimeOpeningFunction[factorydefinitions.ReplayRuntimeConfigDecoder](calls),
		CaptureLoadedFactorySnapshot: inertRuntimeOpeningFunction[factorydefinitions.LoadedFactorySnapshotCapturer](calls),
		Assembly:                     factorySessionsRoot,
		DurableOpening:               durableOpeningFixture(nil, inertRuntimeOpeningFunction[durableexecution.ScopeAcquisition](calls)),
		FactoryScaffoldInitializer:   inertRuntimeOpeningFunction[factorysessions.FactoryScaffoldInitializer](calls),
		EditableFactoryValidator:     inertRuntimeOpeningFunction[factorysessions.EditableFactoryValidator](calls),
		ProcessRuntimeFactory:        processRuntimeFactoryConstructionStub{},
		GenerateSessionID:            inertRuntimeOpeningFunction[factorysessions.SessionIDGenerator](calls),
		GenerateRuntimeInstanceID:    inertRuntimeOpeningFunction[factorysessions.RuntimeInstanceIDGenerator](calls),
		ResolveHome:                  inertRuntimeOpeningFunction[factorysessions.HomeDirectoryResolver](calls),
		ProviderIdentities:           inertRuntimeOpeningFunction[factorysessions.ProviderIdentityResolver](calls),
		WorkService:                  work.MaterializationService(constructionMaterializer{calls: calls}),
		AutomationService:            automations.Root{},
		ModelService:                 &modelsConstructionStub{},
		RecordingsService:            &recordingsRootConstructionStub{},
		RecordingsRuntime:            &recordingsRootConstructionStub{},
		RecordingProjections:         &openingCoordinatorProjection{},
		WebhooksService:              webhooksConstructionStub{},
		WorkerService:                &workersConstructionStub{},
		ProviderCommandRunner:        workersRootBindingProbeRunner{tag: "provider"},
		ScriptCommandRunner:          workersRootBindingProbeRunner{tag: "script"},
		EnsureBackendScope:           inertRuntimeOpeningFunction[operatorsettings.BackendScopeEnsurer](calls),
	}
}

func inertRuntimeOpeningFunction[T any](calls *int) T {
	functionType := reflect.TypeOf((*T)(nil)).Elem()
	function := reflect.MakeFunc(functionType, func([]reflect.Value) []reflect.Value {
		(*calls)++
		results := make([]reflect.Value, functionType.NumOut())
		for index := range results {
			results[index] = reflect.Zero(functionType.Out(index))
		}
		return results
	})
	return function.Interface().(T)
}

type providerSessionsConstructionStub struct{ providersessions.Service }
type workflowDefinitionsConstructionStub struct {
	factoryruntime.JavaScriptWorkflowDefinitions
}
type workflowPreviewConstructionStub struct {
	factoryruntime.WorkflowPreviewOperation
}
type validatorConstructionStub struct{ factorydefinitions.Validator }
type namedPathsConstructionStub struct {
	factorydefinitions.NamedPathResolver
}
type factorySessionsConstructionStub struct {
	roles.SessionGateway
	roles.RuntimeAssembly
}
type processRuntimeFactoryConstructionStub struct{ roles.ProcessRuntimeFactory }
type modelsConstructionStub struct{ models.Service }
type recordingsRootConstructionStub struct {
	recordings.Service
	recordings.RuntimeScopeService
	replayInputs recordings.ReplayInputLoader
}

type webhooksConstructionStub struct{ webhooks.Service }
type workersConstructionStub struct{ workers.Service }

type workersRootBindingProbeRunner struct{ tag string }

func (workersRootBindingProbeRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, nil
}

type constructionMaterializer struct{ calls *int }

func (materializer constructionMaterializer) MaterializeContentURL(
	context.Context,
	string,
) (string, work.ContentCleanup, error) {
	(*materializer.calls)++
	return "", nil, nil
}

type historicalReplayInputsRecorder struct {
	portable recordings.PortableRecording
	events   *[]string
	requests []recordings.LoadReplayInputRequest
}

func (recorder *historicalReplayInputsRecorder) LoadReplayInput(
	request recordings.LoadReplayInputRequest,
) (recordings.LoadReplayInputResult, error) {
	*recorder.events = append(*recorder.events, "replay-input")
	recorder.requests = append(recorder.requests, request)
	return recordings.LoadReplayInputResult{Portable: &recorder.portable}, nil
}

var _ recordings.ReplayInputLoader = (*historicalReplayInputsRecorder)(nil)

func (stub *recordingsRootConstructionStub) LoadReplayInput(
	request recordings.LoadReplayInputRequest,
) (recordings.LoadReplayInputResult, error) {
	if stub.replayInputs == nil {
		return recordings.LoadReplayInputResult{}, nil
	}
	return stub.replayInputs.LoadReplayInput(request)
}

var _ recordings.Service = (*recordingsRootConstructionStub)(nil)
var _ recordings.RuntimeScopeService = (*recordingsRootConstructionStub)(nil)

type selectedOpeningMaterializer struct {
	calls int
	input string
}

func (materializer *selectedOpeningMaterializer) MaterializeContentURL(_ context.Context, rawURL string) (string, work.ContentCleanup, error) {
	materializer.calls++
	materializer.input = rawURL
	return "/tmp/identity.png", func() {}, nil
}

// durableOpeningFixture controls the configuration and acquisition effects of
// the fixed opening owner used by live and replay component fixtures.
func durableOpeningFixture(execution durableexecution.Service, acquire durableexecution.ScopeAcquisition) *DurableOpening {
	return NewDurableOpening(
		func(string) (operatorsettings.Config, error) { return operatorsettings.Config{}, nil },
		func(ctx context.Context, facts durableexecution.ScopeFacts, clock factoryruntime.Clock, logger *zap.Logger) (durableexecution.Service, func(context.Context) error, error) {
			owned, release, err := acquire(ctx, facts, clock, logger)
			if owned == nil && err == nil {
				owned = execution
			}
			return owned, release, err
		},
		func(identity string) (string, error) { return identity, nil }, false,
	)
}

func (fixture runtimeOpeningFixture) snapshotSelection() *RuntimeSnapshotSelection {
	var resolve factorydefinitions.RuntimeSnapshotOperation
	if fixture.Definitions != nil {
		resolve = fixture.Definitions.ResolveRuntimeSnapshot
	}
	var paths factorydefinitions.CurrentFactoryDirectoryResolver
	if fixture.NamedPaths != nil {
		paths = fixture.NamedPaths.ResolveCurrentDir
	}
	return NewRuntimeSnapshotSelection(resolve, fixture.DecodeReplayConfig, fixture.RecordingsRuntime, paths, fixture.ResolveHome)
}
