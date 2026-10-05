package customer_journeys_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The two sessions share the customer's process clock. Advance it only after
// both public dispatches complete; this local sequence owns the stop/peer invariant.
func TestAutomationsCronAndIntervalSessionsStopIndependentlyAndRestart(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	clock := clockwork.NewFakeClockAt(start)
	sourceDir := support.ScaffoldFactory(t, cronSessionFactoryConfig("owned-cron-A"))
	peerDir := support.ScaffoldFactory(t, cronSessionFactoryConfig("owned-cron-B"))
	support.ClearSeedInputs(t, sourceDir)
	support.ClearSeedInputs(t, peerDir)
	server, routes := startCronSessionHost(t, clock)
	sourceID := support.OpenFactorySessionAt(t, server.URL(), sourceDir).Session.Id
	peerID := support.OpenFactorySessionAt(t, server.URL(), peerDir).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, server.URL(), peerID) })
	assertCronSessionTick(t, server.URL(), sourceID, sourceDir, routes["owned-cron-A"], "owned-cron-A", start)
	assertCronSessionTick(t, server.URL(), peerID, peerDir, routes["owned-cron-B"], "owned-cron-B", start)
	assertCronJitterPayload(t, waitForCronSubmission(t, routes["owned-cron-jitter"], "owned-cron-jitter", start, 10*time.Second), "owned-cron-jitter", start, 5*time.Second)
	advanceCronClock(t, clock, 2)
	due := start.Add(time.Minute)
	first := assertCronSessionTick(t, server.URL(), sourceID, sourceDir, routes["owned-cron-A"], "owned-cron-A", due)
	assertCronSessionTick(t, server.URL(), peerID, peerDir, routes["owned-cron-B"], "owned-cron-B", due)
	support.CloseFactorySessionAt(t, server.URL(), sourceID)
	advanceCronClock(t, clock, 1)
	due = due.Add(time.Minute)
	assertCronSessionTick(t, server.URL(), peerID, peerDir, routes["owned-cron-B"], "owned-cron-B", due)
	select {
	case record := <-routes["owned-cron-A"]:
		t.Fatalf("joined source admitted after stop: %#v", record)
	default:
	}
	restartedID := support.OpenFactorySessionAt(t, server.URL(), sourceDir).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, server.URL(), restartedID) })
	restarted := assertCronSessionTick(t, server.URL(), restartedID, sourceDir, routes["owned-cron-A"], "owned-cron-A", due)
	if restarted.Request.WorkID == first.Request.WorkID {
		t.Fatal("new nominal tick reused earlier Work identity")
	}
	advanceCronClock(t, clock, 2)
	due = due.Add(time.Minute)
	assertCronSessionTick(t, server.URL(), restartedID, sourceDir, routes["owned-cron-A"], "owned-cron-A", due)
	assertCronSessionTick(t, server.URL(), peerID, peerDir, routes["owned-cron-B"], "owned-cron-B", due)
	for _, name := range []string{"owned-cron-A", "owned-cron-B"} {
		if len(routes[name]) != 0 {
			t.Fatalf("duplicate cron admission for %s", name)
		}
	}
	assertCronCustomerWorkCount(t, server.URL(), restartedID, 2)
	assertCronCustomerWorkCount(t, server.URL(), peerID, 4)
}

func assertCronCustomerWorkCount(t *testing.T, baseURL, sessionID string, minimum int) {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, sessionID, "/work"))
	// The peer also owns the jitter characterization source. All visible Work
	// must be ordinary output; consumed system-time Work stays hidden by policy.
	if len(listed.Results) < minimum {
		t.Fatalf("cron output count=%d, want at least %d", len(listed.Results), minimum)
	}
	for _, item := range listed.Results {
		if item.State == nil || item.State.Name != "init" || support.StringPointerValue(item.WorkTypeName) != "task" {
			t.Fatalf("unexpected visible cron Work: %#v", item)
		}
	}
}

