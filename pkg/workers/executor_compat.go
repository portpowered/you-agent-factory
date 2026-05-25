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

func RunnerFromProvider(provider Provider) Runner {
	return workerexecutor.RunnerFromProvider(provider)
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
