package recording_sidecar_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// This finite declarative journey consumes the upstream build's artifact. The
// real CLI must drain and exit successfully even when Runtime rejects the
// completed Worker's business result. Reopening the same directory/profile
// must preserve physical capture facts while exposing the failed Work separately.
func TestCompletedFactoryCaptureFailedWorkIdleRestart(t *testing.T) {
	t.Parallel()
	binary := invokeArtifactBinary(t)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	home, temp := filepath.Join(project, "home"), filepath.Join(project, "temp")
	for _, path := range []string{home, temp} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	env := cleanupEnvironment(home, temp)
	factory := writeCleanupFactory(t, project, node, "", "")
	writeFailedWorkCaptureMock(t, project, factory, node)
	record := filepath.Join(project, "failed-work.json")
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	output := historyCLI(t, ctx, binary, project, env, "run", "--dir", factory, "--record", record, "--quiet", "--with-mock-workers="+filepath.Join(project, "mock.json"))
	t.Logf("finite run idle exit=0 stdout=%s", output)
	removeHistorySeeds(t, factory)
	reader := startHistoryHost(t, ctx, binary, project, env)
	before := readHistorySnapshot(t, ctx, binary, project, env, reader.url)
	want := before.Observation
	assertCompletedFailedWorkArtifact(t, want)
	assertRecordingIntegrityCapturedCounters(t, before.Logs, map[string]any{
		"origin": "SYNTHETIC", "model": "gpt-5",
		"inputTokens": float64(11), "outputTokens": float64(7), "totalTokens": float64(18),
	})
	reader.stop(t, ctx, binary, project, env)
	host := startHistoryHost(t, ctx, binary, project, env, "--dir", factory, "--record", record, "--continuously")
	after := readCompletedFactorySnapshot(t, ctx, binary, project, env, host.url, want.WorkerSessionId)
	if !reflect.DeepEqual(want, after.Observation) {
		t.Fatalf("restart changed capture facts: before=%+v after=%+v", want, after.Observation)
	}
	if !reflect.DeepEqual(before.Logs, after.Logs) {
		t.Fatal("restored runtime changed committed captured logs")
	}
	assertOriginalFactoryCaptureReads(t, ctx, binary, project, env, host.url, want)
	host.stop(t, ctx, binary, project, env)
}

func readCompletedFactorySnapshot(t *testing.T, ctx context.Context, binary, project string, env []string, server, id string) historySnapshot {
	t.Helper()
	raw := historyCLI(t, ctx, binary, project, env, "--server", server, "--json", "worker-sessions", "show", "--worker-session-id", id)
	var observation api.WorkerSessionObservation
	if err := json.Unmarshal(raw, &observation); err != nil {
		t.Fatal(err)
	}
	assertCompletedFailedWorkArtifact(t, observation)
	assertHistoryHTTP(t, ctx, server, observation)
	logs := readHistoryLogs(t, ctx, binary, project, env, server, id)
	// Restored Runtime event streams retain their canonical Factory provenance;
	// captured logs are the Worker-owned records, so compare each read contract.
	assertHistoryMCPViews(t, ctx, binary, project, env, server, observation, logs, []string{"LIST", "READ", "logs"})
	return historySnapshot{Observation: observation, Logs: logs}
}

