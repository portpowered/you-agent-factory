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
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// WorkerCommandRunnerAdapter projects a replaceable low-level process effect
// into the Workers-owned command port.
type WorkerCommandRunnerAdapter func(platformprocess.CommandRunner) workers.CommandRunner

// ApplicationRuntimeOpening opens the application view of one Factory Sessions
// runtime. Consumers receive this narrow operation rather than the
// process-scoped grouped construction type.
type ApplicationRuntimeOpening interface {
	OpenApplicationRuntime(
		context.Context,
		*factorysessions.RuntimeOpeningRequest,
	) (roles.OpenedApplicationRuntime, error)
}

// InvocationRuntimeOpening opens the invocation-only view of one Factory
// Sessions runtime. Consumers receive this narrow operation rather than the
// process-scoped grouped construction type.
type InvocationRuntimeOpening interface {
	OpenInvocationRuntime(
		context.Context,
		*factorysessions.RuntimeOpeningRequest,
	) (roles.OpenedInvocationRuntime, error)
}

// ExecutionRuntimeOpening opens the durable-execution view of one Factory
// Sessions runtime. It keeps direct execution on the same authoritative
// opening capability while preserving its smaller operation surface.
type ExecutionRuntimeOpening interface {
	OpenExecutionRuntime(
		context.Context,
		*factorysessions.RuntimeOpeningRequest,
	) (roles.OpenedExecutionRuntime, error)
}

// The owner-port contracts below are the Factory Sessions-owned construction
// vocabulary for the one process-scoped runtime-opening factory. Each
// contract names one owner and contains only the fixed collaborators selected
// for that owner by canonical Wire composition. Runtime opening receives the
// contracts as separate constructor arguments; there is no aggregate
// dependency bag or secondary graph for an operation to consult.

// ProviderSessionsPorts contains the Provider Sessions-owned runtime
// collaborators.
type ProviderSessionsPorts struct {
	Service providersessions.Service
}

// FactoryRuntimePorts contains Factory Runtime's opening collaborators.
type FactoryRuntimePorts struct {
	Logger                          *zap.Logger
	FactoryWorkflows                factoryruntime.JavaScriptWorkflowDefinitions
	WorkflowPreview                 factoryruntime.WorkflowPreviewOperation
	WorkersRuntimeExecutorsFactory  factoryruntime.WorkersRuntimeExecutorsFactory
	ProviderInvocationFactory       factoryruntime.ProviderInvocationExecutorFactory
	WorkersMockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory
	FactoryRuntimeAssembler         FactoryRuntimeAssembler
	ResolveClock                    factoryruntime.ClockResolver
	NewSessionLogger                factoryruntime.SessionLoggerFactory
	Clock                           factoryruntime.Clock
	ProviderOverride                workers.Provider
	SubmissionRecorder              recordings.SubmissionRecorder
	DispatchRecorder                recordings.DispatchRecorder
}

// FactoryDefinitionsPorts contains Factory Definitions-owned opening
// collaborators.
type FactoryDefinitionsPorts struct {
	Validator                     factorydefinitions.Validator
	NamedPaths                    factorydefinitions.NamedPathResolver
	Factory                       FactoryDefinitionsFactory
	InitialFactorySnapshotFactory factorydefinitions.InitialFactorySnapshotFactory
	LoadFactory                   factorydefinitions.LoadedFactoryLoader
	NewLoadedFactory              factorydefinitions.LoadedFactorySourceFactory
	DecodeReplayConfig            factorydefinitions.ReplayRuntimeConfigDecoder
	CaptureLoadedFactorySnapshot  factorydefinitions.LoadedFactorySnapshotCapturer
}

