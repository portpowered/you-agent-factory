package httpserver

import (
	"context"
	"fmt"
	"net"
	"sync"
)

// StarterWithListener binds a server starter to one process-owned listener.
// The listener is consumed at most once.
func StarterWithListener(listener net.Listener) Starter {
	var mu sync.Mutex
	used := false
	return func(ctx context.Context, request StartRequest) error {
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
			request.OnBound(Binding{Port: port})
		}
		return Serve(ctx, request.Handler, listener, request.Logger)
	}
}
