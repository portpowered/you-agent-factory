package run

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/portpowered/infinite-you/pkg/service"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

// APIServerStarterWithListener returns the production API server starter bound
// to one process-owned listener. The listener is consumed at most once and the
// normal API server remains responsible for serving and closing it.
func APIServerStarterWithListener(listener net.Listener) service.APIServerStarter {
	var mu sync.Mutex
	used := false
	return func(
		ctx context.Context,
		runtime apisurface.APISurface,
		port int,
		logger *zap.Logger,
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
		return serveAPIServer(ctx, runtime, port, logger, func() {}, listener)
	}
}