// FactorySessionsPorts contains Factory Sessions-owned opening collaborators.
type FactorySessionsPorts struct {
	Service                        factorysessions.Service
	RuntimeAssembly                roles.RuntimeAssembly
	DurableExecutionFactory        DurableExecutionFactory
	FactorySessionExecutionFactory FactorySessionExecutionFactory
	FactoryScaffoldInitializer     factorysessions.FactoryScaffoldInitializer
	EditableFactoryValidator       factorysessions.EditableFactoryValidator
	ProcessRuntimeFactory          roles.ProcessRuntimeFactory
	GenerateRuntimeInstanceID      factorysessions.RuntimeInstanceIDGenerator
	ResolveHome                    factorysessions.HomeDirectoryResolver
	ProviderIdentities             factorysessions.ProviderIdentityResolver
	InvocationMetricsRecorder      roles.InvocationMetricsRecorder
}

// WorkPorts contains Work-owned opening collaborators.
type WorkPorts struct {
	Factory             WorkFactory
	ContentMaterializer work.ContentMaterializer
}

// AutomationsPorts contains Automations-owned opening collaborators.
type AutomationsPorts struct {
	Factory              AutomationFactory
	HostedSourcesFactory AutomationHostedSourcesFactory
}

// WebhooksPorts contains the Webhooks root used to attach hosted delivery to
// the runtime's canonical recording stream.
type WebhooksPorts struct {
	Service webhooks.Service
}

// ModelsPorts contains the Models root used while opening a session.
type ModelsPorts struct {
	Service models.Service
}

// RecordingsPorts contains Recordings-owned opening collaborators.
type RecordingsPorts struct {
	ProjectionFactory      RecordingsProjectionFactory
	ServiceFactory         RecordingsServiceFactory
	LifecycleFactory       RecordingLifecycleFactory
	RuntimeLedgerFactory   RuntimeLedgerFactory
	RuntimeRecorderFactory recordings.RuntimeRecorderFactory
	ReplayClockFactory     ReplayClockFactory
	ReplayExecutionFactory recordings.ReplayExecutionFactory
	ReplayInputs           recordings.ReplayInputLoader
}

// WorkersPorts contains Workers-owned opening collaborators.
type WorkersPorts struct {
	ExecutionFactory                 WorkerExecutionFactory
	RuntimeFactory                   WorkersRuntimeFactory
	LocalRuntimeHooksFactory         WorkersLocalRuntimeHooksFactory
	AdaptCommandRunner               WorkerCommandRunnerAdapter
	ProviderFromCommandRunnerFactory ProviderFromCommandRunnerFactory
	ProviderCommandRunner            ProviderCommandRunner
	ScriptCommandRunner              ScriptCommandRunner
}

// ProviderCommandRunner and ScriptCommandRunner are distinct Wire keys for
// the two Workers-owned command ports. They expose the same narrow Workers
// command contract without allowing Wire to bind one selected runner to both
// effect owners.
type ProviderCommandRunner interface {
	workers.CommandRunner
}

type ScriptCommandRunner interface {
	workers.CommandRunner
}

// OperatorSettingsPorts contains the Operator Settings capability used
// to establish the session backend scope.
type OperatorSettingsPorts struct {
	EnsureBackendScope operatorsettings.BackendScopeEnsurer
}

