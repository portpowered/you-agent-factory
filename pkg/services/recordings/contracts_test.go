package recordings_test

import (
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	canonicalledgerevents "github.com/portpowered/infinite-you/pkg/services/recordings/internal/events"
)

var _ recordings.Ledger = (*canonicalledgerevents.FactoryEventHistory)(nil)
