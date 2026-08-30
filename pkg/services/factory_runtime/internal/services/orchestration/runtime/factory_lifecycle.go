package runtime

import (
	"fmt"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func recordSessionLifecycleCompletionFromFactory(
	f *factoryImpl,
	tick int,
	factoryState interfaces.FactoryState,
	reason string,
	eventTime time.Time,
) error {
	if f == nil || f.eventHistory == nil || f.cfg == nil {
		return nil
	}
	f.completionMu.Lock()
	defer f.completionMu.Unlock()
	sessionID := sessionIDFromFactoryConfig(f.cfg)
	factoryCfg := factoryConfigFromFactoryConfig(f.cfg)
	if phaser, ok := f.eventHistory.(recordings.SessionLifecycleCompletionPhaser); ok {
		if !f.completionResultRecorded {
			phaser.RecordSessionLifecycleResultUpdated(
				sessionID, factoryCfg, tick, factoryState, reason, eventTime,
			)
			f.completionResultRecorded = true
		}
		if !f.completionEventRecorded {
			phaser.RecordSessionLifecycleCompleted(
				sessionID, factoryCfg, tick, factoryState, reason, eventTime,
			)
			f.completionEventRecorded = true
		}
		if persist := f.completionDurabilityGateFunc(); persist != nil {
			if err := persist(); err != nil {
				return fmt.Errorf("persist Factory Session completion: %w", err)
			}
		}
		return nil
	}
	if !f.completionEventRecorded {
		f.eventHistory.RecordSessionLifecycleCompletion(
			sessionID, factoryCfg, tick, factoryState, reason, eventTime,
		)
		f.completionResultRecorded = true
		f.completionEventRecorded = true
	}
	if persist := f.completionDurabilityGateFunc(); persist != nil {
		if err := persist(); err != nil {
			return fmt.Errorf("persist Factory Session completion: %w", err)
		}
	}
	return nil
}

func publishDeferredSessionCompletion(eventHistory recordings.RuntimeLedger) {
	if publisher, ok := eventHistory.(recordings.DeferredSessionCompletionPublisher); ok {
		publisher.PublishDeferredSessionCompletion()
	}
}

// SetCompletionDurabilityGate binds the Recordings terminal persistence gate
// that must complete before a terminal source session is advertised. The gate
// is optional so in-memory runtime callers retain the established behavior.
func (f *factoryImpl) SetCompletionDurabilityGate(persist func() error) {
	if f == nil {
		return
	}
	f.mu.Lock()
	f.completionDurabilityGate = persist
	f.mu.Unlock()
}

func (f *factoryImpl) completionDurabilityGateFunc() func() error {
	if f == nil {
		return nil
	}
	f.mu.RLock()
	persist := f.completionDurabilityGate
	f.mu.RUnlock()
	return persist
}