// Factory is the process-scoped, inert Factory Session opening operation.
// Wire selects all implementation functions and fixed owner effects once.
// Invocation and durable-execution openings receive only operation data;
// application transport binding is composed by the canonical Wire adapter.
type Factory struct {
	durableExecutionFactory          DurableExecutionFactory
	workerExecutionFactory           WorkerExecutionFactory
	modelService                     models.Service
	workFactory                      WorkFactory
	automationFactory                AutomationFactory
	factorySessionsService           factorysessions.Service
	factorySessionExecutionFactory   FactorySessionExecutionFactory
	recordingsProjectionFactory      RecordingsProjectionFactory
	recordingsServiceFactory         RecordingsServiceFactory
	recordingLifecycleFactory        RecordingLifecycleFactory
	webhooksService                  webhooks.Service
	runtimeLedgerFactory             RuntimeLedgerFactory
	runtimeRecorderFactory           recordings.RuntimeRecorderFactory
	replayClockFactory               ReplayClockFactory
	replayExecutionFactory           recordings.ReplayExecutionFactory
	workersRuntimeFactory            WorkersRuntimeFactory
	workersRuntimeExecutorsFactory   factoryruntime.WorkersRuntimeExecutorsFactory
	providerInvocationFactory        factoryruntime.ProviderInvocationExecutorFactory
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
	baseLogger                       *zap.Logger
	adaptWorkerCommandRunner         WorkerCommandRunnerAdapter
	providerFromCommandRunnerFactory ProviderFromCommandRunnerFactory
	processRuntimeFactory            roles.ProcessRuntimeFactory
	ensureOperatorBackendScope       operatorsettings.BackendScopeEnsurer
	generateRuntimeInstanceID        factorysessions.RuntimeInstanceIDGenerator
	resolveHome                      factorysessions.HomeDirectoryResolver
	providerIdentities               factorysessions.ProviderIdentityResolver
	factorySessionsRuntimeAssembly   roles.RuntimeAssembly
	clock                            factoryruntime.Clock
	providerOverride                 workers.Provider
	invocationMetricsRecorder        roles.InvocationMetricsRecorder
	providerCommandRunner            workers.CommandRunner
	scriptCommandRunner              workers.CommandRunner
	submissionRecorder               recordings.SubmissionRecorder
	dispatchRecorder                 recordings.DispatchRecorder
}

var (
	_ ApplicationRuntimeOpening = (*Factory)(nil)
	_ InvocationRuntimeOpening  = (*Factory)(nil)
	_ ExecutionRuntimeOpening   = (*Factory)(nil)
)

