package execution_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Each immutable cohort owns one root-built process. Wall/scheduler advancement
// is an ordered customer journey because it intentionally affects every session
// in that process. The two cohorts overlap and have separate profiles and effects.
func TestProcessTimeJourneys(t *testing.T) {
	t.Parallel()
	isolation := newCohortIsolation()
	t.Run("TestSelectedProcessTimeJourney", func(t *testing.T) {
		t.Parallel()
		c := startTimeCohort(t, false)
		runDefinitionsJourney(t, c)
		runHostedJourney(t, c, false)
		runCohortIsolation(t, c, isolation, false)
		runWebhookJourney(t, c, false)
	})
	t.Run("TestSpecializedSourceTimeJourney", func(t *testing.T) {
		t.Parallel()
		c := startTimeCohort(t, true)
		// SaveNow belongs to Runtime; exact construction overrides have Wire units.
		runDefinitionsJourney(t, c)
		runHostedJourney(t, c, true)
		runCohortIsolation(t, c, isolation, true)
		runWebhookJourney(t, c, true)
	})
}
func runDefinitionsJourney(t *testing.T, c *timeCohort) {
	t.Helper()
	dir := support.ScaffoldFactory(t, idleTimeConfig())
	support.ClearSeedInputs(t, dir)
	id := support.OpenFactorySessionAt(t, c.url, dir).Session.Id
	peerDir := support.ScaffoldFactory(t, idleTimeConfig())
	support.ClearSeedInputs(t, peerDir)
	peer := support.OpenFactorySessionAt(t, c.url, peerDir).Session.Id
	defer support.CloseFactorySessionAt(t, c.url, peer)
	beforePeer := support.GetJSON[factoryapi.Factory](t, factoryURL(c.url, peer))
	current := support.GetJSON[factoryapi.Factory](t, factoryURL(c.url, id))
	current = saveTimeVersion(t, c, id, current, c.wall.Now())
	c.wall.SetTick(60)
	current = saveTimeVersion(t, c, id, current, c.wall.Now())
	afterPeer := support.GetJSON[factoryapi.Factory](t, factoryURL(c.url, peer))
	if !reflect.DeepEqual(beforePeer, afterPeer) {
		t.Fatal("save mutated peer Current Factory")
	}
	support.CloseFactorySessionAt(t, c.url, id)
	id = support.OpenFactorySessionAt(t, c.url, dir).Session.Id
	defer support.CloseFactorySessionAt(t, c.url, id)
	if got := support.GetJSON[factoryapi.Factory](t, factoryURL(c.url, id)); !reflect.DeepEqual(got, current) {
		t.Fatal("public reopen lost saved Factory")
	}
	t.Log("D01: selected wall advancement, persisted reopen, logical increments and peer isolation")
	// Holding wall equal to the prior version exercises its monotonic fallback.
	current = saveTimeVersion(t, c, id, current, current.Version.Physical.Add(time.Nanosecond))
	t.Log("D02: unchanged wall advances prior physical by 1ns")
	stale := current
	data := putTimeFactory(t, c.url, id, stale, http.StatusConflict)
	var rejected factoryapi.ErrorResponse
	if err := json.Unmarshal(data, &rejected); err != nil {
		t.Fatal(err)
	}
	if rejected.Code != factoryapi.ErrorResponseCodeSTALEFACTORYVERSION {
		t.Fatalf("stale response = %#v", rejected)
	}
	assertUnchangedFactory(t, c, id, current)
	t.Log("D03: stale base returns typed conflict without mutation")
	invalid := advanceTimeBase(t, current)
	// A duplicate work type is invalid without changing unrelated worker policy.
	workTypes := append(*invalid.WorkTypes, (*invalid.WorkTypes)[0])
	invalid.WorkTypes = &workTypes
	data = putTimeFactory(t, c.url, id, invalid, http.StatusBadRequest)
	if err := json.Unmarshal(data, &rejected); err != nil {
		t.Fatal(err)
	}
	if rejected.Code != factoryapi.ErrorResponseCodeINVALIDFACTORY || rejected.Targets == nil || len(*rejected.Targets) == 0 {
		t.Fatalf("invalid response = %#v", rejected)
	}
	assertUnchangedFactory(t, c, id, current)
	t.Log("D04: invalid topology returns typed validation targets without mutation")
}

