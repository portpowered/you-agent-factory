package cli_rest_journeys_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The parent owns the reusable root. Each cell owns its Session and routes;
// HTTP parity observes the same customer summaries as Process.Execute.
func testWorkerSessionsScopedPhysicalFacts(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"success", "business"} {
		t.Run("F-"+name, func(t *testing.T) {
			t.Parallel()
			c := newWorkerSessionsCLICase(t)
			if name == "business" {
				path := filepath.Join(c.factoryDir, "factory.json")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var doc map[string]any
				if err := json.Unmarshal(data, &doc); err != nil {
					t.Fatal(err)
				}
				doc["workstations"].([]any)[0].(map[string]any)["outcomeFormat"] = "decision-envelope"
				data, err = json.Marshal(doc)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			route := "worker-session-physical-" + name
			c.registerRoutes(t, route)
			f, ctx := c.fixture, t.Context()
			sessionID := c.openSession(t)
			workID := submitWork(t, ctx, f.process, functionalEnvironment(f.homeDir), c.factoryDir, f.baseURL, sessionID, route)
			row := waitForWorkerSessionState(t, ctx, f.process, functionalEnvironment(f.homeDir), c.factoryDir, f.baseURL, sessionID, workID, "COMPLETED")
			waitForScopedUsageCommit(t, ctx, f.baseURL+"/factory-sessions/"+sessionID+"/worker-sessions?workId="+workID)
			shown := assertPhysicalWorkerParity(t, c, sessionID, row.WorkerSessionID, true)
			if shown.FactorySessionId == nil || *shown.FactorySessionId != sessionID || shown.WorkId == nil || *shown.WorkId != workID {
				t.Fatalf("physical attribution: %+v", shown)
			}
			// Work processing completes independently; observe its public outcome.
			support.WaitForSessionTerminalStatus(t, f.baseURL, sessionID, 20*time.Second)
			work := support.GetJSON[factoryapi.Work](t, f.baseURL+"/factory-sessions/"+sessionID+"/work/"+workID)
			want := factoryapi.WorkStateTypeTERMINAL
			if name == "business" {
				want = factoryapi.WorkStateTypeFAILED
			}
			if work.State == nil || work.State.Type != want {
				t.Fatalf("independent Work outcome for %s: %+v", name, work)
			}
			assertPhysicalWorkerParity(t, c, sessionID, row.WorkerSessionID, true)
		})
	}
	t.Run("F-live-and-fence", testScopedPhysicalLiveAndFence)
	t.Run("F-error", testWorkerSessionsListWorkScopedSelectedReadFailure)
	t.Run("F-empty", testWorkerSessionsListWorkScopedEmpty)
}

