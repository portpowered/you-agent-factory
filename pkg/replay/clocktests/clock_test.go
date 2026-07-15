package replay_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestNewArtifactClock_UsesRecordedTickEventTimes(t *testing.T) {
	base := time.Date(2026, time.April, 25, 20, 59, 3, 0, time.UTC)
	tickFour := time.Date(2026, time.April, 25, 21, 0, 0, 1067100, time.UTC)
	tickEight := tickFour.Add(40 * time.Second)
	clock := replay.NewArtifactClock(&interfaces.ReplayArtifact{
		RecordedAt: base,
		Events: []factoryapi.FactoryEvent{
			{
				Context: factoryapi.FactoryEventContext{
					Tick:      0,
					EventTime: base,
				},
			},
			{
				Context: factoryapi.FactoryEventContext{
					Tick:      4,
					EventTime: tickFour,
				},
			},
			{
				Context: factoryapi.FactoryEventContext{
					Tick:      8,
					EventTime: tickEight,
				},
			},
		},
	})

	clock.SetTick(4)
	if got := clock.Now(); !got.Equal(tickFour) {
		t.Fatalf("tick 4 Now() = %s, want %s", got, tickFour)
	}

	clock.SetTick(6)
	wantInterpolated := tickFour.Add(20 * time.Second)
	if got := clock.Now(); !got.Equal(wantInterpolated) {
		t.Fatalf("tick 6 Now() = %s, want %s", got, wantInterpolated)
	}
}
