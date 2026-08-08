package cli

import "github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"

// ListOperation is the composition-facing Worker Sessions list role.
type ListOperation func(ListConfig) error

// BindList returns a list operation bound to one injected HTTP protocol.
func BindList(transport clihttp.Protocol) ListOperation {
	if transport == nil {
		return nil
	}
	return NewList(transport)
}
