package wire

import (
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

func TestHostedClockUsesSelectedProcessTime(t *testing.T) {
	t.Parallel()
	wall := platformclock.NewDeterministic(time.Unix(100, 0), time.Second)
	scheduler := platformclock.NewDeterministic(time.Unix(500, 0), time.Second)
	edges := serviceedges.Edges{Clock: wall, ProcessScheduler: scheduler}
	selected := provideAutomationHostedClock(edges)
	if got := selected.Now(); !got.Equal(wall.Now()) {
		t.Fatalf("Now = %v, want %v", got, wall.Now())
	}
	wait := selected.After(2 * time.Second)
	wall.SetTick(10)
	if !selected.Now().Equal(wall.Now()) {
		t.Fatal("wall advancement did not reach hosted observations")
	}
	assertProcessTimeHeld(t, wait)
	scheduler.SetTick(1)
	assertProcessTimeHeld(t, wait)
	scheduler.SetTick(2)
	select {
	case got := <-wait:
		if !got.Equal(scheduler.Now()) {
			t.Fatalf("After = %v, want scheduler time", got)
		}
	default:
		t.Fatal("scheduler advancement did not release hosted wait")
	}
	override := platformclock.NewDeterministic(time.Unix(900, 0), time.Second)
	edges.HostedClock = override
	if got := provideAutomationHostedClock(edges); got != override {
		t.Fatal("explicit hosted clock lost identity")
	}
	overridden := provideAutomationHostedClock(edges)
	overrideWait := overridden.After(time.Second)
	scheduler.SetTick(20)
	assertProcessTimeHeld(t, overrideWait)
	override.SetTick(1)
	if !overridden.Now().Equal(override.Now()) {
		t.Fatal("override Now lost")
	}
	select {
	case <-overrideWait:
	default:
		t.Fatal("override After lost")
	}
	peer := provideAutomationHostedClock(serviceedges.Edges{Clock: scheduler, ProcessScheduler: wall})
	if !peer.Now().Equal(scheduler.Now()) || !selected.Now().Equal(wall.Now()) {
		t.Fatal("independent cohort observations changed")
	}
}

func TestFactoryDefinitionClockUsesSelectedProcessTime(t *testing.T) {
	t.Parallel()
	wall := platformclock.NewDeterministic(time.Unix(100, 0), time.Second)
	edges := serviceedges.Edges{Clock: wall}
	selected := provideFactoryDefinitionClock(edges)
	if selected != wall {
		t.Fatal("definition default lost selected clock identity")
	}
	wall.SetTick(4)
	if !selected.Now().Equal(wall.Now()) {
		t.Fatal("definition observations did not advance")
	}
	override := platformclock.NewDeterministic(time.Unix(900, 0), time.Second)
	edges.FactoryDefinitionClock = override
	if got := provideFactoryDefinitionClock(edges); got != override || !got.Now().Equal(override.Now()) {
		t.Fatal("explicit definition clock lost identity or observation")
	}
	peer := provideFactoryDefinitionClock(serviceedges.Edges{Clock: override})
	if !peer.Now().Equal(override.Now()) || !selected.Now().Equal(wall.Now()) {
		t.Fatal("independent definition cohort observations changed")
	}
}

func assertProcessTimeHeld(t *testing.T, wait <-chan time.Time) {
	t.Helper()
	select {
	case got := <-wait:
		t.Fatalf("wait released before scheduler advancement: %v", got)
	default:
	}
}