func NewFactory(
	providerSessions *ProviderSessionsPorts,
	factoryRuntime *FactoryRuntimePorts,
	factoryDefinitions *FactoryDefinitionsPorts,
	factorySessions *FactorySessionsPorts,
	workPorts *WorkPorts,
	automations *AutomationsPorts,
	modelsPorts *ModelsPorts,
	recordingsPorts *RecordingsPorts,
	webhooksPorts *WebhooksPorts,
	workersPorts *WorkersPorts,
	operatorSettings *OperatorSettingsPorts,
) (*Factory, error) {
	if err := validateOwnerPorts(
		providerSessions,
		factoryRuntime,
		factoryDefinitions,
		factorySessions,
		workPorts,
		automations,
		modelsPorts,
		recordingsPorts,
		webhooksPorts,
		workersPorts,
		operatorSettings,
	); err != nil {
		return nil, err
	}

	return &Factory{
		durableExecutionFactory:          factorySessions.DurableExecutionFactory,
		workerExecutionFactory:           workersPorts.ExecutionFactory,
		modelService:                     modelsPorts.Service,
		workFactory:                      workPorts.Factory,
		automationFactory:                automations.Factory,
		factorySessionsService:           factorySessions.Service,
		factorySessionsRuntimeAssembly:   factorySessions.RuntimeAssembly,
		factorySessionExecutionFactory:   factorySessions.FactorySessionExecutionFactory,
		recordingsProjectionFactory:      recordingsPorts.ProjectionFactory,
		recordingsServiceFactory:         recordingsPorts.ServiceFactory,
		recordingLifecycleFactory:        recordingsPorts.LifecycleFactory,
		webhooksService:                  webhooksPorts.Service,
		runtimeLedgerFactory:             recordingsPorts.RuntimeLedgerFactory,
		runtimeRecorderFactory:           recordingsPorts.RuntimeRecorderFactory,
		replayClockFactory:               recordingsPorts.ReplayClockFactory,
		replayExecutionFactory:           recordingsPorts.ReplayExecutionFactory,
		workersRuntimeFactory:            workersPorts.RuntimeFactory,
		workersRuntimeExecutorsFactory:   factoryRuntime.WorkersRuntimeExecutorsFactory,
		providerInvocationFactory:        factoryRuntime.ProviderInvocationFactory,
		workersMockCommandRunnerFactory:  factoryRuntime.WorkersMockCommandRunnerFactory,
		automationHostedSourcesFactory:   automations.HostedSourcesFactory,
		workersLocalRuntimeHooksFactory:  workersPorts.LocalRuntimeHooksFactory,
		factoryDefinitionsFactory:        factoryDefinitions.Factory,
		factoryScaffoldInitializer:       factorySessions.FactoryScaffoldInitializer,
		editableFactoryValidator:         factorySessions.EditableFactoryValidator,
		initialFactorySnapshotFactory:    factoryDefinitions.InitialFactorySnapshotFactory,
		factoryRuntimeAssembler:          factoryRuntime.FactoryRuntimeAssembler,
		workService:                      work.MaterializationService(workPorts.ContentMaterializer),
		providerSessions:                 providerSessions.Service,
		factoryDefinitionValidator:       factoryDefinitions.Validator,
		namedPaths:                       factoryDefinitions.NamedPaths,
		factoryWorkflows:                 factoryRuntime.FactoryWorkflows,
		workflowPreview:                  factoryRuntime.WorkflowPreview,
		loadFactory:                      factoryDefinitions.LoadFactory,
		newLoadedFactory:                 factoryDefinitions.NewLoadedFactory,
		decodeReplayConfig:               factoryDefinitions.DecodeReplayConfig,
		replayInputs:                     recordingsPorts.ReplayInputs,
		captureLoadedFactorySnapshot:     factoryDefinitions.CaptureLoadedFactorySnapshot,
		resolveClock:                     factoryRuntime.ResolveClock,
		newSessionLogger:                 factoryRuntime.NewSessionLogger,
		baseLogger:                       factoryRuntime.Logger,
		adaptWorkerCommandRunner:         workersPorts.AdaptCommandRunner,
		providerFromCommandRunnerFactory: workersPorts.ProviderFromCommandRunnerFactory,
		processRuntimeFactory:            factorySessions.ProcessRuntimeFactory,
		ensureOperatorBackendScope:       operatorSettings.EnsureBackendScope,
		generateRuntimeInstanceID:        factorySessions.GenerateRuntimeInstanceID,
		resolveHome:                      factorySessions.ResolveHome,
		providerIdentities:               factorySessions.ProviderIdentities,
		clock:                            factoryRuntime.Clock,
		providerOverride:                 factoryRuntime.ProviderOverride,
		invocationMetricsRecorder:        factorySessions.InvocationMetricsRecorder,
		providerCommandRunner:            workersPorts.ProviderCommandRunner,
		scriptCommandRunner:              workersPorts.ScriptCommandRunner,
		submissionRecorder:               factoryRuntime.SubmissionRecorder,
		dispatchRecorder:                 factoryRuntime.DispatchRecorder,
	}, nil
}

// validateOwnerPorts checks the fixed owner contracts in declaration order.
// It deliberately performs no collaborator calls, so an incomplete process
// graph fails before any operation-scoped work can begin.
func validateOwnerPorts(
	providerSessions *ProviderSessionsPorts,
	factoryRuntime *FactoryRuntimePorts,
	factoryDefinitions *FactoryDefinitionsPorts,
	factorySessions *FactorySessionsPorts,
	workPorts *WorkPorts,
	automations *AutomationsPorts,
	modelsPorts *ModelsPorts,
	recordingsPorts *RecordingsPorts,
	webhooksPorts *WebhooksPorts,
	workersPorts *WorkersPorts,
	operatorSettings *OperatorSettingsPorts,
) error {
	for _, validate := range []func() error{
		func() error { return validateProviderSessions(providerSessions) },
		func() error { return validateFactoryRuntime(factoryRuntime) },
		func() error { return validateFactoryDefinitions(factoryDefinitions) },
		func() error { return validateFactorySessions(factorySessions) },
		func() error { return validateWork(workPorts) },
		func() error { return validateAutomations(automations) },
		func() error { return validateModels(modelsPorts) },
		func() error { return validateRecordings(recordingsPorts) },
		func() error { return validateWebhooks(webhooksPorts) },
		func() error { return validateWorkers(workersPorts) },
		func() error { return validateOperatorSettings(operatorSettings) },
	} {
		if err := validate(); err != nil {
			return err
		}
	}
	return nil
}

