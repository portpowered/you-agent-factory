package isolation_and_recovery_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The generations are ordered because the customer's recording must be sealed
// before resume. Both use the same inert process and distinct explicit Sessions.
func TestInvocationScheduleRecordingRecoveryContinuesSequence(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldFactory(t, invocationRecoveryConfig())
	project, home := t.TempDir(), t.TempDir()
	source := filepath.Join(project, "source.json")
	clock := &invocationRecoveryClock{FakeClock: clockwork.NewFakeClockAt(time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)), armed: make(chan struct{}, 8)}
	var calls atomic.Int32
	process := support.BuildProcess(t, serviceedges.Edges{
		Clock:               clock,
		ScriptCommandRunner: invocationRecoveryRunner{calls: &calls},
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			return ctx.Value(restartProbeServerKey{}).(*support.ProcessAPIServer).Start(ctx, request)
		},
	})
	support.CleanupProcess(t, process)
	var prefix []factoryapi.FactoryEvent
	var sourceBytes []byte
	var completedID string
	for generation := range 2 {
		api, sessionID := support.NewProcessAPIServer(), uuid.NewString()
		target := source
		args := []string{"you", "run", "--dir", dir, "--session", sessionID, "--continuously", "--with-server", "--server", "http://127.0.0.1:1", "--quiet"}
		if generation == 1 {
			target = filepath.Join(project, "successor.json")
			args = append(args, "--resume", source)
		}
		args = append(args, "--record", target)
		inputs := support.FakeInputs(context.WithValue(t.Context(), restartProbeServerKey{}, api), args)
		inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
		inputs.Input.WorkingDirectory = project
		command := startRestartProbeCommand(t, process, inputs.Input)
		base := restartProbeReadyURL(t, api, command)
		if generation == 0 {
			invokeRecoveryController(t, base, sessionID)
		} else {
			history := support.GetFactoryEventsForSessionAt(t, base, sessionID)
			assertInvocationRecoveryHistory(t, history, prefix)
			assertInvocationRecoveryWork(t, base, sessionID, "1", completedID)
		}
		select {
		case <-clock.armed:
		case <-time.After(15 * time.Second): //nolint:testsleep // Native failure ceiling for the injected scheduler's registration signal, not schedule progress.
			t.Fatalf("generation %d scheduler did not arm; stderr=%s", generation, inputs.Stderr())
		}
		if generation == 1 {
			if calls.Load() != 1 {
				t.Fatalf("resume repeated trigger-at-start/completed execution: calls=%d", calls.Load())
			}
			clock.Advance(time.Hour)
		}
		sequence := "1"
		if generation == 1 {
			sequence = "2"
		}
		item := waitInvocationRecoveryWork(t, base, sessionID, sequence)
		if generation == 0 {
			completedID = support.StringPointerValue(item.WorkId)
		} else {
			prior := assertInvocationRecoveryWork(t, base, sessionID, "1", completedID)
			if support.StringPointerValue(prior.SupersededBy) != support.StringPointerValue(item.WorkId) || item.SupersededBy != nil {
				t.Fatalf("recurring successor relationship changed: prior=%#v next=%#v", prior, item)
			}
			if support.StringPointerValue(item.WorkId) == completedID || calls.Load() != 2 {
				t.Fatalf("next occurrence reused completed Work or executed twice: work=%#v calls=%d", item, calls.Load())
			}
		}
		history := support.GetFactoryEventsForSessionAt(t, base, sessionID)
		assertInvocationRecoveryDispatches(t, history, generation+1)
		if generation == 0 {
			prefix = history
		}
		restartProbeShutdown(t, base, command)
		sourceBytes = assertInvocationRecoverySource(t, source, sourceBytes)
	}
}

func assertInvocationRecoverySource(t *testing.T, source string, original []byte) []byte {
	t.Helper()
	sealed, err := os.ReadFile(source)
	if err != nil || len(sealed) == 0 {
		t.Fatalf("sealed source: %v", err)
	}
	if original != nil && !bytes.Equal(original, sealed) {
		t.Fatal("resume modified the source recording")
	}
	return sealed
}

func assertInvocationRecoveryDispatches(t *testing.T, history []factoryapi.FactoryEvent, count int) {
	t.Helper()
	dispatches := support.ObserveDispatchEvents(t, history)
	if len(dispatches) != count {
		t.Fatalf("dispatch count=%d, want %d", len(dispatches), count)
	}
	for _, dispatch := range dispatches {
		if dispatch.Response == nil || dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf("recovered execution lacks accepted completion: %#v", dispatch)
		}
	}
}

