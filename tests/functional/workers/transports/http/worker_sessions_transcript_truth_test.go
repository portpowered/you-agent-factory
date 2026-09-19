package http_test

import (
	"bufio"
	"context"
	"encoding/json"
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

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	workerTranscriptRedactionSentinel = "api_key-secret-token"
	workerTranscriptPrivatePath       = "worker-session-rollout-secret.jsonl"
)

type workerSessionTranscriptFixture struct {
	serverURL  string
	factoryDir string
	env        []string
	runner     *remoteInvokeContinueRunner
	release    func()
}

type workerSessionTranscriptScenario struct {
	sessionID string
	workID    string
	dispatch  routeCharacterizationDispatch
	streamed  workerSessionTerminalEventsResult
}

// TestWorkerSessionWorkScopedTranscriptAndStreamUseStableIdentity proves the
// public Work association survives retained/live delivery and exact provider
// transcript projection through HTTP and the Process.Execute CLI boundary.
func TestWorkerSessionWorkScopedTranscriptAndStreamUseStableIdentity(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()

	fixture := startWorkerSessionTranscriptFixture(t)
	scenario := runWorkerSessionTranscriptScenario(t, ctx, fixture)
	httpTranscript := readWorkScopedTerminalTranscript(t, fixture.serverURL, scenario)
	assertWorkerSessionTranscriptCLIParity(t, ctx, fixture, scenario, httpTranscript)
}

func startWorkerSessionTranscriptFixture(t *testing.T) workerSessionTranscriptFixture {
	t.Helper()
	homeDir := t.TempDir()
	writeWorkerTranscriptRolloutWithRedactionSentinel(t, homeDir, remoteWorkerSessionProviderID)
	providerOutput := readRemoteProviderFixture(t, "codex", "success", "stdout.jsonl")
	gate := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(gate) }) }
	runner := newRemoteInvokeContinueRunner(gate, providerOutput)

	factoryDir := support.ScaffoldSingleStepFactory(t, "worker-session-work-transcript-truth")
	support.WriteAgentConfig(t, factoryDir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "fixture-model"))
	env := remoteFunctionalEnvironment(homeDir)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env:                       env,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
			ProviderSessionResolveHomeDirectory: func() (string, error) {
				return homeDir, nil
			},
		},
	})
	t.Cleanup(func() { server.Stop(t) })
	t.Cleanup(release)
	return workerSessionTranscriptFixture{
		serverURL: server.URL(), factoryDir: factoryDir, env: env,
		runner: runner, release: release,
	}
}

func runWorkerSessionTranscriptScenario(
	t *testing.T,
	ctx context.Context,
	fixture workerSessionTranscriptFixture,
) workerSessionTranscriptScenario {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, fixture.serverURL, fixture.factoryDir)
	sessionID := opened.Session.Id
	factoryEvents := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(fixture.serverURL, sessionID))

	workName := "worker-session-transcript-truth-work"
	submitted := support.SubmitSessionWorkAt(t, fixture.serverURL, sessionID, factoryapi.SubmitWorkRequest{
		Name:         &workName,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "inspect this provider transcript by Worker Session identity"},
	})
	workID := support.StringPointerValue(submitted.WorkId)
	if workID == "" {
		t.Fatalf("submitted Work response = %#v, want Work ID", submitted)
	}
	fixture.runner.waitStarted(t)
	dispatch := waitForRouteCharacterizationAssociation(t, factoryEvents, workID)
	if dispatch.workerSessionID == "" || dispatch.dispatchID == "" {
		t.Fatalf("Work dispatch association = %#v, want stable Worker Session and attempt IDs", dispatch)
	}

	eventResponse, eventCancel := openFactoryWorkerSessionEventStream(t, fixture.serverURL, sessionID, dispatch.workerSessionID)
	defer eventCancel()
	defer eventResponse.Body.Close()
	firstFrame := make(chan factoryapi.WorkerSessionEvent, 1)
	eventResults := make(chan workerSessionTerminalEventsResult, 1)
	go func() {
		frames, phase, err := readWorkScopedWorkerSessionEventStream(eventResponse, firstFrame)
		eventResults <- workerSessionTerminalEventsResult{frames: frames, phase: phase, err: err}
	}()

	var retained factoryapi.WorkerSessionEvent
	select {
	case retained = <-firstFrame:
	case result := <-eventResults:
		t.Fatalf("Worker Session stream ended before its first retained record: phase=%q err=%v frames=%#v", result.phase, result.err, result.frames)
	case <-ctx.Done():
		t.Fatalf("waiting for retained Worker Session event: %v", ctx.Err())
	}
	if retained.Delivery != "RECORD" {
		t.Fatalf("first Work-scoped Worker Session delivery = %q, want retained RECORD", retained.Delivery)
	}
	assertWorkScopedWorkerSessionEventIdentity(t, retained, sessionID, dispatch.workerSessionID, workID)

	fixture.release()
	fixture.runner.waitFirstCompleted(t)
	support.WaitForSessionTerminalStatus(t, fixture.serverURL, sessionID, routeCharacterizationTimeout)
	var streamed workerSessionTerminalEventsResult
	select {
	case streamed = <-eventResults:
	case <-ctx.Done():
		t.Fatalf("waiting for terminal Worker Session stream: %v", ctx.Err())
	}
	if streamed.err != nil || streamed.phase != "TERMINAL" {
		t.Fatalf("Work-scoped Worker Session stream = phase %q, err %v, frames=%#v; want terminal dispatch response", streamed.phase, streamed.err, streamed.frames)
	}
	assertWorkScopedWorkerSessionEventSequence(t, streamed.frames, sessionID, dispatch.workerSessionID, workID, retained)
	return workerSessionTranscriptScenario{sessionID: sessionID, workID: workID, dispatch: dispatch, streamed: streamed}
}

