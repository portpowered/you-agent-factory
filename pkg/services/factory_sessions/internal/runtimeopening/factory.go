package runtimeopening

import (
	"context"
	"fmt"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
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

// Factory is the process-scoped, inert Factory Session opening operation.
// Wire selects all implementation functions once; OpenRuntime supplies only
// invocation data and external edges.
type Factory struct {
	durableExecutionFactory         DurableExecutionFactory
	workerExecutionFactory          WorkerExecutionFactory
	modelService                    models.Service
	workFactory                     WorkFactory
	automationFactory               AutomationFactory
	factorySessionsService          factorysessions.Service
	factorySessionExecutionFactory  FactorySessionExecutionFactory
	recordingsProjectionFactory     RecordingsProjectionFactory
	recordingsFactory               RecordingsFactory
	runtimeLedgerFactory            RuntimeLedgerFactory
	runtimeRecorderFactory          recordings.RuntimeRecorderFactory
	replayClockFactory              ReplayClockFactory
	replayExecutionFactory          recordings.ReplayExecutionFactory
	workersRuntimeFactory           WorkersRuntimeFactory
	workersRuntimeExecutorsFactory  factoryruntime.WorkersRuntimeExecutorsFactory
	workersMockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory
	workerHostedPollersFactory      WorkerHostedPollersFactory
	workersLocalRuntimeHooksFactory WorkersLocalRuntimeHooksFactory
	factoryDefinitionsFactory       FactoryDefinitionsFactory
	factoryScaffoldInitializer      factorysessions.FactoryScaffoldInitializer
	editableFactoryValidator        factorysessions.EditableFactoryValidator
	initialFactorySnapshotFactory   factorydefinitions.InitialFactorySnapshotFactory
	factoryRuntimeAssembler         FactoryRuntimeAssembler
	contentMaterializer             work.ContentMaterializer
	providerSessions                providersessions.Service
	factoryDefinitionValidator      factorydefinitions.Validator
	namedPaths                      factorydefinitions.NamedPathResolver
	factoryWorkflows                factoryruntime.JavaScriptWorkflowDefinitions
	workflowPreview                 factoryruntime.WorkflowPreviewOperation
	loadFactory                     factorydefinitions.LoadedFactoryLoader
	newLoadedFactory                factorydefinitions.LoadedFactorySourceFactory
	decodeReplayConfig              factorydefinitions.ReplayRuntimeConfigDecoder
	loadReplay                      recordings.ReplayArtifactLoader
	captureLoadedFactorySnapshot    factorydefinitions.LoadedFactorySnapshotCapturer
	resolveClock                    factoryruntime.ClockResolver
	newSessionLogger                factoryruntime.SessionLoggerFactory
	adaptWorkerCommandRunner        WorkerCommandRunnerAdapter
	providerFromCommandRunnerFactory ProviderFromCommandRunnerFactory
	processRuntimeFactory           roles.ProcessRuntimeFactory
	ensureOperatorBackendScope      operatorsettings.BackendScopeEnsurer
	generateRuntimeInstanceID       factorysessions.RuntimeInstanceIDGenerator
	resolveHome                     factorysessions.HomeDirectoryResolver
	replayFiles                     fileeffects.ReplayRecordingReader
	providerIdentities              factorysessions.ProviderIdentityResolver
}

// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func NewFactory(
	providerSessions providersessions.Service,
	factoryWorkflows factoryruntime.JavaScriptWorkflowDefinitions,
	workflowPreview factoryruntime.WorkflowPreviewOperation,
	factoryDefinitionValidator factorydefinitions.Validator,
	namedPaths factorydefinitions.NamedPathResolver,
	durableExecutionFactory DurableExecutionFactory,
	workerExecutionFactory WorkerExecutionFactory,
	modelService models.Service,
	workFactory WorkFactory,
	automationFactory AutomationFactory,
	factorySessionsService factorysessions.Service,
	factorySessionExecutionFactory FactorySessionExecutionFactory,
	recordingsProjectionFactory RecordingsProjectionFactory,
	recordingsFactory RecordingsFactory,
	runtimeLedgerFactory RuntimeLedgerFactory,
	runtimeRecorderFactory recordings.RuntimeRecorderFactory,
	replayClockFactory ReplayClockFactory,
	replayExecutionFactory recordings.ReplayExecutionFactory,
	workersRuntimeFactory WorkersRuntimeFactory,
	workersRuntimeExecutorsFactory factoryruntime.WorkersRuntimeExecutorsFactory,
	workersMockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory,
	workerHostedPollersFactory WorkerHostedPollersFactory,
	workersLocalRuntimeHooksFactory WorkersLocalRuntimeHooksFactory,
	factoryDefinitionsFactory FactoryDefinitionsFactory,
	factoryScaffoldInitializer factorysessions.FactoryScaffoldInitializer,
	editableFactoryValidator factorysessions.EditableFactoryValidator,
	initialFactorySnapshotFactory factorydefinitions.InitialFactorySnapshotFactory,
	factoryRuntimeAssembler FactoryRuntimeAssembler,
	contentMaterializer work.ContentMaterializer,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	newLoadedFactory factorydefinitions.LoadedFactorySourceFactory,
	decodeReplayConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	loadReplay recordings.ReplayArtifactLoader,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	resolveClock factoryruntime.ClockResolver,
	newSessionLogger factoryruntime.SessionLoggerFactory,
	adaptWorkerCommandRunner WorkerCommandRunnerAdapter,
	providerFromCommandRunnerFactory ProviderFromCommandRunnerFactory,
	processRuntimeFactory roles.ProcessRuntimeFactory,
	ensureOperatorBackendScope operatorsettings.BackendScopeEnsurer,
	generateRuntimeInstanceID factorysessions.RuntimeInstanceIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	replayFiles fileeffects.ReplayRecordingReader,
	providerIdentities factorysessions.ProviderIdentityResolver,
) (*Factory, error) {
	if workflowPreview == nil {
		return nil, fmt.Errorf("Factory Runtime workflow preview operation is required")
	}
	if resolveClock == nil {
		return nil, fmt.Errorf("Factory Runtime clock resolver is required")
	}
	if newSessionLogger == nil {
		return nil, fmt.Errorf("Factory Runtime session logger factory is required")
	}
	if adaptWorkerCommandRunner == nil {
		return nil, fmt.Errorf("Worker command runner adapter is required")
	}
	if processRuntimeFactory == nil {
		return nil, fmt.Errorf("Factory Sessions process runtime factory is required")
	}
	if ensureOperatorBackendScope == nil {
		return nil, fmt.Errorf("Operator Settings backend-scope ensurer is required")
	}
	if generateRuntimeInstanceID == nil {
		return nil, fmt.Errorf("Factory Session runtime instance ID generator is required")
	}
	if resolveHome == nil {
		return nil, fmt.Errorf("Factory Session home-directory resolver is required")
	}
	if namedPaths == nil {
		return nil, fmt.Errorf("named Factory path resolver is required")
	}
	if replayFiles == nil {
		return nil, fmt.Errorf("Factory Session replay recording reader is required")
	}
	if factorySessionsService == nil {
		return nil, fmt.Errorf("Factory Sessions service is required")
	}
	if providerIdentities == nil {
		return nil, fmt.Errorf("provider identity resolver is required")
	}
	return &Factory{
		durableExecutionFactory:         durableExecutionFactory,
		workerExecutionFactory:          workerExecutionFactory,
		modelService:                    modelService,
		workFactory:                     workFactory,
		automationFactory:               automationFactory,
		factorySessionsService:          factorySessionsService,
		factorySessionExecutionFactory:  factorySessionExecutionFactory,
		recordingsProjectionFactory:     recordingsProjectionFactory,
		recordingsFactory:               recordingsFactory,
		runtimeLedgerFactory:            runtimeLedgerFactory,
		runtimeRecorderFactory:          runtimeRecorderFactory,
		replayClockFactory:              replayClockFactory,
		replayExecutionFactory:          replayExecutionFactory,
		workersRuntimeFactory:           workersRuntimeFactory,
		workersRuntimeExecutorsFactory:  workersRuntimeExecutorsFactory,
		workersMockCommandRunnerFactory: workersMockCommandRunnerFactory,
		workerHostedPollersFactory:      workerHostedPollersFactory,
		workersLocalRuntimeHooksFactory: workersLocalRuntimeHooksFactory,
		factoryDefinitionsFactory:       factoryDefinitionsFactory,
		factoryScaffoldInitializer:      factoryScaffoldInitializer,
		editableFactoryValidator:        editableFactoryValidator,
		initialFactorySnapshotFactory:   initialFactorySnapshotFactory,
		factoryRuntimeAssembler:         factoryRuntimeAssembler,
		contentMaterializer:             contentMaterializer,
		providerSessions:                providerSessions,
		factoryDefinitionValidator:      factoryDefinitionValidator,
		namedPaths:                      namedPaths,
		factoryWorkflows:                factoryWorkflows,
		workflowPreview:                 workflowPreview,
		loadFactory:                     loadFactory,
		newLoadedFactory:                newLoadedFactory,
		decodeReplayConfig:              decodeReplayConfig,
		loadReplay:                      loadReplay,
		captureLoadedFactorySnapshot:    captureLoadedFactorySnapshot,
		resolveClock:                    resolveClock,
		newSessionLogger:                newSessionLogger,
		adaptWorkerCommandRunner:         adaptWorkerCommandRunner,
		providerFromCommandRunnerFactory: providerFromCommandRunnerFactory,
		processRuntimeFactory:            processRuntimeFactory,
		ensureOperatorBackendScope:      ensureOperatorBackendScope,
		generateRuntimeInstanceID:       generateRuntimeInstanceID,
		resolveHome:                     resolveHome,
		replayFiles:                     replayFiles,
		providerIdentities:              providerIdentities,
	}, nil
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
		f.recordingsFactory,
		f.runtimeLedgerFactory,
		f.runtimeRecorderFactory,
		f.replayClockFactory,
		f.replayExecutionFactory,
		f.workersRuntimeFactory,
		f.workersRuntimeExecutorsFactory,
		f.workersMockCommandRunnerFactory,
		f.workerHostedPollersFactory,
		f.workersLocalRuntimeHooksFactory,
		f.factoryDefinitionsFactory,
		f.factoryScaffoldInitializer,
		f.editableFactoryValidator,
		f.initialFactorySnapshotFactory,
		f.factoryRuntimeAssembler,
		f.contentMaterializer,
		f.providerSessions,
		f.factoryDefinitionValidator,
		f.namedPaths,
		f.factoryWorkflows,
		f.workflowPreview,
		f.loadFactory,
		f.newLoadedFactory,
		f.decodeReplayConfig,
		f.loadReplay,
		f.captureLoadedFactorySnapshot,
		f.resolveClock,
		f.newSessionLogger,
		f.adaptWorkerCommandRunner,
		f.providerFromCommandRunnerFactory,
		f.processRuntimeFactory,
		f.ensureOperatorBackendScope,
		f.generateRuntimeInstanceID,
		f.resolveHome,
		f.replayFiles,
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
