// Package workersessions exposes the Worker Sessions CLI façade used by root
// command composition while implementation remains service-owned.
package workersessions

import (
	workersessionscli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli"
)

type ListConfig = workersessionscli.ListConfig
type ListOperation = workersessionscli.ListOperation
type ShowConfig = workersessionscli.ShowConfig
type ShowOperation = workersessionscli.ShowOperation
type ReadConfig = workersessionscli.ReadConfig
type ReadOperation = workersessionscli.ReadOperation
type StreamConfig = workersessionscli.StreamConfig
type StreamOperation = workersessionscli.StreamOperation
type InvokeConfig = workersessionscli.InvokeConfig
type InvokeOperation = workersessionscli.InvokeOperation
type ContinueConfig = workersessionscli.ContinueConfig
type ContinueOperation = workersessionscli.ContinueOperation
type InterruptConfig = workersessionscli.InterruptConfig
type InterruptOperation = workersessionscli.InterruptOperation
type LocalInvokeBoundary = workersessionscli.LocalInvokeBoundary
type LocalInterruptBoundary = workersessionscli.LocalInterruptBoundary
type LocalControlBoundary = workersessionscli.LocalControlBoundary
type ControlConfig = workersessionscli.ControlConfig
type ControlOperation = workersessionscli.ControlOperation
type CLIError = workersessionscli.CLIError

const StreamModeConflictCode = workersessionscli.StreamModeConflictCode

func NewStreamModeConflictError() *CLIError {
	return workersessionscli.NewStreamModeConflictError()
}
