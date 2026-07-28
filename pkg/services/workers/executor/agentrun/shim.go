// Package agentrun is a transitional compile shim that re-exports agent-run
// workstation executor helpers from the private destination. Baseline deletion
// of this path is owned by DEL-WRK.
package agentrun

import (
	private "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
)

type (
	HarnessInput           = private.HarnessInput
	HarnessResult          = private.HarnessResult
	HarnessAdapter         = private.HarnessAdapter
	LibraryHarnessAdapter  = private.LibraryHarnessAdapter
	AgentRunExecutor       = private.AgentRunExecutor
	AgentRunEventRecorder  = private.AgentRunEventRecorder
	ToolDiagnostic         = private.ToolDiagnostic
	ToolDiagnosticRecorder = private.ToolDiagnosticRecorder
	PolicyToolExecutor     = private.PolicyToolExecutor
)

var (
	NewLibraryHarnessAdapter        = private.NewLibraryHarnessAdapter
	NewAgentRunExecutor             = private.NewAgentRunExecutor
	NewAgentRunExecutorWithDependencies = private.NewAgentRunExecutorWithDependencies
	NewToolDiagnosticRecorder       = private.NewToolDiagnosticRecorder
	NewPolicyToolExecutor           = private.NewPolicyToolExecutor
)
