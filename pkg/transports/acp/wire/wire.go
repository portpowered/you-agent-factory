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
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/stdio"
)

// NewServer constructs the production ACP stdio Server. Construction alone
// performs no reads, writes, goroutine starts, process starts, endpoint
// binding, session creation, or persistence.
//
// chatSessions and catalog are the canonical Chat Sessions collaborators
// "session/new" dispatches to. factorySessions is the canonical Factory
// Sessions service and eventsService is the canonical aggregate
// stream ordinary prompt delivery drains. This package only injects them.
func NewServer(
	logger logging.Logger,
	chatSessions chatsessions.Service,
	catalog chatsessions.FactoryTargetCatalogService,
	factorySessions factorysessions.Service,
	eventsService events.Service,
	resolveHomeDir func() (string, error),
	responseBridge acp.ResponseBridge,
	wireRecorder acp.WireRecorder,
	startResolver acp.FactorySessionStartResolver,
	invocationScope acp.InvocationScopeFactory,
) acp.Server {
	return stdio.New(
		logger, chatSessions, catalog, factorySessions, eventsService,
		resolveHomeDir, responseBridge, wireRecorder, startResolver, invocationScope,
	)
}
