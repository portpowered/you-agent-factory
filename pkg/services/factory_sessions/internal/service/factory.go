package service

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
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	legacyservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// WorkerCommandRunnerAdapter is a composition-only identity adapter for the
// shared platform process effect. Workers performs its richer adaptation
// privately at its own service boundary.
type WorkerCommandRunnerAdapter func(platformprocess.CommandRunner) platformprocess.CommandRunner

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
	RuntimeRoot                     FactoryRuntimeRoot
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

// Root owns the process-scoped Factory Sessions state and fixed collaborators.
// Its Assembly is bound after the other process services have been composed.
type Root struct {
	*legacyservice.Assembly
	startFlights                     singleflight.Group
	liveChangeCoordinator            factorysessioncontracts.LiveChangeCoordinator
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

func NewRoot(
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
) (*Root, error) {
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

	root := &Root{
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
	root.runtimeRoot = factoryRuntime.RuntimeRoot
	return root, nil
}

func (r *Root) buildProcessDurableExecution() (durableexecution.Service, error) {
	home, err := r.resolveHome()
	if err != nil {
		return nil, fmt.Errorf("construct Factory Sessions durable owner: resolve home: %w", err)
	}
	processDurable, err := r.factorySessionExecutionFactory(
		home,
		factorysessions.PersistencePolicyEnabled,
		r.providerOverride,
		r.clock,
		nil,
		factoryruntime.JavaScriptWorkerSettings{},
		nil,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("construct Factory Sessions durable owner: %w", err)
	}
	if binder, ok := processDurable.(interface {
		SetWorkerExecution(interface {
			Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)
		}, factoryruntime.ResourceCapacityLeaseAdmission, string, string, providers.Service, *workers.MockWorkersConfig, platformprocess.CommandRunner)
	}); ok {
		binder.SetWorkerExecution(r.workerService, nil, "", "", r.providerOverride, nil, nil)
	}
	return processDurable, nil
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

func (r *Root) openRuntime(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	logger *zap.Logger,
) (runtimeProducts, error) {
	return r.openRuntimeWithOptions(ctx, request, logger, nil, nil)
}

func (r *Root) openRuntimeWithSnapshot(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	logger *zap.Logger,
	definitionSnapshot *factorydefinitions.RuntimeSnapshot,
) (runtimeProducts, error) {
	return r.openRuntimeWithOptions(ctx, request, logger, definitionSnapshot, nil)
}

func (r *Root) openRuntimeWithReplayInput(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	logger *zap.Logger,
	replayInput *recordings.LoadReplayInputResult,
) (runtimeProducts, error) {
	return r.openRuntimeWithOptions(ctx, request, logger, nil, replayInput)
}

func (r *Root) openRuntimeWithOptions(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	logger *zap.Logger,
	definitionSnapshot *factorydefinitions.RuntimeSnapshot,
	replayInput *recordings.LoadReplayInputResult,
) (runtimeProducts, error) {
	return openRuntime(
		ctx, request, logger,
		r.clock,
		r.providerOverride,
		r.invocationMetricsRecorder,
		r.providerCommandRunner,
		r.scriptCommandRunner,
		r.submissionRecorder,
		r.dispatchRecorder,
		r.durableExecutionFactory,
		r.workerService,
		r.modelService,
		r.automationService,
		r.factorySessionsRuntimeAssembly,
		r.factorySessionExecutionFactory,
		r.recordingsService,
		r.recordingsRuntime,
		r.workersMockCommandRunnerFactory,
		r.factoryDefinitions,
		r.definitionRuntimeRouter,
		r.factoryScaffoldInitializer,
		r.editableFactoryValidator,
		r.initialFactorySnapshotFactory,
		r.factoryRuntimeAssembler,
		r.workService,
		r.providerSessions,
		r.factoryDefinitionValidator,
		r.namedPaths,
		r.factoryWorkflows,
		r.workflowPreview,
		r.loadFactory,
		r.newLoadedFactory,
		r.decodeReplayConfig,
		r.captureLoadedFactorySnapshot,
		r.webhooksService,
		r.resolveClock,
		r.newSessionLogger,
		r.providerFromCommandRunnerFactory,
		r.processRuntimeFactory,
		r.ensureOperatorBackendScope,
		r.generateRuntimeInstanceID,
		r.resolveHome,
		r.providerIdentities,
		definitionSnapshot,
		replayInput,
	)
}

func (r *Root) openForRequest(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
) (runtimeProducts, error) {
	// Historical replay, whether portable or legacy, is an inspection-only
	// product and must select its detached projection before live Factory
	// Runtime assembly. Resume remains an explicit live successor path below.
	if request != nil && request.Recordings.ReplayPath != "" {
		// A compatibility Factory without a Runtime root still needs the direct
		// historical/replay opener used by narrow tests and migration callers.
		// Canonical Wire always supplies the root, so classify the input there
		// before deciding whether activation is required.
		if r.runtimeRoot == nil || r.replayInputs == nil {
			return r.openRuntime(ctx, request, r.baseLogger)
		}
		input, err := r.replayInputs.LoadReplayInput(
			recordings.LoadReplayInputRequest{Path: request.Recordings.ReplayPath},
		)
		if err != nil {
			// The loader has already classified and safely detached the
			// replay input. Propagating that result preserves the one-read
			// runtime-opening contract; routing the error through openRuntime
			// would ask the same loader to read the artifact again.
			return runtimeProducts{}, err
		}
		// Offline replay is a detached historical inspection. A caller that
		// explicitly requested a hosted process still owns the established
		// ordinary replay contract, which exposes the replay through its live
		// API and metrics surfaces. Keeping that distinction here prevents the
		// inspection-only product from being wrapped in host-readiness or live
		// transport lifecycle requirements.
		if replayRequestsHistoricalInspection(request) && selectsHistoricalReplayInspection(input) {
			return r.openRuntimeWithReplayInput(ctx, request, r.baseLogger, &input)
		}
		// Hosted replay and legacy V1 JSON retain the ordinary activated runtime
		// path. Keep intentionally incomplete synthetic inputs used by narrow
		// compatibility callers on that same path; the real Recordings loader
		// reports the format before this branch.
		return r.openActivatedRuntimeWithReplayInput(ctx, request, &input)
	}
	if request != nil && strings.TrimSpace(request.Recordings.ResumePath) != "" {
		if r.recordingsRuntime == nil {
			return runtimeProducts{}, fmt.Errorf("open Factory Runtime: Recordings resume input capability is required")
		}
		input, err := r.recordingsRuntime.LoadResumeInput(recordings.LoadResumeInputRequest{
			Path: request.Recordings.ResumePath,
		})
		if err != nil {
			return runtimeProducts{}, fmt.Errorf("open Factory Runtime: load resume input: %w", err)
		}
		return r.openActivatedRuntimeWithResumeInput(ctx, request, &input)
	}
	return r.openActivatedRuntime(ctx, request)
}

func replayRequestsHistoricalInspection(request *factorysessions.RuntimeOpeningRequest) bool {
	if request == nil {
		return false
	}
	host := request.FactorySession.Host
	// The CLI clears Port when the API transport is disabled but retains the
	// parsed AutoPort default. AutoPort is meaningful only with a concrete
	// listener request, so the effective hosting decision is the resolved port.
	return host.Port <= 0
}

func selectsHistoricalReplayInspection(input recordings.LoadReplayInputResult) bool {
	if input.Portable != nil || input.Legacy == nil {
		return true
	}
	if input.LegacyFormat != "" {
		return input.LegacyFormat == string(recordings.RecordedSessionFormatV2JSONL)
	}
	// Preserve the established compatibility contract for narrow callers that
	// return a legacy artifact without the newer framing metadata. Production
	// path loaders always identify V1 versus V2 above.
	return legacyReplayArtifactHasCanonicalEventShape(*input.Legacy)
}
