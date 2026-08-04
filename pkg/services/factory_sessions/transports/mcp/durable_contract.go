package factorysession

import factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"

// DurableExecution composes the Factory Sessions-owned durable execution and
// scoped inventory capabilities required by the MCP tool set. Inventory stays
// separate because its request can intentionally include live sessions.
type DurableExecution interface {
	factorysessions.DurableExecutionService
	factorysessions.SessionInventoryService
}
