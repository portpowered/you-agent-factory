// Package process is a transitional compile shim that re-exports the runners
// process implementation from the private destination. Peers should construct
// through workers/wire; baseline deletion of this path is owned by DEL-WRK.
package process

import (
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
)

type CommandRunner = workerprocess.CommandRunner
type CommandRequest = workerprocess.CommandRequest
type CommandResult = workerprocess.CommandResult
type OutputChunkObserver = workerprocess.OutputChunkObserver
type CommandEnvEntry = workerprocess.CommandEnvEntry
type Clock = workerprocess.Clock
type ClockFunc = workerprocess.ClockFunc
type ExecCommandRunner = workerprocess.ExecCommandRunner
type StreamingAdaptedCommandRunner = workerprocess.StreamingAdaptedCommandRunner
type StreamingExecCommandRunner = workerprocess.StreamingExecCommandRunner
type LoggingCommandRunner = workerprocess.LoggingCommandRunner

var (
	CommandEnvEntriesFromMap = workerprocess.CommandEnvEntriesFromMap
	MergeCommandEnv          = workerprocess.MergeCommandEnv
	AdaptCommandRunner       = workerprocess.AdaptCommandRunner
	ProjectPlatformCommandRunner = workerprocess.ProjectPlatformCommandRunner
	SubprocessRequestBase    = workerprocess.SubprocessRequestBase
	CommandRunnerWithLogging = workerprocess.CommandRunnerWithLogging
)

const (
	OutputStreamStdout = workerprocess.OutputStreamStdout
	OutputStreamStderr = workerprocess.OutputStreamStderr
)
