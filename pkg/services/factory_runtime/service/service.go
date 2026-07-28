package service

import (
	"context"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factory_context "github.com/portpowered/infinite-you/pkg/services/factory_runtime/context"
	factoryinternal "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal"
	factoryhostinternal "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/build"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// RuntimeSidecars owns runtime-scoped metrics, input listeners, and automation.
type RuntimeSidecars = factoryinternal.RuntimeSidecars

// ProgressPublisherFactory builds progress publishers for worker execution.
type ProgressPublisherFactory = factoryinternal.ProgressPublisherFactory

// DispatchCompletionFactory builds dispatch-completion callbacks.
type DispatchCompletionFactory = factoryinternal.DispatchCompletionFactory

// InitialFactorySnapshotFactory builds initial factory snapshots for runtime assembly.
type InitialFactorySnapshotFactory = factoryinternal.InitialFactorySnapshotFactory

// RuntimeFileLoggingPolicy controls whether bundle construction creates a runtime file sink.
type RuntimeFileLoggingPolicy = factoryinternal.RuntimeFileLoggingPolicy

// RuntimeMetricsPolicy controls whether bundle construction creates a runtime metrics sink.
type RuntimeMetricsPolicy = factoryinternal.RuntimeMetricsPolicy

// OrchestratorDefinitionValidation implements the Factory Definition port for
// runtime-owned JavaScript workflow and policy semantics.
type OrchestratorDefinitionValidation = factoryinternal.OrchestratorDefinitionValidation

// RuntimeFactory constructs hosted runtime bundles.
type RuntimeFactory = factoryinternal.RuntimeFactory

// Assembly owns the product-policy dependencies used to assemble each
// session-owned Factory Runtime.
type Assembly = factoryinternal.Assembly

const (
	RuntimeFileLoggingPolicyEnabled  = factoryinternal.RuntimeFileLoggingPolicyEnabled
	RuntimeFileLoggingPolicyDisabled = factoryinternal.RuntimeFileLoggingPolicyDisabled
	RuntimeMetricsPolicyEnabled      = factoryinternal.RuntimeMetricsPolicyEnabled
	RuntimeMetricsPolicyDisabled     = factoryinternal.RuntimeMetricsPolicyDisabled
)

// ValidateRecordReplayPaths enforces mutually exclusive recording modes.
func ValidateRecordReplayPaths(recordPath, replayPath string) error {
	return factoryinternal.ValidateRecordReplayPaths(recordPath, replayPath)
}

// PreseedRuntimeInputs materializes listener-backed inputs before execution.
func PreseedRuntimeInputs(
	ctx context.Context,
	automation automations.Service,
	bundle *factoryhostinternal.Bundle,
) error {
	return factoryinternal.PreseedRuntimeInputs(ctx, automation, bundle)
}

// NewRuntimeSidecars constructs runtime sidecars for automation and metrics.
func NewRuntimeSidecars(automation automations.Service, enabled bool) *RuntimeSidecars {
	return factoryinternal.NewRuntimeSidecars(automation, enabled)
}

// NewRuntimeBuild constructs the canonical runtime-build service from decomposed
// process configuration and domain collaborators.
func NewRuntimeBuild(
	defaultWorkerModelProvider string,
	defaultWorkerModel string,
	applyOperatorDefaults bool,
	recordPath string,
	workflowID string,
	defaultSessionID string,
	workstationLoader factorydefinitions.WorkstationLoader,
	providerOverride workers.Provider,
	providerCommandRunner workers.CommandRunner,
	scriptCommandRunner workers.CommandRunner,
	mockWorkersConfig *workers.MockWorkersConfig,
	runtimeMode factorydefinitions.RuntimeMode,
	runtimeScheduler scheduler.Scheduler,
	workerExecutors map[string]workers.WorkerExecutor,
	workerExecutorDecorator func(string, workers.WorkerExecutor) workers.WorkerExecutor,
	inlineDispatch bool,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
	runtimeLogDir string,
	runtimeLogConfig factory.RuntimeLogStorageConfig,
	runtimeFileLoggingPolicy RuntimeFileLoggingPolicy,
	runtimeMetricsPolicy RuntimeMetricsPolicy,
	runtimeMetricsDir string,
	runtimeMetricsConfig factory.RuntimeMetricsStorageConfig,
	recordFlushInterval time.Duration,
	backendScopeID string,
	factoryRunnerID string,
	verbose bool,
	skipRunnerPrerequisiteValidation bool,
	invocationSkipPermissionsOverride *bool,
	clock factory.Clock,
	baseLogger *zap.Logger,
	runtimeFactory *RuntimeFactory,
	workerExecution workers.RuntimeService,
	runtimeExecutorsFactory factory.WorkersRuntimeExecutorsFactory,
	mockCommandRunnerFactory factory.WorkersMockCommandRunnerFactory,
	progressFactory ProgressPublisherFactory,
	completionFactory DispatchCompletionFactory,
	petriMutationRecorder factory.PetriMutationRecorder,
	worldStateProjector factory.WorldStateProjector,
	runtimeLedgerFactory factory.RuntimeLedgerFactory,
	runtimeRecorderFactory recordings.RuntimeRecorderFactory,
	loadFactory factory.LoadedFactoryLoader,
	initialFactorySnapshot InitialFactorySnapshotFactory,
) (*runtimebuild.Service, error) {
	return factoryinternal.NewRuntimeBuild(
		defaultWorkerModelProvider,
		defaultWorkerModel,
		applyOperatorDefaults,
		recordPath,
		workflowID,
		defaultSessionID,
		workstationLoader,
		providerOverride,
		providerCommandRunner,
		scriptCommandRunner,
		mockWorkersConfig,
		runtimeMode,
		runtimeScheduler,
		workerExecutors,
		workerExecutorDecorator,
		inlineDispatch,
		submissionRecorder,
		dispatchRecorder,
		runtimeLogDir,
		runtimeLogConfig,
		runtimeFileLoggingPolicy,
		runtimeMetricsPolicy,
		runtimeMetricsDir,
		runtimeMetricsConfig,
		recordFlushInterval,
		backendScopeID,
		factoryRunnerID,
		verbose,
		skipRunnerPrerequisiteValidation,
		invocationSkipPermissionsOverride,
		clock,
		baseLogger,
		runtimeFactory,
		workerExecution,
		runtimeExecutorsFactory,
		mockCommandRunnerFactory,
		progressFactory,
		completionFactory,
		petriMutationRecorder,
		worldStateProjector,
		runtimeLedgerFactory,
		runtimeRecorderFactory,
		loadFactory,
		initialFactorySnapshot,
	)
}

// NewOrchestratorDefinitionValidator returns the runtime-owned orchestrator
// validator injected into Factory Definition validation by Wire.
func NewOrchestratorDefinitionValidator(
	workflows factory.JavaScriptWorkflowDefinitions,
) OrchestratorDefinitionValidation {
	return factoryinternal.NewOrchestratorDefinitionValidator(workflows)
}

// NewRuntimeFactory constructs a hosted runtime bundle factory.
func NewRuntimeFactory(
	quorumPolicy factorydefinitions.QuorumPolicyService,
	outputShaping factorydefinitions.InvocationOutputShapingService,
	workPropagation factorydefinitions.WorkPropagationPolicyService,
	decisionEnvelopes factorydefinitions.DecisionEnvelopeService,
	loggerFactory factory.RuntimeLoggerFactory,
	runtimeLogs factory.RuntimeLogSinkFactory,
	runtimeMetrics factory.RuntimeMetricsSinkFactory,
	newID factory.IDGenerator,
	workRequestIDs work.RequestIDGenerator,
	runtimeDirs factory.RuntimeDirectoryFileSystem,
	inputFiles factory.InputFileSystem,
	inputDirectoryWalker factory.InputDirectoryWalker,
	orchestrationCompilation factory.OrchestrationCompilation,
) *RuntimeFactory {
	return factoryinternal.NewRuntimeFactory(
		quorumPolicy,
		outputShaping,
		workPropagation,
		decisionEnvelopes,
		loggerFactory,
		runtimeLogs,
		runtimeMetrics,
		newID,
		workRequestIDs,
		runtimeDirs,
		inputFiles,
		inputDirectoryWalker,
		orchestrationCompilation,
	)
}

// RuntimeWorkflowContext derives the canonical project/session execution context.
func RuntimeWorkflowContext(cfg *factorydefinitions.FactoryConfig, sessionID string) *factory_context.FactoryContext {
	return factoryinternal.RuntimeWorkflowContext(cfg, sessionID)
}

// NewAssembly constructs the inert Factory Runtime assembly service selected by Wire.
func NewAssembly(runtimeFactory *RuntimeFactory) (*Assembly, error) {
	return factoryinternal.NewAssembly(runtimeFactory)
}
