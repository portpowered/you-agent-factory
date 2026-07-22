package durableexecution

import factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"

// Service owns durable Factory Session start, lifecycle, inspection, replay,
// and restart behavior behind the Factory Sessions private capability boundary.
type Service interface {
	factorysessions.ExecutionService
}
