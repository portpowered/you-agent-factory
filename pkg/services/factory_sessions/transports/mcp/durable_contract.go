package factorysession

import factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"

// DurableExecution is the MCP adapter's narrow durable capability. The
// production value is supplied by the singular Sessions root; this adapter
// contract avoids publishing a second service authority from the root package.
type DurableExecution = factorysessionwire.DurableExecutionService
