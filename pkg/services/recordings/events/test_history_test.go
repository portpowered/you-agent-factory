package events

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestNewFactoryEventHistoryRejectsMissingClockAndStreamGenerationID(t *testing.T) {
	t.Parallel()
	if history := NewFactoryEventHistory(nil, nil, "stream"); history != nil {
		t.Fatal("history constructed without a clock")
	}
	if history := NewFactoryEventHistory(nil, time.Now, ""); history != nil {
		t.Fatal("history constructed without a stream generation ID")
	}
}

func newTestFactoryEventHistory(
	topology recordings.InitialStructureSource,
	now func() time.Time,
	runtimeConfigs ...interfaces.RuntimeDefinitionLookup,
) *FactoryEventHistory {
	return NewFactoryEventHistory(topology, now, "test-stream-generation", runtimeConfigs...)
}