func assertInvocationRecoveryHistory(t *testing.T, history, prefix []factoryapi.FactoryEvent) {
	t.Helper()
	position := 0
	for _, expected := range prefix {
		for position < len(history) && history[position].Id != expected.Id {
			position++
		}
		if position == len(history) {
			t.Fatalf("resume lost/reordered historical event %s", expected.Id)
		}
		// Replay enriches startup observations with recording metadata and Session
		// attribution. Every previously emitted value must survive that enrichment.
		before, err := json.Marshal(expected)
		if err != nil {
			t.Fatal(err)
		}
		after, err := json.Marshal(history[position])
		if err != nil {
			t.Fatal(err)
		}
		var left, right any
		if err := json.Unmarshal(before, &left); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(after, &right); err != nil {
			t.Fatal(err)
		}
		if !invocationRecoveryContains(left, right) {
			t.Fatalf("resume changed historical event %s: before=%s after=%s", expected.Id, before, after)
		}
		position++
	}
}

func invocationRecoveryContains(expected, actual any) bool {
	switch expected := expected.(type) {
	case map[string]any:
		got, ok := actual.(map[string]any)
		if !ok {
			return false
		}
		for key, value := range expected {
			if !invocationRecoveryContains(value, got[key]) {
				return false
			}
		}
		return true
	case []any:
		got, ok := actual.([]any)
		if !ok || len(got) != len(expected) {
			return false
		}
		for index, value := range expected {
			if !invocationRecoveryContains(value, got[index]) {
				return false
			}
		}
		return true
	default:
		return expected == actual
	}
}

type invocationRecoveryClock struct {
	*clockwork.FakeClock
	armed chan struct{}
}

func (clock *invocationRecoveryClock) AfterFunc(duration time.Duration, callback func()) clockwork.Timer {
	timer := clock.FakeClock.AfterFunc(duration, callback)
	// The only hour-long callback in this fixture is the selected duration job.
	if duration == time.Hour {
		clock.armed <- struct{}{}
	}
	return timer
}

type invocationRecoveryRunner struct{ calls *atomic.Int32 }

func (runner invocationRecoveryRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	return platformprocess.CommandResult{Stdout: []byte("selected recurring result")}, nil
}

func invokeRecoveryController(t *testing.T, base, sessionID string) {
	t.Helper()
	restartProbePost(t, base+"/factory-sessions/"+sessionID+"/invocations", []byte(`{"requestId":"recorded-controller","args":{"every":"1h","triggerAtStart":"true"},"timeoutMillis":1000}`))
}

func waitInvocationRecoveryWork(t *testing.T, base, sessionID, sequence string) factoryapi.Work {
	t.Helper()
	// Recurring occurrences share a name. The public history selector retains
	// completed predecessors after the next occurrence supersedes that name.
	endpoint := support.SessionWorkURL(base, sessionID, "/work?includeSuperseded=true")
	// Observe the public terminal projection; command completion alone precedes
	// publication and cannot prove that resumed Work is available to the customer.
	item, err := support.WaitForObservation(15*time.Second, func() (factoryapi.Work, error) {
		listed := support.GetJSON[factoryapi.ListWorkResponse](t, endpoint)
		for _, item := range listed.Results {
			if item.Tags != nil && (*item.Tags)["agent_factory.time.sequence"] == sequence && item.State != nil && item.State.Name == "complete" {
				return item, nil
			}
		}
		return factoryapi.Work{}, nil
	}, func(item factoryapi.Work) bool { return item.WorkId != nil })
	if err != nil {
		listed := support.GetJSON[factoryapi.ListWorkResponse](t, endpoint)
		encoded, marshalErr := json.Marshal(listed)
		t.Fatalf("completed sequence %s: %v; public Work=%s marshal=%v", sequence, err, encoded, marshalErr)
	}
	return item
}

func assertInvocationRecoveryWork(t *testing.T, base, sessionID, sequence, workID string) factoryapi.Work {
	t.Helper()
	item := waitInvocationRecoveryWork(t, base, sessionID, sequence)
	if support.StringPointerValue(item.WorkId) != workID {
		t.Fatalf("historical completed Work identity changed: %#v", item)
	}
	return item
}