func assertPhysicalWorkerParity(t *testing.T, c *workerSessionsCLICase, scope, id string, terminal bool) factoryapi.WorkerSessionObservation {
	t.Helper()
	f := c.fixture
	var shown factoryapi.WorkerSessionObservation
	for _, session := range []string{"", scope} {
		args := []string{"--server", f.baseURL, "worker-sessions", "show", "--worker-session-id", id, "--output", "json"}
		if session != "" {
			args = append(args, "--session", session)
		}
		inputs := executeCLI(t, t.Context(), f.process, functionalEnvironment(f.homeDir), c.factoryDir, args...)
		var row factoryapi.WorkerSessionObservation
		decodeCLIJSON(t, inputs, &row)
		path := "/worker-sessions/" + id
		if session != "" {
			path = "/factory-sessions/" + session + path
		}
		httpRow := support.GetJSON[factoryapi.WorkerSessionObservation](t, f.baseURL+path)
		// Confirmation belongs to the independent Factory durability gate;
		// running elapsed duration advances between these sequential reads.
		httpRow.ConfirmationState = row.ConfirmationState
		if !terminal {
			httpRow.DurationMillis = row.DurationMillis
		}
		if !reflect.DeepEqual(row, httpRow) {
			t.Fatalf("CLI/HTTP physical summary mismatch: %+v %+v", row, httpRow)
		}
		if session == "" {
			shown = row
		} else {
			row.ConfirmationState = shown.ConfirmationState
			if !terminal {
				row.DurationMillis = shown.DurationMillis
			}
			if !reflect.DeepEqual(shown, row) {
				t.Fatalf("scoped/top-level physical summary mismatch: %+v %+v", shown, row)
			}
		}
	}
	if terminal {
		if shown.State != "COMPLETED" || shown.EndedAt == nil || shown.DurationMillis == nil || shown.TerminalCause == nil || *shown.TerminalCause != "COMPLETED" || shown.Transcript != "AVAILABLE" || shown.RecordingHealth == nil || *shown.RecordingHealth != "COMPLETE" {
			t.Fatalf("physical completion facts: %+v", shown)
		}
		var original factoryapi.WorkerSessionTranscriptResponse
		for _, session := range []string{"", scope} {
			args := []string{"--server", f.baseURL, "worker-sessions", "read", "--worker-session-id", id, "--output", "json"}
			if session != "" {
				args = append(args, "--session", session)
			}
			inputs := executeCLI(t, t.Context(), f.process, functionalEnvironment(f.homeDir), c.factoryDir, args...)
			var transcript factoryapi.WorkerSessionTranscriptResponse
			decodeCLIJSON(t, inputs, &transcript)
			if transcript.State != "COMPLETED" || len(transcript.Entries) == 0 {
				t.Fatalf("physical transcript: %+v", transcript)
			}
			if session == "" {
				original = transcript
			} else {
				// The published transcript field records the explicit read selector,
				// while the summary owns physical Factory Session attribution.
				if transcript.FactorySessionId == nil || *transcript.FactorySessionId != scope {
					t.Fatalf("transcript selector lost: %+v", transcript)
				}
				transcript.FactorySessionId = original.FactorySessionId
				if !reflect.DeepEqual(original, transcript) {
					t.Fatalf("scoped transcript differs: %+v %+v", original, transcript)
				}
			}
		}
	} else if shown.State != "RUNNING" || shown.EndedAt != nil || shown.TerminalCause != nil || shown.RecordingHealth == nil || *shown.RecordingHealth != "INCOMPLETE" || shown.RecordingHealthReason != nil {
		t.Fatalf("live owned capture falsely interrupted: %+v", shown)
	}
	return shown
}

func testScopedPhysicalLiveAndFence(t *testing.T) {
	t.Parallel()
	c := newWorkerSessionsCLICase(t)
	f := c.fixture
	ids, scopes := make([]string, 2), make([]string, 2)
	for index, name := range []string{"live", "peer"} {
		route := "worker-session-physical-" + name
		c.registerRoutes(t, route)
		f.runner.mu.Lock()
		gate := f.runner.definitions[route].gate
		f.runner.mu.Unlock()
		t.Cleanup(gate.release)
		scopes[index] = c.openSession(t)
		// The explicit same Work ID in two scopes must never lend authority.
		request := fmt.Sprintf(`{"name":%q,"workId":"physical-shared-work","workTypeName":"task","payload":{}}`, route)
		executeCLI(t, t.Context(), f.process, functionalEnvironment(f.homeDir), c.factoryDir, "--server", f.baseURL, "--json", "submit", "batch", "--session", scopes[index], `{"requestId":"physical-request","type":"FACTORY_REQUEST_BATCH","works":[`+request+`]}`)
		row := waitForWorkerSessionState(t, t.Context(), f.process, functionalEnvironment(f.homeDir), c.factoryDir, f.baseURL, scopes[index], "physical-shared-work", "RUNNING")
		ids[index] = row.WorkerSessionID
		assertPhysicalWorkerParity(t, c, scopes[index], ids[index], false)
	}
	for _, command := range []string{"show", "read"} {
		inputs, err := executeCLIExpectError(t, t.Context(), f.process, functionalEnvironment(f.homeDir), c.factoryDir, "--server", f.baseURL, "worker-sessions", command, "--session", scopes[1], "--worker-session-id", ids[0], "--output", "json")
		if err == nil || strings.TrimSpace(inputs.Stdout()) != "" {
			t.Fatalf("foreign %s disclosed physical capture: %v %s", command, err, inputs.Stdout())
		}
	}
	f.runner.mu.Lock()
	gate := f.runner.definitions["worker-session-physical-live"].gate
	f.runner.mu.Unlock()
	gate.release()
	waitForWorkerSessionState(t, t.Context(), f.process, functionalEnvironment(f.homeDir), c.factoryDir, f.baseURL, scopes[0], "physical-shared-work", "COMPLETED")
	assertPhysicalWorkerParity(t, c, scopes[0], ids[0], true)
	assertPhysicalWorkerParity(t, c, scopes[1], ids[1], false)
}