func validateProviderSessions(group *ProviderSessionsPorts) error {
	if err := requireRuntimeOpeningPorts("Provider Sessions", group); err != nil {
		return err
	}
	return validateRuntimeOpeningRequirements("Provider Sessions",
		runtimeOpeningRequirement{"service", group.Service},
	)
}

func validateFactoryRuntime(group *FactoryRuntimePorts) error {
	if err := requireRuntimeOpeningPorts("Factory Runtime", group); err != nil {
		return err
	}
	return validateRuntimeOpeningRequirements("Factory Runtime",
		runtimeOpeningRequirement{"logger", group.Logger},
		runtimeOpeningRequirement{"JavaScript workflow definitions", group.FactoryWorkflows},
		runtimeOpeningRequirement{"workflow preview operation", group.WorkflowPreview},
		runtimeOpeningRequirement{"Workers runtime executors factory", group.WorkersRuntimeExecutorsFactory},
		runtimeOpeningRequirement{"provider-invocation executor factory", group.ProviderInvocationFactory},
		runtimeOpeningRequirement{"Workers mock command runner factory", group.WorkersMockCommandRunnerFactory},
		runtimeOpeningRequirement{"runtime assembler", group.FactoryRuntimeAssembler},
		runtimeOpeningRequirement{"clock resolver", group.ResolveClock},
		runtimeOpeningRequirement{"session logger factory", group.NewSessionLogger},
		runtimeOpeningRequirement{"clock", group.Clock},
	)
}