func advanceTimeBase(t *testing.T, current factoryapi.Factory) factoryapi.Factory {
	t.Helper()
	if current.Version == nil {
		t.Fatal("Current Factory lacks version")
	}
	version := *current.Version
	version.Logical++
	version.Physical = version.Physical.Add(time.Nanosecond)
	current.Version = &version
	return current
}

func saveTimeVersion(t *testing.T, c *timeCohort, id string, current factoryapi.Factory, physical time.Time) factoryapi.Factory {
	t.Helper()
	data := putTimeFactory(t, c.url, id, advanceTimeBase(t, current), http.StatusOK)
	var saved factoryapi.Factory
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Version == nil || !saved.Version.Physical.Equal(physical) || saved.Version.Logical != current.Version.Logical+1 {
		t.Fatalf("save version=%#v, want physical %v logical %v", saved.Version, physical, current.Version.Logical+1)
	}
	assertUnchangedFactory(t, c, id, saved)
	return saved
}

func assertUnchangedFactory(t *testing.T, c *timeCohort, id string, want factoryapi.Factory) {
	t.Helper()
	if got := support.GetJSON[factoryapi.Factory](t, factoryURL(c.url, id)); !reflect.DeepEqual(got, want) {
		t.Fatal("public Factory readback changed")
	}
}

func runHostedJourney(t *testing.T, c *timeCohort, specialized bool) {
	t.Helper()
	periodicID := c.open(t, "periodic")
	first := c.routes["periodic"].await(t)
	first.reply <- journeyReply{status: http.StatusOK, body: hostedTimeBody(false)}
	wait := c.hosted.await(t, 10*time.Second)
	assertTimeWork(t, c, periodicID, 1)
	c.routes["periodic"].assertHeld(t)
	advanceUnselectedTime(c, specialized)
	assertSourceWaitHeld(t, wait)
	c.routes["periodic"].assertHeld(t)
	c.hosted.SetTick(9)
	assertSourceWaitHeld(t, wait)
	c.routes["periodic"].assertHeld(t)
	c.hosted.SetTick(10)
	next := c.routes["periodic"].await(t)
	next.reply <- journeyReply{status: http.StatusOK, body: hostedTimeBody(false)}
	_ = c.hosted.await(t, 10*time.Second)
	assertTimeWork(t, c, periodicID, 1)
	support.CloseFactorySessionAt(t, c.url, periodicID)
	t.Log("H01: immediate initial poll, held interval, selected scheduler progression and deduplicated attributed Work/events")
	runHostedRetry(t, c, specialized)
	runHostedCancel(t, c)
	if specialized {
		t.Log("H04: specialized clock owns HTTP-date observations and scheduling; process advancement cannot release waits")
	}
}

func advanceUnselectedTime(c *timeCohort, specialized bool) {
	c.wall.SetTick(120)
	if specialized {
		c.process.SetTick(1000)
	}
}

func runHostedRetry(t *testing.T, c *timeCohort, specialized bool) {
	t.Helper()
	id := c.open(t, "retry")
	call := c.routes["retry"].await(t)
	now := c.wall.Now()
	if specialized {
		now = c.hosted.Now()
	}
	call.reply <- journeyReply{status: http.StatusTooManyRequests, body: `{"errors":[{"message":"limited"}]}`, header: http.Header{"Retry-After": {now.Add(30 * time.Second).Format(http.TimeFormat)}}}
	wait := c.hosted.await(t, 30*time.Second)
	assertTimeWork(t, c, id, 0)
	c.wall.SetTick(180)
	if specialized {
		c.process.SetTick(2000)
	}
	assertSourceWaitHeld(t, wait)
	c.routes["retry"].assertHeld(t)
	c.hosted.SetTick(39)
	assertSourceWaitHeld(t, wait)
	c.routes["retry"].assertHeld(t)
	c.hosted.SetTick(40)
	retry := c.routes["retry"].await(t)
	if retry.request.URL.String() != call.request.URL.String() || retry.request.Header.Get("Authorization") != call.request.Header.Get("Authorization") {
		t.Fatal("retry changed source identity")
	}
	retry.reply <- journeyReply{status: http.StatusOK, body: hostedTimeBody(false)}
	_ = c.hosted.await(t, 10*time.Second)
	assertTimeWork(t, c, id, 1)
	support.CloseFactorySessionAt(t, c.url, id)
	assertHostedRetryDiagnostics(t, c)
	t.Log("H02: HTTP-date delay uses selected wall; wall-only and insufficient scheduler advancement cannot retry; exact delay recovers Work")
}

