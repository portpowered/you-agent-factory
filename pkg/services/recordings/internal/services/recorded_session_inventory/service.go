// Package recordedsessioninventory defines the private Recordings subservice
// contract for the read-only dated Factory Session history inventory.
package recordedsessioninventory

import recordings "github.com/portpowered/infinite-you/pkg/services/recordings"

// Service owns discovery of finalized recording artifacts without exposing
// filesystem or replay-decoder implementation details to its callers.
type Service interface {
	recordings.RecordedSessionInventory
}
