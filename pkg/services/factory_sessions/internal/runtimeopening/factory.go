package runtimeopening

import (
	"context"
	"fmt"
	"reflect"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// WorkerCommandRunnerAdapter projects a replaceable low-level process effect
// into the Workers-owned command port.
type WorkerCommandRunnerAdapter func(platformprocess.CommandRunner) workers.CommandRunner

// Dependencies is the Factory Sessions-owned construction input for the one
// process-scoped runtime-opening factory. Groups are construction vocabulary:
// they select fixed collaborators once in canonical Wire composition and do
// not expose a runtime service locator or an alternate opening path.
//
// Validation walks groups and members in the declaration order below. That
// stable order keeps incomplete composition failures deterministic and inert.
type Dependencies struct {
	ProviderSessions   *ProviderSessionsDependencies
	FactoryRuntime     *FactoryRuntimeDependencies
	FactoryDefinitions *FactoryDefinitionsDependencies
	FactorySessions    *FactorySessionsDependencies
	Work               *WorkDependencies
	Automations        *AutomationsDependencies
	Models             *ModelsDependencies
	Recordings         *RecordingsDependencies
	Workers            *WorkersDependencies
	OperatorSettings   *OperatorSettingsDependencies
}

// ProviderSessionsDependencies contains the Provider Sessions-owned runtime
// collaborators.
type ProviderSessionsDependencies struct {
	Service providersessions.Service
}

// FactoryRuntimeDependencies contains Factory Runtime's opening collaborators.
type FactoryRuntimeDependencies struct {
	FactoryWorkflows                factoryruntime.JavaScriptWorkflowDefinitions
	WorkflowPreview                 factoryruntime.WorkflowPreviewOperation
	WorkersRuntimeExecutorsFactory  factoryruntime.WorkersRuntimeExecutorsFactory
	WorkersMockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory
	FactoryRuntimeAssembler         FactoryRuntimeAssembler
	ResolveClock                    factoryruntime.ClockResolver
	NewSessionLogger                factoryruntime.SessionLoggerFactory
}

// FactoryDefinitionsDependencies contains Factory Definitions-owned opening
// collaborators.
type FactoryDefinitionsDependencies struct {
	Validator                     factorydefinitions.Validator
	NamedPaths                    factorydefinitions.NamedPathResolver
	Factory                       FactoryDefinitionsFactory
	InitialFactorySnapshotFactory factorydefinitions.InitialFactorySnapshotFactory
	LoadFactory                   factorydefinitions.LoadedFactoryLoader
	NewLoadedFactory              factorydefinitions.LoadedFactorySourceFactory
	DecodeReplayConfig            factorydefinitions.ReplayRuntimeConfigDecoder
	CaptureLoadedFactorySnapshot  factorydefinitions.LoadedFactorySnapshotCapturer
}

// FactorySessionsDependencies contains Factory Sessions-owned opening
// collaborators.
type FactorySessionsDependencies struct {
	Service                        factorysessions.Service
	DurableExecutionFactory        DurableExecutionFactory
	FactorySessionExecutionFactory FactorySessionExecutionFactory
	FactoryScaffoldInitializer     factorysessions.FactoryScaffoldInitializer
	EditableFactoryValidator       factorysessions.EditableFactoryValidator
	ProcessRuntimeFactory          roles.ProcessRuntimeFactory
	GenerateRuntimeInstanceID      factorysessions.RuntimeInstanceIDGenerator
	ResolveHome                    factorysessions.HomeDirectoryResolver
	ProviderIdentities             factorysessions.ProviderIdentityResolver
}

// WorkDependencies contains Work-owned opening collaborators.
type WorkDependencies struct {
	Factory             WorkFactory
	ContentMaterializer work.ContentMaterializer
}

// AutomationsDependencies contains Automations-owned opening collaborators.
type AutomationsDependencies struct {
	Factory              AutomationFactory
	HostedSourcesFactory AutomationHostedSourcesFactory
}

// ModelsDependencies contains the Models root used while opening a session.
type ModelsDependencies struct {
	Service models.Service
}

// RecordingsDependencies contains Recordings-owned opening collaborators.
type RecordingsDependencies struct {
	ProjectionFactory      RecordingsProjectionFactory
	LifecycleFactory       RecordingLifecycleFactory
	RuntimeLedgerFactory   RuntimeLedgerFactory
	RuntimeRecorderFactory recordings.RuntimeRecorderFactory
	ReplayClockFactory     ReplayClockFactory
	ReplayExecutionFactory recordings.ReplayExecutionFactory
	ReplayInputs           recordings.ReplayInputLoader
}

// WorkersDependencies contains Workers-owned opening collaborators.
type WorkersDependencies struct {
	ExecutionFactory                 WorkerExecutionFactory
	RuntimeFactory                   WorkersRuntimeFactory
	LocalRuntimeHooksFactory         WorkersLocalRuntimeHooksFactory
	AdaptCommandRunner               WorkerCommandRunnerAdapter
	ProviderFromCommandRunnerFactory ProviderFromCommandRunnerFactory
}

// OperatorSettingsDependencies contains the Operator Settings capability used
// to establish the session backend scope.
type OperatorSettingsDependencies struct {
	EnsureBackendScope operatorsettings.BackendScopeEnsurer
}

// Factory is the process-scoped, inert Factory Session opening operation.
// Wire selects all implementation functions once; OpenRuntime supplies only
// invocation data and external edges.
type Factory struct {
	durableExecutionFactory          DurableExecutionFactory
	workerExecutionFactory           WorkerExecutionFactory
	modelService                     models.Service
	workFactory                      WorkFactory
	automationFactory                AutomationFactory
	factorySessionsService           factorysessions.Service
	factorySessionExecutionFactory   FactorySessionExecutionFactory
	recordingsProjectionFactory      RecordingsProjectionFactory
	recordingLifecycleFactory        RecordingLifecycleFactory
	runtimeLedgerFactory             RuntimeLedgerFactory
	runtimeRecorderFactory           recordings.RuntimeRecorderFactory
	replayClockFactory               ReplayClockFactory
	replayExecutionFactory           recordings.ReplayExecutionFactory
	workersRuntimeFactory            WorkersRuntimeFactory
	workersRuntimeExecutorsFactory   factoryruntime.WorkersRuntimeExecutorsFactory
	workersMockCommandRunnerFactory  factoryruntime.WorkersMockCommandRunnerFactory
	automationHostedSourcesFactory   AutomationHostedSourcesFactory
	workersLocalRuntimeHooksFactory  WorkersLocalRuntimeHooksFactory
	factoryDefinitionsFactory        FactoryDefinitionsFactory
	factoryScaffoldInitializer       factorysessions.FactoryScaffoldInitializer
	editableFactoryValidator         factorysessions.EditableFactoryValidator
	initialFactorySnapshotFactory    factorydefinitions.InitialFactorySnapshotFactory
	factoryRuntimeAssembler          FactoryRuntimeAssembler
	workService                      work.Service
	providerSessions                 providersessions.Service
	factoryDefinitionValidator       factorydefinitions.Validator
	namedPaths                       factorydefinitions.NamedPathResolver
	factoryWorkflows                 factoryruntime.JavaScriptWorkflowDefinitions
	workflowPreview                  factoryruntime.WorkflowPreviewOperation
	loadFactory                      factorydefinitions.LoadedFactoryLoader
	newLoadedFactory                 factorydefinitions.LoadedFactorySourceFactory
	decodeReplayConfig               factorydefinitions.ReplayRuntimeConfigDecoder
	replayInputs                     recordings.ReplayInputLoader
	captureLoadedFactorySnapshot     factorydefinitions.LoadedFactorySnapshotCapturer
	resolveClock                     factoryruntime.ClockResolver
	newSessionLogger                 factoryruntime.SessionLoggerFactory
	adaptWorkerCommandRunner         WorkerCommandRunnerAdapter
	providerFromCommandRunnerFactory ProviderFromCommandRunnerFactory
	processRuntimeFactory            roles.ProcessRuntimeFactory
	ensureOperatorBackendScope       operatorsettings.BackendScopeEnsurer
	generateRuntimeInstanceID        factorysessions.RuntimeInstanceIDGenerator
	resolveHome                      factorysessions.HomeDirectoryResolver
	providerIdentities               factorysessions.ProviderIdentityResolver
}

func NewFactory(dependencies Dependencies) (*Factory, error) {
	if err := dependencies.validate(); err != nil {
		return nil, err
	}

	providerSessions := dependencies.ProviderSessions
	factoryRuntime := dependencies.FactoryRuntime
	factoryDefinitions := dependencies.FactoryDefinitions
	factorySessions := dependencies.FactorySessions
	workDependencies := dependencies.Work
	automations := dependencies.Automations
	modelsDependencies := dependencies.Models
	recordingsDependencies := dependencies.Recordings
	workersDependencies := dependencies.Workers
	operatorSettings := dependencies.OperatorSettings

	return &Factory{
		durableExecutionFactory:          factorySessions.DurableExecutionFactory,
		workerExecutionFactory:           workersDependencies.ExecutionFactory,
		modelService:                     modelsDependencies.Service,
		workFactory:                      workDependencies.Factory,
		automationFactory:                automations.Factory,
		factorySessionsService:           factorySessions.Service,
		factorySessionExecutionFactory:   factorySessions.FactorySessionExecutionFactory,
		recordingsProjectionFactory:      recordingsDependencies.ProjectionFactory,
		recordingLifecycleFactory:        recordingsDependencies.LifecycleFactory,
		runtimeLedgerFactory:             recordingsDependencies.RuntimeLedgerFactory,
		runtimeRecorderFactory:           recordingsDependencies.RuntimeRecorderFactory,
		replayClockFactory:               recordingsDependencies.ReplayClockFactory,
		replayExecutionFactory:           recordingsDependencies.ReplayExecutionFactory,
		workersRuntimeFactory:            workersDependencies.RuntimeFactory,
		workersRuntimeExecutorsFactory:   factoryRuntime.WorkersRuntimeExecutorsFactory,
		workersMockCommandRunnerFactory:  factoryRuntime.WorkersMockCommandRunnerFactory,
		automationHostedSourcesFactory:   automations.HostedSourcesFactory,
		workersLocalRuntimeHooksFactory:  workersDependencies.LocalRuntimeHooksFactory,
		factoryDefinitionsFactory:        factoryDefinitions.Factory,
		factoryScaffoldInitializer:       factorySessions.FactoryScaffoldInitializer,
		editableFactoryValidator:         factorySessions.EditableFactoryValidator,
		initialFactorySnapshotFactory:    factoryDefinitions.InitialFactorySnapshotFactory,
		factoryRuntimeAssembler:          factoryRuntime.FactoryRuntimeAssembler,
		workService:                      work.MaterializationService(workDependencies.ContentMaterializer),
		providerSessions:                 providerSessions.Service,
		factoryDefinitionValidator:       factoryDefinitions.Validator,
		namedPaths:                       factoryDefinitions.NamedPaths,
		factoryWorkflows:                 factoryRuntime.FactoryWorkflows,
		workflowPreview:                  factoryRuntime.WorkflowPreview,
		loadFactory:                      factoryDefinitions.LoadFactory,
		newLoadedFactory:                 factoryDefinitions.NewLoadedFactory,
		decodeReplayConfig:               factoryDefinitions.DecodeReplayConfig,
		replayInputs:                     recordingsDependencies.ReplayInputs,
		captureLoadedFactorySnapshot:     factoryDefinitions.CaptureLoadedFactorySnapshot,
		resolveClock:                     factoryRuntime.ResolveClock,
		newSessionLogger:                 factoryRuntime.NewSessionLogger,
		adaptWorkerCommandRunner:         workersDependencies.AdaptCommandRunner,
		providerFromCommandRunnerFactory: workersDependencies.ProviderFromCommandRunnerFactory,
		processRuntimeFactory:            factorySessions.ProcessRuntimeFactory,
		ensureOperatorBackendScope:       operatorSettings.EnsureBackendScope,
		generateRuntimeInstanceID:        factorySessions.GenerateRuntimeInstanceID,
		resolveHome:                      factorySessions.ResolveHome,
		providerIdentities:               factorySessions.ProviderIdentities,
	}, nil
}

func (dependencies Dependencies) validate() error {
	for _, validate := range []func() error{
		dependencies.validateProviderSessions,
		dependencies.validateFactoryRuntime,
		dependencies.validateFactoryDefinitions,
		dependencies.validateFactorySessions,
		dependencies.validateWork,
		dependencies.validateAutomations,
		dependencies.validateModels,
		dependencies.validateRecordings,
		dependencies.validateWorkers,
		dependencies.validateOperatorSettings,
	} {
		if err := validate(); err != nil {
			return err
		}
	}
	return nil
}

func (dependencies Dependencies) validateProviderSessions() error {
	group := dependencies.ProviderSessions
	if err := requireRuntimeOpeningGroup("Provider Sessions", group); err != nil {
		return err
	}
	return validateRuntimeOpeningRequirements("Provider Sessions",
		runtimeOpeningRequirement{"service", group.Service},
	)
}

func (dependencies Dependencies) validateFactoryRuntime() error {
	group := dependencies.FactoryRuntime
	if err := requireRuntimeOpeningGroup("Factory Runtime", group); err != nil {
		return err
	}
	return validateRuntimeOpeningRequirements("Factory Runtime",
		runtimeOpeningRequirement{"JavaScript workflow definitions", group.FactoryWorkflows},
		runtimeOpeningRequirement{"workflow preview operation", group.WorkflowPreview},
		runtimeOpeningRequirement{"Workers runtime executors factory", group.WorkersRuntimeExecutorsFactory},
		runtimeOpeningRequirement{"Workers mock command runner factory", group.WorkersMockCommandRunnerFactory},
		runtimeOpeningRequirement{"runtime assembler", group.FactoryRuntimeAssembler},
		runtimeOpeningRequirement{"clock resolver", group.ResolveClock},
		runtimeOpeningRequirement{"session logger factory", group.NewSessionLogger},
	)
}

func (dependencies Dependencies) validateFactoryDefinitions() error {
	group := dependencies.FactoryDefinitions
	if err := requireRuntimeOpeningGroup("Factory Definitions", group); err != nil {
		return err
	}
	return validateRuntimeOpeningRequirements("Factory Definitions",
		runtimeOpeningRequirement{"validator", group.Validator},
		runtimeOpeningRequirement{"named path resolver", group.NamedPaths},
		runtimeOpeningRequirement{"factory", group.Factory},
		runtimeOpeningRequirement{"initial factory snapshot factory", group.InitialFactorySnapshotFactory},
		runtimeOpeningRequirement{"loaded factory loader", group.LoadFactory},
		runtimeOpeningRequirement{"loaded factory source factory", group.NewLoadedFactory},
		runtimeOpeningRequirement{"replay runtime config decoder", group.DecodeReplayConfig},
		runtimeOpeningRequirement{"loaded factory snapshot capturer", group.CaptureLoadedFactorySnapshot},
	)
}

func (dependencies Dependencies) validateFactorySessions() error {
	group := dependencies.FactorySessions
	if err := requireRuntimeOpeningGroup("Factory Sessions", group); err != nil {
		return err
	}
	return validateRuntimeOpeningRequirements("Factory Sessions",
		runtimeOpeningRequirement{"service", group.Service},
		runtimeOpeningRequirement{"durable execution factory", group.DurableExecutionFactory},
		runtimeOpeningRequirement{"session execution factory", group.FactorySessionExecutionFactory},
		runtimeOpeningRequirement{"factory scaffold initializer", group.FactoryScaffoldInitializer},
		runtimeOpeningRequirement{"editable factory validator", group.EditableFactoryValidator},
		runtimeOpeningRequirement{"process runtime factory", group.ProcessRuntimeFactory},
		runtimeOpeningRequirement{"runtime instance ID generator", group.GenerateRuntimeInstanceID},
		runtimeOpeningRequirement{"home directory resolver", group.ResolveHome},
		runtimeOpeningRequirement{"provider identity resolver", group.ProviderIdentities},
	)
}

func (dependencies Dependencies) validateWork() error {
	group := dependencies.Work
	if err := requireRuntimeOpeningGroup("Work", group); err != nil {
		return err
	}
	return validateRuntimeOpeningRequirements("Work",
		runtimeOpeningRequirement{"factory", group.Factory},
		runtimeOpeningRequirement{"content materializer", group.ContentMaterializer},
	)
}

func (dependencies Dependencies) validateAutomations() error {
	group := dependencies.Automations
	if err := requireRuntimeOpeningGroup("Automations", group); err != nil {
		return err
	}
	return validateRuntimeOpeningRequirements("Automations",
		runtimeOpeningRequirement{"factory", group.Factory},
		runtimeOpeningRequirement{"hosted sources factory", group.HostedSourcesFactory},
	)
}

func (dependencies Dependencies) validateModels() error {
	group := dependencies.Models
	if err := requireRuntimeOpeningGroup("Models", group); err != nil {
		return err
	}
	return validateRuntimeOpeningRequirements("Models",
		runtimeOpeningRequirement{"service", group.Service},
	)
}

func (dependencies Dependencies) validateRecordings() error {
	group := dependencies.Recordings
	if err := requireRuntimeOpeningGroup("Recordings", group); err != nil {
		return err
	}
	return validateRuntimeOpeningRequirements("Recordings",
		runtimeOpeningRequirement{"projection factory", group.ProjectionFactory},
		runtimeOpeningRequirement{"lifecycle factory", group.LifecycleFactory},
		runtimeOpeningRequirement{"runtime ledger factory", group.RuntimeLedgerFactory},
		runtimeOpeningRequirement{"runtime recorder factory", group.RuntimeRecorderFactory},
		runtimeOpeningRequirement{"replay clock factory", group.ReplayClockFactory},
		runtimeOpeningRequirement{"replay execution factory", group.ReplayExecutionFactory},
		runtimeOpeningRequirement{"replay input loader", group.ReplayInputs},
	)
}

func (dependencies Dependencies) validateWorkers() error {
	group := dependencies.Workers
	if err := requireRuntimeOpeningGroup("Workers", group); err != nil {
		return err
	}
	return validateRuntimeOpeningRequirements("Workers",
		runtimeOpeningRequirement{"execution factory", group.ExecutionFactory},
		runtimeOpeningRequirement{"runtime factory", group.RuntimeFactory},
		runtimeOpeningRequirement{"local runtime hooks factory", group.LocalRuntimeHooksFactory},
		runtimeOpeningRequirement{"command runner adapter", group.AdaptCommandRunner},
		runtimeOpeningRequirement{"provider-from-command-runner factory", group.ProviderFromCommandRunnerFactory},
	)
}

func (dependencies Dependencies) validateOperatorSettings() error {
	group := dependencies.OperatorSettings
	if err := requireRuntimeOpeningGroup("Operator Settings", group); err != nil {
		return err
	}
	return validateRuntimeOpeningRequirements("Operator Settings",
		runtimeOpeningRequirement{"backend scope ensurer", group.EnsureBackendScope},
	)
}

type runtimeOpeningRequirement struct {
	member string
	value  any
}

func requireRuntimeOpeningGroup(owner string, group any) error {
	if missingRuntimeOpeningDependency(group) {
		return fmt.Errorf("Factory Sessions runtime-opening %s group is required", owner)
	}
	return nil
}

func validateRuntimeOpeningRequirements(owner string, requirements ...runtimeOpeningRequirement) error {
	for _, requirement := range requirements {
		if missingRuntimeOpeningDependency(requirement.value) {
			return fmt.Errorf("Factory Sessions runtime-opening %s %s is required", owner, requirement.member)
		}
	}
	return nil
}

func missingRuntimeOpeningDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (f *Factory) openRuntime(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	effects ExternalEffects,
	logger *zap.Logger,
) (runtimeProducts, error) {
	return openRuntime(
		ctx, request, effects, logger,
		f.durableExecutionFactory,
		f.workerExecutionFactory,
		f.modelService,
		f.workFactory,
		f.automationFactory,
		f.factorySessionsService,
		f.factorySessionExecutionFactory,
		f.recordingsProjectionFactory,
		f.recordingLifecycleFactory,
		f.runtimeLedgerFactory,
		f.runtimeRecorderFactory,
		f.replayClockFactory,
		f.replayExecutionFactory,
		f.workersRuntimeFactory,
		f.workersRuntimeExecutorsFactory,
		f.workersMockCommandRunnerFactory,
		f.automationHostedSourcesFactory,
		f.workersLocalRuntimeHooksFactory,
		f.factoryDefinitionsFactory,
		f.factoryScaffoldInitializer,
		f.editableFactoryValidator,
		f.initialFactorySnapshotFactory,
		f.factoryRuntimeAssembler,
		f.workService,
		f.providerSessions,
		f.factoryDefinitionValidator,
		f.namedPaths,
		f.factoryWorkflows,
		f.workflowPreview,
		f.loadFactory,
		f.newLoadedFactory,
		f.decodeReplayConfig,
		f.replayInputs,
		f.captureLoadedFactorySnapshot,
		f.resolveClock,
		f.newSessionLogger,
		f.adaptWorkerCommandRunner,
		f.providerFromCommandRunnerFactory,
		f.processRuntimeFactory,
		f.ensureOperatorBackendScope,
		f.generateRuntimeInstanceID,
		f.resolveHome,
		f.providerIdentities,
	)
}

// OpenApplicationRuntime opens one Factory Session and returns only the roles
// required to assemble its process lifecycle and customer transports.
func (f *Factory) OpenApplicationRuntime(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	effects ExternalEffects,
	logger *zap.Logger,
) (roles.OpenedApplicationRuntime, error) {
	opened, err := f.openRuntime(ctx, request, effects, logger)
	return opened.application, err
}

// ModelsRoot returns the process-scoped accepted Models root used by runtime
// opening and Models CLI presentation collaborators.
func (f *Factory) ModelsRoot() models.Service {
	if f == nil {
		return nil
	}
	return f.modelService
}

// OpenInvocationRuntime opens one Factory Session and returns only the roles
// required by one-shot model or Factory invocation.
func (f *Factory) OpenInvocationRuntime(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	effects ExternalEffects,
	logger *zap.Logger,
) (roles.OpenedInvocationRuntime, error) {
	opened, err := f.openRuntime(ctx, request, effects, logger)
	return opened.invocation, err
}

// OpenExecutionRuntime opens one Factory Session and returns only the durable
// execution and workflow roles required by runtime-backed execution clients.
func (f *Factory) OpenExecutionRuntime(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	effects ExternalEffects,
	logger *zap.Logger,
) (roles.OpenedExecutionRuntime, error) {
	opened, err := f.openRuntime(ctx, request, effects, logger)
	return opened.execution, err
}
