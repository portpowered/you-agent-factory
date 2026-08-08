package cli

import "github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"

// ListOperation is the composition-facing Worker Sessions list role.
type ListOperation func(ListConfig) error

// ShowOperation is the composition-facing Worker Sessions show role.
type ShowOperation func(ShowConfig) error

// BindList returns a list operation bound to one injected HTTP protocol.
func BindList(transport clihttp.Protocol) ListOperation {
	if transport == nil {
		return nil
	}
	return NewList(transport)
}

// BindShow returns a show operation bound to one injected HTTP protocol.
func BindShow(transport clihttp.Protocol) ShowOperation {
	if transport == nil {
		return nil
	}
	return NewShow(transport)
}
