package wire

import (
	"os"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
	acpwire "github.com/portpowered/infinite-you/pkg/transports/acp/wire"
)

// acpServerResolveHomeDir is the ACP stdio server's own home-directory
// resolver type, distinct from every other func() (string, error) provider
// this graph registers, so Wire's generated bundle can bind it uniquely.
type acpServerResolveHomeDir func() (string, error)

// provideACPServerResolveHomeDir constructs the operator home directory
// resolver the production ACP stdio server uses to derive the Operator
// Settings document path and Factory discovery roots for "session/new". It
// is os.UserHomeDir directly, with no dependency bag or lookup indirection.
func provideACPServerResolveHomeDir() acpServerResolveHomeDir {
	return os.UserHomeDir
}

// provideACPServer constructs the production ACP stdio Server from the same
// canonical chatsessions.Service and chatsessions.FactoryTargetCatalogService
// instances the rest of this graph composes, so the real "session/new",
// "session/set_config_option", and "/factory" consumer observes the one
// process-scoped Chat Sessions authority instead of a second, independently
// constructed instance. Construction alone performs no I/O; it starts no
// goroutine, process, listener, session, or persistence.
func provideACPServer(
	logger logging.Logger,
	chatSessions chatsessions.Service,
	catalog chatsessions.FactoryTargetCatalogService,
	resolveHomeDir acpServerResolveHomeDir,
) acp.Server {
	return acpwire.NewServer(logger, chatSessions, catalog, resolveHomeDir)
}
