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
