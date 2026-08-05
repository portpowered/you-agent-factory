package runtimeopening

import (
	"context"
	"reflect"
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