// TestWorkerSessionsReplayOnlyRedirectsWellFormedNDJSON proves that the
// published you process can finish a finite replay through shell redirection
// without cancellation or diagnostics contaminating stdout.
func testProvidersessionscliWorkerSessionsReplayOnlyRedirectsWellFormedNDJSON(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	fixture := newWorkerSessionReplayFixture(t, ctx, "worker-session-replay-only-redirect", "session_fixture_codex_replay_redirect")
	defer fixture.stop(t)
	workID := submitWork(t, ctx, fixture.process, fixture.env, fixture.factoryDir, fixture.baseURL, fixture.sessionID, "worker-session-replay-only-redirect")
	waitForWorkerSession(t, ctx, fixture.process, fixture.env, fixture.factoryDir, fixture.baseURL, fixture.sessionID, workID)
	streamWorkerSession(t, ctx, fixture.process, fixture.env, fixture.factoryDir, fixture.baseURL, fixture.sessionID, fixture.providerSessionID, "COMPLETED")

	contents, diagnostics := runBuiltWorkerSessionReplay(t, ctx, fixture)
	assertWorkerSessionReplayCapture(t, contents, diagnostics)
	assertProviderCommandRoutesSince(t, fixture.runner, fixture.routeStart, map[string]struct{}{fixture.requestID: {}})
	fixture.caseFixture.closeRoute(t, fixture.requestID)
}

type workerSessionReplayFixture struct {
	caseFixture       *workerSessionsCLICase
	process           support.Process
	factoryDir        string
	env               []string
	baseURL           string
	sessionID         string
	requestID         string
	providerSessionID string
	runner            *providerCommandRouteRunner
	routeStart        int
}

// TestWSRFT001OpeningRecordPrecedesProviderOutput exercises the customer
// Worker Session stream against the production root-built process and asserts
// the retained history order directly. The provider command runner replays the
// sanitized Codex fixture; no Mock Worker or timing sleep participates in the
// observation.
//
// WSR-FT-001: opening-first, provider-before-output, terminal-last.
// golden: tests/functional/internal/support/testdata/provider-sessions/codex/success/manifest.json
func testProvidersessionscliWSRFT001OpeningRecordPrecedesProviderOutput(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	fixture := newWorkerSessionReplayFixture(t, ctx, "wsr-ft-001", "session_fixture_codex_wsr_ft_001")
	defer fixture.stop(t)
	workID := submitWork(t, ctx, fixture.process, fixture.env, fixture.factoryDir, fixture.baseURL, fixture.sessionID, "wsr-ft-001")
	waitForWorkerSession(t, ctx, fixture.process, fixture.env, fixture.factoryDir, fixture.baseURL, fixture.sessionID, workID)
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, fixture.sessionID, 30*time.Second)
	assertScopedWorkerSessionList(t, listWorkerSessionsForFactorySession(t, fixture.baseURL, fixture.sessionID, workID), fixture.sessionID, fixture.providerSessionID, workID)
	frames := replayWorkerSessionFrames(t, ctx, fixture)
	assertWSRWorkerSessionHistory(t, frames, fixture.sessionID, workID, "COMPLETED")
	assertProviderCommandRoutesSince(t, fixture.runner, fixture.routeStart, map[string]struct{}{fixture.requestID: {}})
	fixture.caseFixture.closeRoute(t, fixture.requestID)
}

// TestWSRFT002LiveAndReplayCorrelationRemainStable compares the public live
// Worker Session observation with the replay-only stream. The opening's exact
// timestamp and identity must survive both projections, while the stream's
// provider-native records remain after the opening lifecycle record.
//
// WSR-FT-002: live/replay correlation and exact opening timestamp.
func testProvidersessionscliWSRFT002LiveAndReplayCorrelationRemainStable(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	fixture := newWorkerSessionReplayFixture(t, ctx, "wsr-ft-002", "session_fixture_codex_wsr_ft_002")
	defer fixture.stop(t)
	workID := submitWork(t, ctx, fixture.process, fixture.env, fixture.factoryDir, fixture.baseURL, fixture.sessionID, "wsr-ft-002")
	waitForWorkerSession(t, ctx, fixture.process, fixture.env, fixture.factoryDir, fixture.baseURL, fixture.sessionID, workID)
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, fixture.sessionID, 30*time.Second)
	live := listWorkerSessionsForFactorySession(t, fixture.baseURL, fixture.sessionID, workID)
	if len(live.Sessions) != 1 {
		t.Fatalf("live Worker Session observations = %#v, want exactly one", live)
	}
	assertScopedWorkerSessionList(t, live, fixture.sessionID, fixture.providerSessionID, workID)
	frames := replayWorkerSessionFrames(t, ctx, fixture)
	assertWSRLiveReplayCorrelation(t, live.Sessions[0], frames, fixture.sessionID, workID)
	assertProviderCommandRoutesSince(t, fixture.runner, fixture.routeStart, map[string]struct{}{fixture.requestID: {}})
	fixture.caseFixture.closeRoute(t, fixture.requestID)
}

