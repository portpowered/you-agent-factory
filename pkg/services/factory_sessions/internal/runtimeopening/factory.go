package runtimeopening

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// WorkerCommandRunnerAdapter is a composition-only identity adapter for the
// shared platform process effect. Workers performs its richer adaptation
// privately at its own service boundary.
type WorkerCommandRunnerAdapter func(platformprocess.CommandRunner) platformprocess.CommandRunner

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

// ProviderOverrideService is the optional request-scoped Providers root
// replacement selected by process composition. The distinct interface keeps
// Wire from confusing the override with the process-owned Providers root when
// both are available in the same provider set.
type ProviderOverrideService interface {
	providers.Service
}

// FactoryRuntimePorts contains Factory Runtime's opening collaborators.
type FactoryRuntimePorts struct {
	Logger                          *zap.Logger
	FactoryWorkflows                factoryruntime.JavaScriptWorkflowDefinitions
	WorkflowPreview                 factoryruntime.WorkflowPreviewOperation
	WorkersMockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory
	FactoryRuntimeAssembler         FactoryRuntimeAssembler
	RuntimeRootFactory              RuntimeRootFactory
	ResolveClock                    factoryruntime.ClockResolver
	NewSessionLogger                factoryruntime.SessionLoggerFactory
	Clock                           factoryruntime.Clock
	ProviderOverride                ProviderOverrideService
	SubmissionRecorder              recordings.SubmissionRecorder
	DispatchRecorder                recordings.DispatchRecorder
}

// FactoryDefinitionsPorts contains Factory Definitions-owned opening
// collaborators.
type FactoryDefinitionsPorts struct {
	Validator                     factorydefinitions.Validator
	NamedPaths                    factorydefinitions.NamedPathResolver
	Service                       factorydefinitions.Service
	RuntimeRouter                 *factorysessions.DefinitionRuntimeRouter
	InitialFactorySnapshotFactory factorydefinitions.InitialFactorySnapshotFactory
	LoadFactory                   factorydefinitions.LoadedFactoryLoader
	NewLoadedFactory              factorydefinitions.LoadedFactorySourceFactory
	DecodeReplayConfig            factorydefinitions.ReplayRuntimeConfigDecoder
	CaptureLoadedFactorySnapshot  factorydefinitions.LoadedFactorySnapshotCapturer
}

// FactorySessionsPorts contains Factory Sessions-owned opening collaborators.
type FactorySessionsPorts struct {
	RuntimeAssembly                roles.RuntimeAssembly
	DurableExecutionFactory        DurableExecutionFactory
	FactorySessionExecutionFactory FactorySessionExecutionFactory
	FactoryScaffoldInitializer     factorysessions.FactoryScaffoldInitializer
	EditableFactoryValidator       factorysessions.EditableFactoryValidator
	ProcessRuntimeFactory          roles.ProcessRuntimeFactory
	GenerateSessionID              factorysessions.SessionIDGenerator
	GenerateRuntimeInstanceID      factorysessions.RuntimeInstanceIDGenerator
	ResolveHome                    factorysessions.HomeDirectoryResolver
	ProviderIdentities             factorysessions.ProviderIdentityResolver
	InvocationMetricsRecorder      roles.InvocationMetricsRecorder
}

// WorkPorts contains Work-owned opening collaborators.
type WorkPorts struct {
	Service work.Service
}

// AutomationsPorts contains Automations-owned opening collaborators.
type AutomationsPorts struct {
	Service automations.Service
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
	Service recordings.Service
	Runtime recordings.RuntimeOpening
}

// WorkersPorts contains Workers-owned opening collaborators.
type WorkersPorts struct {
	// Service is the one process-scoped Workers root. Factory Session opening
	// consumes it as a capability and never constructs a replacement runtime.
	Service                          workers.Service
	ProviderFromCommandRunnerFactory ProviderFromCommandRunnerFactory
	ProviderCommandRunner            ProviderCommandRunner
	ScriptCommandRunner              ScriptCommandRunner
}

// ProviderCommandRunner and ScriptCommandRunner are distinct Wire keys for
// the two platform process effects. They remain separate so composition
// cannot accidentally bind one selected runner to both effect owners.
type ProviderCommandRunner interface {
	platformprocess.CommandRunner
}

