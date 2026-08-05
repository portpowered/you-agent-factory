package factorysession

import factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"

// DurableExecution composes the two Factory Sessions-owned capabilities this
// representation adapter needs: durable execution and scoped session inventory.
// Inventory stays separate because its request can intentionally include live
// sessions; no durable operation is widened into live control.
type DurableExecution interface {
	factorysessions.DurableExecutionService
	factorysessions.SessionInventoryService
}