func replayWorkerSessionFrames(
	t *testing.T,
	ctx context.Context,
	fixture workerSessionReplayFixture,
) []factoryapi.WorkerSessionEvent {
	t.Helper()
	inputs := executeCLI(t, ctx, fixture.process, fixture.env, fixture.factoryDir,
		"--server", fixture.baseURL, "worker-sessions", "stream",
		"--session", fixture.sessionID, "--provider", "codex", "--kind", "session_id", "--id", fixture.providerSessionID,
		"--replay-only", "--output", "json",
	)
	var frames []factoryapi.WorkerSessionEvent
	for index, line := range nonEmptyLines(inputs.Stdout()) {
		var frame factoryapi.WorkerSessionEvent
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatalf("decode WSR replay frame %d: %v\nline:%s", index+1, err, line)
		}
		if frame.Event.Position == 0 || frame.WorkerSessionId == "" {
			continue
		}
		frames = append(frames, frame)
	}
	if len(frames) == 0 {
		t.Fatalf("replay stream contained no Worker Session records:\n%s", inputs.Stdout())
	}
	return frames
}

func assertWSRWorkerSessionHistory(
	t *testing.T,
	frames []factoryapi.WorkerSessionEvent,
	factorySessionID string,
	workID string,
	wantTerminal string,
) {
	t.Helper()
	if frames[0].Event.SourceType == "factory_event" {
		assertCanonicalWorkerSessionHistory(t, frames, workID, wantTerminal)
		return
	}
	if frames[0].Event.Position != 1 {
		t.Fatalf("first Worker Session position = %d, want 1", frames[0].Event.Position)
	}
	if frames[0].Event.SourceType != "worker_session_lifecycle" {
		t.Fatalf("first Worker Session source type = %q, want worker_session_lifecycle", frames[0].Event.SourceType)
	}
	if got := stringValue(frames[0].Event.Payload, "kind"); got != "SESSION" || stringValue(frames[0].Event.Payload, "phase") != "STARTED" {
		t.Fatalf("first Worker Session payload = %#v, want SESSION/STARTED", frames[0].Event.Payload)
	}
	providerOutputSeen := false
	providerBindingSeen := false
	providerBindingIndex := -1
	firstProviderOutput := -1
	terminalSeen := false
	for index, frame := range frames {
		if frame.WorkerSessionId != frames[0].WorkerSessionId || !containsString(frame.WorkIds, workID) {
			t.Fatalf("frame[%d] correlation = %#v, want worker %s and Work %s", index, frame, frames[0].WorkerSessionId, workID)
		}
		if index > 0 && frame.Event.Position <= frames[index-1].Event.Position {
			t.Fatalf("Worker Session positions are not increasing: frame[%d]=%d previous=%d", index, frame.Event.Position, frames[index-1].Event.Position)
		}
		if frame.Event.SourceType == "worker_session_lifecycle" &&
			stringValue(frame.Event.Payload, "phase") == "UPDATED" &&
			providerValue(frame.Event.Payload) == "codex" {
			providerBindingSeen = true
			if providerBindingIndex == -1 {
				providerBindingIndex = index
			}
		}
		if frame.Event.SourceType != "worker_session_lifecycle" {
			providerOutputSeen = true
			if firstProviderOutput == -1 {
				firstProviderOutput = index
			}
		}
		if frame.Event.SourceType == "worker_session_lifecycle" &&
			(stringValue(frame.Event.Payload, "phase") == "COMPLETED" ||
				stringValue(frame.Event.Payload, "phase") == "FAILED" ||
				stringValue(frame.Event.Payload, "phase") == "CANCELED") {
			if terminalSeen {
				t.Fatalf("Worker Session history has multiple terminal lifecycle records: %#v", frames)
			}
			terminalSeen = true
			if stringValue(frame.Event.Payload, "status") != wantTerminal {
				t.Fatalf("terminal Worker Session status = %q, want %q; payload=%#v", stringValue(frame.Event.Payload, "status"), wantTerminal, frame.Event.Payload)
			}
			if index != len(frames)-1 {
				t.Fatalf("terminal lifecycle record at frame %d, want final frame %d", index, len(frames)-1)
			}
		}
	}
	if !providerOutputSeen {
		t.Fatalf("Worker Session history has no provider-authored records: %#v", frames)
	}
	if !providerBindingSeen || firstProviderOutput == -1 || providerBindingIndex >= firstProviderOutput {
		t.Fatalf("Worker Session history did not bind codex before provider output: %#v", frames)
	}
	if !terminalSeen {
		t.Fatalf("Worker Session history has no terminal lifecycle record: %#v", frames)
	}
}