func writeFailedWorkCaptureMock(t *testing.T, project, factory, node string) {
	t.Helper()
	path := filepath.Join(factory, "factory.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	config["workstations"].([]any)[0].(map[string]any)["outcomeFormat"] = "decision-envelope"
	raw, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	script := `console.log(JSON.stringify({decision:'ACCEPTED',output:'history-restart completed capture marker',recorded_output_work:[{workTypeId:'task',state:'unknown-business-state',content:[{type:'text',text:'invalid proposal'}]}]}));`
	if err := os.WriteFile(filepath.Join(project, "worker.cjs"), []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(map[string]any{"mockWorkers": []any{map[string]any{
		"workerName": "worker", "workstationName": "process", "runType": "script",
		"scriptConfig": map[string]any{"command": node, "args": []string{filepath.Join(project, "worker.cjs")}},
		"usage":        map[string]any{"provider": "codex", "model": "gpt-5", "inputTokens": 11, "outputTokens": 7},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "mock.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertCompletedFailedWorkArtifact(t *testing.T, got api.WorkerSessionObservation) {
	t.Helper()
	assertCompletedArtifactIdentity(t, got)
	assertCompletedArtifactTerminal(t, got)
	assertCompletedArtifactUsage(t, got)
}

func assertCompletedArtifactIdentity(t *testing.T, got api.WorkerSessionObservation) {
	t.Helper()
	// The script executor configures no model; codex/gpt-5 are usage declaration facts.
	if got.FactorySessionId == nil || got.WorkId == nil || got.AttemptId == "" ||
		got.Provider == nil || *got.Provider != "script" || got.Model != nil {
		encoded, err := json.Marshal(got)
		t.Fatalf("completed capture lost identity: %s (marshal error=%v)", encoded, err)
	}
}

func assertCompletedArtifactTerminal(t *testing.T, got api.WorkerSessionObservation) {
	t.Helper()
	if got.State != "COMPLETED" || got.TerminalCause == nil || *got.TerminalCause != api.WorkerSessionTerminalCauseCompleted ||
		got.RecordingHealth == nil || *got.RecordingHealth != "COMPLETE" ||
		got.StartedAt == nil || got.EndedAt == nil || got.DurationMillis == nil {
		t.Fatalf("completed capture lost facts: %+v", got)
	}
}

func assertCompletedArtifactUsage(t *testing.T, got api.WorkerSessionObservation) {
	t.Helper()
	u := got.TokenUsage
	if u == nil || u.Origin == nil || *u.Origin != api.ProviderSessionTokenUsageOriginSYNTHETIC || u.InputTokens == nil || *u.InputTokens != 11 || u.OutputTokens == nil || *u.OutputTokens != 7 || u.TotalTokens == nil || *u.TotalTokens != 18 {
		t.Fatalf("completed capture usage: %+v", u)
	}
}

func assertOriginalFactoryCaptureReads(t *testing.T, ctx context.Context, binary, project string, env []string, server string, want api.WorkerSessionObservation) {
	t.Helper()
	raw := historyCLI(t, ctx, binary, project, env, "--server", server, "--json", "worker-sessions", "show", "--session", *want.FactorySessionId, "--worker-session-id", want.WorkerSessionId)
	var got api.WorkerSessionObservation
	if err := json.Unmarshal(raw, &got); err != nil || !reflect.DeepEqual(want, got) {
		t.Fatalf("original scope CLI: %s (%v)", raw, err)
	}
	for _, path := range []string{
		"/factory-sessions/" + *want.FactorySessionId + "/worker-sessions/" + want.WorkerSessionId,
		"/factory-sessions/~default/work/" + *want.WorkId,
	} {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, server+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		err = json.NewDecoder(response.Body).Decode(&body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK || err != nil {
			t.Fatalf("HTTP %s: status=%d body=%v error=%v", path, response.StatusCode, body, err)
		}
		if strings.Contains(path, "/worker-sessions/") {
			encoded, _ := json.Marshal(body)
			if err := json.Unmarshal(encoded, &got); err != nil || !reflect.DeepEqual(want, got) {
				t.Fatalf("original scope HTTP: %s (%v)", encoded, err)
			}
		} else if state, ok := body["state"].(map[string]any); !ok || state["type"] != "FAILED" {
			t.Fatalf("business Work failure hidden by completed Worker: %v", body)
		}
	}
}

// Both selected hosts expose the same committed records through the delivered
// CLI and real HTTP boundary. The existing MCP client asserts the same frames.
func assertHistoryReplay(t *testing.T, ctx context.Context, binary, project string, env []string, server string, logs api.WorkerSessionLogPage) {
	t.Helper()
	output := historyCLI(t, ctx, binary, project, env, "--server", server, "--json", "worker-sessions", "stream", "--worker-session-id", logs.WorkerSessionId, "--replay-only")
	var frames []api.WorkerSessionEvent
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		var frame api.WorkerSessionEvent
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			t.Fatal(err)
		}
		frames = append(frames, frame)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	// The CLI stream v1 shape omits commit timestamps and cursors.
	// Compare its published source-record fields; HTTP/MCP compare all facts.
	cliLogs := logs
	cliLogs.Events = append([]api.WorkerSessionEvent(nil), logs.Events...)
	for i := range cliLogs.Events {
		cliLogs.Events[i].Event.CapturedAt = nil
		cliLogs.Events[i].Event.Cursor = api.WorkerSessionEventCursor{}
	}
	assertHistoryReplayFrames(t, frames, cliLogs)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server+"/worker-sessions/"+logs.WorkerSessionId+"/events?replayOnly=true", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("HTTP archive replay status=%d", response.StatusCode)
	}
	frames = nil
	scanner = bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var frame api.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame); err != nil {
			t.Fatal(err)
		}
		frames = append(frames, frame)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	assertHistoryReplayFrames(t, frames, logs)
}

func assertHistoryReplayFrames(t *testing.T, frames []api.WorkerSessionEvent, logs api.WorkerSessionLogPage) {
	t.Helper()
	if len(frames) != len(logs.Events) || len(frames) == 0 {
		t.Fatalf("replay records=%d logs=%d frames=%+v", len(frames), len(logs.Events), frames)
	}
	for i, frame := range frames {
		if frame.WorkerSessionId != logs.WorkerSessionId || !reflect.DeepEqual(frame.Event, logs.Events[i].Event) {
			t.Fatalf("replay record %d differs from committed logs: %+v %+v", i, frame, logs.Events[i])
		}
	}
	last := frames[len(frames)-1]
	if last.Delivery != api.WorkerSessionEventDelivery("TERMINAL_REPLAY") || last.ReplaySummary == nil || !last.ReplaySummary.Complete || last.ReplaySummary.EventsEmitted != int64(len(frames)) {
		t.Fatalf("missing complete finite replay: %+v", last)
	}
}