func assertSourceWaitHeld(t *testing.T, wait timeWait) {
	t.Helper()
	select {
	case <-wait.channel:
		t.Fatal("source wait fired before its selected deadline")
	default:
	}
}

func assertHostedRetryDiagnostics(t *testing.T, c *timeCohort) {
	t.Helper()
	found := false
	for _, entry := range c.logs.All() {
		fields := entry.ContextMap()
		if entry.Message == "hosted linear poller restarting" && fields["delay_source"] == "provider" && fields["selected_delay"] == 30*time.Second {
			found = true
		}
		data, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "source-time-credential-") {
			t.Fatal("hosted diagnostics exposed resolved credential")
		}
	}
	if !found {
		t.Fatal("provider delay diagnostic missing")
	}
}

func runHostedCancel(t *testing.T, c *timeCohort) {
	t.Helper()
	blockedID, peerID := c.open(t, "blocked"), c.open(t, "peer")
	blocked, peer := c.routes["blocked"].await(t), c.routes["peer"].await(t)
	support.CloseFactorySessionAt(t, c.url, blockedID)
	select {
	case <-blocked.done:
	case <-time.After(30 * time.Second):
		t.Fatal("closed HTTP did not join")
	}
	if blocked.request.Context().Err() == nil {
		t.Fatal("closed HTTP context was not canceled")
	}
	blocked.reply <- journeyReply{status: http.StatusOK, body: strings.ReplaceAll(hostedTimeBody(false), "issue-time", "issue-closed-only")}
	backoffID := c.open(t, "backoff")
	backoff := c.routes["backoff"].await(t)
	backoff.reply <- journeyReply{status: http.StatusTooManyRequests, header: http.Header{"Retry-After": {"30"}}}
	oldWait := c.hosted.await(t, 30*time.Second)
	support.CloseFactorySessionAt(t, c.url, backoffID)
	c.hosted.SetTick(70)
	// The old wait is delivered only after the public close has joined its
	// source. A healthy peer's completed cycle acknowledges continued activity.
	select {
	case <-oldWait.channel:
	default:
		t.Fatal("old wait was not released")
	}
	peer.reply <- journeyReply{status: http.StatusOK, body: hostedTimeBody(false)}
	_ = c.hosted.await(t, 10*time.Second)
	assertTimeWork(t, c, peerID, 1)
	c.routes["blocked"].assertHeld(t)
	c.routes["backoff"].assertHeld(t)
	assertClosedTimeSession(t, c, blockedID)
	assertClosedTimeSession(t, c, backoffID)
	if c.lateAdmissions.Load() != 0 {
		t.Fatal("closed source admitted late Work")
	}
	support.CloseFactorySessionAt(t, c.url, peerID)
	t.Log("H03/H05: public close joins blocked HTTP and backoff; late release causes no HTTP/admission and healthy peer retains attributed terminal Work/events")
}

func assertClosedTimeSession(t *testing.T, c *timeCohort, id string) {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, c.url+"/factory-sessions?scope=live")
	for _, session := range listed.Sessions {
		if session.Id == id {
			t.Fatal("deleted session remains live")
		}
	}
}

func assertTimeWork(t *testing.T, c *timeCohort, id string, count int) {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(c.url, id, "/work"))
	if len(listed.Results) != count {
		t.Fatalf("Work count=%d, want %d: %#v", len(listed.Results), count, listed.Results)
	}
	if count == 0 {
		return
	}
	if !support.HasWorkAtCustomerState(listed, "linear:issue-time", support.WorkCustomerLocation("story", "queued")) {
		t.Fatalf("attributed terminal Work missing: %#v", listed.Results)
	}
	item := listed.Results[0]
	if item.RequestId == nil || item.Tags == nil || (*item.Tags)["linear_issue_id"] != "issue-time" {
		t.Fatalf("Work source correlation missing: %#v", item)
	}
	events := support.GetFactoryEventsForSessionAt(t, c.url, id)
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeWorkRequest {
			return
		}
	}
	t.Fatal("public session lacks Work request event")
}

func hostedTimeBody(empty bool) string {
	if empty {
		return `{"data":{"issues":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}`
	}
	return `{"data":{"issues":{"nodes":[{"id":"issue-time","identifier":"ENG-301","title":"Selected source time","description":"Session admission","updatedAt":"2039-01-01T08:10:00Z","url":"https://linear.app/example/issue/ENG-301","team":{"id":"team-owned","key":"ENG","name":"Engineering"},"state":{"id":"state-owned","name":"Todo","type":"unstarted"},"assignee":null}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}`
}