func assertWSRLiveReplayCorrelation(
	t *testing.T,
	live factoryapi.WorkerSessionObservation,
	frames []factoryapi.WorkerSessionEvent,
	factorySessionID string,
	workID string,
) {
	t.Helper()
	assertWSRWorkerSessionHistory(t, frames, factorySessionID, workID, "COMPLETED")
	if frames[0].Event.SourceType == "factory_event" {
		if live.WorkerSessionId != frames[0].WorkerSessionId || live.StartedAt == nil {
			t.Fatalf("live Worker Session = %#v, replay opening = %#v", live, frames[0])
		}
		// Canonical Factory-event payloads preserve dispatch correlation and
		// ordering; lifecycle startedAt is intentionally a projection field and
		// is not duplicated into every source event payload.
		return
	}
	if live.WorkerSessionId != frames[0].WorkerSessionId || live.State != factoryapi.WorkerSessionObservationStateCompleted {
		t.Fatalf("live Worker Session = %#v, replay opening = %#v", live, frames[0])
	}
	if live.StartedAt == nil {
		t.Fatal("live Worker Session omitted startedAt")
	}
	startedAt := stringValue(frames[0].Event.Payload, "startedAt")
	if startedAt == "" {
		t.Fatalf("replay opening omitted startedAt: %#v", frames[0].Event.Payload)
	}
	parsed, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		t.Fatalf("parse replay opening startedAt %q: %v", startedAt, err)
	}
	if !parsed.Equal(*live.StartedAt) {
		t.Fatalf("live startedAt = %s, replay startedAt = %s; live=%#v opening=%#v", live.StartedAt.Format(time.RFC3339Nano), parsed.Format(time.RFC3339Nano), live, frames[0].Event.Payload)
	}
	if live.AttemptId != stringValue(frames[0].Event.Payload, "attemptId") {
		t.Fatalf("live attemptId = %q, replay attemptId = %q", live.AttemptId, stringValue(frames[0].Event.Payload, "attemptId"))
	}
}

func listWorkerSessionsForFactorySession(t *testing.T, baseURL, factorySessionID, workID string) factoryapi.ListWorkerSessionsResponse {
	t.Helper()
	if strings.TrimSpace(factorySessionID) == "" || strings.TrimSpace(workID) == "" {
		t.Fatal("Factory Session and Work identities are required for a scoped Worker Session list")
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(factorySessionID) + "/worker-sessions?workId=" + url.QueryEscape(workID)
	return support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, endpoint)
}

func assertScopedWorkerSessionList(t *testing.T, listed factoryapi.ListWorkerSessionsResponse, factorySessionID, providerSessionID, workID string) {
	t.Helper()
	if len(listed.Sessions) != 1 {
		t.Fatalf("scoped Worker Session list = %#v, want exactly one observation", listed)
	}
	session := listed.Sessions[0]
	if session.FactorySessionId == nil || *session.FactorySessionId != factorySessionID {
		t.Fatalf("scoped Worker Session Factory Session = %#v, want %s", session.FactorySessionId, factorySessionID)
	}
	if session.WorkId == nil || *session.WorkId != workID {
		t.Fatalf("scoped Worker Session Work = %#v, want %s", session.WorkId, workID)
	}
	if session.ProviderSession == nil || session.ProviderSession.Id != providerSessionID {
		t.Fatalf("scoped Worker Session provider identity = %#v, want %s", session.ProviderSession, providerSessionID)
	}
}

