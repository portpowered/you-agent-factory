package workers

import (
	"time"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/work"
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
type WorkerExecutor = workerexecutor.WorkerExecutor
type WorkstationRequestExecutor = workerexecutor.WorkstationRequestExecutor
type Runner = workerexecutor.Runner

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

func PanicAsFailedResult(dispatch work.WorkDispatch, recovered any, duration time.Duration) workerexecution.WorkResult {
	return workerexecutor.PanicAsFailedResult(dispatch, recovered, duration)
}

func WorkLogFields(metadata work.ExecutionMetadata, keysAndValues ...any) []any {
	return workerexecutor.WorkLogFields(metadata, keysAndValues...)
}
