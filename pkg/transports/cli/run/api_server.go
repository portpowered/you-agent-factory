package run

import (
	"context"
	"fmt"
	"net"
	"sync"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
)

// APIServerStarterWithListener returns the production API server starter bound
// to one process-owned listener. The listener is consumed at most once and the
// normal API server remains responsible for serving and closing it.
func APIServerStarterWithListener(listener net.Listener) platformhttpserver.Starter {
	var mu sync.Mutex
	used := false
	return func(
		ctx context.Context,
		request platformhttpserver.StartRequest,
	) error {
		mu.Lock()
		if used {
			mu.Unlock()
			return fmt.Errorf("process-owned API server listener was already used")
		}
		used = true
		mu.Unlock()
		if listener == nil {
			return fmt.Errorf("process-owned API server listener is required")
		}
		if request.OnBound != nil {
			port := request.Port
			if address, ok := listener.Addr().(*net.TCPAddr); ok {
				port = address.Port
			}
			request.OnBound(platformhttpserver.Binding{Port: port})
		}
		return platformhttpserver.Serve(ctx, request.Handler, listener, request.Logger)
	}
}
