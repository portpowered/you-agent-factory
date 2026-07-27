// Package wire constructs the Recordings canonical-ledger subservice.
package wire

import (
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	canonicalledger "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger"
	ledgerservice "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger/internal/service"
)

// NewService constructs the private canonical ledger owner from the runtime
// ledger seam selected by the application graph.
func NewService(ledger recordings.Ledger) canonicalledger.Service {
	if ledger == nil {
		return nil
	}
	return ledgerservice.New(ledger)
}
