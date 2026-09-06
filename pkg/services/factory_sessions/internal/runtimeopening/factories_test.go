package runtimeopening

import (
	"bytes"
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
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

func TestSetWorkerExecutionRejectsMissingRequiredSetter(t *testing.T) {
	workerService := &workerExecutionBindingWorkerService{Service: nil}
	err := setWorkerExecution("session-42", struct{}{}, workerService, nil, "runtime-1", "generation-1", nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "session-42") || !strings.Contains(err.Error(), "setter is required") {
		t.Fatalf("missing setter error = %v, want session and required setter", err)
	}
}

func TestSetWorkerExecutionRejectsMissingWorkersService(t *testing.T) {
	var setter struct{ workerExecutionSetter }
	err := setWorkerExecution("session-43", setter, nil, nil, "runtime-1", "generation-1", nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "session-43") || !strings.Contains(err.Error(), "Workers service is required") {
		t.Fatalf("missing Workers service error = %v, want session and required service", err)
	}
}

type workerExecutionBindingWorkerService struct {
	workers.Service
}

// runtimeOpeningFixture is test-only assembly syntax. Production callers use
// the ten separate owner-port arguments exposed by NewFactory; keeping this
// fixture aggregate local makes omission and identity cases concise without
// reintroducing an aggregate production contract.
type runtimeOpeningFixture struct {
	ProviderSessions   *ProviderSessionsPorts
	FactoryRuntime     *FactoryRuntimePorts
	FactoryDefinitions *FactoryDefinitionsPorts
	FactorySessions    *FactorySessionsPorts
	Work               *WorkPorts
	Automations        *AutomationsPorts
	Models             *ModelsPorts
	Recordings         *RecordingsPorts
	Webhooks           *WebhooksPorts
	Workers            *WorkersPorts
	OperatorSettings   *OperatorSettingsPorts
}

func (fixture runtimeOpeningFixture) newFactory() (*Factory, error) {
	return NewFactory(
		fixture.ProviderSessions,
		fixture.FactoryRuntime,
		fixture.FactoryDefinitions,
		fixture.FactorySessions,
		fixture.Work,
		fixture.Automations,
		fixture.Models,
		fixture.Recordings,
		fixture.Webhooks,
		fixture.Workers,
		fixture.OperatorSettings,
	)
}

func TestNewFactoryRejectsEveryMissingRuntimeOpeningOwnerPorts(t *testing.T) {
	t.Parallel()

	tests := []runtimeOpeningDependencyOmission{
		{"Provider Sessions owner ports", func(dependencies *runtimeOpeningFixture) { dependencies.ProviderSessions = nil }},
		{"Factory Runtime owner ports", func(dependencies *runtimeOpeningFixture) { dependencies.FactoryRuntime = nil }},
		{"Factory Definitions owner ports", func(dependencies *runtimeOpeningFixture) { dependencies.FactoryDefinitions = nil }},
		{"Factory Sessions owner ports", func(dependencies *runtimeOpeningFixture) { dependencies.FactorySessions = nil }},
		{"Work owner ports", func(dependencies *runtimeOpeningFixture) { dependencies.Work = nil }},
		{"Automations owner ports", func(dependencies *runtimeOpeningFixture) { dependencies.Automations = nil }},
		{"Models owner ports", func(dependencies *runtimeOpeningFixture) { dependencies.Models = nil }},
		{"Recordings owner ports", func(dependencies *runtimeOpeningFixture) { dependencies.Recordings = nil }},
		{"Webhooks owner ports", func(dependencies *runtimeOpeningFixture) { dependencies.Webhooks = nil }},
		{"Workers owner ports", func(dependencies *runtimeOpeningFixture) { dependencies.Workers = nil }},
		{"Operator Settings owner ports", func(dependencies *runtimeOpeningFixture) { dependencies.OperatorSettings = nil }},
	}

	for _, test := range tests {
		t.Run(test.requirement, func(t *testing.T) {
			calls := 0
			dependencies := validRuntimeOpeningOwnerPorts(&calls)
			test.omit(&dependencies)

			factory, err := dependencies.newFactory()
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
			dependencies := validRuntimeOpeningOwnerPorts(&calls)
			test.omit(&dependencies)

			factory, err := dependencies.newFactory()
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
	dependencies := validRuntimeOpeningOwnerPorts(&calls)
	dependencies.ProviderSessions = nil
	dependencies.FactoryRuntime = nil

	factory, err := dependencies.newFactory()
	if factory != nil {
		t.Fatalf("NewFactory() = %#v, want nil factory", factory)
	}
	if got, want := err.Error(), "Factory Sessions runtime-opening Provider Sessions owner ports are required"; got != want {
		t.Fatalf("NewFactory() error = %q, want first stable requirement %q", got, want)
	}
	if calls != 0 {
		t.Fatalf("NewFactory() invoked %d collaborator functions, want inert failure", calls)
	}
}

func TestNewFactoryRetainsExactGroupedCollaborators(t *testing.T) {
	t.Parallel()

	calls := 0
	dependencies := validRuntimeOpeningOwnerPorts(&calls)
	materializer := &identityConstructionMaterializer{}
	dependencies.Work.Service = work.MaterializationService(materializer)

	factory, err := dependencies.newFactory()
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	if factory == nil {
		t.Fatal("NewFactory() = nil, want factory")
	}

	assertProviderSessionsPortsRetained(t, factory, dependencies)
	assertFactoryRuntimePortsRetained(t, factory, dependencies)
	assertFactoryDefinitionsPortsRetained(t, factory, dependencies)
	assertFactorySessionsPortsRetained(t, factory, dependencies)
	assertWorkPortsRetained(t, factory, dependencies, materializer)
	assertAutomationsPortsRetained(t, factory, dependencies)
	assertModelsPortsRetained(t, factory, dependencies)
	assertRecordingsPortsRetained(t, factory, dependencies)
	assertWebhooksPortsRetained(t, factory, dependencies)
	assertWorkersPortsRetained(t, factory, dependencies)
	assertOperatorSettingsPortsRetained(t, factory, dependencies)
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
	dependencies := validRuntimeOpeningOwnerPorts(&calls)
	replayInputs := &historicalReplayInputsRecorder{portable: portable, events: &events}
	recordingsRoot := &recordingsRootConstructionStub{replayInputs: replayInputs}
	dependencies.Recordings.Service = recordingsRoot
	dependencies.Recordings.Runtime = recordingsRoot
	dependencies.FactorySessions.GenerateRuntimeInstanceID = func() string {
		events = append(events, "runtime-instance-id")
		return "historical-runtime"
	}
	dependencies.FactoryRuntime.NewSessionLogger = func(*zap.Logger, string, string, string) *zap.Logger {
		events = append(events, "session-logger")
		return zap.NewNop()
	}

	factory, err := dependencies.newFactory()
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	opened, err := factory.OpenApplicationRuntime(
		t.Context(),
		&factorysessions.RuntimeOpeningRequest{
			FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{Directory: t.TempDir()},
			Recordings:        recordings.RuntimeOpeningRequest{ReplayPath: "recording.json"},
		},
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

func assertProviderSessionsPortsRetained(t *testing.T, factory *Factory, dependencies runtimeOpeningFixture) {
	t.Helper()
	assertRuntimeOpeningDependencyIdentity(t, "Provider Sessions service", factory.providerSessions, dependencies.ProviderSessions.Service)
}

func assertFactoryRuntimePortsRetained(t *testing.T, factory *Factory, dependencies runtimeOpeningFixture) {
	t.Helper()
	group := dependencies.FactoryRuntime
	assertRuntimeOpeningDependencyIdentity(t, "Factory Runtime workflows", factory.factoryWorkflows, group.FactoryWorkflows)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Runtime preview", factory.workflowPreview, group.WorkflowPreview)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Runtime mock runner", factory.workersMockCommandRunnerFactory, group.WorkersMockCommandRunnerFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Runtime assembler", factory.factoryRuntimeAssembler, group.FactoryRuntimeAssembler)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Runtime clock", factory.resolveClock, group.ResolveClock)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Runtime selected clock", factory.clock, group.Clock)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Runtime base logger", factory.baseLogger, group.Logger)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Runtime logger", factory.newSessionLogger, group.NewSessionLogger)
}

func assertFactoryDefinitionsPortsRetained(t *testing.T, factory *Factory, dependencies runtimeOpeningFixture) {
	t.Helper()
	group := dependencies.FactoryDefinitions
	assertRuntimeOpeningDependencyIdentity(t, "Factory Definitions validator", factory.factoryDefinitionValidator, group.Validator)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Definitions paths", factory.namedPaths, group.NamedPaths)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Definitions service", factory.factoryDefinitions, group.Service)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Definitions runtime router", factory.definitionRuntimeRouter, group.RuntimeRouter)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Definitions initial snapshot", factory.initialFactorySnapshotFactory, group.InitialFactorySnapshotFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Definitions loader", factory.loadFactory, group.LoadFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Definitions source", factory.newLoadedFactory, group.NewLoadedFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Definitions replay decoder", factory.decodeReplayConfig, group.DecodeReplayConfig)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Definitions snapshot capturer", factory.captureLoadedFactorySnapshot, group.CaptureLoadedFactorySnapshot)
}

func assertFactorySessionsPortsRetained(t *testing.T, factory *Factory, dependencies runtimeOpeningFixture) {
	t.Helper()
	group := dependencies.FactorySessions
	assertRuntimeOpeningDependencyIdentity(t, "Factory Sessions runtime assembly", factory.factorySessionsRuntimeAssembly, group.RuntimeAssembly)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Sessions durable execution", factory.durableExecutionFactory, group.DurableExecutionFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Sessions execution", factory.factorySessionExecutionFactory, group.FactorySessionExecutionFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Sessions scaffold", factory.factoryScaffoldInitializer, group.FactoryScaffoldInitializer)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Sessions editable validation", factory.editableFactoryValidator, group.EditableFactoryValidator)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Sessions process runtime", factory.processRuntimeFactory, group.ProcessRuntimeFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Sessions session ID", factory.generateSessionID, group.GenerateSessionID)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Sessions runtime ID", factory.generateRuntimeInstanceID, group.GenerateRuntimeInstanceID)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Sessions home", factory.resolveHome, group.ResolveHome)
	assertRuntimeOpeningDependencyIdentity(t, "Factory Sessions provider identities", factory.providerIdentities, group.ProviderIdentities)
}

