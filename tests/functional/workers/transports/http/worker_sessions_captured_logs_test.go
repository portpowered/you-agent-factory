package http_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"reflect"
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
	close(gate)
	runner.waitCompleted(t)
	ended := waitCapturedTerminal(t, server.URL(), "captured-worker")
	assertCapturedHeadContinuation(t, server, resume, ended)
	assertCapturedLogsCLIHTTPParity(t, server, "captured-worker")
	assertCapturedSummaryUsage(t, server, "captured-worker")
	if ended.CommittedPosition <= active.CommittedPosition || !ended.Events[0].Event.CapturedAt.Equal(*active.Events[0].Event.CapturedAt) {
		t.Fatal("terminal logs lost the active captured prefix")
	}
	assertCapturedPages(t, server, ended)
	assertCapturedReplayTimes(t, server.URL(), ended)
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
	functionalevidence.Covers(t, "cli/you.worker-sessions.read", "rest/readWorkerSessionLogs")
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
		if frame.Event.Position != want.Position || frame.Event.CapturedAt == nil || !frame.Event.CapturedAt.Equal(*want.CapturedAt) {
			t.Fatalf("SSE and logs differ at record %d: %+v %+v", index, frame.Event, want)
		}
	}
}