func startCronSessionHost(t *testing.T, clock *clockwork.FakeClock) (*support.FunctionalAPIServer, map[string]chan work.FactorySubmissionRecord) {
	t.Helper()
	hostDir := support.ScaffoldFactory(t, map[string]any{"workTypes": []map[string]any{{
		"name": "idle", "states": []map[string]string{{"name": "init", "type": "INITIAL"}, {"name": "done", "type": "TERMINAL"}},
	}}})
	support.ClearSeedInputs(t, hostDir)
	routes := map[string]chan work.FactorySubmissionRecord{
		"owned-cron-A":      make(chan work.FactorySubmissionRecord, 8),
		"owned-cron-B":      make(chan work.FactorySubmissionRecord, 8),
		"owned-cron-jitter": make(chan work.FactorySubmissionRecord, 8),
	}
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: hostDir,
		Edges: serviceedges.Edges{
			Clock: clock, ScriptCommandRunner: scriptCycleRouter{},
			SubmissionRecorder: func(record work.FactorySubmissionRecord) {
				route := routes[record.Request.Tags[interfaces.TimeWorkTagKeyCronWorkstation]]
				if route == nil {
					t.Errorf("unexpected cron admission: %#v", record)
					return
				}
				select {
				case route <- record:
				default:
					t.Error("owned cron admission overflow")
				}
			},
		},
		BeforeStart: func(tb testing.TB, process support.Process, input root.Input) {
			for _, route := range routes {
				if len(route) != 0 {
					tb.Fatal("cron admitted before activation")
				}
			}
			support.InitializeCustomerHomeWithProcess(tb, process, input.Env, hostDir)
		},
	})
	t.Cleanup(func() { server.Stop(t) })
	return server, routes
}

func advanceCronClock(t *testing.T, clock *clockwork.FakeClock, schedulers int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	// BlockUntil observes registered scheduler timers, avoiding a scheduling sleep.
	if err := clock.BlockUntilContext(ctx, schedulers); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Minute)
}

func cronSessionFactoryConfig(name string) map[string]any {
	cfg := cronCompositionFactoryConfig()
	cfg["workers"] = []map[string]string{{"name": "cron-worker", "type": "SCRIPT_WORKER", "command": "owned-cron-worker"}}
	ws := cfg["workstations"].([]map[string]any)[0]
	ws["name"] = name
	if name == "owned-cron-A" {
		// Exercise duration scheduling through the canonical lifecycle owner.
		// The peer retains calendar scheduling, including jitter and expiry.
		ws["cron"] = map[string]any{"every": "1m", "triggerAtStart": true}
	}
	extra := map[string]any{
		"name": "owned-cron-malformed", "behavior": "CRON", "worker": "cron-worker",
		"cron":    map[string]any{"schedule": "not-a-cron", "triggerAtStart": true},
		"outputs": []map[string]string{{"workType": "task", "state": "init"}},
	}
	if name == "owned-cron-B" {
		extra["name"] = "owned-cron-jitter"
		extra["cron"] = map[string]any{"schedule": "* * * * *", "triggerAtStart": true, "jitter": "5s", "expiryWindow": "10s"}
	}
	cfg["workstations"] = []map[string]any{ws, extra}
	return cfg
}

func assertCronSessionTick(t *testing.T, baseURL, sessionID, directory string, route <-chan work.FactorySubmissionRecord,
	workstation string, nominal time.Time,
) work.FactorySubmissionRecord {
	t.Helper()
	record := waitForCronSubmission(t, route, workstation, nominal, 10*time.Second)
	identity := sha256.Sum256([]byte(directory + "\x00" + workstation + "\x00" + nominal.UTC().Format(time.RFC3339Nano)))
	if record.Request.WorkTypeID != interfaces.SystemTimeWorkTypeID ||
		record.Request.WorkID != "time-"+hex.EncodeToString(identity[:16]) || record.Request.RequestID != "request-"+record.Request.WorkID {
		t.Fatalf("scheduled Work identity/state = %#v", record.Request)
	}
	assertCronJitterPayload(t, record, workstation, nominal, 0)
	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(baseURL, sessionID))
	defer stream.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var history []factoryapi.FactoryEvent
	dispatchID := ""
	for {
		event := stream.NextEventContext(ctx)
		history = append(history, event)
		if (event.Type == factoryapi.FactoryEventTypeDispatchRequest || event.Type == factoryapi.FactoryEventTypeDispatchResponse) &&
			(event.Context.SessionId == nil || *event.Context.SessionId != sessionID) {
			t.Fatalf("cron event belongs to wrong session: %#v", event.Context)
		}
		if event.Type == factoryapi.FactoryEventTypeDispatchRequest {
			request, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				t.Fatal(err)
			}
			if request.TransitionId != workstation {
				continue
			}
			for _, item := range support.DispatchInputWorksFromHistory(t, history, event, request) {
				if support.StringPointerValue(item.WorkId) == record.Request.WorkID {
					dispatchID = support.StringPointerValue(event.Context.DispatchId)
					assertCronScheduledInput(t, item, workstation, nominal)
				}
			}
		}
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse || dispatchID == "" || support.StringPointerValue(event.Context.DispatchId) != dispatchID {
			continue
		}
		response, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatal(err)
		}
		if response.Outcome != factoryapi.WorkOutcomeAccepted || response.OutputWork == nil || len(*response.OutputWork) != 1 {
			t.Fatalf("cron output = %#v", response)
		}
		output := (*response.OutputWork)[0]
		// Dispatch completion precedes publication of the public Work projection.
		listed, err := support.WaitForObservation(10*time.Second,
			func() (factoryapi.ListWorkResponse, error) {
				return readWorkSnapshot(ctx, support.SessionWorkURL(baseURL, sessionID, "/work"))
			},
			func(listed factoryapi.ListWorkResponse) bool {
				return support.HasWorkAtCustomerState(listed, support.StringPointerValue(output.WorkId), support.WorkCustomerLocation("task", "init"))
			},
		)
		if err != nil {
			t.Fatalf("cron public output missing: %v; last Work: %#v", err, listed)
		}
		return record
	}
}

