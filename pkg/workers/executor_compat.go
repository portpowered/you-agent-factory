package workers

import (
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	workerexecutor "github.com/portpowered/infinite-you/pkg/workers/executor"
)

type AgentContext = workerexecutor.AgentContext
type Tool = workerexecutor.Tool
type OutputParser = workerexecutor.OutputParser
type AgentExecutor = workerexecutor.AgentExecutor
type AgentExecutorOption = workerexecutor.AgentExecutorOption
type ScriptExecutor = workerexecutor.ScriptExecutor
type ScriptEventRecorder = workerexecutor.ScriptEventRecorder
type ScriptExecutorOption = workerexecutor.ScriptExecutorOption
type NoopExecutor = workerexecutor.NoopExecutor
type WorkstationExecutor = workerexecutor.WorkstationExecutor
type WorkerPool = workerexecutor.WorkerPool
type WorkerRunner = workerexecutor.WorkerRunner

const (
	WorkLogEventWorkerPoolSubmitted         = workerexecutor.WorkLogEventWorkerPoolSubmitted
	WorkLogEventWorkerPoolExecutorEntered   = workerexecutor.WorkLogEventWorkerPoolExecutorEntered
	WorkLogEventWorkerPoolResponseSubmitted = workerexecutor.WorkLogEventWorkerPoolResponseSubmitted
	WorkLogEventCommandRunnerRequested      = workerexecutor.WorkLogEventCommandRunnerRequested
	WorkLogEventCommandRunnerCompleted      = workerexecutor.WorkLogEventCommandRunnerCompleted
	WorkLogEventCommandRunnerRequestDetails = workerexecutor.WorkLogEventCommandRunnerRequestDetails
	WorkLogEventCommandRunnerOutputDetails  = workerexecutor.WorkLogEventCommandRunnerOutputDetails
)

func WithLogger(logger logging.Logger) AgentExecutorOption {
	return workerexecutor.WithLogger(logger)
}

func NewAgentExecutor(runtimeConfig interfaces.RuntimeDefinitionLookup, provider Provider, opts ...AgentExecutorOption) *AgentExecutor {
	return workerexecutor.NewAgentExecutor(runtimeConfig, provider, opts...)
}

func NewAgentExecutorWithRunner(runtimeConfig interfaces.RuntimeDefinitionLookup, runner Runner, opts ...AgentExecutorOption) *AgentExecutor {
	return workerexecutor.NewAgentExecutorWithRunner(runtimeConfig, runner, opts...)
}

func RunnerFromProvider(provider Provider) Runner {
	return workerexecutor.RunnerFromProvider(provider)
}

func WithScriptEventRecorder(recorder ScriptEventRecorder) ScriptExecutorOption {
	return workerexecutor.WithScriptEventRecorder(recorder)
}

func WithScriptFactoryDir(factoryDir string) ScriptExecutorOption {
	return workerexecutor.WithScriptFactoryDir(factoryDir)
}

func NewScriptExecutor(def *interfaces.WorkerConfig, logger logging.Logger, opts ...ScriptExecutorOption) *ScriptExecutor {
	return workerexecutor.NewScriptExecutor(def, logger, opts...)
}

func NewScriptExecutorWithRunner(def *interfaces.WorkerConfig, runner CommandRunner, logger logging.Logger, opts ...ScriptExecutorOption) *ScriptExecutor {
	return workerexecutor.NewScriptExecutorWithRunner(def, runner, logger, opts...)
}

func ResolveModelOperationBindings(
	workstationDef *interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.WorkerConfig,
	inputTokens []interfaces.Token,
) ([]interfaces.ResolvedModelOperationBinding, error) {
	return workerexecutor.ResolveModelOperationBindings(workstationDef, workerDef, inputTokens)
}

func NewWorkerPool(logger logging.Logger) *WorkerPool {
	return workerexecutor.NewWorkerPool(logger)
}

func PanicAsFailedResult(dispatch interfaces.WorkDispatch, recovered any, duration time.Duration) interfaces.WorkResult {
	return workerexecutor.PanicAsFailedResult(dispatch, recovered, duration)
}

func WorkLogFields(metadata interfaces.ExecutionMetadata, keysAndValues ...any) []any {
	return workerexecutor.WorkLogFields(metadata, keysAndValues...)
}