func assertWorkPortsRetained(
	t *testing.T,
	factory *Factory,
	dependencies runtimeOpeningFixture,
	materializer *identityConstructionMaterializer,
) {
	t.Helper()
	assertRuntimeOpeningDependencyIdentity(t, "Work service", factory.workService, dependencies.Work.Service)
	path, cleanup, err := factory.workService.MaterializeContentURL(t.Context(), "file:///identity.png")
	if err != nil || path != "/tmp/identity.png" || cleanup == nil {
		t.Fatalf("Work materialization = (%q, %v, %v), want exact injected materializer", path, cleanup, err)
	}
	cleanup()
	if materializer.calls != 1 || materializer.input != "file:///identity.png" {
		t.Fatalf("Work materializer calls = %d with %q, want injected instance once", materializer.calls, materializer.input)
	}
}

func assertAutomationsPortsRetained(t *testing.T, factory *Factory, dependencies runtimeOpeningFixture) {
	t.Helper()
	group := dependencies.Automations
	assertRuntimeOpeningDependencyIdentity(t, "Automations service", factory.automationService, group.Service)
}

func assertModelsPortsRetained(t *testing.T, factory *Factory, dependencies runtimeOpeningFixture) {
	t.Helper()
	assertRuntimeOpeningDependencyIdentity(t, "Models service", factory.modelService, dependencies.Models.Service)
}

