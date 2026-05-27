package replay_test

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
)

func TestDeterministicClock_AdvancesFromLogicalTick(t *testing.T) {
	base := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	clock := replay.NewDeterministicClock(base, 10*time.Millisecond)

	if got := clock.Now(); !got.Equal(base) {
		t.Fatalf("initial Now() = %s, want %s", got, base)
	}

	clock.SetTick(3)
	want := base.Add(30 * time.Millisecond)
	if got := clock.Now(); !got.Equal(want) {
		t.Fatalf("tick 3 Now() = %s, want %s", got, want)
	}
}

func TestDeterministicClock_RepeatedReplayTicksProduceSameTimes(t *testing.T) {
	base := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	var firstRun []time.Time
	for _, tick := range []int{1, 2, 5, 8} {
		clock := replay.NewDeterministicClock(base, 0)
		clock.SetTick(tick)
		firstRun = append(firstRun, clock.Now())
	}

	for i, tick := range []int{1, 2, 5, 8} {
		clock := replay.NewDeterministicClock(base, 0)
		clock.SetTick(tick)
		if got := clock.Now(); !got.Equal(firstRun[i]) {
			t.Fatalf("tick %d repeated Now() = %s, want %s", tick, got, firstRun[i])
		}
	}
}

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
