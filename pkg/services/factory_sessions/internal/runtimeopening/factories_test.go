package runtimeopening

import (
	"bytes"
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

func TestWorkFactoryProjectsMaterializationServiceFromContentMaterializer(t *testing.T) {
	t.Parallel()

	var factory WorkFactory = func(resolver work.RuntimeResolver) work.Service {
		if resolver != nil {
			t.Fatalf("WorkFactory resolver = %#v, want nil for materialization projection", resolver)
		}
		return work.MaterializationService(materializerStub{})
	}

	service := factory(nil)
	if service == nil {
		t.Fatal("WorkFactory() = nil, want materialization service")
	}
	path, cleanup, err := service.MaterializeContentURL(t.Context(), "file:///fixtures/runtimeopening.png")
	if err != nil || path != "/tmp/runtimeopening.png" || cleanup == nil {
		t.Fatalf("MaterializeContentURL = (%q, %v, %v)", path, cleanup, err)
	}
	cleanup()
}

type materializerStub struct{}

func (materializerStub) MaterializeContentURL(_ context.Context, rawURL string) (string, work.ContentCleanup, error) {
	return "/tmp/runtimeopening.png", func() {}, nil
}

func TestNewFactoryRejectsEveryMissingRuntimeOpeningGroup(t *testing.T) {
	t.Parallel()

	tests := []runtimeOpeningDependencyOmission{
		{"Provider Sessions group", func(dependencies *Dependencies) { dependencies.ProviderSessions = nil }},
		{"Factory Runtime group", func(dependencies *Dependencies) { dependencies.FactoryRuntime = nil }},
		{"Factory Definitions group", func(dependencies *Dependencies) { dependencies.FactoryDefinitions = nil }},
		{"Factory Sessions group", func(dependencies *Dependencies) { dependencies.FactorySessions = nil }},
		{"Work group", func(dependencies *Dependencies) { dependencies.Work = nil }},
		{"Automations group", func(dependencies *Dependencies) { dependencies.Automations = nil }},
		{"Models group", func(dependencies *Dependencies) { dependencies.Models = nil }},
		{"Recordings group", func(dependencies *Dependencies) { dependencies.Recordings = nil }},
		{"Workers group", func(dependencies *Dependencies) { dependencies.Workers = nil }},
		{"Operator Settings group", func(dependencies *Dependencies) { dependencies.OperatorSettings = nil }},
	}

	for _, test := range tests {
		t.Run(test.requirement, func(t *testing.T) {
			calls := 0
			dependencies := validRuntimeOpeningDependencies(&calls)
			test.omit(&dependencies)

			factory, err := NewFactory(dependencies)
			if factory != nil {
				t.Fatalf("NewFactory() = %#v, want nil factory", factory)
			}
			if err == nil || !strings.Contains(err.Error(), test.requirement) {
				t.Fatalf("NewFactory() error = %v, want actionable %q error", err, test.requirement)
			}
			if calls != 0 {
				t.Fatalf("NewFactory() invoked %d collaborator functions, want inert failure", calls)
			}
		})
	}
}

func TestNewFactoryRejectsEveryMissingRuntimeOpeningMember(t *testing.T) {
	t.Parallel()

	for _, test := range runtimeOpeningMemberOmissions() {
		t.Run(test.requirement, func(t *testing.T) {
			calls := 0
			dependencies := validRuntimeOpeningDependencies(&calls)
			test.omit(&dependencies)

			factory, err := NewFactory(dependencies)
			if factory != nil {
				t.Fatalf("NewFactory() = %#v, want nil factory", factory)
			}
			if err == nil || !strings.Contains(err.Error(), test.requirement) {
				t.Fatalf("NewFactory() error = %v, want actionable %q error", err, test.requirement)
			}
			if calls != 0 {
				t.Fatalf("NewFactory() invoked %d collaborator functions, want inert failure", calls)
			}
		})
	}
}

func TestNewFactoryUsesStableFirstMissingRequirementAndRemainsInert(t *testing.T) {
	t.Parallel()

	calls := 0
	dependencies := validRuntimeOpeningDependencies(&calls)
	dependencies.ProviderSessions = nil
	dependencies.FactoryRuntime = nil

	factory, err := NewFactory(dependencies)
	if factory != nil {
		t.Fatalf("NewFactory() = %#v, want nil factory", factory)
	}
	if got, want := err.Error(), "Factory Sessions runtime-opening Provider Sessions group is required"; got != want {
		t.Fatalf("NewFactory() error = %q, want first stable requirement %q", got, want)
	}
	if calls != 0 {
		t.Fatalf("NewFactory() invoked %d collaborator functions, want inert failure", calls)
	}
}

func TestNewFactoryConstructsInertFactoryWithExactModelsRoot(t *testing.T) {
	t.Parallel()

	calls := 0
	dependencies := validRuntimeOpeningDependencies(&calls)
	wantModels := dependencies.Models.Service

	factory, err := NewFactory(dependencies)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	if factory == nil {
		t.Fatal("NewFactory() = nil, want factory")
	}
	if got := factory.ModelsRoot(); got != wantModels {
		t.Fatalf("ModelsRoot() = %T, want exact injected Models root %T", got, wantModels)
	}
	if calls != 0 {
		t.Fatalf("NewFactory() invoked %d collaborator functions, want inert construction", calls)
	}
}

func TestNewFactoryRetainsExactGroupedCollaborators(t *testing.T) {
	t.Parallel()

	calls := 0
	dependencies := validRuntimeOpeningDependencies(&calls)
	materializer := &identityConstructionMaterializer{}
	dependencies.Work.ContentMaterializer = materializer

	factory, err := NewFactory(dependencies)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	if factory == nil {
		t.Fatal("NewFactory() = nil, want factory")
	}

	assertProviderSessionsDependenciesRetained(t, factory, dependencies)
	assertFactoryRuntimeDependenciesRetained(t, factory, dependencies)
	assertFactoryDefinitionsDependenciesRetained(t, factory, dependencies)
	assertFactorySessionsDependenciesRetained(t, factory, dependencies)
	assertWorkDependenciesRetained(t, factory, dependencies, materializer)
	assertAutomationsDependenciesRetained(t, factory, dependencies)
	assertModelsDependenciesRetained(t, factory, dependencies)
	assertRecordingsDependenciesRetained(t, factory, dependencies)
	assertWorkersDependenciesRetained(t, factory, dependencies)
	assertOperatorSettingsDependenciesRetained(t, factory, dependencies)
	if calls != 0 {
		t.Fatalf("NewFactory() invoked %d collaborator functions, want inert construction", calls)
	}
}

func TestNewFactoryOpensHistoricalReplayWithoutLiveRuntimeCollaborators(t *testing.T) {
	t.Parallel()

	portable, err := recordings.DecodePortableRecording(bytes.NewReader(runtimeLoadPortablePayload(t, nil)))
	if err != nil {
		t.Fatalf("decode portable recording: %v", err)
	}
	var events []string
	calls := 0
	dependencies := validRuntimeOpeningDependencies(&calls)
	replayInputs := &historicalReplayInputsRecorder{portable: portable, events: &events}
	dependencies.Recordings.ReplayInputs = replayInputs
	dependencies.FactorySessions.GenerateRuntimeInstanceID = func() string {
		events = append(events, "runtime-instance-id")
		return "historical-runtime"
	}
	dependencies.FactoryRuntime.NewSessionLogger = func(*zap.Logger, string, string, string) *zap.Logger {
		events = append(events, "session-logger")
		return zap.NewNop()
	}

	factory, err := NewFactory(dependencies)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	opened, err := factory.OpenApplicationRuntime(
		t.Context(),
		&factorysessions.RuntimeOpeningRequest{
			FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{Directory: t.TempDir()},
			Recordings:        recordings.RuntimeOpeningRequest{ReplayPath: "recording.json"},
		},
		ExternalEffects{},
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("OpenApplicationRuntime() error = %v", err)
	}
	if opened.HistoricalReplay == nil {
		t.Fatal("OpenApplicationRuntime() HistoricalReplay = nil, want inspection-only replay")
	}
	if opened.Process == nil {
		t.Fatal("OpenApplicationRuntime() Process = nil, want historical replay lifecycle")
	}
	if !slices.Equal(events, []string{"runtime-instance-id", "session-logger", "replay-input"}) {
		t.Fatalf("historical replay opening events = %v, want no live runtime collaborators", events)
	}
	if len(replayInputs.requests) != 1 || replayInputs.requests[0].Path != "recording.json" {
		t.Fatalf("replay input requests = %#v, want recording.json once", replayInputs.requests)
	}
	if calls != 0 {
		t.Fatalf("historical replay invoked %d live runtime collaborator functions, want none", calls)
	}
}

func assertProviderSessionsDependenciesRetained(t *testing.T, factory *Factory, dependencies Dependencies) {
	t.Helper()
	assertRuntimeOpeningDependencyIdentity(t, "Provider Sessions service", factory.providerSessions, dependencies.ProviderSessions.Service)
}

func assertFactoryRuntimeDependenciesRetained(t *testing.T, factory *Factory, dependencies Dependencies) {
	t.Helper()
	group := dependencies.FactoryRuntime
	assertRuntimeOpeningDependencyIdentity(t, "Factory Runtime workflows", factory.factoryWorkflows, group.FactoryWorkflows)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Runtime preview", factory.workflowPreview, group.WorkflowPreview)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Runtime executors", factory.workersRuntimeExecutorsFactory, group.WorkersRuntimeExecutorsFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Runtime mock runner", factory.workersMockCommandRunnerFactory, group.WorkersMockCommandRunnerFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Runtime assembler", factory.factoryRuntimeAssembler, group.FactoryRuntimeAssembler)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Runtime clock", factory.resolveClock, group.ResolveClock)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Runtime logger", factory.newSessionLogger, group.NewSessionLogger)
}

