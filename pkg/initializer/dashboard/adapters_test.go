package dashboard

import (
	"context"
	"errors"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
)

func TestRuntimeDashboardReaderBuildsBoundedRenderInput(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 10, 23, 0, 0, 0, time.UTC)
	snapshot := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{TickCount: 7}
	reader := NewRuntimeDashboardReader(fakeRuntimeDashboardReads{snapshot: snapshot})

	input, err := reader.ReadDashboard(context.Background(), now)
	if err != nil {
		t.Fatalf("ReadDashboard() error = %v", err)
	}
	if input.EngineState.TickCount != 7 || input.RenderData.ActiveExecutionsByDispatchID == nil || !input.Now.Equal(now) {
		t.Fatalf("ReadDashboard() input = %+v, want tick 7 at %s", input, now)
	}
}

func TestRuntimeDashboardReaderPropagatesReadFailures(t *testing.T) {
	t.Parallel()

	snapshotFailure := errors.New("snapshot unavailable")
	reader := NewRuntimeDashboardReader(fakeRuntimeDashboardReads{snapshotErr: snapshotFailure})
	if _, err := reader.ReadDashboard(context.Background(), time.Time{}); !errors.Is(err, snapshotFailure) {
		t.Fatalf("ReadDashboard() error = %v, want %v", err, snapshotFailure)
	}

	eventsFailure := errors.New("events unavailable")
	reader = NewRuntimeDashboardReader(fakeRuntimeDashboardReads{
		snapshot:  &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{},
		eventsErr: eventsFailure,
	})
	if _, err := reader.ReadDashboard(context.Background(), time.Time{}); !errors.Is(err, eventsFailure) {
		t.Fatalf("ReadDashboard() error = %v, want %v", err, eventsFailure)
	}
}

func TestDashboardRendererFuncAndClockTimingAdaptDependencies(t *testing.T) {
	t.Parallel()

	want := DashboardRenderInput{Now: time.Date(2026, time.July, 10, 23, 1, 0, 0, time.UTC)}
	var got DashboardRenderInput
	DashboardRendererFunc(func(input DashboardRenderInput) { got = input }).RenderDashboard(want)
	if got.Now != want.Now {
		t.Fatalf("rendered time = %s, want %s", got.Now, want.Now)
	}

	timing := ClockTiming{Clock: fixedClock{now: want.Now}}
	if now := timing.Now(); now != want.Now {
		t.Fatalf("Now() = %s, want %s", now, want.Now)
	}
	ticker := timing.NewTicker(time.Hour)
	if ticker == nil || ticker.C() == nil {
		t.Fatal("NewTicker() returned unusable ticker")
	}
	ticker.Stop()
}

type fakeRuntimeDashboardReads struct {
	snapshot    *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	snapshotErr error
	events      []factoryapi.FactoryEvent
	eventsErr   error
}

func (f fakeRuntimeDashboardReads) GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	return f.snapshot, f.snapshotErr
}

func (f fakeRuntimeDashboardReads) GetFactoryEvents(context.Context) ([]factoryapi.FactoryEvent, error) {
	return f.events, f.eventsErr
}

type fixedClock struct{ now time.Time }

func (f fixedClock) Now() time.Time { return f.now }
