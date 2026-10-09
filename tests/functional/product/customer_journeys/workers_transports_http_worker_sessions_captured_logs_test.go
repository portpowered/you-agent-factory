package customer_journeys_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// The Direct origin must remain Direct; using a Factory dispatch here would
// miss the public ID-only admission/read boundary. All files and command edges
// are scenario-owned, and the provider fixture writes no native transcript.
func TestWorkerSessionCapturedLogsCLIHTTPParity(t *testing.T) {
	t.Parallel()
	gate := make(chan struct{})
	runner := newFunctionalWorkerGate(gate)
	dir := support.ScaffoldSingleStepFactory(t, "captured-direct-recovery")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))
	home := t.TempDir()
	config := support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Env:   []string{"HOME=" + home, "USERPROFILE=" + home},
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: capturedRecordingDirectory(dir)},
	}
	server := support.StartFunctionalAPIServer(t, config)
	start := postDirectWorkerSession(t, t.Context(), server.URL(), "captured-request", "captured-worker", "captured-dispatch")
	_ = start.Body.Close()
	if start.StatusCode != http.StatusAccepted {
		t.Fatalf("direct start status=%d", start.StatusCode)
	}
	runner.waitStarted(t)
	active := assertCapturedLogsCLIHTTPParity(t, server, "captured-worker")
	resume := assertCapturedActiveHead(t, server, active)
	shown := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/captured-worker")
	if shown.ProviderSession != nil || shown.WorkerSessionId != "captured-worker" {
		t.Fatalf("active unassociated Worker identity: %+v", shown)
	}
	followed := readCapturedLiveFollow(t, server, "captured-worker", gate)
	runner.waitCompleted(t)
	ended := waitCapturedTerminal(t, server.URL(), "captured-worker")
	assertCapturedFollowPages(t, server, ended, followed, resume)
	assertCapturedHeadContinuation(t, server, resume, ended)
	assertCapturedLogsCLIHTTPParity(t, server, "captured-worker")
	assertCapturedSummaryUsage(t, server, "captured-worker")
	if ended.CommittedPosition <= active.CommittedPosition || !ended.Events[0].Event.CapturedAt.Equal(*active.Events[0].Event.CapturedAt) {
		t.Fatal("terminal logs lost the active captured prefix")
	}
	assertCapturedPages(t, server, ended)
	assertCapturedReplayTimes(t, server.URL(), ended)
	assertCapturedFollowReconnect(t, server.URL(), ended)
	// Stop the sole writer before reopening this store. A fresh root has no
	// live Worker registry or provider-native files to recover identity from.
	server.Close(t)
	restarted := support.StartFunctionalAPIServer(t, config)
	recovered := assertCapturedLogsCLIHTTPParity(t, restarted, "captured-worker")
	if !reflect.DeepEqual(recovered.Events, ended.Events) || recovered.CommittedPosition != ended.CommittedPosition || recovered.RecordingGenerationId != ended.RecordingGenerationId {
		t.Fatal("restart changed the committed history or generation")
	}
	assertCapturedSummaryUsage(t, restarted, "captured-worker")
	archived := support.GetJSON[factoryapi.WorkerSessionObservation](t, restarted.URL()+"/worker-sessions/captured-worker")
	if archived.State != factoryapi.WorkerSessionObservationStateCompleted || archived.ProviderSession != nil || archived.StartedAt == nil {
		t.Fatalf("archived captured identity lost facts: %+v", archived)
	}
	assertArchivedCapturedTiming(t, restarted, archived, recovered)
	assertCapturedReplayTimes(t, restarted.URL(), recovered)
	assertArchivedCapturedCLIReplay(t, restarted, recovered, true)
	assertArchivedCapturedMCPReplay(t, restarted, recovered, true)
	restarted.Close(t)
	assertIncompleteCapturedFollow(t, config, ended)
	assertOversizedCapturedPayload(t, config, ended)
	assertDamagedCapturedRecovery(t, config, ended)
	assertUnreadableCapturedRecovery(t, config, ended)
	functionalevidence.Covers(t, "cli/you.worker-sessions.read", "rest/readWorkerSessionLogs")
}

