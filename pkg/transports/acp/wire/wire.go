// Package wire is the ACP stdio transport composition boundary.
//
// Wire performs construction only: it injects the explicit collaborators an
// ACP acp.Server needs and starts no goroutine, process, listener, session,
// or persistence. Callers inject their own streams into the returned
// Server's Serve method; wire never reads process-global stdio itself, so
// this package introduces no dependency bag, service locator, or secondary
// injector.
package wire

import (
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/stdio"
)

// NewServer constructs the production ACP stdio Server. Construction alone
// performs no reads, writes, goroutine starts, process starts, endpoint
// binding, session creation, or persistence.
func NewServer(logger logging.Logger) acp.Server {
	return stdio.New(logger)
}