type ScriptCommandRunner interface {
	platformprocess.CommandRunner
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
	workerService                    workers.Service
	modelService                     models.Service
	automationService                automations.Service
	factorySessionExecutionFactory   FactorySessionExecutionFactory
	recordingsService                recordings.Service
	recordingsRuntime                recordings.RuntimeOpening
	replayInputs                     recordings.ReplayInputLoader
	webhooksService                  webhooks.Service
	workersMockCommandRunnerFactory  factoryruntime.WorkersMockCommandRunnerFactory
	factoryDefinitions               factorydefinitions.Service
	definitionRuntimeRouter          *factorysessions.DefinitionRuntimeRouter
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
	captureLoadedFactorySnapshot     factorydefinitions.LoadedFactorySnapshotCapturer
	resolveClock                     factoryruntime.ClockResolver
	newSessionLogger                 factoryruntime.SessionLoggerFactory
	baseLogger                       *zap.Logger
	providerFromCommandRunnerFactory ProviderFromCommandRunnerFactory
	processRuntimeFactory            roles.ProcessRuntimeFactory
	ensureOperatorBackendScope       operatorsettings.BackendScopeEnsurer
	generateSessionID                factorysessions.SessionIDGenerator
	generateRuntimeInstanceID        factorysessions.RuntimeInstanceIDGenerator
	resolveHome                      factorysessions.HomeDirectoryResolver
	providerIdentities               factorysessions.ProviderIdentityResolver
	factorySessionsRuntimeAssembly   roles.RuntimeAssembly
	runtimeRoot                      FactoryRuntimeRoot
	clock                            factoryruntime.Clock
	providerOverride                 providers.Service
	invocationMetricsRecorder        roles.InvocationMetricsRecorder
	providerCommandRunner            platformprocess.CommandRunner
	scriptCommandRunner              platformprocess.CommandRunner
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
	automationsPorts *AutomationsPorts,
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
		automationsPorts,
		modelsPorts,
		recordingsPorts,
		webhooksPorts,
		workersPorts,
		operatorSettings,
	); err != nil {
		return nil, err
	}

	factory := &Factory{
		durableExecutionFactory:          factorySessions.DurableExecutionFactory,
		workerService:                    workersPorts.Service,
		modelService:                     modelsPorts.Service,
		automationService:                automationsPorts.Service,
		factorySessionsRuntimeAssembly:   factorySessions.RuntimeAssembly,
		factorySessionExecutionFactory:   factorySessions.FactorySessionExecutionFactory,
		recordingsService:                recordingsPorts.Service,
		recordingsRuntime:                recordingsPorts.Runtime,
		replayInputs:                     recordingsPorts.Runtime,
		webhooksService:                  webhooksPorts.Service,
		workersMockCommandRunnerFactory:  factoryRuntime.WorkersMockCommandRunnerFactory,
		factoryDefinitions:               factoryDefinitions.Service,
		definitionRuntimeRouter:          factoryDefinitions.RuntimeRouter,
		factoryScaffoldInitializer:       factorySessions.FactoryScaffoldInitializer,
		editableFactoryValidator:         factorySessions.EditableFactoryValidator,
		initialFactorySnapshotFactory:    factoryDefinitions.InitialFactorySnapshotFactory,
		factoryRuntimeAssembler:          factoryRuntime.FactoryRuntimeAssembler,
		workService:                      workPorts.Service,
		providerSessions:                 providerSessions.Service,
		factoryDefinitionValidator:       factoryDefinitions.Validator,
		namedPaths:                       factoryDefinitions.NamedPaths,
		factoryWorkflows:                 factoryRuntime.FactoryWorkflows,
		workflowPreview:                  factoryRuntime.WorkflowPreview,
		loadFactory:                      factoryDefinitions.LoadFactory,
		newLoadedFactory:                 factoryDefinitions.NewLoadedFactory,
		decodeReplayConfig:               factoryDefinitions.DecodeReplayConfig,
		captureLoadedFactorySnapshot:     factoryDefinitions.CaptureLoadedFactorySnapshot,
		resolveClock:                     factoryRuntime.ResolveClock,
		newSessionLogger:                 factoryRuntime.NewSessionLogger,
		baseLogger:                       factoryRuntime.Logger,
		providerFromCommandRunnerFactory: workersPorts.ProviderFromCommandRunnerFactory,
		processRuntimeFactory:            factorySessions.ProcessRuntimeFactory,
		generateSessionID:                factorySessions.GenerateSessionID,
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
	}
	if factoryRuntime.RuntimeRootFactory != nil {
		runtimeRoot, err := factoryRuntime.RuntimeRootFactory(factory.activateRuntime)
		if err != nil {
			return nil, fmt.Errorf("construct Factory Runtime root: %w", err)
		}
		factory.runtimeRoot = runtimeRoot
	}
	return factory, nil
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
		runtimeOpeningRequirement{"service", group.Service},
		runtimeOpeningRequirement{"runtime router", group.RuntimeRouter},
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
		runtimeOpeningRequirement{"runtime assembly", group.RuntimeAssembly},
		runtimeOpeningRequirement{"durable execution factory", group.DurableExecutionFactory},
		runtimeOpeningRequirement{"session execution factory", group.FactorySessionExecutionFactory},
		runtimeOpeningRequirement{"factory scaffold initializer", group.FactoryScaffoldInitializer},
		runtimeOpeningRequirement{"editable factory validator", group.EditableFactoryValidator},
		runtimeOpeningRequirement{"process runtime factory", group.ProcessRuntimeFactory},
		runtimeOpeningRequirement{"session ID generator", group.GenerateSessionID},
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
		runtimeOpeningRequirement{"service", group.Service},
	)
}

func validateAutomations(group *AutomationsPorts) error {
	if err := requireRuntimeOpeningPorts("Automations", group); err != nil {
		return err
	}
	return validateRuntimeOpeningRequirements("Automations",
		runtimeOpeningRequirement{"service", group.Service},
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
		runtimeOpeningRequirement{"service", group.Service},
		runtimeOpeningRequirement{"runtime", group.Runtime},
	)
}

