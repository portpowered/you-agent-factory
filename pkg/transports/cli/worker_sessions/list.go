// Package workersessions exposes the Worker Sessions CLI façade used by root
// command composition while implementation remains service-owned.
package workersessions

import (
	workersessionscli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

type ListConfig = workersessionscli.ListConfig
type ListOperation = workersessionscli.ListOperation

func NewList(transport clihttp.Protocol) ListOperation {
	return workersessionscli.NewList(transport)
}

func BindList(transport clihttp.Protocol) ListOperation {
	return workersessionscli.BindList(transport)
}
