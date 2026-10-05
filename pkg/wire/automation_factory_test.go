package wire

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

type automationFactoryCommandRunner struct {
	request platformprocess.CommandRequest
}

func (r *automationFactoryCommandRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.request = request
	return platformprocess.CommandResult{Stdout: []byte("selected-script-edge")}, nil
}
func TestAutomationsCommandRunnerPreservesSelectedScriptEdge(t *testing.T) {
	t.Parallel()
	selected := &automationFactoryCommandRunner{}
	runner, err := provideAutomationsCommandRunner(serviceedges.Edges{ScriptCommandRunner: selected})
	if err != nil {
		t.Fatal(err)
	}
	request := platformprocess.CommandRequest{Command: "owned-script", Args: []string{"resume"}, Stdin: []byte("opaque-input")}
	result, err := runner.Run(context.Background(), request)
	if !reflect.DeepEqual(selected.request, request) {
		t.Fatalf("script request = %+v, want %+v", selected.request, request)
	}
	if err != nil || string(result.Stdout) != "selected-script-edge" {
		t.Fatalf("selected execution = %+v, %v", result, err)
	}
}

type automationNowOnlyClock struct{}

func (automationNowOnlyClock) Now() time.Time { return time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC) }

func TestAutomationsClockPreservesSelectedSchedulingAndNowOnlyCompatibility(t *testing.T) {
	t.Parallel()
	clock := clockwork.NewFakeClockAt(time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC))
	scheduler := platformclock.NewDeterministic(time.Unix(42, 0), time.Second)
	selected := provideAutomationsClock(clock, scheduler)
	timer := selected.NewTimer(time.Minute)
	clock.Advance(time.Minute)
	select {
	case fired := <-timer.Chan():
		if !fired.Equal(clock.Now()) {
			t.Fatalf("scheduled instant = %v, want %v", fired, clock.Now())
		}
	default:
		t.Fatal("selected clock advancement did not fire timer")
	}
	// Facts keep their selected origin while waits use the normalized scheduler.
	view := provideAutomationsClock(automationNowOnlyClock{}, scheduler)
	if !view.Now().Equal(automationNowOnlyClock{}.Now()) || view.Since(view.Now().Add(-time.Hour)) != time.Hour || view.Until(view.Now().Add(time.Hour)) != time.Hour {
		t.Fatal("Now-only facts were replaced")
	}
	controlled := view.NewTimer(time.Second)
	defer controlled.Stop()
	scheduler.SetTick(1)
	select {
	case instant := <-controlled.Chan():
		if !instant.Equal(scheduler.Now()) {
			t.Fatal("timer used fact time instead of scheduler time")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("selected scheduler did not fire")
	}
	wall := provideAutomationsClock(automationNowOnlyClock{}, platformclock.Real{})
	ready := wall.NewTimer(0)
	defer ready.Stop()
	select {
	case <-ready.Chan():
	case <-time.After(30 * time.Second):
		t.Fatal("Now-only scheduling timer did not fire")
	}
}