func validateWorkers(group *WorkersPorts) error {
	if err := requireRuntimeOpeningPorts("Workers", group); err != nil {
		return err
	}
	if missingRuntimeOpeningDependency(group.Service) {
		return fmt.Errorf("Factory Sessions runtime-opening Workers service is required")
	}
	return validateRuntimeOpeningRequirements("Workers",
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
	return f.openRuntimeWithOptions(ctx, request, logger, nil, nil)
}

func (f *Factory) openRuntimeWithSnapshot(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	logger *zap.Logger,
	definitionSnapshot *factorydefinitions.RuntimeSnapshot,
) (runtimeProducts, error) {
	return f.openRuntimeWithOptions(ctx, request, logger, definitionSnapshot, nil)
}

func (f *Factory) openRuntimeWithReplayInput(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	logger *zap.Logger,
	replayInput *recordings.LoadReplayInputResult,
) (runtimeProducts, error) {
	return f.openRuntimeWithOptions(ctx, request, logger, nil, replayInput)
}

func (f *Factory) openRuntimeWithOptions(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	logger *zap.Logger,
	definitionSnapshot *factorydefinitions.RuntimeSnapshot,
	replayInput *recordings.LoadReplayInputResult,
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
		f.workerService,
		f.modelService,
		f.automationService,
		f.factorySessionsRuntimeAssembly,
		f.factorySessionExecutionFactory,
		f.recordingsService,
		f.recordingsRuntime,
		f.workersMockCommandRunnerFactory,
		f.factoryDefinitions,
		f.definitionRuntimeRouter,
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
		f.captureLoadedFactorySnapshot,
		f.webhooksService,
		f.resolveClock,
		f.newSessionLogger,
		f.providerFromCommandRunnerFactory,
		f.processRuntimeFactory,
		f.ensureOperatorBackendScope,
		f.generateRuntimeInstanceID,
		f.resolveHome,
		f.providerIdentities,
		definitionSnapshot,
		replayInput,
	)
}

// OpenApplicationRuntime opens one Factory Session and returns only the roles
// required to assemble its process lifecycle and customer transports.
func (f *Factory) OpenApplicationRuntime(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
) (roles.OpenedApplicationRuntime, error) {
	opened, err := f.openForRequest(ctx, request)
	return opened.application, err
}

func (f *Factory) openForRequest(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
) (runtimeProducts, error) {
	// Historical portable replay is an inspection-only product and must not
	// acquire live Runtime state. Legacy replay artifacts still use the normal
	// activation path and therefore retain the live replay behavior.
	if request != nil && request.Recordings.ReplayPath != "" {
		// A compatibility Factory without a Runtime root still needs the direct
		// historical/replay opener used by narrow tests and migration callers.
		// Canonical Wire always supplies the root, so classify the input there
		// before deciding whether activation is required.
		if f.runtimeRoot == nil || f.replayInputs == nil {
			return f.openRuntime(ctx, request, f.baseLogger)
		}
		input, err := f.replayInputs.LoadReplayInput(
			recordings.LoadReplayInputRequest{Path: request.Recordings.ReplayPath},
		)
		if err != nil || input.Portable != nil || input.Legacy == nil || input.Legacy.Factory == nil {
			if err != nil {
				// The loader has already classified and safely detached the
				// replay input. Propagating that result preserves the one-read
				// runtime-opening contract; routing the error through openRuntime
				// would ask the same loader to read the artifact again.
				return runtimeProducts{}, err
			}
			return f.openRuntimeWithReplayInput(ctx, request, f.baseLogger, &input)
		}
		return f.openActivatedRuntimeWithReplayInput(ctx, request, &input)
	}
	if request != nil && strings.TrimSpace(request.Recordings.ResumePath) != "" {
		if f.recordingsRuntime == nil {
			return runtimeProducts{}, fmt.Errorf("open Factory Runtime: Recordings resume input capability is required")
		}
		input, err := f.recordingsRuntime.LoadResumeInput(recordings.LoadResumeInputRequest{
			Path: request.Recordings.ResumePath,
		})
		if err != nil {
			return runtimeProducts{}, fmt.Errorf("open Factory Runtime: load resume input: %w", err)
		}
		return f.openActivatedRuntimeWithResumeInput(ctx, request, &input)
	}
	return f.openActivatedRuntime(ctx, request)
}

// OpenInvocationRuntime opens one Factory Session and returns only the roles
// required by one-shot model or Factory invocation.
func (f *Factory) OpenInvocationRuntime(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
) (roles.OpenedInvocationRuntime, error) {
	opened, err := f.openForRequest(ctx, request)
	return opened.invocation, err
}

// OpenExecutionRuntime opens one Factory Session and returns only the durable
// execution and workflow roles required by runtime-backed execution clients.
func (f *Factory) OpenExecutionRuntime(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
) (roles.OpenedExecutionRuntime, error) {
	opened, err := f.openForRequest(ctx, request)
	return opened.execution, err
}