func providerValue(payload map[string]interface{}) string {
	provenance, _ := payload["provenance"].(map[string]interface{})
	provider, _ := provenance["provider"].(string)
	return provider
}

func newWorkerSessionReplayFixture(t *testing.T, ctx context.Context, requestID, providerSessionID string) workerSessionReplayFixture {
	t.Helper()
	caseFixture := newWorkerSessionsCLICase(t)
	shared := caseFixture.fixture
	caseFixture.registerRoutes(t, requestID)
	routeStart := shared.runner.CallCount()
	sessionID := caseFixture.openSession(t)
	return workerSessionReplayFixture{
		caseFixture: caseFixture,
		process:     shared.process,
		factoryDir:  caseFixture.factoryDir,
		env:         functionalEnvironment(shared.homeDir), baseURL: shared.baseURL,
		sessionID: sessionID, requestID: requestID, providerSessionID: providerSessionID,
		runner: shared.runner, routeStart: routeStart,
	}
}

func (fixture workerSessionReplayFixture) stop(t *testing.T) {
	t.Helper()
	fixture.caseFixture.cleanup(t)
}

func runBuiltWorkerSessionReplay(t *testing.T, ctx context.Context, fixture workerSessionReplayFixture) ([]byte, string) {
	t.Helper()
	inputs := support.FakeInputs(ctx, []string{
		"you", "--verbose", "--server", fixture.baseURL, "worker-sessions", "stream",
		"--session", fixture.sessionID, "--provider", "codex", "--kind", "session_id", "--id", fixture.providerSessionID,
		"--replay-only", "--output", "json",
	})
	inputs.Input.WorkingDirectory = fixture.factoryDir
	inputs.Input.Env = fixture.env
	if err := fixture.process.Execute(inputs.Input); err != nil {
		t.Fatalf("root process replay-only stream: %v\nstderr:\n%s", err, inputs.Stderr())
	}
	return []byte(inputs.Stdout()), inputs.Stderr()
}

func assertWorkerSessionReplayCapture(t *testing.T, contents []byte, diagnostics string) {
	t.Helper()
	lines := nonEmptyLines(string(contents))
	if len(lines) == 0 {
		t.Fatalf("replay capture is empty, want event records and a complete summary")
	}
	var previousPosition uint64
	eventsEmitted := 0
	var summary *workerSessionReplaySummaryJSON
	for index, line := range lines {
		var frame streamFrameJSON
		if err := json.Unmarshal([]byte(line), &frame); err == nil && frame.Event != nil {
			if frame.Delivery == "" {
				t.Fatalf("replay line %d = %#v, want an event delivery", index+1, frame)
			}
			if frame.Event.Position <= previousPosition {
				t.Fatalf("replay event positions are not canonical: previous=%d current=%d", previousPosition, frame.Event.Position)
			}
			previousPosition = frame.Event.Position
			eventsEmitted++
			if frame.ReplaySummary != nil {
				summary = frame.ReplaySummary
			}
			continue
		}
		var standalone workerSessionReplaySummaryJSON
		if err := json.Unmarshal([]byte(line), &standalone); err != nil {
			t.Fatalf("decode replay line %d: %v\nline:%s", index+1, err, line)
		}
		summary = &standalone
	}
	if eventsEmitted == 0 || summary == nil {
		t.Fatalf("replay capture omitted event records or summary: events=%d summary=%#v\n%s", eventsEmitted, summary, contents)
	}
	if summary.Kind != "replay-summary" || !summary.Complete ||
		(summary.Reason != "session-completed" && summary.Reason != "recording-complete") {
		t.Fatalf("replay summary = %#v, want complete terminal summary", *summary)
	}
	if summary.EventsEmitted != int64(eventsEmitted) {
		t.Fatalf("replay summary eventsEmitted = %d, want %d event records", summary.EventsEmitted, eventsEmitted)
	}
	if strings.Contains(string(contents), "worker sessions stream request") || strings.Contains(string(contents), "worker sessions stream response") {
		t.Fatalf("verbose diagnostics contaminated redirected stdout:\n%s", contents)
	}
	if strings.TrimSpace(diagnostics) == "" {
		t.Fatal("--verbose produced no stderr diagnostics to verify stream diagnostics stay off stdout")
	}
}