func assertFactoryDefinitionsDependenciesRetained(t *testing.T, factory *Factory, dependencies Dependencies) {
	t.Helper()
	group := dependencies.FactoryDefinitions
	assertRuntimeOpeningDependencyIdentity(t, "Factory Definitions validator", factory.factoryDefinitionValidator, group.Validator)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Definitions paths", factory.namedPaths, group.NamedPaths)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Definitions factory", factory.factoryDefinitionsFactory, group.Factory)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Definitions initial snapshot", factory.initialFactorySnapshotFactory, group.InitialFactorySnapshotFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Definitions loader", factory.loadFactory, group.LoadFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Definitions source", factory.newLoadedFactory, group.NewLoadedFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Definitions replay decoder", factory.decodeReplayConfig, group.DecodeReplayConfig)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Definitions snapshot capturer", factory.captureLoadedFactorySnapshot, group.CaptureLoadedFactorySnapshot)
}

func assertFactorySessionsDependenciesRetained(t *testing.T, factory *Factory, dependencies Dependencies) {
	t.Helper()
	group := dependencies.FactorySessions
	assertRuntimeOpeningDependencyIdentity(t, "Factory Sessions service", factory.factorySessionsService, group.Service)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Sessions durable execution", factory.durableExecutionFactory, group.DurableExecutionFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Sessions execution", factory.factorySessionExecutionFactory, group.FactorySessionExecutionFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Sessions scaffold", factory.factoryScaffoldInitializer, group.FactoryScaffoldInitializer)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Sessions editable validation", factory.editableFactoryValidator, group.EditableFactoryValidator)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Sessions process runtime", factory.processRuntimeFactory, group.ProcessRuntimeFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Sessions runtime ID", factory.generateRuntimeInstanceID, group.GenerateRuntimeInstanceID)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Sessions home", factory.resolveHome, group.ResolveHome)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Sessions provider identities", factory.providerIdentities, group.ProviderIdentities)
}