func assertCronScheduledInput(t *testing.T, item factoryapi.Work, workstation string, nominal time.Time) {
	t.Helper()
	publicPayload, err := json.Marshal(item.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var facts map[string]string
	if err := json.Unmarshal(publicPayload, &facts); err != nil {
		t.Fatal(err)
	}
	if facts["cron_workstation"] != workstation || facts["nominal_at"] != nominal.UTC().Format(time.RFC3339Nano) || facts["jitter"] != "0s" {
		t.Fatalf("scheduled public Work payload = %#v", item.Payload)
	}
	if item.State == nil || item.State.Name != interfaces.SystemTimePendingState {
		t.Fatalf("scheduled public Work state = %#v", item)
	}
}

func cronCompositionFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"workers": []map[string]string{{"name": "cron-worker"}},
		"workstations": []map[string]any{{
			"name":     "scheduled-task",
			"behavior": "CRON",
			"worker":   "cron-worker",
			"cron": map[string]any{
				"schedule":       "* * * * *",
				"triggerAtStart": true,
				"expiryWindow":   "10s",
			},
			"outputs": []map[string]string{{"workType": "task", "state": "init"}},
		}},
	}
}

func assertCronJitterPayload(
	t *testing.T,
	record work.FactorySubmissionRecord,
	workstation string,
	nominalAt time.Time,
	maxJitter time.Duration,
) {
	t.Helper()

	var payload map[string]string
	if err := json.Unmarshal(record.Request.Payload, &payload); err != nil {
		t.Fatalf("cron submission payload is not JSON: %v", err)
	}
	if payload["cron_workstation"] != workstation {
		t.Fatalf("cron submission payload workstation = %q, want %q", payload["cron_workstation"], workstation)
	}

	nominal, err := time.Parse(time.RFC3339Nano, payload["nominal_at"])
	if err != nil {
		t.Fatalf("cron submission nominal_at = %q: %v", payload["nominal_at"], err)
	}
	if !nominal.Equal(nominalAt.UTC()) {
		t.Fatalf("cron submission nominal_at = %s, want %s", nominal, nominalAt.UTC())
	}

	jitter, err := time.ParseDuration(payload["jitter"])
	if err != nil {
		t.Fatalf("cron submission jitter = %q: %v", payload["jitter"], err)
	}
	if jitter < 0 || jitter > maxJitter {
		t.Fatalf("cron submission jitter = %s, want inclusive [0, %s]", jitter, maxJitter)
	}

	dueAt, err := time.Parse(time.RFC3339Nano, payload["due_at"])
	if err != nil {
		t.Fatalf("cron submission due_at = %q: %v", payload["due_at"], err)
	}
	if !dueAt.Equal(nominal.Add(jitter)) {
		t.Fatalf("cron submission due_at = %s, want nominal+jitter=%s", dueAt, nominal.Add(jitter))
	}
}

func waitForCronSubmission(
	t *testing.T,
	submissions <-chan work.FactorySubmissionRecord,
	workstation string,
	nominalAt time.Time,
	timeout time.Duration,
) work.FactorySubmissionRecord {
	t.Helper()

	wantNominalAt := nominalAt.UTC().Format(time.RFC3339Nano)
	deadline := time.After(timeout)
	for {
		select {
		case record := <-submissions:
			if record.Request.Tags[interfaces.TimeWorkTagKeyCronWorkstation] != workstation {
				continue
			}
			if got := record.Request.Tags[interfaces.TimeWorkTagKeyNominalAt]; got != wantNominalAt {
				t.Fatalf("cron submission nominal_at = %q, want %q", got, wantNominalAt)
			}
			return record
		case <-deadline:
			t.Fatalf("timed out waiting for cron submission from %q at %s", workstation, wantNominalAt)
		}
	}
}