func readWorkScopedTerminalTranscript(
	t *testing.T,
	serverURL string,
	scenario workerSessionTranscriptScenario,
) factoryapi.WorkerSessionTranscriptResponse {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, workerSessionsListURL(serverURL, scenario.sessionID, scenario.workID))
	if len(listed.Sessions) != 1 {
		t.Fatalf("Work-scoped Worker Session list = %#v, want one attempt", listed.Sessions)
	}
	observation := listed.Sessions[0]
	if observation.WorkerSessionId != scenario.dispatch.workerSessionID || observation.AttemptId != scenario.dispatch.dispatchID ||
		observation.State != factoryapi.WorkerSessionObservationStateCompleted || observation.FactorySessionId == nil || *observation.FactorySessionId != scenario.sessionID ||
		!reflect.DeepEqual(observation.WorkIds, []string{scenario.workID}) || !observation.ProviderSessionAvailable ||
		observation.ProviderSession == nil || observation.ProviderSession.Id != remoteWorkerSessionProviderID ||
		string(observation.Transcript) != "AVAILABLE" || observation.Parse.EventCount == 0 {
		t.Fatalf("Work-scoped terminal observation = %#v, want exact Work/attempt/provider and readable transcript projection", observation)
	}

	transcriptURL := fmt.Sprintf("%s/factory-sessions/%s/worker-sessions/%s/transcript", strings.TrimSuffix(serverURL, "/"), url.PathEscape(scenario.sessionID), url.PathEscape(scenario.dispatch.workerSessionID))
	transcript := support.GetJSON[factoryapi.WorkerSessionTranscriptResponse](t, transcriptURL)
	assertWorkScopedTranscript(t, transcript, scenario.sessionID, scenario.workID, scenario.dispatch, remoteWorkerSessionProviderID)
	transcriptBytes, err := json.Marshal(transcript)
	if err != nil {
		t.Fatalf("marshal HTTP Worker Session transcript: %v", err)
	}
	assertWorkerTranscriptRedacted(t, string(transcriptBytes))
	return transcript
}

func assertWorkerSessionTranscriptCLIParity(
	t *testing.T,
	ctx context.Context,
	fixture workerSessionTranscriptFixture,
	scenario workerSessionTranscriptScenario,
	httpTranscript factoryapi.WorkerSessionTranscriptResponse,
) {
	t.Helper()
	clientRunner := testutil.NewProviderCommandRunner()
	client := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: clientRunner})
	support.CleanupProcess(t, client)
	cliRead := executeRemoteWorkerCLI(t, ctx, client, fixture.env, fixture.factoryDir, fixture.serverURL,
		"--json", "worker-sessions", "read", "--session", scenario.sessionID, "--worker-session-id", scenario.dispatch.workerSessionID)
	var cliTranscript factoryapi.WorkerSessionTranscriptResponse
	decodeRemoteWorkerJSON(t, cliRead.Stdout(), &cliTranscript)
	if !reflect.DeepEqual(cliTranscript, httpTranscript) {
		t.Fatalf("stable-ID CLI and HTTP transcripts differ:\nCLI: %#v\nHTTP: %#v", cliTranscript, httpTranscript)
	}
	assertWorkerTranscriptRedacted(t, cliRead.Stdout())

	cliStream := executeRemoteWorkerCLI(t, ctx, client, fixture.env, fixture.factoryDir, fixture.serverURL,
		"--json", "worker-sessions", "stream", "--session", scenario.sessionID, "--worker-session-id", scenario.dispatch.workerSessionID, "--replay-only")
	cliEvents, summary := decodeWorkScopedWorkerSessionCLIStream(t, cliStream.Stdout())
	if summary == nil || !summary.Complete || summary.EventsEmitted != int64(len(cliEvents)) {
		t.Fatalf("CLI Worker Session replay summary = %#v for %d records, want complete exact replay", summary, len(cliEvents))
	}
	if len(cliEvents) != len(scenario.streamed.frames) {
		t.Fatalf("HTTP live and CLI retained event counts differ: HTTP=%d CLI=%d", len(scenario.streamed.frames), len(cliEvents))
	}
	for index, cliEvent := range cliEvents {
		if cliEvent.WorkerSessionID != scenario.dispatch.workerSessionID || cliEvent.FactorySessionID != scenario.sessionID ||
			!reflect.DeepEqual(cliEvent.WorkIDs, []string{scenario.workID}) || cliEvent.Event == nil ||
			cliEvent.Event.Position != scenario.streamed.frames[index].Event.Position ||
			cliEvent.Event.SourceType != scenario.streamed.frames[index].Event.SourceType ||
			cliEvent.Event.SourceID != scenario.streamed.frames[index].Event.SourceId ||
			cliEvent.Event.SourceSequence != scenario.streamed.frames[index].Event.SourceSequence ||
			cliEvent.Event.SourceEventID != scenario.streamed.frames[index].Event.SourceEventId {
			t.Fatalf("CLI/HTTP event identity at index %d differs: CLI=%#v HTTP=%#v", index, cliEvent, scenario.streamed.frames[index])
		}
	}
	assertWorkerTranscriptRedacted(t, cliStream.Stdout())
	if clientRunner.CallCount() != 0 {
		t.Fatalf("remote Worker Session reads caused local provider fallback: %d calls", clientRunner.CallCount())
	}
}

