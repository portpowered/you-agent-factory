package automations

import (
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// F-T02/F-T03/F-T07: command completion and timer registration are barriers;
// exact Work and resume environment are observed through public session state.
func assertSelectedScriptTime(t *testing.T, url string, facts *platformclock.Deterministic, scheduler *selectedTimeScheduler,
	dirA, dirB string, routeA, routeB *scriptCycleRoute,
) {
	t.Helper()
	opened := selectedTimeActivate(t, scheduler, func() factoryapi.OpenFactorySessionResponse { return support.OpenFactorySessionAt(t, url, dirA) })
	idA := opened.Session.Id
	idB := selectedTimeActivate(t, scheduler, func() factoryapi.OpenFactorySessionResponse { return support.OpenFactorySessionAt(t, url, dirB) }).Session.Id
	t.Cleanup(func() {
		if idA != "" {
			support.CloseFactorySessionAt(t, url, idA)
		}
	})
	t.Cleanup(func() {
		if idB != "" {
			support.CloseFactorySessionAt(t, url, idB)
		}
	})
	firstA, firstB := awaitScriptCycleCommand(t, routeA), awaitScriptCycleCommand(t, routeB)
	cursor, checkpoint := "opaque-selected-cursor", "opaque-selected-checkpoint"
	firstA.output <- scriptCycleOutput(t, cursor, checkpoint)
	timerA := scheduler.await(t, 25*time.Millisecond)
	readScriptQueuedWork(t, url, idA, scriptPollerExternalWorkID)
	facts.SetTick(10)
	selectedTimeAbsent(t, routeA.entered)
	scheduler.advance(24 * time.Millisecond)
	selectedTimeAbsent(t, routeA.entered)
	scheduler.advance(time.Millisecond)
	secondA := awaitScriptCycleCommand(t, routeA)
	assertScriptResumeEnvironment(t, secondA.request, cursor, checkpoint)
	secondA.output <- nil
	timerA = scheduler.await(t, 50*time.Millisecond)
	firstB.output <- []byte("{malformed output")
	scheduler.await(t, 25*time.Millisecond)
	support.CloseFactorySessionAt(t, url, idA)
	idA = ""
	if !timerA.stopped.Load() {
		t.Fatal("close did not retire frozen script backoff")
	}
	scheduler.advance(25 * time.Millisecond)
	secondB := awaitScriptCycleCommand(t, routeB)
	selectedTimeAbsent(t, routeA.entered)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(url, idB, "/work"))
	if len(listed.Results) != 0 {
		t.Fatal("malformed script output admitted Work")
	}
	secondB.output <- scriptCycleOutput(t, "peer-cursor", "peer-checkpoint")
	scheduler.await(t, 50*time.Millisecond)
	readScriptQueuedWork(t, url, idB, scriptPollerExternalWorkID)
	reopened := selectedTimeActivate(t, scheduler, func() factoryapi.OpenFactorySessionResponse { return support.OpenFactorySessionAt(t, url, dirA) })
	idA = reopened.Session.Id
	if idA == opened.Session.Id || reopened.Session.FolderPath != opened.Session.FolderPath {
		t.Fatal("reactivation changed logical folder or reused live identity")
	}
	resumed := awaitScriptCycleCommand(t, routeA)
	assertScriptResumeEnvironment(t, resumed.request, cursor, checkpoint)
	resumed.output <- nil
	scheduler.await(t, 25*time.Millisecond)
	scheduler.advance(25 * time.Millisecond)
	afterEmpty := awaitScriptCycleCommand(t, routeA)
	assertScriptResumeEnvironment(t, afterEmpty.request, cursor, checkpoint)
	listed = support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(url, idA, "/work"))
	if len(listed.Results) != 0 {
		t.Fatal("empty cycle admitted Work after restart")
	}
	support.CloseFactorySessionAt(t, url, idA)
	idA = ""
	selectedTimeReceive(t, afterEmpty.canceled)
	support.CloseFactorySessionAt(t, url, idB)
	idB = ""
	selectedTimeAbsent(t, routeA.entered)
	selectedTimeAbsent(t, routeB.entered)
}