func assertRecordingsPortsRetained(t *testing.T, factory *Factory, dependencies runtimeOpeningFixture) {
	t.Helper()
	group := dependencies.Recordings
	assertRuntimeOpeningDependencyIdentity(t, "Recordings service", factory.recordingsService, group.Service)
	assertRuntimeOpeningDependencyIdentity(t, "Recordings runtime", factory.recordingsRuntime, group.Runtime)
}

func assertWebhooksPortsRetained(t *testing.T, factory *Factory, dependencies runtimeOpeningFixture) {
	t.Helper()
	assertRuntimeOpeningDependencyIdentity(t, "Webhooks service", factory.webhooksService, dependencies.Webhooks.Service)
}

func assertWorkersPortsRetained(t *testing.T, factory *Factory, dependencies runtimeOpeningFixture) {
	t.Helper()
	group := dependencies.Workers
	assertRuntimeOpeningDependencyIdentity(t, "Workers service", factory.workerService, group.Service)
	assertRuntimeOpeningDependencyIdentity(t, "Workers provider adapter", factory.providerFromCommandRunnerFactory, group.ProviderFromCommandRunnerFactory)
	assertRuntimeOpeningDependencyIdentity(t, "Workers provider command runner", factory.providerCommandRunner, group.ProviderCommandRunner)
	assertRuntimeOpeningDependencyIdentity(t, "Workers script command runner", factory.scriptCommandRunner, group.ScriptCommandRunner)
}