func writeWorkerTranscriptRolloutWithRedactionSentinel(t *testing.T, homeDir, sessionID string) {
	t.Helper()
	directory := filepath.Join(homeDir, ".codex", "sessions", "2026", "07", "27")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create Codex rollout directory: %v", err)
	}
	contents := readRemoteProviderFixture(t, "codex", "success", "rollout.jsonl")
	redactedUnknown := []byte(`{"type":"C:\\private\\` + workerTranscriptPrivatePath + `","payload":{"type":"` + workerTranscriptRedactionSentinel + `"}}` + "\n")
	contents = append(contents, redactedUnknown...)
	path := filepath.Join(directory, "rollout-"+sessionID+".jsonl")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("write Codex rollout fixture: %v", err)
	}
}

func openFactoryWorkerSessionEventStream(t *testing.T, baseURL, sessionID, workerSessionID string) (*http.Response, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), functionalWorkerSignalTimeout)
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) +
		"/worker-sessions/" + url.PathEscape(workerSessionID) + "/events"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		cancel()
		t.Fatalf("construct Factory Session Worker Session event request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		cancel()
		t.Fatalf("GET Factory Session Worker Session events: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		cancel()
		t.Fatalf("GET Factory Session Worker Session events status = %d, body = %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return response, cancel
}

func readWorkScopedWorkerSessionEventStream(
	response *http.Response,
	firstFrame chan<- factoryapi.WorkerSessionEvent,
) ([]factoryapi.WorkerSessionEvent, string, error) {
	frames := make([]factoryapi.WorkerSessionEvent, 0)
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var frame factoryapi.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &frame); err != nil {
			return frames, "", fmt.Errorf("decode Work-scoped Worker Session event: %w", err)
		}
		frames = append(frames, frame)
		if len(frames) == 1 && firstFrame != nil {
			firstFrame <- frame
		}
		if frame.Delivery == "SOURCE_FAILURE" {
			return frames, "", fmt.Errorf("Worker Session stream source failure %s: %s", stringValue(frame.ErrorCode), stringValue(frame.ErrorMessage))
		}
		if frame.Delivery == "TERMINAL" || frame.Delivery == "TERMINAL_REPLAY" {
			return frames, "TERMINAL", nil
		}
	}
	if err := scanner.Err(); err != nil {
		return frames, "", err
	}
	return frames, "", fmt.Errorf("Work-scoped Worker Session stream ended before terminal lifecycle event")
}

func assertWorkScopedWorkerSessionEventIdentity(t *testing.T, frame factoryapi.WorkerSessionEvent, sessionID, workerSessionID, workID string) {
	t.Helper()
	if frame.WorkerSessionId != workerSessionID || frame.FactorySessionId == nil || *frame.FactorySessionId != sessionID ||
		!reflect.DeepEqual(frame.WorkIds, []string{workID}) || frame.Event.Position <= 0 {
		t.Fatalf("Work-scoped Worker Session event identity = %#v, want Factory Session %q, Worker Session %q, Work %q", frame, sessionID, workerSessionID, workID)
	}
}

