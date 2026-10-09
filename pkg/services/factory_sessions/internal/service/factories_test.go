package service

import (
	"context"
	"errors"
	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/recordingreplay"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
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
	_, err := NewExecutionBinding(nil, nil).bindWorkerScope("session-42", struct{}{}, nil, "runtime-1", "generation-1", nil, nil, nil)
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
	WorkflowPreview              factoryruntime.WorkflowPreviewOperation
	RuntimeRoot                  FactoryRuntimeRoot
	ResolveClock                 factoryruntime.ClockResolver
	NewSessionLogger             factoryruntime.SessionLoggerFactory
	Clock                        factoryruntime.Clock
	ProviderOverride             ProviderOverrideService
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
	ProcessRuntimeFactory        roles.ProcessRuntimeFactory
	GenerateSessionID            factorysessions.SessionIDGenerator
	GenerateRuntimeInstanceID    factorysessions.RuntimeInstanceIDGenerator
	ResolveHome                  factorysessions.HomeDirectoryResolver
	ProviderIdentities           factorysessions.ProviderIdentityResolver
	WorkService                  work.Service
	WebhooksService              webhooks.Service
	ModelService                 models.Service
	RecordingsService            recordings.Service
	RecordingsRuntime            recordings.RuntimeScopeService
	WorkerService                workers.Service
	ProviderCommandRunner        ProviderCommandRunner
	EnsureBackendScope           operatorsettings.BackendScopeEnsurer
	InitialActivation            factoryruntime.InitialRuntimeActivationOperation
}