func assertArchivedCapturedTiming(t *testing.T, server *support.FunctionalAPIServer, archived factoryapi.WorkerSessionObservation, logs factoryapi.WorkerSessionLogPage) {
	t.Helper()
	stamp := logs.Events[len(logs.Events)-1].Event.CapturedAt
	if stamp == nil || archived.StartedAt == nil || archived.EndedAt == nil || !archived.EndedAt.Equal(*stamp) || archived.DurationMillis == nil || *archived.DurationMillis != stamp.Sub(*archived.StartedAt).Milliseconds() || archived.DurationBasis != "RECORDED_TIMESTAMPS" {
		t.Fatalf("archived summary lost host capture timing: %+v terminal=%v", archived, stamp)
	}
	listed := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, server.URL()+"/worker-sessions?history=archived")
	if len(listed.Sessions) != 1 || !reflect.DeepEqual(listed.Sessions[0], archived) {
		t.Fatalf("archived list/show facts differ: %+v %+v", listed, archived)
	}
	inputs := support.FakeInputs(t.Context(), []string{"you", "worker-sessions", "show", "--worker-session-id", archived.WorkerSessionId, "--server", server.URL(), "--output", "json"})
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatal(err)
	}
	var cli factoryapi.WorkerSessionObservation
	if err := json.Unmarshal([]byte(inputs.Stdout()), &cli); err != nil || !reflect.DeepEqual(cli, archived) {
		t.Fatalf("archived CLI/HTTP facts differ: %+v %v", cli, err)
	}
}

type capturedRecordingDirectory string

func (d capturedRecordingDirectory) Getwd() (string, error) { return string(d), nil }

func readCapturedContinuation(t *testing.T, server *support.FunctionalAPIServer, id, token string) factoryapi.WorkerSessionLogPage {
	t.Helper()
	page := support.GetJSON[factoryapi.WorkerSessionLogPage](t, server.URL()+"/worker-sessions/"+url.PathEscape(id)+"/logs?nextToken="+url.QueryEscape(token))
	inputs := support.FakeInputs(t.Context(), []string{"you", "worker-sessions", "read", "--worker-session-id", id, "--view", "logs", "--next-token", token, "--server", server.URL(), "--output", "json"})
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI captured continuation: %v %s", err, inputs.Stderr())
	}
	var cli factoryapi.WorkerSessionLogPage
	if err := json.Unmarshal([]byte(inputs.Stdout()), &cli); err != nil || !reflect.DeepEqual(page, cli) {
		t.Fatalf("CLI/HTTP continuation differs: CLI=%+v HTTP=%+v error=%v", cli, page, err)
	}
	return page
}

func assertCapturedActiveHead(t *testing.T, server *support.FunctionalAPIServer, active factoryapi.WorkerSessionLogPage) string {
	t.Helper()
	if active.NextToken == nil {
		t.Fatal("active captured head omitted resume token")
	}
	empty := readCapturedContinuation(t, server, active.WorkerSessionId, *active.NextToken)
	if empty.Events == nil || len(empty.Events) != 0 || empty.CommittedPosition != active.CommittedPosition || empty.NextToken == nil || *empty.NextToken != *active.NextToken || empty.Health != active.Health {
		t.Fatalf("active at-head read lost empty array, watermark or resume token: %+v", empty)
	}
	return *empty.NextToken
}

func assertCapturedHeadContinuation(t *testing.T, server *support.FunctionalAPIServer, token string, ended factoryapi.WorkerSessionLogPage) {
	t.Helper()
	continued := readCapturedContinuation(t, server, ended.WorkerSessionId, token)
	if len(continued.Events) == 0 || continued.CommittedPosition != ended.CommittedPosition || continued.NextToken != nil {
		t.Fatalf("resume did not reach the captured terminal: %+v", continued)
	}
	first := continued.Events[0].Event.Position
	if first <= 1 || !reflect.DeepEqual(continued.Events, ended.Events[first-1:]) {
		t.Fatal("resume duplicated or changed records from the active prefix")
	}
	repeated := readCapturedContinuation(t, server, ended.WorkerSessionId, token)
	if !reflect.DeepEqual(repeated, continued) {
		t.Fatal("repeated continuation changed a completed capture")
	}
}

