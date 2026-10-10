package cli_rest_journeys_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The parent owns the reusable root. Each cell owns its Session and routes;
// HTTP parity observes the same customer summaries as Process.Execute.
func testWorkerSessionsScopedPhysicalFacts(t *testing.T) {
	t.Parallel()
	f := workerSessionsCLIProcess(t)
	// Reconstruction has a distinct HTTP effect shape: four independently
	// owned hosts share one reusable root and the immutable provider edge.
	servers := map[int]*support.ProcessAPIServer{}
	for port := 60200; port < 60204; port++ {
		servers[port] = support.NewProcessAPIServer()
	}
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{ProviderCommandRunner: f.runner, APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
		return servers[request.Port].Start(ctx, request)
	}})
	if err != nil {
		t.Fatal(err)
	}
	for index, name := range []string{"success", "business"} {
		t.Run("F-"+name, func(t *testing.T) {
			t.Parallel()
			c := newWorkerSessionsCLICase(t)
			configurePhysicalOutcome(t, c, name)
			route := "worker-session-physical-" + name
			c.registerRoutes(t, route)
			ctx := t.Context()
			port := 60200 + index*2
			sessionID := uuid.NewString()

			first := startPhysicalFactsHost(t, process, c, f.homeDir, port, sessionID, "--record")
			c.fixture = &workerSessionsCLISharedFixture{process: process, homeDir: f.homeDir, runner: f.runner, baseURL: servers[port].WaitForURL(t)}
			local := c.fixture
			workID := submitWork(t, ctx, local.process, functionalEnvironment(f.homeDir), c.factoryDir, local.baseURL, sessionID, route)
			row := waitForWorkerSessionState(t, ctx, local.process, functionalEnvironment(f.homeDir), c.factoryDir, local.baseURL, sessionID, workID, "COMPLETED")
			waitForScopedUsageCommit(t, ctx, local.baseURL+"/factory-sessions/"+sessionID+"/worker-sessions?workId="+workID)
			shown := assertPhysicalWorkerParity(t, c, sessionID, row.WorkerSessionID, true)
			if shown.FactorySessionId == nil || *shown.FactorySessionId != sessionID || shown.WorkId == nil || *shown.WorkId != workID {
				t.Fatalf("physical attribution: %+v", shown)
			}
			// Work processing completes independently; observe its public outcome.
			support.WaitForSessionTerminalStatus(t, local.baseURL, sessionID, 20*time.Second)
			work := support.GetJSON[factoryapi.Work](t, local.baseURL+"/factory-sessions/"+sessionID+"/work/"+workID)
			want := factoryapi.WorkStateTypeTERMINAL
			if name == "business" {
				want = factoryapi.WorkStateTypeFAILED
			}
			if work.State == nil || work.State.Type != want {
				t.Fatalf("independent Work outcome for %s: %+v", name, work)
			}
			assertPhysicalWorkerParity(t, c, sessionID, row.WorkerSessionID, true)
			// A joined close and reopen of this owned board restores the same
			// captured attempt without restoring execution authority.
			if err := first(); err != nil {
				t.Fatal(err)
			}
			reopened := uuid.NewString()
			startPhysicalFactsHost(t, process, c, f.homeDir, port+1, reopened, "--resume")
			local.baseURL = servers[port+1].WaitForURL(t)
			after := assertPhysicalWorkerParity(t, c, reopened, row.WorkerSessionID, true)
			after.ConfirmationState = shown.ConfirmationState
			if !reflect.DeepEqual(shown, after) {
				t.Fatalf("reopened physical facts changed: %+v %+v", shown, after)
			}
			restored := support.GetJSON[factoryapi.Work](t, local.baseURL+"/factory-sessions/"+reopened+"/work/"+workID)
			if restored.State == nil || restored.State.Type != want {
				t.Fatalf("reopened Work outcome changed: %+v", restored)
			}
		})
	}
	t.Run("F-live-and-fence", testScopedPhysicalLiveAndFence)
	t.Run("F-error", testWorkerSessionsListWorkScopedSelectedReadFailure)
	t.Run("F-empty", testWorkerSessionsListWorkScopedEmpty)
}