func (fixture runtimeOpeningFixture) newFactory() (*Root, error) {
	initialEngine := NewRuntimeInitialEngine(fixture.snapshotSelection().Resolve, fixture.InitialActivation)
	inventory, _ := fixture.Assembly.(recordings.RecordedSessionInventory)
	return NewRoot(
		NewRuntimeOpening(fixture.preparation(), fixture.DurableOpening, initialEngine, NewExecutionBinding(fixture.ProviderOverride, fixture.ProviderCommandRunner), recordingreplay.NewBehavior(), fixture.RecordingsService, fixture.RecordingsRuntime, fixture.Clock, fixture.ResolveClock, fixture.ProviderOverride, fixture.GenerateRuntimeInstanceID, nil, fixture.GenerateSessionID, inventory, fixture.resourceAcquisition(),
			NewRuntimeOpeningCompletion(fixture.Assembly, fixture.RuntimeRouter, fixture.WebhooksService, fixture.ProcessRuntimeFactory),
			NewRuntimeOpeningBinding(nil, fixture.RecordingsService, fixture.ProviderOverride, fixture.ProviderCommandRunner)),
		fixture.ProviderSessions,
		fixture.Logger,
		fixture.WorkflowPreview,
		fixture.RuntimeRoot,
		fixture.Definitions,
		fixture.snapshotSelection(),
		fixture.Assembly,
		fixture.GenerateSessionID,
		fixture.GenerateRuntimeInstanceID,
		fixture.ResolveHome,
		fixture.WorkService,
		fixture.ModelService,
		fixture.RecordingsService,
		fixture.RecordingsRuntime,
		fixture.WorkerService,
		nil,
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
		ProcessRuntimeFactory:        processRuntimeFactoryConstructionStub{},
		GenerateSessionID:            inertRuntimeOpeningFunction[factorysessions.SessionIDGenerator](calls),
		GenerateRuntimeInstanceID:    inertRuntimeOpeningFunction[factorysessions.RuntimeInstanceIDGenerator](calls),
		ResolveHome:                  inertRuntimeOpeningFunction[factorysessions.HomeDirectoryResolver](calls),
		ProviderIdentities:           inertRuntimeOpeningFunction[factorysessions.ProviderIdentityResolver](calls),
		WorkService:                  work.MaterializationService(constructionMaterializer{calls: calls}),
		ModelService:                 &modelsConstructionStub{},
		RecordingsService:            &recordingsRootConstructionStub{},
		RecordingsRuntime:            &recordingsRootConstructionStub{},
		RecordingProjections:         &openingCoordinatorProjection{},
		WebhooksService:              webhooksConstructionStub{},
		WorkerService:                &workersConstructionStub{},
		ProviderCommandRunner:        workersRootBindingProbeRunner{tag: "provider"},
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

func (fixture runtimeOpeningFixture) preparation() *RuntimePreparation {
	var resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver
	if fixture.NamedPaths != nil {
		resolveCurrentDir = fixture.NamedPaths.ResolveCurrentDir
	}
	var replayClock func(*factorydefinitions.ReplayArtifact) recordings.Clock
	if fixture.RecordingsRuntime != nil {
		replayClock = fixture.RecordingsRuntime.ReplayClock
	}
	loading := NewRuntimeInputLoading(fixture.LoadFactory, fixture.NewLoadedFactory, fixture.DecodeReplayConfig, fixture.RecordingsRuntime, fixture.CaptureLoadedFactorySnapshot, fixture.NewSessionLogger, fixture.Logger)
	return NewRuntimePreparation(loading.Load, resolveCurrentDir, fixture.GenerateRuntimeInstanceID,
		fixture.ResolveHome, fixture.EnsureBackendScope, fixture.ProviderIdentities, fixture.Validator,
		replayClock, fixture.ResolveClock)
}

// Preparation tests control every independently owned collaborator. No real
// definition loader, validator, recording, filesystem or application participates.
type preparationSource struct {
	factorydefinitions.MutableLoadedFactorySource
	config *factorydefinitions.FactoryConfig
}

func (source preparationSource) FactoryConfig() *factorydefinitions.FactoryConfig {
	return source.config
}
func (source preparationSource) MutateWorkers(mutate func(*factorydefinitions.FactoryWorkerConfig) error) error {
	for i := range source.config.Workers {
		if err := mutate(&source.config.Workers[i]); err != nil {
			return err
		}
	}
	return nil
}

type preparationValidator struct {
	factorydefinitions.Validator
	validate func(*factorydefinitions.FactoryConfig) factorydefinitions.ValidationResult
}

func (validator preparationValidator) ValidateBlockingLoad(_ context.Context, config *factorydefinitions.FactoryConfig) factorydefinitions.ValidationResult {
	return validator.validate(config)
}

func TestRuntimePreparationRetainsSelectedFactsAndOrder(t *testing.T) {
	t.Parallel()
	var order []string
	var inputs []RuntimeInputLoadRequest
	logger := zap.NewNop()
	selectedClock := clockwork.NewFakeClock()
	preparation := NewRuntimePreparation(func(request RuntimeInputLoadRequest) (RuntimeLoad, error) {
		order = append(order, "load")
		inputs = append(inputs, request)
		return RuntimeLoad{LoadedFactoryCfg: preparationSource{config: &factorydefinitions.FactoryConfig{
			Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "selected", Type: "MODEL_WORKER", ModelProvider: "alias"}},
		}}}, nil
	}, func(string) (string, error) { t.Fatal("explicit source consulted Current Factory"); return "", nil },
		func() string { t.Fatal("explicit runtime identity was replaced"); return "" },
		func() (string, error) { t.Fatal("absolute selection consulted home"); return "", nil },
		func(path string) (operatorsettings.ResolvedBackendScope, error) {
			order = append(order, "scope")
			return operatorsettings.ResolvedBackendScope{BackendScopeID: path}, nil
		}, func(string) (string, error) { order = append(order, "provider"); return "codex", nil },
		preparationValidator{validate: func(config *factorydefinitions.FactoryConfig) factorydefinitions.ValidationResult {
			order = append(order, "validate")
			if config.Workers[0].ModelProvider != "codex" {
				t.Fatal("definition validated before concrete provider resolution")
			}
			return factorydefinitions.ValidationResult{}
		}}, func(*factorydefinitions.ReplayArtifact) recordings.Clock {
			t.Fatal("live selection consulted replay clock")
			return nil
		},
		func(factoryruntime.Clock) factoryruntime.Clock {
			t.Fatal("live selection consulted fallback clock")
			return nil
		})
	for _, id := range []string{"first", "peer"} {
		order = nil
		definition := factorydefinitions.RuntimeSelection{Directory: preparationPath(id, "root"), SourcePath: preparationPath(id, "source"), ExecutionBaseDir: preparationPath(id, "base")}
		snapshot := &factorydefinitions.RuntimeSnapshot{FactoryDir: definition.SourcePath}
		replay := &recordings.LoadReplayInputResult{}
		session := factorysessions.SessionStartRequest{SessionID: id, RuntimeSelection: &factorysessions.SessionRuntimeSelection{SystemConfigPath: id}}
		worker := workers.RuntimeSelection{WorkerReasoningEffort: id}
		defaults := operatorsettings.ResolvedDefaults{WorkerModel: id}
		prepared, root, load, clock, gotLogger, err := preparation.Prepare(context.Background(), definition,
			factoryruntime.RuntimeSelection{RuntimeInstanceID: id}, session, false, worker, recordings.RuntimeSelection{}, id, defaults, logger, selectedClock, snapshot, replay)
		if err != nil {
			t.Fatal(err)
		}

		wantPrepared := preparedRuntime{Definition: definition, Runtime: factoryruntime.RuntimeSelection{RuntimeInstanceID: id},
			Session: session, Workers: worker, Recordings: recordings.RuntimeSelection{}, ModelCacheDirectory: id,
			OperatorDefaults: defaults, DefinitionSnapshot: snapshot}
		wantRoot := RuntimeRoot{FactoryRootDir: filepath.Clean(definition.Directory), RuntimeInstanceID: id, BaseLogger: logger}
		if !reflect.DeepEqual(prepared, wantPrepared) || root != wantRoot || clock != selectedClock || gotLogger != logger || load.LoadedFactoryCfg == nil {
			t.Fatalf("selected facts/effects changed: %#v %#v", prepared, root)
		}
		wantInput := RuntimeInputLoadRequest{Dir: filepath.Clean(definition.SourcePath), ExecutionBaseDir: definition.ExecutionBaseDir,
			FactoryRootDir: definition.Directory, SessionID: id, OperatorDefaults: defaults,
			ResolvedSnapshot: snapshot, PreloadedReplayInput: replay, HistoricalInspection: true}
		input := inputs[len(inputs)-1]
		if !reflect.DeepEqual(input, wantInput) || snapshot.FactoryDir != definition.SourcePath {
			t.Fatalf("selected loader input changed: %#v", input)
		}

		if !reflect.DeepEqual(order, []string{"load", "scope", "provider", "provider", "validate"}) {
			t.Fatalf("preparation order = %v", order)
		}
	}
	if inputs[0].ResolvedSnapshot.FactoryDir == inputs[1].ResolvedSnapshot.FactoryDir || inputs[0].SessionID != "first" {
		t.Fatal("peer preparation replaced selected facts")
	}
}

