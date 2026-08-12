// Package workersessions exposes the Worker Sessions CLI façade used by root
// command composition while implementation remains service-owned.
package workersessions

import (
	workersessionscli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
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
type LocalInvokeBoundary = workersessionscli.LocalInvokeBoundary
type CLIError = workersessionscli.CLIError

const StreamModeConflictCode = workersessionscli.StreamModeConflictCode

func NewStreamModeConflictError() *CLIError {
	return workersessionscli.NewStreamModeConflictError()
}

func NewList(transport clihttp.Protocol) ListOperation {
	return workersessionscli.NewList(transport)
}

func BindList(transport clihttp.Protocol) ListOperation {
	return workersessionscli.BindList(transport)
}

func NewShow(transport clihttp.Protocol) ShowOperation {
	return workersessionscli.NewShow(transport)
}

func BindShow(transport clihttp.Protocol) ShowOperation {
	return workersessionscli.BindShow(transport)
}

func NewRead(transport clihttp.Protocol) ReadOperation {
	return workersessionscli.NewRead(transport)
}

func BindRead(transport clihttp.Protocol) ReadOperation {
	return workersessionscli.BindRead(transport)
}

func NewStream(transport clihttp.Protocol) StreamOperation {
	return workersessionscli.NewStream(transport)
}

func BindStream(transport clihttp.Protocol) StreamOperation {
	return workersessionscli.BindStream(transport)
}

func NewInvoke(transport clihttp.Protocol, local LocalInvokeBoundary) InvokeOperation {
	return workersessionscli.NewInvoke(transport, local)
}

func BindInvoke(transport clihttp.Protocol, local LocalInvokeBoundary) InvokeOperation {
	return workersessionscli.BindInvoke(transport, local)
}

func NewContinue(transport clihttp.Protocol, local LocalInvokeBoundary) ContinueOperation {
	return workersessionscli.NewContinue(transport, local)
}

func BindContinue(transport clihttp.Protocol, local LocalInvokeBoundary) ContinueOperation {
	return workersessionscli.BindContinue(transport, local)
}
