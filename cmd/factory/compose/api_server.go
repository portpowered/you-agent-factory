package compose

import (
	"context"
	"net"

	api "github.com/portpowered/infinite-you/pkg/transports/http"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

// ServeAPIServer starts the factory HTTP API on listener using api.NewServer with
// the wired FactoryService (or any apisurface.APISurface implementation).
func ServeAPIServer(
	ctx context.Context,
	runtime apisurface.APISurface,
	port int,
	logger *zap.Logger,
	listener net.Listener,
) error {
	srv := api.NewServer(runtime, port, logger)
	return srv.Serve(ctx, listener)
}
