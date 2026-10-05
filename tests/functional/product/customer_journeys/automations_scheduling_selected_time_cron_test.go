package customer_journeys_test

import (
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// F-T04/F-T08 retain the public nominal/due/expiry/identity observer from the
// legacy journey while selecting a TimerSource that is not a clockwork.Clock.
func assertSelectedCronTime(t *testing.T, url string, facts *platformclock.Deterministic, scheduler *selectedTimeScheduler,
	routes map[string]chan work.FactorySubmissionRecord,
) {
	t.Helper()
	facts.SetTick(60000)
	dirA := support.ScaffoldFactory(t, cronSessionFactoryConfig("owned-cron-A"))
	cfgB := cronSessionFactoryConfig("owned-cron-B")
	// The legacy journey retains jitter; this cell isolates calendar due work.
	cfgB["workstations"] = cfgB["workstations"].([]map[string]any)[:1]
	dirB := support.ScaffoldFactory(t, cfgB)
	support.ClearSeedInputs(t, dirA)
	support.ClearSeedInputs(t, dirB)
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
	assertCronSessionTick(t, url, idA, dirA, routes["owned-cron-A"], "owned-cron-A", facts.Now())
	assertCronSessionTick(t, url, idB, dirB, routes["owned-cron-B"], "owned-cron-B", facts.Now())
	scheduler.await(t, time.Minute)
	scheduler.await(t, time.Minute)
	facts.SetTick(120000)
	selectedTimeAbsent(t, routes["owned-cron-A"])
	selectedTimeAbsent(t, routes["owned-cron-B"])
	scheduler.advance(59 * time.Second)
	selectedTimeAbsent(t, routes["owned-cron-A"])
	selectedTimeAbsent(t, routes["owned-cron-B"])
	scheduler.advance(time.Second)
	assertCronSessionTick(t, url, idA, dirA, routes["owned-cron-A"], "owned-cron-A", facts.Now())
	assertCronSessionTick(t, url, idB, dirB, routes["owned-cron-B"], "owned-cron-B", facts.Now())
	// Both pending timers are observed before close, then only the peer advances.
	timerA := scheduler.await(t, time.Minute)
	peerTimer := scheduler.await(t, time.Minute)
	support.CloseFactorySessionAt(t, url, idA)
	idA = ""
	if !timerA.stopped.Load() && !peerTimer.stopped.Load() {
		t.Fatal("cron close did not retire its selected wait")
	}
	facts.SetTick(180000)
	scheduler.advance(time.Minute)
	assertCronSessionTick(t, url, idB, dirB, routes["owned-cron-B"], "owned-cron-B", facts.Now())
	selectedTimeAbsent(t, routes["owned-cron-A"])
	scheduler.await(t, time.Minute)
	reopened := selectedTimeActivate(t, scheduler, func() factoryapi.OpenFactorySessionResponse { return support.OpenFactorySessionAt(t, url, dirA) })
	idA = reopened.Session.Id
	if idA == opened.Session.Id || reopened.Session.FolderPath != opened.Session.FolderPath {
		t.Fatal("cron reactivation identity changed")
	}
	assertCronSessionTick(t, url, idA, dirA, routes["owned-cron-A"], "owned-cron-A", facts.Now())
	scheduler.await(t, time.Minute)
	facts.SetTick(240000)
	scheduler.advance(time.Minute)
	assertCronSessionTick(t, url, idA, dirA, routes["owned-cron-A"], "owned-cron-A", facts.Now())
	assertCronSessionTick(t, url, idB, dirB, routes["owned-cron-B"], "owned-cron-B", facts.Now())
	assertCronCustomerWorkCount(t, url, idA, 2)
	assertCronCustomerWorkCount(t, url, idB, 4)
	support.CloseFactorySessionAt(t, url, idA)
	idA = ""
	// B is closed here so its clock no longer contributes to watcher eligibility.
	support.CloseFactorySessionAt(t, url, idB)
	idB = ""
}
