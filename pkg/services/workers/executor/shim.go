// Package executor is a transitional compile shim that re-exports workstation
// executor implementations from the private destination. Peers should construct
// through workers/wire; baseline deletion of this path is owned by DEL-WRK.
package executor

import (
	private "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor"
)

type (
	WorkerExecutor               = private.WorkerExecutor
	WorkstationRequestExecutor   = private.WorkstationRequestExecutor
	AgentContext                 = private.AgentContext
	Tool                         = private.Tool
	OutputParser                 = private.OutputParser
	WorkstationExecutor          = private.WorkstationExecutor
	WorkstationBehaviorRouter    = private.WorkstationBehaviorRouter
	AgentExecutor                = private.AgentExecutor
	ScriptExecutor               = private.ScriptExecutor
	ScriptEventRecorder          = private.ScriptEventRecorder
	ScriptFactory                = private.ScriptFactory
	CommandRunner                = private.CommandRunner
	CommandRequest               = private.CommandRequest
	CommandResult                = private.CommandResult
	ExecCommandRunner            = private.ExecCommandRunner
	LoggingCommandRunner         = private.LoggingCommandRunner
	ProviderError                = private.ProviderError
	DefaultPromptRenderer        = private.DefaultPromptRenderer
	NoopExecutor                 = private.NoopExecutor
)

const (
	WorkLogEventCommandRunnerRequested      = private.WorkLogEventCommandRunnerRequested
	WorkLogEventCommandRunnerCompleted      = private.WorkLogEventCommandRunnerCompleted
	WorkLogEventCommandRunnerRequestDetails = private.WorkLogEventCommandRunnerRequestDetails
	WorkLogEventCommandRunnerOutputDetails  = private.WorkLogEventCommandRunnerOutputDetails
)

var (
	NewScriptFactory            = private.NewScriptFactory
	NewAgentExecutor            = private.NewAgentExecutor
	NewAgentExecutorWithRunner  = private.NewAgentExecutorWithRunner
	RunnerFromProvider          = private.RunnerFromProvider
	InputTokens                 = private.InputTokens
	WorkDispatchInputTokens     = private.WorkDispatchInputTokens
	WorkLogFields               = private.WorkLogFields
)