func startPhysicalFactsHost(t *testing.T, process support.ApplicationProcess, c *workerSessionsCLICase, home string, port int, scope, mode string) func() error {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--dir", c.factoryDir, "--session", scope, mode, filepath.Join(c.factoryDir, "board.json"), "--continuously", "--with-server", "--listen", fmt.Sprintf("127.0.0.1:%d", port), "--quiet"})
	inputs.Env, inputs.WorkingDirectory = functionalEnvironment(home), c.factoryDir
	command := startWorkerSessionsCLIHostedCommand(process, inputs.Input)
	var once sync.Once
	var stopErr error
	stop := func() error { once.Do(func() { stopErr = command.stop() }); return stopErr }
	t.Cleanup(func() {
		if err := stop(); err != nil {
			t.Error(err)
		}
	})
	return stop
}

func configurePhysicalOutcome(t *testing.T, c *workerSessionsCLICase, name string) {
	t.Helper()
	if name != "business" {
		return
	}
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
			// Continuation admission is a current scoped capability, not a
			// physical execution fact. A restored board cannot revive this row.
			if shown.FactorySessionId != nil && scope != *shown.FactorySessionId && row.Revivable != nil && *row.Revivable {
				t.Fatal("reopen restored execution authority")
			}
			row.Revivable = shown.Revivable
			row.ConfirmationState = shown.ConfirmationState
			if !terminal {
				row.DurationMillis = shown.DurationMillis
			}
			if !reflect.DeepEqual(shown, row) {
				a, _ := json.Marshal(shown)
				b, _ := json.Marshal(row)
				t.Fatalf("scoped/top-level physical summary mismatch: %s %s", a, b)
			}
		}
	}
	if terminal {
		assertPhysicalCompletion(t, shown)
		assertPhysicalTranscriptParity(t, c, scope, shown)
	} else {
		assertPhysicalLive(t, shown)
	}
	assertPhysicalMCPParity(t, c, shown, terminal)
	return shown
}

func assertPhysicalCompletion(t *testing.T, shown factoryapi.WorkerSessionObservation) {
	t.Helper()
	if shown.State != "COMPLETED" || shown.EndedAt == nil || shown.DurationMillis == nil || shown.TerminalCause == nil || *shown.TerminalCause != "COMPLETED" || shown.Transcript != "AVAILABLE" || shown.RecordingHealth == nil || *shown.RecordingHealth != "COMPLETE" {
		t.Fatalf("physical completion facts: %+v", shown)
	}
}
func assertPhysicalLive(t *testing.T, shown factoryapi.WorkerSessionObservation) {
	t.Helper()
	if shown.State != "RUNNING" || shown.EndedAt != nil || shown.TerminalCause != nil || shown.RecordingHealth == nil || *shown.RecordingHealth != "INCOMPLETE" || shown.RecordingHealthReason != nil {
		t.Fatalf("live owned capture falsely interrupted: %+v", shown)
	}
}

func assertPhysicalTranscriptParity(t *testing.T, c *workerSessionsCLICase, scope string, shown factoryapi.WorkerSessionObservation) {
	t.Helper()
	f := c.fixture
	var original factoryapi.WorkerSessionTranscriptResponse
	for _, session := range []string{"", scope} {
		args := []string{"--server", f.baseURL, "worker-sessions", "read", "--worker-session-id", shown.WorkerSessionId, "--output", "json"}
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
			// Scoped transcript attribution retains the original execution owner.
			if transcript.FactorySessionId == nil || !reflect.DeepEqual(transcript.FactorySessionId, shown.FactorySessionId) {
				t.Fatalf("transcript physical owner lost: %+v", transcript)
			}
			transcript.FactorySessionId = original.FactorySessionId
			if !reflect.DeepEqual(original, transcript) {
				t.Fatalf("scoped transcript differs: %+v %+v", original, transcript)
			}
		}
	}
}

