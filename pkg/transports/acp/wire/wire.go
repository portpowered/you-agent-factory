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
	"github.com/portpowered/infinite-you/pkg/services/events"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/stdio"
)

// NewServer constructs the production ACP stdio Server. Construction alone
// performs no reads, writes, goroutine starts, process starts, endpoint
// binding, session creation, or persistence.
//
// chatSessions and catalog are the canonical Chat Sessions collaborators
// "session/new" dispatches to, factoryTarget is the consumer-owned Factory
// Sessions shim ordinary prompt delegation starts or invokes against,
// eventsService is the canonical Events collaborator an admitted prompt
// turn drains chat-session/<id>/events through before falling back to V1
// synchronous final text, and resolveHomeDir resolves the operator home
// directory that call uses to derive the Operator Settings document path
// and Factory discovery roots. This package injects exactly the instances
// its caller supplies; it never resolves them itself.
func NewServer(
	logger logging.Logger,
	chatSessions chatsessions.Service,
	catalog chatsessions.FactoryTargetCatalogService,
	factoryTarget acp.FactoryTargetService,
	eventsService events.Service,
	resolveHomeDir func() (string, error),
) acp.Server {
	return stdio.New(logger, chatSessions, catalog, factoryTarget, eventsService, resolveHomeDir)
}