func assertOperatorSettingsPortsRetained(t *testing.T, factory *Factory, dependencies runtimeOpeningFixture) {
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
	omit        func(*runtimeOpeningFixture)
}

func runtimeOpeningMemberOmissions() []runtimeOpeningDependencyOmission {
	return []runtimeOpeningDependencyOmission{
		{"Provider Sessions service", func(d *runtimeOpeningFixture) { d.ProviderSessions.Service = nil }},
		{"Factory Runtime logger", func(d *runtimeOpeningFixture) { d.FactoryRuntime.Logger = nil }},
		{"Factory Runtime JavaScript workflow definitions", func(d *runtimeOpeningFixture) { d.FactoryRuntime.FactoryWorkflows = nil }},
		{"Factory Runtime workflow preview operation", func(d *runtimeOpeningFixture) { d.FactoryRuntime.WorkflowPreview = nil }},
		{"Factory Runtime Workers mock command runner factory", func(d *runtimeOpeningFixture) { d.FactoryRuntime.WorkersMockCommandRunnerFactory = nil }},
		{"Factory Runtime runtime assembler", func(d *runtimeOpeningFixture) { d.FactoryRuntime.FactoryRuntimeAssembler = nil }},
		{"Factory Runtime clock resolver", func(d *runtimeOpeningFixture) { d.FactoryRuntime.ResolveClock = nil }},
		{"Factory Runtime session logger factory", func(d *runtimeOpeningFixture) { d.FactoryRuntime.NewSessionLogger = nil }},
		{"Factory Runtime clock", func(d *runtimeOpeningFixture) { d.FactoryRuntime.Clock = nil }},
		{"Factory Definitions validator", func(d *runtimeOpeningFixture) { d.FactoryDefinitions.Validator = nil }},
		{"Factory Definitions named path resolver", func(d *runtimeOpeningFixture) { d.FactoryDefinitions.NamedPaths = nil }},
		{"Factory Definitions service", func(d *runtimeOpeningFixture) { d.FactoryDefinitions.Service = nil }},
		{"Factory Definitions runtime router", func(d *runtimeOpeningFixture) { d.FactoryDefinitions.RuntimeRouter = nil }},
		{"Factory Definitions initial factory snapshot factory", func(d *runtimeOpeningFixture) { d.FactoryDefinitions.InitialFactorySnapshotFactory = nil }},
		{"Factory Definitions loaded factory loader", func(d *runtimeOpeningFixture) { d.FactoryDefinitions.LoadFactory = nil }},
		{"Factory Definitions loaded factory source factory", func(d *runtimeOpeningFixture) { d.FactoryDefinitions.NewLoadedFactory = nil }},
		{"Factory Definitions replay runtime config decoder", func(d *runtimeOpeningFixture) { d.FactoryDefinitions.DecodeReplayConfig = nil }},
		{"Factory Definitions loaded factory snapshot capturer", func(d *runtimeOpeningFixture) { d.FactoryDefinitions.CaptureLoadedFactorySnapshot = nil }},
		{"Factory Sessions runtime assembly", func(d *runtimeOpeningFixture) { d.FactorySessions.RuntimeAssembly = nil }},
		{"Factory Sessions durable execution factory", func(d *runtimeOpeningFixture) { d.FactorySessions.DurableExecutionFactory = nil }},
		{"Factory Sessions session execution factory", func(d *runtimeOpeningFixture) { d.FactorySessions.FactorySessionExecutionFactory = nil }},
		{"Factory Sessions factory scaffold initializer", func(d *runtimeOpeningFixture) { d.FactorySessions.FactoryScaffoldInitializer = nil }},
		{"Factory Sessions editable factory validator", func(d *runtimeOpeningFixture) { d.FactorySessions.EditableFactoryValidator = nil }},
		{"Factory Sessions process runtime factory", func(d *runtimeOpeningFixture) { d.FactorySessions.ProcessRuntimeFactory = nil }},
		{"Factory Sessions session ID generator", func(d *runtimeOpeningFixture) { d.FactorySessions.GenerateSessionID = nil }},
		{"Factory Sessions runtime instance ID generator", func(d *runtimeOpeningFixture) { d.FactorySessions.GenerateRuntimeInstanceID = nil }},
		{"Factory Sessions home directory resolver", func(d *runtimeOpeningFixture) { d.FactorySessions.ResolveHome = nil }},
		{"Factory Sessions provider identity resolver", func(d *runtimeOpeningFixture) { d.FactorySessions.ProviderIdentities = nil }},
		{"Work service", func(d *runtimeOpeningFixture) { d.Work.Service = nil }},
		{"Automations service", func(d *runtimeOpeningFixture) { d.Automations.Service = nil }},
		{"Models service", func(d *runtimeOpeningFixture) { d.Models.Service = nil }},
		{"Recordings service", func(d *runtimeOpeningFixture) { d.Recordings.Service = nil }},
		{"Recordings runtime", func(d *runtimeOpeningFixture) { d.Recordings.Runtime = nil }},
		{"Webhooks service", func(d *runtimeOpeningFixture) { d.Webhooks.Service = nil }},
		{"Workers service", func(d *runtimeOpeningFixture) { d.Workers.Service = nil }},
		{"Workers provider-from-command-runner factory", func(d *runtimeOpeningFixture) { d.Workers.ProviderFromCommandRunnerFactory = nil }},
		{"Workers provider command runner", func(d *runtimeOpeningFixture) { d.Workers.ProviderCommandRunner = nil }},
		{"Workers script command runner", func(d *runtimeOpeningFixture) { d.Workers.ScriptCommandRunner = nil }},
		{"Operator Settings backend scope ensurer", func(d *runtimeOpeningFixture) { d.OperatorSettings.EnsureBackendScope = nil }},
	}
}