func TestRuntimePreparationFailureRetainsCauseAndZeroResults(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"path", "load", "scope", "provider", "validation"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			cause := errors.New("controlled preparation failure")
			preparation := NewRuntimePreparation(func(RuntimeInputLoadRequest) (RuntimeLoad, error) {
				if stage == "load" {
					return RuntimeLoad{SessionLogger: zap.NewNop()}, cause
				}
				return RuntimeLoad{LoadedFactoryCfg: preparationSource{config: &factorydefinitions.FactoryConfig{
					Workers: []factorydefinitions.FactoryWorkerConfig{{Type: "MODEL_WORKER", ModelProvider: "alias"}},
				}}}, nil
			}, func(dir string) (string, error) {
				if stage == "path" {
					return "", cause
				}
				return dir, nil
			},
				func() string { return "selected-runtime" }, func() (string, error) { return preparationPath("home"), nil },
				func(string) (operatorsettings.ResolvedBackendScope, error) {
					if stage == "scope" {
						return operatorsettings.ResolvedBackendScope{}, cause
					}
					return operatorsettings.ResolvedBackendScope{BackendScopeID: "selected"}, nil
				},
				func(string) (string, error) {
					if stage == "provider" {
						return "", cause
					}
					return "codex", nil
				},
				preparationValidator{validate: func(*factorydefinitions.FactoryConfig) factorydefinitions.ValidationResult {
					if stage == "validation" {
						return factorydefinitions.ValidationResult{Targets: []factorydefinitions.ValidationTarget{{Code: "factory.invalid", Severity: factorydefinitions.ValidationSeverityError}}}
					}
					t.Fatal("failure reached definition validation")
					return factorydefinitions.ValidationResult{}
				}}, nil, nil)
			session := factorysessions.SessionStartRequest{RuntimeSelection: &factorysessions.SessionRuntimeSelection{SystemConfigPath: "selected"}}
			prepared, root, load, clock, logger, err := preparation.Prepare(context.Background(), factorydefinitions.RuntimeSelection{Directory: preparationPath("root")},
				factoryruntime.RuntimeSelection{}, session, false, workers.RuntimeSelection{}, recordings.RuntimeSelection{}, "", operatorsettings.ResolvedDefaults{}, zap.NewNop(), clockwork.NewFakeClock(), nil, nil)
			if stage == "validation" {
				var blocking *factorydefinitions.BlockingFactoryLoadError
				if !errors.As(err, &blocking) {
					t.Fatalf("validation lost typed failure: %v", err)
				}
			} else if !errors.Is(err, cause) {
				t.Fatalf("%s lost cause: %v", stage, err)
			}
			if !reflect.DeepEqual(prepared, preparedRuntime{}) || root != (RuntimeRoot{}) || !reflect.DeepEqual(load, RuntimeLoad{}) || clock != nil || logger != nil {
				t.Fatal("failed preparation exposed partially successful facts")
			}
		})
	}
}

