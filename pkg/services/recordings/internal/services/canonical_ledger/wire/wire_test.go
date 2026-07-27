package wire_test

import (
	"testing"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	canonicalledgerwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger/wire"
)

type unusedLedger struct {
	recordings.Ledger
}

func TestNewServiceConstructsCanonicalLedgerOwner(t *testing.T) {
	t.Parallel()

	if service := canonicalledgerwire.NewService(&unusedLedger{}); service == nil {
		t.Fatal("NewService returned nil")
	}
	if service := canonicalledgerwire.NewService(nil); service != nil {
		t.Fatal("NewService(nil) = service, want nil")
	}
}
