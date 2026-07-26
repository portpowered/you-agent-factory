// Package projectionquery defines the Recordings-owned projection-query
// capability. Consumers outside Recordings use the Recordings root service
// instead of this private subservice contract.
package projectionquery

import recordings "github.com/portpowered/infinite-you/pkg/services/recordings"

// Service owns canonical world-state reduction, derived Recordings queries,
// and reconnect validation behind the parent-private subservice boundary.
type Service interface {
	recordings.ProjectionService
}
