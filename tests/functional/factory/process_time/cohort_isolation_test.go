package process_time_test

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type cohortIsolation struct {
	ready    chan struct{}
	advanced chan struct{}
	observed chan struct{}
	finished chan struct{}
}

func newCohortIsolation() *cohortIsolation {
	return &cohortIsolation{ready: make(chan struct{}), advanced: make(chan struct{}), observed: make(chan struct{}), finished: make(chan struct{})}
}

// Only each cohort's owner advances its sources. The rendezvous proves the
// processes actually overlap while one has held work, without shared mutation.
func runCohortIsolation(t *testing.T, c *timeCohort, isolation *cohortIsolation, specialized bool) {
	t.Helper()
	dir := support.ScaffoldFactory(t, idleTimeConfig())
	support.ClearSeedInputs(t, dir)
	id := support.OpenFactorySessionAt(t, c.url, dir).Session.Id
	defer support.CloseFactorySessionAt(t, c.url, id)
	current := support.GetJSON[factoryapi.Factory](t, factoryURL(c.url, id))
	current = saveTimeVersion(t, c, id, current, c.wall.Now())
	wall := c.wall.Now()
	wait := c.process.After(time.Second)
	registered := c.process.await(t, time.Second)
	if specialized {
		close(isolation.ready)
		awaitCohortSignal(t, isolation.advanced)
		assertUnchangedFactory(t, c, id, current)
		if !c.wall.Now().Equal(wall) {
			t.Fatal("peer changed process wall")
		}
		assertSourceWaitHeld(t, registered)
		close(isolation.observed)
		advanceWebhookScheduler(c.process, 1)
		close(isolation.finished)
	} else {
		awaitCohortSignal(t, isolation.ready)
		c.wall.SetTick(int(c.wall.Now().Sub(time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC))/time.Second) + 1)
		advanceWebhookScheduler(c.process, 1)
		current = saveTimeVersion(t, c, id, current, c.wall.Now())
		close(isolation.advanced)
		awaitCohortSignal(t, isolation.observed)
		awaitCohortSignal(t, isolation.finished)
		assertUnchangedFactory(t, c, id, current)
		if !c.wall.Now().Equal(wall.Add(time.Second)) {
			t.Fatal("peer changed selected wall")
		}
	}
	select {
	case <-wait:
	default:
		t.Fatal("owner advancement did not release its wait")
	}
	t.Log("C01: overlapping immutable cohorts retain independent wall, scheduling and public Factory versions while peer work is held; scoped Work/events proved in both journeys")
}

func awaitCohortSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(30 * time.Second):
		t.Fatal("peer cohort did not reach observation barrier")
	}
}
