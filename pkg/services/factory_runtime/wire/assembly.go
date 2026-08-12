package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimeinternal "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// RuntimeFactory constructs hosted runtime bundles.
type RuntimeFactory = factoryruntimeinternal.RuntimeFactory

// Assembly owns the product-policy dependencies used to assemble each
// session-owned Factory Runtime.
type Assembly = factoryruntimeinternal.Assembly

// NewRuntimeFactory constructs a hosted runtime bundle factory.
func NewRuntimeFactory(
	quorumPolicy factorydefinitions.QuorumPolicyService,
	outputShaping factorydefinitions.InvocationOutputShapingService,
	workPropagation factorydefinitions.WorkPropagationPolicyService,
	workService work.Service,
	decisionEnvelopes factorydefinitions.DecisionEnvelopeService,
	loggerFactory factoryruntime.RuntimeLoggerFactory,
	runtimeLogs factoryruntime.RuntimeLogSinkFactory,
	runtimeMetrics factoryruntime.RuntimeMetricsSinkFactory,
	newID factoryruntime.IDGenerator,
	workRequestIDs work.RequestIDGenerator,
	runtimeDirs factoryruntime.RuntimeDirectoryFileSystem,
	inputFiles factoryruntime.InputFileSystem,
	inputDirectoryWalker factoryruntime.InputDirectoryWalker,
	orchestrationCompilation factoryruntime.OrchestrationCompilation,
	providerSessions providersessions.Service,
	workerPoolBoundaryFactory factoryruntime.WorkstationPoolBoundaryFactory,
) *RuntimeFactory {
	return factoryruntimeinternal.NewRuntimeFactory(
		quorumPolicy,
		outputShaping,
		workPropagation,
		workService,
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
		providerSessions,
		workerPoolBoundaryFactory,
	)
}

// NewAssembly constructs the inert Factory Runtime assembly service selected by
// Wire. It does not start a runtime or sidecar.
func NewAssembly(
	runtimeFactory *RuntimeFactory,
	workerSessionsFactory factoryruntime.WorkerSessionsFactory,
) (*Assembly, error) {
	return factoryruntimeinternal.NewAssembly(runtimeFactory, workerSessionsFactory)
}

// NewOrchestratorDefinitionValidator returns the runtime-owned orchestrator
// validator injected into Factory Definition validation by Wire.
func NewOrchestratorDefinitionValidator(
	workflows factoryruntime.JavaScriptWorkflowDefinitions,
) factorydefinitions.OrchestratorDefinitionValidator {
	return factoryruntimeinternal.NewOrchestratorDefinitionValidator(workflows)
}