func TestRuntimePreparationHistoricalInspectionSkipsLiveEffects(t *testing.T) {
	t.Parallel()
	logger := zap.NewNop()
	historical := &recordingreplay.RecordingReplayProjection{}
	preparation := NewRuntimePreparation(func(request RuntimeInputLoadRequest) (RuntimeLoad, error) {
		if request.Dir != preparationPath("root") || !request.HistoricalInspection {
			t.Fatalf("historical request = %#v", request)
		}
		return RuntimeLoad{HistoricalReplay: historical, SessionLogger: logger}, nil
	}, func(string) (string, error) { t.Fatal("replay consulted Current Factory"); return "", nil }, nil, func() (string, error) { return preparationPath("home"), nil },
		func(string) (operatorsettings.ResolvedBackendScope, error) {
			t.Fatal("historical inspection resolved live backend scope")
			return operatorsettings.ResolvedBackendScope{}, nil
		},
		func(string) (string, error) { t.Fatal("historical inspection resolved live provider"); return "", nil },
		preparationValidator{validate: func(*factorydefinitions.FactoryConfig) factorydefinitions.ValidationResult {
			t.Fatal("historical inspection validated live definition")
			return factorydefinitions.ValidationResult{}
		}},
		func(*factorydefinitions.ReplayArtifact) recordings.Clock {
			t.Fatal("historical inspection selected live clock")
			return nil
		}, nil)
	_, _, load, clock, gotLogger, err := preparation.Prepare(context.Background(), factorydefinitions.RuntimeSelection{Directory: preparationPath("root")},
		factoryruntime.RuntimeSelection{RuntimeInstanceID: "historical"}, factorysessions.SessionStartRequest{}, false, workers.RuntimeSelection{}, recordings.RuntimeSelection{ReplayPath: "selected.json"}, "", operatorsettings.ResolvedDefaults{}, zap.NewNop(), nil, nil, nil)
	if err != nil || load.HistoricalReplay != historical || clock != nil || gotLogger != logger {
		t.Fatalf("historical effects = %#v %v %v", load, clock, err)
	}
}

