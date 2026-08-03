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
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/stdio"
)

// NewServer constructs the production ACP stdio Server. Construction alone
// performs no reads, writes, goroutine starts, process starts, endpoint
// binding, session creation, or persistence.
//
// chatSessions and catalog are the canonical Chat Sessions collaborators
// "session/new" dispatches to, and resolveHomeDir resolves the operator
// home directory that call uses to derive the Operator Settings document
// path and Factory discovery roots. This package injects exactly the
// instances its caller supplies; it never resolves them itself.
func NewServer(
	logger logging.Logger,
	chatSessions chatsessions.Service,
	catalog chatsessions.FactoryTargetCatalogService,
	resolveHomeDir func() (string, error),
) acp.Server {
	return stdio.New(logger, chatSessions, catalog, resolveHomeDir)
}
