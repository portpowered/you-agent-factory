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
func StarterWithListener(
	listener net.Listener,
	commitReader platformprocessmemory.CommitReader,
	commandLineReader CommandLineReader,
) Starter {
	var mu sync.Mutex
	used := false
	if commitReader == nil {
		commitReader = platformprocessmemory.CurrentCommit
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
		return Serve(ctx, HandlerWithDiagnostics(request.Handler, request.Pprof, commitReader, commandLineReader), listener, request.Logger)
	}
}

// BindingObserver attaches an already-bound endpoint to a handler's owner.
// The returned release removes that binding when this host exits.
type BindingObserver interface {
	ObserveHostBinding(Binding) func()
}

// NewObservedStarter reports the concrete binding to participating handlers
// before readiness, and releases it after the selected starter joins. Handlers
// without a binding observer preserve their original lifecycle.
func NewObservedStarter(starter Starter) Starter {
	return func(ctx context.Context, request StartRequest) error {
		owner, ok := request.Handler.(BindingObserver)
		if !ok {
			return starter(ctx, request)
		}
		onBound := request.OnBound
		var release func()
		defer func() {
			if release != nil {
				release()
			}
		}()
		request.OnBound = func(binding Binding) {
			if release != nil {
				release()
			}
			release = owner.ObserveHostBinding(binding)
			if onBound != nil {
				onBound(binding)
			}
		}
		return starter(ctx, request)
	}
}
