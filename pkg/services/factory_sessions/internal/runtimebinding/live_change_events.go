package runtimebinding

import (
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type liveChangeEventLog struct {
	ledger recordings.Ledger
}

// NewLiveChangeEventLog adapts the runtime's canonical Recordings ledger to
// the narrow Factory Sessions admission port.
func NewLiveChangeEventLog(ledger recordings.Ledger) factorysessions.LiveChangeEventLog {
	if ledger == nil {
		return nil
	}
	return liveChangeEventLog{ledger: ledger}
}

func (log liveChangeEventLog) AppendLiveChangeEvent(event interfaces.FactoryEvent) (interfaces.FactoryEvent, error) {
	if log.ledger == nil {
		return interfaces.FactoryEvent{}, fmt.Errorf("recordings ledger is unavailable")
	}
	if appender, ok := log.ledger.(interface {
		AppendRecordedEventWithResult(interfaces.FactoryEvent) (interfaces.FactoryEvent, error)
	}); ok {
		return appender.AppendRecordedEventWithResult(event)
	}
	before := len(log.ledger.CanonicalEvents())
	log.ledger.AppendRecordedEvent(event)
	events := log.ledger.CanonicalEvents()
	if len(events) <= before {
		return interfaces.FactoryEvent{}, fmt.Errorf("recordings ledger did not append live change event")
	}
	return events[len(events)-1], nil
}

func (log liveChangeEventLog) LiveChangeEvents() []interfaces.FactoryEvent {
	if log.ledger == nil {
		return nil
	}
	return log.ledger.CanonicalEvents()
}