func assertWorkScopedWorkerSessionEventSequence(
	t *testing.T,
	frames []factoryapi.WorkerSessionEvent,
	sessionID, workerSessionID, workID string,
	first factoryapi.WorkerSessionEvent,
) {
	t.Helper()
	if len(frames) < 2 || frames[0].Event.Position != first.Event.Position {
		t.Fatalf("Worker Session stream frames = %#v, want retained opening and later live records", frames)
	}
	lastPosition := int64(0)
	seen := make(map[string]struct{}, len(frames))
	for index, frame := range frames {
		assertWorkScopedWorkerSessionEventIdentity(t, frame, sessionID, workerSessionID, workID)
		if frame.Event.Position <= lastPosition {
			t.Fatalf("Worker Session event position at index %d = %d after %d, want strict order", index, frame.Event.Position, lastPosition)
		}
		lastPosition = frame.Event.Position
		identity := fmt.Sprintf("%s/%s/%d/%s", frame.Event.SourceType, frame.Event.SourceId, frame.Event.SourceSequence, frame.Event.SourceEventId)
		if _, exists := seen[identity]; exists {
			t.Fatalf("Worker Session event identity %q was delivered more than once", identity)
		}
		seen[identity] = struct{}{}
	}
}

func assertWorkScopedTranscript(
	t *testing.T,
	transcript factoryapi.WorkerSessionTranscriptResponse,
	sessionID, workID string,
	dispatch routeCharacterizationDispatch,
	providerSessionID string,
) {
	t.Helper()
	if transcript.WorkerSessionId != dispatch.workerSessionID || transcript.FactorySessionId == nil || *transcript.FactorySessionId != sessionID ||
		!reflect.DeepEqual(transcript.WorkIds, []string{workID}) || transcript.AttemptId != dispatch.dispatchID ||
		transcript.State != string(factoryapi.WorkerSessionObservationStateCompleted) || transcript.ProviderSession.Id != providerSessionID ||
		len(transcript.Entries) == 0 {
		t.Fatalf("Worker Session transcript = %#v, want non-empty exact Work/attempt/provider projection", transcript)
	}
	foundAnswer := false
	lastOrder := 0
	for _, entry := range transcript.Entries {
		if entry.Order <= lastOrder {
			t.Fatalf("transcript entries are not in normalized order: %#v", transcript.Entries)
		}
		lastOrder = entry.Order
		if entry.Text != nil && strings.Contains(*entry.Text, "Codex fixture answer COMPLETE") {
			foundAnswer = true
		}
	}
	if !foundAnswer {
		t.Fatalf("normalized transcript omitted provider answer: %#v", transcript.Entries)
	}
}

func assertWorkerTranscriptRedacted(t *testing.T, output string) {
	t.Helper()
	for _, forbidden := range []string{workerTranscriptRedactionSentinel, workerTranscriptPrivatePath} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("Worker Session transcript output leaked redacted provider material %q", forbidden)
		}
	}
}

type workScopedCLIEvent struct {
	Position       int64           `json:"position"`
	SourceType     string          `json:"sourceType"`
	SourceID       string          `json:"sourceId"`
	SourceSequence int64           `json:"sourceSequence"`
	SourceEventID  string          `json:"sourceEventId"`
	SchemaID       string          `json:"schemaId"`
	Payload        json.RawMessage `json:"payload"`
}

type workScopedCLIStreamFrame struct {
	Delivery         string                      `json:"delivery"`
	WorkerSessionID  string                      `json:"workerSessionId"`
	FactorySessionID string                      `json:"factorySessionId"`
	WorkIDs          []string                    `json:"workIds"`
	Event            *workScopedCLIEvent         `json:"event"`
	ReplaySummary    *workScopedCLIReplaySummary `json:"replaySummary,omitempty"`
}

type workScopedCLIReplaySummary struct {
	Kind          string `json:"kind"`
	Complete      bool   `json:"complete"`
	Reason        string `json:"reason"`
	EventsEmitted int64  `json:"eventsEmitted"`
}

func decodeWorkScopedWorkerSessionCLIStream(t *testing.T, output string) ([]workScopedCLIStreamFrame, *workScopedCLIReplaySummary) {
	t.Helper()
	frames := make([]workScopedCLIStreamFrame, 0)
	var summary *workScopedCLIReplaySummary
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var frame workScopedCLIStreamFrame
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatalf("decode Work-scoped CLI Worker Session frame: %v\nline: %s", err, line)
		}
		if frame.Event != nil {
			frames = append(frames, frame)
			if frame.ReplaySummary != nil {
				summary = frame.ReplaySummary
			}
			continue
		}
		var replay workScopedCLIReplaySummary
		if err := json.Unmarshal([]byte(line), &replay); err != nil {
			t.Fatalf("decode Work-scoped CLI Worker Session replay summary: %v\nline: %s", err, line)
		}
		if replay.Kind != "replay-summary" {
			t.Fatalf("unexpected Worker Session CLI stream record: %s", line)
		}
		summary = &replay
	}
	return frames, summary
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