func assertCapturedSummaryUsage(t *testing.T, server *support.FunctionalAPIServer, id string) {
	t.Helper()
	shown := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/"+url.PathEscape(id))
	if shown.TokenUsage == nil || shown.TokenUsage.InputTokens == nil || *shown.TokenUsage.InputTokens != 1 || shown.TokenUsage.OutputTokens == nil || *shown.TokenUsage.OutputTokens != 1 {
		t.Fatalf("canonical summary lost committed fixture usage: %+v", shown.TokenUsage)
	}
	inputs := support.FakeInputs(t.Context(), []string{"you", "worker-sessions", "show", "--worker-session-id", id, "--server", server.URL(), "--output", "json"})
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI captured summary: %v %s", err, inputs.Stderr())
	}
	var cli factoryapi.WorkerSessionObservation
	if err := json.Unmarshal([]byte(inputs.Stdout()), &cli); err != nil || !reflect.DeepEqual(cli.TokenUsage, shown.TokenUsage) {
		t.Fatalf("CLI/HTTP committed usage differ: %+v %+v %v", cli.TokenUsage, shown.TokenUsage, err)
	}
}

// Recordings commits asynchronously after Events publication. Its public logs
// page has no completion notification; observe the committed terminal rather
// than sleeping or treating the provider-return signal as a storage barrier.
func waitCapturedTerminal(t *testing.T, baseURL, id string) factoryapi.WorkerSessionLogPage {
	t.Helper()
	page, err := support.WaitForObservation(functionalWorkerSignalTimeout, func() (factoryapi.WorkerSessionLogPage, error) {
		return support.GetJSON[factoryapi.WorkerSessionLogPage](t, baseURL+"/worker-sessions/"+url.PathEscape(id)+"/logs"), nil
	}, func(page factoryapi.WorkerSessionLogPage) bool {
		return len(page.Events) > 0 && page.Events[len(page.Events)-1].Delivery == factoryapi.WorkerSessionEventDelivery("TERMINAL_REPLAY")
	})
	if err != nil {
		t.Fatalf("committed terminal logs: %v", err)
	}
	return page
}

func assertCapturedPages(t *testing.T, server *support.FunctionalAPIServer, full factoryapi.WorkerSessionLogPage) {
	t.Helper()
	endpoint := server.URL() + "/worker-sessions/" + url.PathEscape(full.WorkerSessionId) + "/logs"
	page := support.GetJSON[factoryapi.WorkerSessionLogPage](t, endpoint+"?limit=1")
	if page.NextToken == nil {
		t.Fatal("bounded page omitted continuation")
	}
	firstToken := *page.NextToken
	var records []factoryapi.WorkerSessionEvent
	for {
		if len(page.Events) != 1 || page.CommittedPosition != full.CommittedPosition {
			t.Fatalf("page escaped limit or pinned head: %+v", page)
		}
		records = append(records, page.Events...)
		if page.NextToken == nil {
			break
		}
		page = support.GetJSON[factoryapi.WorkerSessionLogPage](t, endpoint+"?limit=1&nextToken="+url.QueryEscape(*page.NextToken))
	}
	if !reflect.DeepEqual(records, full.Events) {
		t.Fatal("pagination changed the committed records")
	}
	inputs := support.FakeInputs(t.Context(), []string{"you", "worker-sessions", "read", "--worker-session-id", full.WorkerSessionId, "--view", "logs", "--limit", "1", "--next-token", firstToken, "--server", server.URL(), "--output", "json"})
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI continuation: %v %s", err, inputs.Stderr())
	}
	var cli factoryapi.WorkerSessionLogPage
	if err := json.Unmarshal([]byte(inputs.Stdout()), &cli); err != nil || len(cli.Events) != 1 || cli.Events[0].Event.Position != 2 || cli.CommittedPosition != full.CommittedPosition {
		t.Fatalf("CLI continuation lost page contract: %+v %v", cli, err)
	}
	for _, query := range []string{"limit=0", "limit=-1", "limit=1001", "nextToken=invalid"} {
		response, err := http.Get(endpoint + "?" + query)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s status=%d, want 400", query, response.StatusCode)
		}
	}
}