func validateFactoryDefinitions(group *FactoryDefinitionsPorts) error {
	if err := requireRuntimeOpeningPorts("Factory Definitions", group); err != nil {
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

func validateFactorySessions(group *FactorySessionsPorts) error {
	if err := requireRuntimeOpeningPorts("Factory Sessions", group); err != nil {
		return err
	}
	return validateRuntimeOpeningRequirements("Factory Sessions",
		runtimeOpeningRequirement{"service", group.Service},
		runtimeOpeningRequirement{"runtime assembly", group.RuntimeAssembly},
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

func validateWork(group *WorkPorts) error {
	if err := requireRuntimeOpeningPorts("Work", group); err != nil {
		return err
	}
	return validateRuntimeOpeningRequirements("Work",
		runtimeOpeningRequirement{"factory", group.Factory},
		runtimeOpeningRequirement{"content materializer", group.ContentMaterializer},
	)
}

func validateAutomations(group *AutomationsPorts) error {
	if err := requireRuntimeOpeningPorts("Automations", group); err != nil {
		return err
	}
	return validateRuntimeOpeningRequirements("Automations",
		runtimeOpeningRequirement{"factory", group.Factory},
		runtimeOpeningRequirement{"hosted sources factory", group.HostedSourcesFactory},
	)
}

func validateModels(group *ModelsPorts) error {
	if err := requireRuntimeOpeningPorts("Models", group); err != nil {
		return err
	}
	return validateRuntimeOpeningRequirements("Models",
		runtimeOpeningRequirement{"service", group.Service},
	)
}

func validateRecordings(group *RecordingsPorts) error {
	if err := requireRuntimeOpeningPorts("Recordings", group); err != nil {
		return err
	}
	return validateRuntimeOpeningRequirements("Recordings",
		runtimeOpeningRequirement{"projection factory", group.ProjectionFactory},
		runtimeOpeningRequirement{"service factory", group.ServiceFactory},
		runtimeOpeningRequirement{"lifecycle factory", group.LifecycleFactory},
		runtimeOpeningRequirement{"runtime ledger factory", group.RuntimeLedgerFactory},
		runtimeOpeningRequirement{"runtime recorder factory", group.RuntimeRecorderFactory},
		runtimeOpeningRequirement{"replay clock factory", group.ReplayClockFactory},
		runtimeOpeningRequirement{"replay execution factory", group.ReplayExecutionFactory},
		runtimeOpeningRequirement{"replay input loader", group.ReplayInputs},
	)
}

func validateWorkers(group *WorkersPorts) error {
	if err := requireRuntimeOpeningPorts("Workers", group); err != nil {
		return err
	}
	return validateRuntimeOpeningRequirements("Workers",
		runtimeOpeningRequirement{"execution factory", group.ExecutionFactory},
		runtimeOpeningRequirement{"runtime factory", group.RuntimeFactory},
		runtimeOpeningRequirement{"local runtime hooks factory", group.LocalRuntimeHooksFactory},
		runtimeOpeningRequirement{"command runner adapter", group.AdaptCommandRunner},
		runtimeOpeningRequirement{"provider-from-command-runner factory", group.ProviderFromCommandRunnerFactory},
		runtimeOpeningRequirement{"provider command runner", group.ProviderCommandRunner},
		runtimeOpeningRequirement{"script command runner", group.ScriptCommandRunner},
	)
}

func validateWebhooks(group *WebhooksPorts) error {
	if err := requireRuntimeOpeningPorts("Webhooks", group); err != nil {
		return err
	}
	return validateRuntimeOpeningRequirements("Webhooks",
		runtimeOpeningRequirement{"service", group.Service},
	)
}

func validateOperatorSettings(group *OperatorSettingsPorts) error {
	if err := requireRuntimeOpeningPorts("Operator Settings", group); err != nil {
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

func requireRuntimeOpeningPorts(owner string, ports any) error {
	if missingRuntimeOpeningDependency(ports) {
		return fmt.Errorf("Factory Sessions runtime-opening %s owner ports are required", owner)
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
	logger *zap.Logger,
) (runtimeProducts, error) {
	return openRuntime(
		ctx, request, logger,
		f.clock,
		f.providerOverride,
		f.invocationMetricsRecorder,
		f.providerCommandRunner,
		f.scriptCommandRunner,
		f.submissionRecorder,
		f.dispatchRecorder,
		f.durableExecutionFactory,
		f.workerExecutionFactory,
		f.modelService,
		f.workFactory,
		f.automationFactory,
		f.factorySessionsRuntimeAssembly,
		f.factorySessionExecutionFactory,
		f.recordingsProjectionFactory,
		f.recordingsServiceFactory,
		f.recordingLifecycleFactory,
		f.runtimeLedgerFactory,
		f.runtimeRecorderFactory,
		f.replayClockFactory,
		f.replayExecutionFactory,
		f.workersRuntimeFactory,
		f.workersRuntimeExecutorsFactory,
		f.providerInvocationFactory,
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
		f.webhooksService,
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
) (roles.OpenedApplicationRuntime, error) {
	opened, err := f.openRuntime(ctx, request, f.baseLogger)
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
) (roles.OpenedInvocationRuntime, error) {
	opened, err := f.openRuntime(ctx, request, f.baseLogger)
	return opened.invocation, err
}

// OpenExecutionRuntime opens one Factory Session and returns only the durable
// execution and workflow roles required by runtime-backed execution clients.
func (f *Factory) OpenExecutionRuntime(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
) (roles.OpenedExecutionRuntime, error) {
	opened, err := f.openRuntime(ctx, request, f.baseLogger)
	return opened.execution, err
}
