package customer_journeys_test

import (
	"path/filepath"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func selectedWatcherPath(dir, id string) string {
	return filepath.Join(dir, interfaces.InputsDir, "task", interfaces.DefaultChannelName, id+".json")
}

func assertSelectedWatcherEmpty(t *testing.T, url, id string) {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(url, id, "/work"))
	if len(listed.Results) != 0 {
		t.Fatalf("ineligible file admitted Work: %#v", listed)
	}
}

// F-T05/F-T06/F-T09 observe real filesystem delivery and controlled scheduling.
// Read gates replace only FactoryRuntimeInputs, never the fsnotify implementation.
func assertSelectedWatcherTime(t *testing.T, url string, facts *platformclock.Deterministic,
	scheduler *selectedTimeScheduler, files *selectedTimeFiles,
) {
	t.Helper()
	dirA := support.ScaffoldFactory(t, filesystemWatcherFactoryConfig())
	dirB := support.ScaffoldFactory(t, filesystemWatcherFactoryConfig())
	support.ClearSeedInputs(t, dirA)
	support.ClearSeedInputs(t, dirB)
	opened := selectedTimeActivate(t, scheduler, func() factoryapi.OpenFactorySessionResponse { return support.OpenFactorySessionAt(t, url, dirA) })
	idA := opened.Session.Id
	files.awaitWatch(t, dirA)
	idB := selectedTimeActivate(t, scheduler, func() factoryapi.OpenFactorySessionResponse { return support.OpenFactorySessionAt(t, url, dirB) }).Session.Id
	files.awaitWatch(t, dirB)
	t.Cleanup(func() {
		if idA != "" {
			support.CloseFactorySessionAt(t, url, idA)
		}
	})
	t.Cleanup(func() { support.CloseFactorySessionAt(t, url, idB) })
	writeWatcherIngressFile(t, dirA, interfaces.DefaultChannelName, "pending", "pending item")
	pending := scheduler.await(t, 100*time.Millisecond)
	facts.SetTick(245000)
	assertSelectedWatcherEmpty(t, url, idA)
	scheduler.advance(99 * time.Millisecond)
	assertSelectedWatcherEmpty(t, url, idA)
	support.CloseFactorySessionAt(t, url, idA)
	idA = ""
	if !pending.stopped.Load() {
		t.Fatal("close did not retire pending debounce")
	}
	writeWatcherIngressFile(t, dirB, interfaces.DefaultChannelName, "peer", "peer item")
	scheduler.await(t, 100*time.Millisecond)
	scheduler.advance(100 * time.Millisecond)
	assertWatcherIngressWork(t, readAutomationCompletedWork(t, url, idB, "complete", "peer"), map[string]string{"peer": "peer item"})
	if files.count("watcher-pending") != 0 {
		t.Fatal("stopped debounce admitted late Work")
	}
	reopened := selectedTimeActivate(t, scheduler, func() factoryapi.OpenFactorySessionResponse { return support.OpenFactorySessionAt(t, url, dirA) })
	idA = reopened.Session.Id
	files.awaitWatch(t, dirA)
	if idA == opened.Session.Id || reopened.Session.FolderPath != opened.Session.FolderPath {
		t.Fatal("watcher reactivation changed logical target")
	}
	assertWatcherIngressWork(t, readAutomationCompletedWork(t, url, idA, "complete", "pending"), map[string]string{"pending": "pending item"})
	scheduler.advance(100 * time.Millisecond)
	assertSelectedWatcherRetry(t, url, idA, dirA, scheduler, files)
	assertSelectedWatcherRetryStop(t, url, &idA, idB, dirA, dirB, scheduler, files)
}

func assertSelectedWatcherRetryStop(t *testing.T, url string, idA *string, idB, dirA, dirB string, scheduler *selectedTimeScheduler, files *selectedTimeFiles) {
	t.Helper()
	// A second running callback waits on a frozen retry. Public close must cancel
	// its timer and join it before B can establish the positive peer barrier.
	files.hold(selectedWatcherPath(dirA, "cancel-retry"), 5)
	writeWatcherIngressFile(t, dirA, interfaces.DefaultChannelName, "cancel-retry", "retained retry item")
	scheduler.await(t, 100*time.Millisecond)
	scheduler.advance(100 * time.Millisecond)
	selectedTimeReceive(t, files.reads)
	retry := scheduler.await(t, 50*time.Millisecond)
	support.CloseFactorySessionAt(t, url, *idA)
	*idA = ""
	if !retry.stopped.Load() {
		t.Fatal("close left a running callback's retry timer")
	}
	writeWatcherIngressFile(t, dirB, interfaces.DefaultChannelName, "peer-after-retry", "peer retry item")
	scheduler.await(t, 100*time.Millisecond)
	scheduler.advance(100 * time.Millisecond)
	readAutomationCompletedWork(t, url, idB, "complete", "peer", "peer-after-retry")
	selectedTimeAbsent(t, files.reads)
	if files.count("watcher-cancel-retry") != 0 {
		t.Fatal("stopped retry admitted late Work")
	}
	files.hold(selectedWatcherPath(dirA, "cancel-retry"), 0)
	*idA = selectedTimeActivate(t, scheduler, func() factoryapi.OpenFactorySessionResponse { return support.OpenFactorySessionAt(t, url, dirA) }).Session.Id
	files.awaitWatch(t, dirA)
	readAutomationCompletedWork(t, url, *idA, "complete", "pending", "retry", "cancel-retry", "barrier")
	events := support.GetFactoryEventsForSessionAt(t, url, *idA)
	for _, id := range []string{"pending", "retry", "cancel-retry", "barrier"} {
		support.AssertSingleWorkRequestEvent(t, events, "watcher-"+id, id, "task")
	}
}

func assertSelectedWatcherRetry(t *testing.T, url, sessionID, dir string, scheduler *selectedTimeScheduler, files *selectedTimeFiles) {
	t.Helper()
	path := selectedWatcherPath(dir, "retry")
	files.hold(path, 1)
	writeWatcherIngressFile(t, dir, interfaces.DefaultChannelName, "retry", "retry item")
	scheduler.await(t, 100*time.Millisecond)
	scheduler.advance(100 * time.Millisecond)
	if got := selectedTimeReceive(t, files.reads); got != filepath.Clean(path) {
		t.Fatalf("empty read=%s, want %s", got, path)
	}
	scheduler.await(t, 50*time.Millisecond)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(url, sessionID, "/work"))
	if len(listed.Results) != 1 {
		t.Fatal("empty retry admitted Work before wait")
	}
	scheduler.advance(49 * time.Millisecond)
	selectedTimeAbsent(t, files.reads)
	scheduler.advance(time.Millisecond)
	assertWatcherIngressWork(t, readAutomationCompletedWork(t, url, sessionID, "complete", "pending", "retry"), map[string]string{"pending": "pending item", "retry": "retry item"})
	// Re-observe an already handled identity and introduce a positive barrier.
	writeWatcherIngressFile(t, dir, interfaces.DefaultChannelName, "retry", "retry item")
	writeWatcherIngressFile(t, dir, interfaces.DefaultChannelName, "barrier", "barrier item")
	scheduler.await(t, 100*time.Millisecond)
	scheduler.await(t, 100*time.Millisecond)
	scheduler.advance(100 * time.Millisecond)
	readAutomationCompletedWork(t, url, sessionID, "complete", "barrier")
	events := support.GetFactoryEventsForSessionAt(t, url, sessionID)
	support.AssertSingleWorkRequestEvent(t, events, "watcher-retry", "retry", "task")
}