func assertCapturedReplayTimes(t *testing.T, baseURL string, page factoryapi.WorkerSessionLogPage) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), functionalWorkerSignalTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/worker-sessions/"+url.PathEscape(page.WorkerSessionId)+"/events?replayOnly=true", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("replay status=%d: %s", response.StatusCode, body)
	}
	frames, err := readWorkerSessionEventStream(response)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != len(page.Events) {
		t.Fatalf("SSE records=%d, logs=%d", len(frames), len(page.Events))
	}
	for index, frame := range frames {
		want := page.Events[index].Event
		if !reflect.DeepEqual(frame.Event, want) {
			t.Fatalf("SSE and logs differ at record %d: %+v %+v", index, frame.Event, want)
		}
	}
}

// This API-owned cell observes exclusive reconnect at the actual top-level
// route, using the same root host and controlled execution as captured reads.
func assertCapturedFollowReconnect(t *testing.T, baseURL string, page factoryapi.WorkerSessionLogPage) {
	t.Helper()
	if len(page.Events) < 2 {
		t.Fatal("reconnect fixture needs a committed prefix and tail")
	}
	ack := page.Events[0].Event.Position
	endpoint := fmt.Sprintf("%s/worker-sessions/%s/events?replayOnly=true&after_position=%d", baseURL, url.PathEscape(page.WorkerSessionId), ack)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("reconnect status=%d: %s", response.StatusCode, body)
	}
	frames, err := readWorkerSessionEventStream(response)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != len(page.Events)-1 {
		t.Fatalf("reconnect records=%d, want %d after position %d", len(frames), len(page.Events)-1, ack)
	}
	for index, frame := range frames {
		if !reflect.DeepEqual(frame.Event, page.Events[index+1].Event) {
			t.Fatalf("reconnect duplicated or changed committed record %d: %+v", index, frame)
		}
	}
}

// Factory streams retain Factory Event identity, which is independent of the
// Worker journal's sequence. Those events have no Worker commit timestamp;
// neither eventTime nor a coincident journal position may supply capturedAt.
func assertCapturedFactoryReplayCompatibility(t *testing.T, server *support.FunctionalAPIServer, sessionID, workID, workerID string, terminal bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), functionalWorkerSignalTimeout)
	defer cancel()
	endpoint := server.URL() + "/factory-sessions/" + url.PathEscape(sessionID) + "/worker-sessions/" + url.PathEscape(workerID) + "/events?replayOnly=true"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Factory replay status=%d", response.StatusCode)
	}
	frames := readCapturedFiniteFactoryReplay(t, response, terminal)
	inputs := support.FakeInputs(ctx, []string{"you", "--json", "worker-sessions", "stream", "--session", sessionID, "--worker-session-id", workerID, "--replay-only", "--server", server.URL()})
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("Factory CLI replay: %v %s", err, inputs.Stderr())
	}
	cliFrames, summary := decodeWorkScopedWorkerSessionCLIStream(t, inputs.Stdout())
	if summary == nil || summary.Complete != terminal || summary.EventsEmitted != int64(len(cliFrames)) || len(cliFrames) != len(frames) {
		t.Fatalf("Factory replay completion/count: summary=%+v CLI=%d HTTP=%d", summary, len(cliFrames), len(frames))
	}
	for index, frame := range frames {
		assertWorkScopedWorkerSessionEventIdentity(t, frame, sessionID, workerID, workID)
		assertCapturedFactoryReplayIdentity(t, cliFrames[index].Event, frame.Event)
	}
}

