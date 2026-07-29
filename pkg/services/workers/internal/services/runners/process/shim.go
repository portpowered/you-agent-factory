// Package process preserves the private Runners import while command-runner
// contracts and adapters live at the Workers service root.
package process

import "github.com/portpowered/infinite-you/pkg/services/workers"

type CommandRunner = workers.CommandRunner
type CommandRequest = workers.CommandRequest
type CommandResult = workers.CommandResult
type OutputChunkObserver = workers.OutputChunkObserver
type CommandEnvEntry = workers.CommandEnvEntry
type Clock = workers.Clock
type ClockFunc = workers.ClockFunc
type ExecCommandRunner = workers.ExecCommandRunner
type StreamingAdaptedCommandRunner = workers.StreamingAdaptedCommandRunner
type StreamingExecCommandRunner = workers.StreamingExecCommandRunner
type LoggingCommandRunner = workers.LoggingCommandRunner

const (
	OutputStreamStdout = workers.OutputStreamStdout
	OutputStreamStderr = workers.OutputStreamStderr
)

var (
	CommandEnvEntriesFromMap     = workers.CommandEnvEntriesFromMap
	MergeCommandEnv              = workers.MergeCommandEnv
	AdaptCommandRunner           = workers.AdaptCommandRunner
	ProjectPlatformCommandRunner = workers.ProjectPlatformCommandRunner
	SubprocessRequestBase        = workers.SubprocessRequestBase
	CommandRunnerWithLogging     = workers.CommandRunnerWithLogging
)
