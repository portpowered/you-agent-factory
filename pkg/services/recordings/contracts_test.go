package recordings_test

import (
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryevents "github.com/portpowered/infinite-you/pkg/services/recordings/events"
)

var _ recordings.Ledger = (*factoryevents.FactoryEventHistory)(nil)