func assertCapturedFactoryReplayIdentity(t *testing.T, cli *workScopedCLIEvent, event factoryapi.WorkerSessionEventRecord) {
	t.Helper()
	if cli == nil || event.SourceType != "factory_event" || event.CapturedAt != nil ||
		cli.Position != event.Position || cli.SourceType != event.SourceType || cli.SourceID != event.SourceId || cli.SourceEventID != event.SourceEventId ||
		cli.SourceSequence != event.SourceSequence || cli.SchemaID != event.SchemaId || cli.CapturedAt != nil {
		t.Fatalf("Factory replay identity or unknown commit time changed: CLI=%+v HTTP=%+v", cli, event)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(cli.Payload, &payload); err != nil || !reflect.DeepEqual(payload, event.Payload) {
		t.Fatalf("Factory replay changed payload: CLI=%s HTTP=%+v error=%v", cli.Payload, event.Payload, err)
	}
}

func readCapturedFiniteFactoryReplay(t *testing.T, response *http.Response, terminal bool) []factoryapi.WorkerSessionEvent {
	t.Helper()
	var frames []factoryapi.WorkerSessionEvent
	var summary *factoryapi.WorkerSessionReplaySummary
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var frame factoryapi.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &frame); err != nil {
			t.Fatal(err)
		}
		if frame.ReplaySummary != nil {
			summary = frame.ReplaySummary
		}
		switch frame.Delivery {
		case "RECORD", "TERMINAL_REPLAY":
			frames = append(frames, frame)
		case "REPLAY_SUMMARY":
		default:
			t.Fatalf("unexpected finite replay delivery: %+v", frame)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(frames) == 0 || summary == nil || summary.Complete != terminal || summary.EventsEmitted != int64(len(frames)) {
		t.Fatalf("Factory HTTP replay completion/count: frames=%d summary=%+v", len(frames), summary)
	}
	return frames
}

func assertArchivedCapturedCLIReplay(t *testing.T, server *support.FunctionalAPIServer, logs factoryapi.WorkerSessionLogPage, complete bool) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "--server", server.URL(), "--json", "worker-sessions", "stream", "--worker-session-id", logs.WorkerSessionId, "--replay-only"})
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("archived CLI replay: %v, %s", err, inputs.Stderr())
	}
	frames, summary := decodeArchivedCLIReplay(t, inputs.Stdout())
	if len(frames) > 0 && frames[len(frames)-1].Delivery == "REPLAY_SUMMARY" {
		summary = frames[len(frames)-1].ReplaySummary
		frames = frames[:len(frames)-1]
	}
	if len(frames) != len(logs.Events) {
		t.Fatalf("CLI replay records=%d logs=%d", len(frames), len(logs.Events))
	}
	for i, frame := range frames {
		// CLI v1 stream exposes source records; commit times/cursors are on
		// logs and HTTP/MCP frames. Preserve that existing CLI shape.
		want := logs.Events[i].Event
		want.CapturedAt, want.Cursor = nil, factoryapi.WorkerSessionEventCursor{}
		if frame.WorkerSessionId != logs.WorkerSessionId || !reflect.DeepEqual(frame.Event, want) {
			t.Fatalf("CLI replay record %d differs from logs: %+v %+v", i, frame, logs.Events[i])
		}
	}
	last := frames[len(frames)-1]
	if summary == nil {
		summary = last.ReplaySummary
	}
	if summary == nil || summary.Complete != complete || summary.EventsEmitted != int64(len(frames)) {
		t.Fatalf("archive did not finish completely: %+v", last)
	}
}

func decodeArchivedCLIReplay(t *testing.T, output string) ([]factoryapi.WorkerSessionEvent, *factoryapi.WorkerSessionReplaySummary) {
	t.Helper()
	scanner := bufio.NewScanner(strings.NewReader(output))
	var frames []factoryapi.WorkerSessionEvent
	var summary *factoryapi.WorkerSessionReplaySummary
	for scanner.Scan() {
		var frame factoryapi.WorkerSessionEvent
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			t.Fatal(err)
		}
		if frame.Delivery == "" {
			var standalone factoryapi.WorkerSessionReplaySummary
			if err := json.Unmarshal(scanner.Bytes(), &standalone); err != nil || standalone.Kind != "replay-summary" {
				t.Fatalf("unknown CLI replay output: %s", scanner.Bytes())
			}
			summary = &standalone
			continue
		}
		frames = append(frames, frame)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return frames, summary
}