// MCP READ selects a canonical Worker ID on the same host; its current contract
// has no Factory selector. Compare its physical facts with the scoped observer.
func assertPhysicalMCPParity(t *testing.T, c *workerSessionsCLICase, expected factoryapi.WorkerSessionObservation, terminal bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	input, writer := io.Pipe()
	reader, output := io.Pipe()
	done := make(chan error, 1)
	f := c.fixture
	go func() {
		err := f.process.Execute(root.Input{Args: []string{"you", "--server", f.baseURL, "server", "mcp"}, Context: ctx, Env: functionalEnvironment(f.homeDir), WorkingDirectory: c.factoryDir, Stdin: input, Stdout: output, Stderr: io.Discard})
		_ = input.CloseWithError(err)
		_ = output.CloseWithError(err)
		done <- err
	}()
	t.Cleanup(func() {
		cancel()
		_ = writer.Close()
		_ = input.Close()
		_ = output.Close()
		_ = reader.Close()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
				t.Errorf("MCP join: %v", err)
			}
		case <-time.After(20 * time.Second):
			t.Error("MCP did not join")
		}
	})
	client := mcp.NewClient(&mcp.Implementation{Name: "physical-facts", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: reader, Writer: writer}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	assertPhysicalMCPViews(t, ctx, session, c, expected, terminal)
}

func assertPhysicalMCPViews(t *testing.T, ctx context.Context, session *mcp.ClientSession, c *workerSessionsCLICase, expected factoryapi.WorkerSessionObservation, terminal bool) {
	t.Helper()
	f := c.fixture
	views := []string{"summary", "logs"}
	if terminal {
		views = append(views, "transcript")
	}
	for _, view := range views {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: map[string]any{"action": "READ", "view": view, "workerSessionId": expected.WorkerSessionId}})
		if err != nil || result.IsError || len(result.Content) != 1 {
			t.Fatalf("MCP %s: %+v %v", view, result, err)
		}
		var envelope struct {
			Result struct {
				Session    factoryapi.WorkerSessionObservation        `json:"session"`
				Transcript factoryapi.WorkerSessionTranscriptResponse `json:"transcript"`
				Logs       factoryapi.WorkerSessionLogPage            `json:"logs"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &envelope); err != nil {
			t.Fatal(err)
		}
		actual := envelope.Result.Session
		actual.ConfirmationState = expected.ConfirmationState
		if !terminal {
			actual.DurationMillis = expected.DurationMillis
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("MCP %s physical facts differ: %+v %+v", view, actual, expected)
		}
		if view == "logs" && (len(envelope.Result.Logs.Events) == 0 || string(envelope.Result.Logs.Health) != string(*expected.RecordingHealth)) {
			t.Fatalf("MCP capture health/prefix differs: %+v", envelope.Result.Logs)
		}
		if view == "transcript" {
			transcript := support.GetJSON[factoryapi.WorkerSessionTranscriptResponse](t, f.baseURL+"/worker-sessions/"+expected.WorkerSessionId+"/transcript")
			if !reflect.DeepEqual(envelope.Result.Transcript, transcript) {
				t.Fatalf("MCP transcript differs from HTTP: %+v %+v", envelope.Result.Transcript, transcript)
			}
		}
	}
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
		assertPhysicalActiveTranscript(t, c, scopes[index], ids[index])
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

func assertPhysicalActiveTranscript(t *testing.T, c *workerSessionsCLICase, scope, id string) {
	t.Helper()
	f := c.fixture
	inputs, err := executeCLIExpectError(t, t.Context(), f.process, functionalEnvironment(f.homeDir), c.factoryDir, "--server", f.baseURL, "worker-sessions", "read", "--session", scope, "--worker-session-id", id, "--output", "json")
	if err == nil || strings.TrimSpace(inputs.Stdout()) != "" {
		t.Fatalf("active transcript emitted content: %v %s", err, inputs.Stdout())
	}
	response, err := http.Get(f.baseURL + "/factory-sessions/" + scope + "/worker-sessions/" + id + "/transcript")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var failure factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&failure); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode == http.StatusOK || failure.Code != factoryapi.ErrorResponseCodeWORKERSESSIONTRANSCRIPTACTIVE {
		t.Fatalf("active transcript refusal: %d %+v", response.StatusCode, failure)
	}
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
