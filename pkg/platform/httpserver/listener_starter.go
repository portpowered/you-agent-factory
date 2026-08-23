package httpserver

import (
	"context"
	"fmt"
	"net"
	"sync"

	platformprocessmemory "github.com/portpowered/infinite-you/pkg/platform/processmemory"
)

// StarterWithListener binds a server starter to one process-owned listener.
// The listener is consumed at most once.
func StarterWithListener(listener net.Listener, readers ...platformprocessmemory.CommitReader) Starter {
	var mu sync.Mutex
	used := false
	commitReader := platformprocessmemory.CommitReader(platformprocessmemory.CurrentCommit)
	if len(readers) > 0 && readers[0] != nil {
		commitReader = readers[0]
	}
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
			host := request.Host
			port := request.Port
			if address, ok := listener.Addr().(*net.TCPAddr); ok {
				host = address.IP.String()
				port = address.Port
			}
			request.OnBound(Binding{Host: host, Port: port})
		}
		return Serve(ctx, HandlerWithDiagnostics(request.Handler, request.Pprof, commitReader), listener, request.Logger)
	}
}