func invocationRecoveryConfig() map[string]any {
	return map[string]any{
		"name":             "invocation-recovery",
		"invocationReturn": map[string]any{"policy": "EXPLICIT", "workTypeName": "execution", "terminalState": "complete"},
		"invocationSignature": map[string]any{"parameters": []map[string]any{
			{"name": "every", "bindings": []map[string]any{{"kind": "NAMED"}}},
			{"name": "triggerAtStart", "bindings": []map[string]any{{"kind": "NAMED"}}},
		}},
		"workTypes": []map[string]any{
			{"name": "controller", "handlingBehavior": []string{"DEFAULT"}, "states": []map[string]string{{"name": "active", "type": "INITIAL"}, {"name": "failed", "type": "FAILED"}}},
			{"name": "execution", "states": []map[string]string{{"name": "init", "type": "INITIAL"}, {"name": "complete", "type": "TERMINAL"}, {"name": "skipped", "type": "TERMINAL"}, {"name": "failed", "type": "FAILED"}}},
		},
		"workers": []map[string]any{{"name": "execute", "type": "SCRIPT_WORKER", "command": "controlled-recovery"}},
		"workstations": []map[string]any{
			{"name": "schedule", "behavior": "CRON", "worker": "execute", "cron": map[string]any{"every": "${every}", "triggerAtStart": true}, "inputs": []map[string]string{{"workType": "controller", "state": "active"}}, "outputs": []map[string]string{{"workType": "controller", "state": "active"}, {"workType": "execution", "state": "init"}}},
			{"name": "execute", "worker": "execute", "workPropagation": map[string]string{"mode": "PRESERVE_INPUT"}, "inputs": []map[string]string{{"workType": "execution", "state": "init"}}, "outputs": []map[string]string{{"workType": "execution", "state": "complete"}}, "onFailure": []map[string]string{{"workType": "execution", "state": "failed"}}},
		},
	}
}

const (
	automationRecoveryFixture = "tests/functional/sessions/isolation_and_recovery/testdata/automation-work-missing-occupancy.replay.json"
	automationRecoveryWorkID  = "work-recovered-after-automation"
	automationRecoveryTimeID  = "automation-time-daily-refresh-20260820"
)

// TestAutomationWorkWithoutRecordedOccupancyRestoresThroughRecordingProjection
// routes a checked-in recording through the assembled replay and resume
// processes. The recording contains a durable cron time Work whose accepted
// dispatch has no current place occupancy, plus another Work whose completed
// dispatch output is intermediate and is superseded by a later canonical state
// change.
func TestAutomationWorkWithoutRecordedOccupancyRestoresThroughRecordingProjection(t *testing.T) {
	t.Parallel()

	fixturePath := testutil.MustRepoPath(t, automationRecoveryFixture)
	fixture := testutil.LoadReplayArtifact(t, fixturePath)
	assertAutomationRecoveryFixture(t, fixture)

	factoryDir := support.ScaffoldFactory(t, automationRecoveryFactoryConfig())
	reusable := newAutomationRecoveryProcess(t)
	replay := reusable.run(
		t,
		factoryDir,
		"--replay", fixturePath, "--no-record",
	)
	replayed := waitForRecoveredAutomationWork(t, replay.url)
	assertRecoveredAutomationWorkList(t, replayed)
	replay.daemon.Stop(t)

	successorPath := filepath.Join(t.TempDir(), "automation-work-missing-occupancy-successor.replay.json")
	resumed := reusable.run(
		t,
		factoryDir,
		"--resume", fixturePath, "--record", successorPath,
	)
	resumedWork := waitForRecoveredAutomationWork(t, resumed.url)
	assertRecoveredAutomationWorkList(t, resumedWork)
	resumed.daemon.Stop(t)

	successor := testutil.LoadReplayArtifact(t, successorPath)
	dailyRefreshDispatches := 0
	for _, event := range successor.Events {
		if event.Type != interfaces.FactoryEventTypeDispatchRequest {
			continue
		}
		var payload interfaces.DispatchRequestEventPayload
		if err := event.DecodePayload(&payload); err != nil {
			t.Fatalf("decode successor dispatch request %q: %v", event.Id, err)
		}
		if payload.TransitionID == "daily-refresh" {
			dailyRefreshDispatches++
		}
	}
	if dailyRefreshDispatches != 1 {
		t.Fatalf("successor daily-refresh dispatch count = %d, want the one recorded completion without a redispatch", dailyRefreshDispatches)
	}
}

type automationRecoveryProcess struct {
	process         support.Process
	router          *reusableRootAPIServerStarter
	mockWorkersPath string
}

type automationRecoveryRun struct {
	url    string
	daemon *support.ProcessCommand
}

func newAutomationRecoveryProcess(t *testing.T) *automationRecoveryProcess {
	t.Helper()
	router := &reusableRootAPIServerStarter{}
	process := support.BuildProcess(t, serviceedges.Edges{APIServerStarter: router.start})
	support.CleanupProcess(t, process)
	mockWorkersPath := filepath.Join(t.TempDir(), "mock-workers.json")
	payload, err := json.Marshal(workerexecution.NewEmptyMockWorkersConfig())
	if err != nil {
		t.Fatalf("marshal empty mock-worker configuration: %v", err)
	}
	if err := os.WriteFile(mockWorkersPath, payload, 0o600); err != nil {
		t.Fatalf("write empty mock-worker configuration: %v", err)
	}
	return &automationRecoveryProcess{
		process:         process,
		router:          router,
		mockWorkersPath: mockWorkersPath,
	}
}