func preparationPath(parts ...string) string {
	root := string(filepath.Separator)
	if filepath.Separator == '\\' {
		root = "C:\\"
	}
	return filepath.Join(append([]string{root}, parts...)...)
}

func TestRuntimeOpeningPreparationFailureDoesNotAllocateCanonicalMetricsIdentity(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"selected", "peer"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			cause := errors.New(id + " controlled input loading failure")
			identityRequests, loadRequests := 0, 0
			preparation := NewRuntimePreparation(func(RuntimeInputLoadRequest) (RuntimeLoad, error) {
				loadRequests++
				return RuntimeLoad{}, cause
			}, func(dir string) (string, error) { return dir, nil }, nil,
				func() (string, error) { return preparationPath("home"), nil }, nil, nil, nil, nil, nil)
			operation := NewRuntimeOpening(preparation, nil, nil, nil, nil,
				&recordingsRootConstructionStub{}, &recordingsRootConstructionStub{}, nil, nil, nil,
				func() string { identityRequests++; return id + "-metrics-identity" }, nil, nil, nil, nil, nil, nil)
			if loadRequests != 0 || identityRequests != 0 {
				t.Fatal("construction performed request-scoped work")
			}
			session := &factorysessions.SessionStartRequest{RuntimeSelection: &factorysessions.SessionRuntimeSelection{}}
			lifecycle, replay, closeArtifacts, activation, selectedRuntime, err := operation.openRuntimeWithOptions(context.Background(),
				factorydefinitions.RuntimeSelection{Directory: preparationPath("root")},
				factoryruntime.RuntimeSelection{RuntimeInstanceID: id + "-already-selected"}, session, false,
				workers.RuntimeSelection{}, recordings.RuntimeSelection{}, "", operatorsettings.ResolvedDefaults{}, zap.NewNop(), nil, nil)
			if !errors.Is(err, cause) || lifecycle != nil || replay != nil || closeArtifacts != nil || activation != nil || selectedRuntime != nil || loadRequests != 1 || identityRequests != 0 || session.RuntimeSelection.CanonicalSessionID != "" {
				t.Fatalf("failed opening: lifecycle=%v replay=%v cleanup=%v activation=%v runtime=%v error=%v loads=%d allocations=%d", lifecycle, replay, closeArtifacts != nil, activation, selectedRuntime, err, loadRequests, identityRequests)
			}
		})
	}
}

func TestRuntimePreparationRejectsInvalidSelectionsBeforeLoading(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"home", "empty runtime identity", "conflicting recording"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			cause := errors.New("controlled home failure")
			definition := factorydefinitions.RuntimeSelection{Directory: preparationPath("root")}
			recording := recordings.RuntimeSelection{}
			if stage == "home" {
				definition.Directory = "~/root"
			}
			if stage == "conflicting recording" {
				recording.RecordPath, recording.ReplayPath = "selected.json", "selected.json"
			}
			preparation := NewRuntimePreparation(func(RuntimeInputLoadRequest) (RuntimeLoad, error) {
				t.Fatal("invalid selection reached loader")
				return RuntimeLoad{}, nil
			},
				nil, func() string { return "" }, func() (string, error) { return "", cause }, nil, nil, nil, nil, nil)
			_, _, _, _, _, err := preparation.Prepare(context.Background(), definition, factoryruntime.RuntimeSelection{},
				factorysessions.SessionStartRequest{}, false, workers.RuntimeSelection{}, recording, "", operatorsettings.ResolvedDefaults{}, zap.NewNop(), nil, nil, nil)
			if err == nil {
				t.Fatal("invalid selection succeeded")
			}
			if stage == "home" && !errors.Is(err, cause) {
				t.Fatalf("home failure lost cause: %v", err)
			}
		})
	}
}

func (fixture runtimeOpeningFixture) resourceAcquisition() *RuntimeResourceAcquisition {
	var open DurableResourceOpening
	if fixture.DurableOpening != nil {
		open = fixture.DurableOpening.Open
	}
	return NewRuntimeResourceAcquisition(open, fixture.ModelService, fixture.ProviderOverride)
}
