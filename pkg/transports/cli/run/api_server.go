package run

import (
	"net"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
)

// APIServerStarterWithListener returns the production API server starter bound
// to one process-owned listener. The listener is consumed at most once and the
// normal API server remains responsible for serving and closing it.
func APIServerStarterWithListener(listener net.Listener) platformhttpserver.Starter {
	return platformhttpserver.StarterWithListener(listener)
}