func validRuntimeOpeningOwnerPorts(calls *int) runtimeOpeningFixture {
	factorySessionsRoot := &factorySessionsConstructionStub{}
	return runtimeOpeningFixture{
		ProviderSessions: &ProviderSessionsPorts{
			Service: providerSessionsConstructionStub{},
		},
		FactoryRuntime: &FactoryRuntimePorts{
			Logger:                          zap.NewNop(),
			FactoryWorkflows:                workflowDefinitionsConstructionStub{},
			WorkflowPreview:                 workflowPreviewConstructionStub{},
			WorkersMockCommandRunnerFactory: inertRuntimeOpeningFunction[factoryruntime.WorkersMockCommandRunnerFactory](calls),
			FactoryRuntimeAssembler:         factoryRuntimeAssemblerConstructionStub{},
			ResolveClock:                    inertRuntimeOpeningFunction[factoryruntime.ClockResolver](calls),
			NewSessionLogger:                inertRuntimeOpeningFunction[factoryruntime.SessionLoggerFactory](calls),
			Clock:                           openingCoordinatorClock{},
		},
		FactoryDefinitions: &FactoryDefinitionsPorts{
			Validator:                     validatorConstructionStub{},
			NamedPaths:                    namedPathsConstructionStub{},
			Service:                       factoryDefinitionsConstructionStub{},
			RuntimeRouter:                 &factorysessions.DefinitionRuntimeRouter{},
			InitialFactorySnapshotFactory: inertRuntimeOpeningFunction[factorydefinitions.InitialFactorySnapshotFactory](calls),
			LoadFactory:                   inertRuntimeOpeningFunction[factorydefinitions.LoadedFactoryLoader](calls),
			NewLoadedFactory:              inertRuntimeOpeningFunction[factorydefinitions.LoadedFactorySourceFactory](calls),
			DecodeReplayConfig:            inertRuntimeOpeningFunction[factorydefinitions.ReplayRuntimeConfigDecoder](calls),
			CaptureLoadedFactorySnapshot:  inertRuntimeOpeningFunction[factorydefinitions.LoadedFactorySnapshotCapturer](calls),
		},
		FactorySessions: &FactorySessionsPorts{
			RuntimeAssembly:                factorySessionsRoot,
			DurableExecutionFactory:        inertRuntimeOpeningFunction[DurableExecutionFactory](calls),
			FactorySessionExecutionFactory: inertRuntimeOpeningFunction[FactorySessionExecutionFactory](calls),
			FactoryScaffoldInitializer:     inertRuntimeOpeningFunction[factorysessions.FactoryScaffoldInitializer](calls),
			EditableFactoryValidator:       inertRuntimeOpeningFunction[factorysessions.EditableFactoryValidator](calls),
			ProcessRuntimeFactory:          processRuntimeFactoryConstructionStub{},
			GenerateSessionID:              inertRuntimeOpeningFunction[factorysessions.SessionIDGenerator](calls),
			GenerateRuntimeInstanceID:      inertRuntimeOpeningFunction[factorysessions.RuntimeInstanceIDGenerator](calls),
			ResolveHome:                    inertRuntimeOpeningFunction[factorysessions.HomeDirectoryResolver](calls),
			ProviderIdentities:             inertRuntimeOpeningFunction[factorysessions.ProviderIdentityResolver](calls),
		},
		Work: &WorkPorts{
			Service: work.MaterializationService(constructionMaterializer{calls: calls}),
		},
		Automations: &AutomationsPorts{
			Service: automations.Root{},
		},
		Models: &ModelsPorts{Service: &modelsConstructionStub{}},
		Recordings: &RecordingsPorts{
			Service: &recordingsRootConstructionStub{},
			Runtime: &recordingsRootConstructionStub{},
		},
		Webhooks: &WebhooksPorts{Service: webhooksConstructionStub{}},
		Workers: &WorkersPorts{
			Service:                          &workersConstructionStub{},
			ProviderFromCommandRunnerFactory: inertRuntimeOpeningFunction[ProviderFromCommandRunnerFactory](calls),
			ProviderCommandRunner:            workersRootBindingProbeRunner{tag: "provider"},
			ScriptCommandRunner:              workersRootBindingProbeRunner{tag: "script"},
		},
		OperatorSettings: &OperatorSettingsPorts{
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
type factorySessionsConstructionStub struct {
	factorysessions.Service
	roles.RuntimeAssembly
}
type factoryRuntimeAssemblerConstructionStub struct{ FactoryRuntimeAssembler }
type processRuntimeFactoryConstructionStub struct{ roles.ProcessRuntimeFactory }
type modelsConstructionStub struct{ models.Service }
type recordingsRootConstructionStub struct {
	recordings.Service
	recordings.RuntimeOpening
	replayInputs recordings.ReplayInputLoader
}

func (*recordingsRootConstructionStub) Projection() recordings.ProjectionService {
	return &openingCoordinatorProjection{}
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

func (stub *recordingsRootConstructionStub) LoadReplayInput(
	request recordings.LoadReplayInputRequest,
) (recordings.LoadReplayInputResult, error) {
	if stub.replayInputs == nil {
		return recordings.LoadReplayInputResult{}, nil
	}
	return stub.replayInputs.LoadReplayInput(request)
}

var _ recordings.Service = (*recordingsRootConstructionStub)(nil)
var _ recordings.RuntimeOpening = (*recordingsRootConstructionStub)(nil)