func assertWorkDependenciesRetained(
	t *testing.T,
	factory *Factory,
	dependencies Dependencies,
	materializer *identityConstructionMaterializer,
) {
	t.Helper()
	assertRuntimeOpeningDependencyIdentity(t, "Work factory", factory.workFactory, dependencies.Work.Factory)
	path, cleanup, err := factory.workService.MaterializeContentURL(t.Context(), "file:///identity.png")
	if err != nil || path != "/tmp/identity.png" || cleanup == nil {
		t.Fatalf("Work materialization = (%q, %v, %v), want exact injected materializer", path, cleanup, err)
	}
	cleanup()
	if materializer.calls != 1 || materializer.input != "file:///identity.png" {
		t.Fatalf("Work materializer calls = %d with %q, want injected instance once", materializer.calls, materializer.input)
	}
}

func assertAutomationsDependenciesRetained(t *testing.T, factory *Factory, dependencies Dependencies) {
	t.Helper()
	group := dependencies.Automations
	assertRuntimeOpeningDependencyIdentity(t, "Automations factory", factory.automationFactory, group.Factory)
	assertRuntimeOpeningDependencyIdentity(t, "Automations hosted sources", factory.automationHostedSourcesFactory, group.HostedSourcesFactory)
}

func assertModelsDependenciesRetained(t *testing.T, factory *Factory, dependencies Dependencies) {
	t.Helper()
	assertRuntimeOpeningDependencyIdentity(t, "Models service", factory.modelService, dependencies.Models.Service)
}