func (reusable *automationRecoveryProcess) run(
	t *testing.T,
	factoryDir string,
	extraArgs ...string,
) automationRecoveryRun {
	t.Helper()
	api := support.NewProcessAPIServer()
	reusable.router.setCurrent(api)
	args := append([]string{
		"you", "run",
		"--continuously", "--with-server", "--quiet", "--dir", factoryDir,
		"--with-mock-workers", reusable.mockWorkersPath,
	}, extraArgs...)
	inputs := support.FakeInputs(t.Context(), args)
	home := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = factoryDir
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		if stderr := inputs.Stderr(); stderr != "" {
			t.Logf("automation recovery daemon stderr: %s", stderr)
		}
	})
	daemon := support.StartProcessCommand(t, reusable.process, inputs.Input)
	baseURL := api.WaitForURL(t)
	support.WaitForStatus(t, baseURL, 15*time.Second, func(status factoryapi.StatusResponse) bool {
		return status.RuntimeStatus != ""
	})
	return automationRecoveryRun{url: baseURL, daemon: daemon}
}

func waitForRecoveredAutomationWork(t *testing.T, baseURL string) factoryapi.ListWorkResponse {
	t.Helper()
	support.WaitForStatus(t, baseURL, 15*time.Second, func(status factoryapi.StatusResponse) bool {
		return status.Categories.Initial == 0 && status.Categories.Processing == 0 && status.Categories.Terminal == 1
	})
	return support.ListDefaultSessionWork(t, baseURL)
}

func assertAutomationRecoveryFixture(t *testing.T, fixture *interfaces.ReplayArtifact) {
	t.Helper()
	if fixture.SchemaVersion != "agent-factory.replay.v1" {
		t.Fatalf("fixture schema version = %q, want agent-factory.replay.v1", fixture.SchemaVersion)
	}
	if len(fixture.Events) == 0 {
		t.Fatal("fixture has no events")
	}
	seenAutomationWork := false
	seenCompletion := false
	for _, event := range fixture.Events {
		if event.Type == interfaces.FactoryEventTypeWorkRequest {
			var payload work.WorkRequestEventPayload
			if err := event.DecodePayload(&payload); err != nil {
				t.Fatalf("decode fixture Work request %q: %v", event.Id, err)
			}
			for _, item := range payload.Works {
				if item.WorkID != automationRecoveryTimeID {
					continue
				}
				seenAutomationWork = item.Tags != nil && item.Tags["agent_factory.source"] == "cron" && item.Tags["agent_factory.cron.workstation"] == "daily-refresh"
			}
		}
		if event.Type == interfaces.FactoryEventTypeDispatchResponse {
			var payload workerexecution.DispatchResponseEventPayload
			if err := event.DecodePayload(&payload); err != nil {
				t.Fatalf("decode fixture dispatch response %q: %v", event.Id, err)
			}
			if event.Context.DispatchID != nil && *event.Context.DispatchID == "dispatch-daily-refresh" && payload.Outcome == "ACCEPTED" {
				seenCompletion = true
			}
		}
	}
	if !seenAutomationWork {
		t.Fatalf("fixture Work %q has no durable cron provenance", automationRecoveryTimeID)
	}
	if !seenCompletion {
		t.Fatal("fixture has no accepted daily-refresh completion")
	}
}

func assertRecoveredAutomationWorkList(t *testing.T, listed factoryapi.ListWorkResponse) {
	t.Helper()
	if len(listed.Results) != 1 {
		t.Fatalf("public Work list length = %d, want only the recoverable Work: %#v", len(listed.Results), listed.Results)
	}
	if !support.HasWorkAtCustomerState(listed, automationRecoveryWorkID, "task:complete") {
		locations := make([]string, 0, len(listed.Results))
		for _, item := range listed.Results {
			if item.WorkTypeName != nil && item.State != nil {
				locations = append(locations, support.WorkItemCustomerLocation(item))
			}
		}
		t.Fatalf("public Work list does not contain %q at task:complete: locations=%v listed=%#v", automationRecoveryWorkID, locations, listed.Results)
	}
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) == automationRecoveryTimeID {
			t.Fatalf("completed automation Work leaked into public Work list: %#v", item)
		}
	}
}

func automationRecoveryFactoryConfig() map[string]any {
	return map[string]any{
		"name": "automation-occupancy-recovery",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "to-complete", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "cron-worker"}},
		"workstations": []map[string]any{
			{
				"name":      "process-work",
				"worker":    "cron-worker",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "to-complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
			{
				"name":     "daily-refresh",
				"behavior": "CRON",
				"worker":   "cron-worker",
				"cron": map[string]any{
					"schedule":       "0 0 1 1 *",
					"triggerAtStart": false,
				},
				"outputs": []map[string]string{{"workType": "task", "state": "init"}},
			},
		},
	}
}