func assertRecordingsDependenciesRetained(t *testing.T, factory *Factory, dependencies Dependencies) {
	t.Helper()
	group := dependencies.Recordings
	assertRuntimeOpeningDependencyIdentity(t, "Recordings projections", factory.recordingsProjectionFactory, group.ProjectionFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Recordings lifecycle", factory.recordingLifecycleFactory, group.LifecycleFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Recordings ledger", factory.runtimeLedgerFactory, group.RuntimeLedgerFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Recordings recorder", factory.runtimeRecorderFactory, group.RuntimeRecorderFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Recordings replay clock", factory.replayClockFactory, group.ReplayClockFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Recordings replay execution", factory.replayExecutionFactory, group.ReplayExecutionFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Recordings replay inputs", factory.replayInputs, group.ReplayInputs)
}

func assertWorkersDependenciesRetained(t *testing.T, factory *Factory, dependencies Dependencies) {
	t.Helper()
	group := dependencies.Workers
	assertRuntimeOpeningDependencyIdentity(t, "Workers execution", factory.workerExecutionFactory, group.ExecutionFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Workers runtime", factory.workersRuntimeFactory, group.RuntimeFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Workers hooks", factory.workersLocalRuntimeHooksFactory, group.LocalRuntimeHooksFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Workers command adapter", factory.adaptWorkerCommandRunner, group.AdaptCommandRunner)
	assertRuntimeOpeningDependencyIdentity(t, "Workers provider adapter", factory.providerFromCommandRunnerFactory, group.ProviderFromCommandRunnerFactory)
}

func assertOperatorSettingsDependenciesRetained(t *testing.T, factory *Factory, dependencies Dependencies) {
	t.Helper()
	assertRuntimeOpeningDependencyIdentity(
		t,
		"Operator Settings backend scope",
		factory.ensureOperatorBackendScope,
		dependencies.OperatorSettings.EnsureBackendScope,
	)
}

func assertRuntimeOpeningDependencyIdentity(t *testing.T, name string, got, want any) {
	t.Helper()
	gotValue := reflect.ValueOf(got)
	wantValue := reflect.ValueOf(want)
	if gotValue.Type() != wantValue.Type() {
		t.Fatalf("%s type = %v, want %v", name, gotValue.Type(), wantValue.Type())
	}
	if gotValue.Kind() == reflect.Func {
		if gotValue.Pointer() != wantValue.Pointer() {
			t.Fatalf("%s function identity changed", name)
		}
		return
	}
	if got != want {
		t.Fatalf("%s = %T(%[2]v), want exact injected %T(%[3]v)", name, got, want)
	}
}

type runtimeOpeningDependencyOmission struct {
	requirement string
	omit        func(*Dependencies)
}

func runtimeOpeningMemberOmissions() []runtimeOpeningDependencyOmission {
	return []runtimeOpeningDependencyOmission{
		{"Provider Sessions service", func(d *Dependencies) { d.ProviderSessions.Service = nil }},
		{"Factory Runtime JavaScript workflow definitions", func(d *Dependencies) { d.FactoryRuntime.FactoryWorkflows = nil }},
		{"Factory Runtime workflow preview operation", func(d *Dependencies) { d.FactoryRuntime.WorkflowPreview = nil }},
		{"Factory Runtime Workers runtime executors factory", func(d *Dependencies) { d.FactoryRuntime.WorkersRuntimeExecutorsFactory = nil }},
		{"Factory Runtime Workers mock command runner factory", func(d *Dependencies) { d.FactoryRuntime.WorkersMockCommandRunnerFactory = nil }},
		{"Factory Runtime runtime assembler", func(d *Dependencies) { d.FactoryRuntime.FactoryRuntimeAssembler = nil }},
		{"Factory Runtime clock resolver", func(d *Dependencies) { d.FactoryRuntime.ResolveClock = nil }},
		{"Factory Runtime session logger factory", func(d *Dependencies) { d.FactoryRuntime.NewSessionLogger = nil }},
		{"Factory Definitions validator", func(d *Dependencies) { d.FactoryDefinitions.Validator = nil }},
		{"Factory Definitions named path resolver", func(d *Dependencies) { d.FactoryDefinitions.NamedPaths = nil }},
		{"Factory Definitions factory", func(d *Dependencies) { d.FactoryDefinitions.Factory = nil }},
		{"Factory Definitions initial factory snapshot factory", func(d *Dependencies) { d.FactoryDefinitions.InitialFactorySnapshotFactory = nil }},
		{"Factory Definitions loaded factory loader", func(d *Dependencies) { d.FactoryDefinitions.LoadFactory = nil }},
		{"Factory Definitions loaded factory source factory", func(d *Dependencies) { d.FactoryDefinitions.NewLoadedFactory = nil }},
		{"Factory Definitions replay runtime config decoder", func(d *Dependencies) { d.FactoryDefinitions.DecodeReplayConfig = nil }},
		{"Factory Definitions loaded factory snapshot capturer", func(d *Dependencies) { d.FactoryDefinitions.CaptureLoadedFactorySnapshot = nil }},
		{"Factory Sessions service", func(d *Dependencies) { d.FactorySessions.Service = nil }},
		{"Factory Sessions durable execution factory", func(d *Dependencies) { d.FactorySessions.DurableExecutionFactory = nil }},
		{"Factory Sessions session execution factory", func(d *Dependencies) { d.FactorySessions.FactorySessionExecutionFactory = nil }},
		{"Factory Sessions factory scaffold initializer", func(d *Dependencies) { d.FactorySessions.FactoryScaffoldInitializer = nil }},
		{"Factory Sessions editable factory validator", func(d *Dependencies) { d.FactorySessions.EditableFactoryValidator = nil }},
		{"Factory Sessions process runtime factory", func(d *Dependencies) { d.FactorySessions.ProcessRuntimeFactory = nil }},
		{"Factory Sessions runtime instance ID generator", func(d *Dependencies) { d.FactorySessions.GenerateRuntimeInstanceID = nil }},
		{"Factory Sessions home directory resolver", func(d *Dependencies) { d.FactorySessions.ResolveHome = nil }},
		{"Factory Sessions provider identity resolver", func(d *Dependencies) { d.FactorySessions.ProviderIdentities = nil }},
		{"Work factory", func(d *Dependencies) { d.Work.Factory = nil }},
		{"Work content materializer", func(d *Dependencies) { d.Work.ContentMaterializer = nil }},
		{"Automations factory", func(d *Dependencies) { d.Automations.Factory = nil }},
		{"Automations hosted sources factory", func(d *Dependencies) { d.Automations.HostedSourcesFactory = nil }},
		{"Models service", func(d *Dependencies) { d.Models.Service = nil }},
		{"Recordings projection factory", func(d *Dependencies) { d.Recordings.ProjectionFactory = nil }},
		{"Recordings lifecycle factory", func(d *Dependencies) { d.Recordings.LifecycleFactory = nil }},
		{"Recordings runtime ledger factory", func(d *Dependencies) { d.Recordings.RuntimeLedgerFactory = nil }},
		{"Recordings runtime recorder factory", func(d *Dependencies) { d.Recordings.RuntimeRecorderFactory = nil }},
		{"Recordings replay clock factory", func(d *Dependencies) { d.Recordings.ReplayClockFactory = nil }},
		{"Recordings replay execution factory", func(d *Dependencies) { d.Recordings.ReplayExecutionFactory = nil }},
		{"Recordings replay input loader", func(d *Dependencies) { d.Recordings.ReplayInputs = nil }},
		{"Workers execution factory", func(d *Dependencies) { d.Workers.ExecutionFactory = nil }},
		{"Workers runtime factory", func(d *Dependencies) { d.Workers.RuntimeFactory = nil }},
		{"Workers local runtime hooks factory", func(d *Dependencies) { d.Workers.LocalRuntimeHooksFactory = nil }},
		{"Workers command runner adapter", func(d *Dependencies) { d.Workers.AdaptCommandRunner = nil }},
		{"Workers provider-from-command-runner factory", func(d *Dependencies) { d.Workers.ProviderFromCommandRunnerFactory = nil }},
		{"Operator Settings backend scope ensurer", func(d *Dependencies) { d.OperatorSettings.EnsureBackendScope = nil }},
	}
}

func validRuntimeOpeningDependencies(calls *int) Dependencies {
	return Dependencies{
		ProviderSessions: &ProviderSessionsDependencies{
			Service: providerSessionsConstructionStub{},
		},
		FactoryRuntime: &FactoryRuntimeDependencies{
			FactoryWorkflows:                workflowDefinitionsConstructionStub{},
			WorkflowPreview:                 workflowPreviewConstructionStub{},
			WorkersRuntimeExecutorsFactory:  inertRuntimeOpeningFunction[factoryruntime.WorkersRuntimeExecutorsFactory](calls),
			WorkersMockCommandRunnerFactory: inertRuntimeOpeningFunction[factoryruntime.WorkersMockCommandRunnerFactory](calls),
			FactoryRuntimeAssembler:         factoryRuntimeAssemblerConstructionStub{},
			ResolveClock:                    inertRuntimeOpeningFunction[factoryruntime.ClockResolver](calls),
			NewSessionLogger:                inertRuntimeOpeningFunction[factoryruntime.SessionLoggerFactory](calls),
		},
		FactoryDefinitions: &FactoryDefinitionsDependencies{
			Validator:                     validatorConstructionStub{},
			NamedPaths:                    namedPathsConstructionStub{},
			Factory:                       inertRuntimeOpeningFunction[FactoryDefinitionsFactory](calls),
			InitialFactorySnapshotFactory: inertRuntimeOpeningFunction[factorydefinitions.InitialFactorySnapshotFactory](calls),
			LoadFactory:                   inertRuntimeOpeningFunction[factorydefinitions.LoadedFactoryLoader](calls),
			NewLoadedFactory:              inertRuntimeOpeningFunction[factorydefinitions.LoadedFactorySourceFactory](calls),
			DecodeReplayConfig:            inertRuntimeOpeningFunction[factorydefinitions.ReplayRuntimeConfigDecoder](calls),
			CaptureLoadedFactorySnapshot:  inertRuntimeOpeningFunction[factorydefinitions.LoadedFactorySnapshotCapturer](calls),
		},
		FactorySessions: &FactorySessionsDependencies{
			Service:                        factorySessionsConstructionStub{},
			DurableExecutionFactory:        inertRuntimeOpeningFunction[DurableExecutionFactory](calls),
			FactorySessionExecutionFactory: inertRuntimeOpeningFunction[FactorySessionExecutionFactory](calls),
			FactoryScaffoldInitializer:     inertRuntimeOpeningFunction[factorysessions.FactoryScaffoldInitializer](calls),
			EditableFactoryValidator:       inertRuntimeOpeningFunction[factorysessions.EditableFactoryValidator](calls),
			ProcessRuntimeFactory:          processRuntimeFactoryConstructionStub{},
			GenerateRuntimeInstanceID:      inertRuntimeOpeningFunction[factorysessions.RuntimeInstanceIDGenerator](calls),
			ResolveHome:                    inertRuntimeOpeningFunction[factorysessions.HomeDirectoryResolver](calls),
			ProviderIdentities:             inertRuntimeOpeningFunction[factorysessions.ProviderIdentityResolver](calls),
		},
		Work: &WorkDependencies{
			Factory:             inertRuntimeOpeningFunction[WorkFactory](calls),
			ContentMaterializer: constructionMaterializer{calls: calls},
		},
		Automations: &AutomationsDependencies{
			Factory:              inertRuntimeOpeningFunction[AutomationFactory](calls),
			HostedSourcesFactory: inertRuntimeOpeningFunction[AutomationHostedSourcesFactory](calls),
		},
		Models: &ModelsDependencies{Service: &modelsConstructionStub{}},
		Recordings: &RecordingsDependencies{
			ProjectionFactory:      inertRuntimeOpeningFunction[RecordingsProjectionFactory](calls),
			LifecycleFactory:       inertRuntimeOpeningFunction[RecordingLifecycleFactory](calls),
			RuntimeLedgerFactory:   inertRuntimeOpeningFunction[RuntimeLedgerFactory](calls),
			RuntimeRecorderFactory: inertRuntimeOpeningFunction[recordings.RuntimeRecorderFactory](calls),
			ReplayClockFactory:     inertRuntimeOpeningFunction[ReplayClockFactory](calls),
			ReplayExecutionFactory: inertRuntimeOpeningFunction[recordings.ReplayExecutionFactory](calls),
			ReplayInputs:           replayInputsConstructionStub{},
		},
		Workers: &WorkersDependencies{
			ExecutionFactory:                 inertRuntimeOpeningFunction[WorkerExecutionFactory](calls),
			RuntimeFactory:                   inertRuntimeOpeningFunction[WorkersRuntimeFactory](calls),
			LocalRuntimeHooksFactory:         inertRuntimeOpeningFunction[WorkersLocalRuntimeHooksFactory](calls),
			AdaptCommandRunner:               inertRuntimeOpeningFunction[WorkerCommandRunnerAdapter](calls),
			ProviderFromCommandRunnerFactory: inertRuntimeOpeningFunction[ProviderFromCommandRunnerFactory](calls),
		},
		OperatorSettings: &OperatorSettingsDependencies{
			EnsureBackendScope: inertRuntimeOpeningFunction[operatorsettings.BackendScopeEnsurer](calls),
		},
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
type factorySessionsConstructionStub struct{ factorysessions.Service }
type factoryRuntimeAssemblerConstructionStub struct{ FactoryRuntimeAssembler }
type processRuntimeFactoryConstructionStub struct{ roles.ProcessRuntimeFactory }
type modelsConstructionStub struct{ models.Service }
type replayInputsConstructionStub struct{ recordings.ReplayInputLoader }

type constructionMaterializer struct{ calls *int }

func (materializer constructionMaterializer) MaterializeContentURL(
	context.Context,
	string,
) (string, work.ContentCleanup, error) {
	(*materializer.calls)++
	return "", nil, nil
}

type identityConstructionMaterializer struct {
	calls int
	input string
}

func (materializer *identityConstructionMaterializer) MaterializeContentURL(
	_ context.Context,
	rawURL string,
) (string, work.ContentCleanup, error) {
	materializer.calls++
	materializer.input = rawURL
	return "/tmp/identity.png", func() {}, nil
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
