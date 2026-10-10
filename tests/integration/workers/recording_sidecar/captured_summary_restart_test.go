package recording_sidecar_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// IR1/IR2 consume an upstream artifact and reopen the same private profile
// only after its writer has joined. Mock execution never needs provider files.
func TestRecordingContentIntegrityRestart(t *testing.T) {
	binary := os.Getenv("INFINITE_YOU_PREBUILT_ARTIFACT")
	if binary == "" {
		if os.Getenv("INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT") == "1" {
			t.Fatal("prebuilt artifact is required")
		}
		t.Skip("prebuilt artifact required")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("recording integrity artifact sha256=%x", sha256.Sum256(artifact))
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	project := t.TempDir()
	home, temp := filepath.Join(project, "home"), filepath.Join(project, "temp")
	for _, path := range []string{home, temp} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	env := cleanupEnvironment(home, temp)
	// No executable provider exists on this child-owned PATH.
	env = append(env, "PATH="+temp)
	factory, config := writeRecordingIntegrityMock(t, project)
	first := startHistoryHost(t, ctx, binary, project, env, "--dir", factory, "--continuously", "--with-mock-workers", config)
	for _, name := range []string{"direct", "omitted"} {
		document := map[string]any{
			"requestId": name + "-request", "workerSessionId": name,
			"execution": map[string]any{
				"workstationName": name, "workingDirectory": project, "runnerId": "codex", "executorProvider": "codex",
				"modelProvider": "codex", "model": "integrity-model", "userMessage": `{"output":"DIRECT_CAPTURE_BETA"}`,
				"dispatch": map[string]any{"dispatchId": name + "-attempt", "workstationName": name},
			},
		}
		path := filepath.Join(project, name+".json")
		writeRecordingIntegrityJSON(t, path, document)
		historyCLI(t, ctx, binary, project, env, "--remote", "--server", first.url, "--json", "worker-sessions", "invoke", "--execution", path)
	}
	before := readRecordingIntegritySnapshots(t, ctx, binary, project, env, first.url)
	first.stop(t, ctx, binary, project, env)
	removeHistorySeeds(t, factory)
	for _, provider := range []string{".codex", ".cursor", ".claude"} {
		if err := os.WriteFile(filepath.Join(home, provider), []byte("provider files unavailable"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	second := startHistoryHost(t, ctx, binary, project, env)
	after := readRecordingIntegritySnapshots(t, ctx, binary, project, env, second.url)
	for id, expected := range before {
		actual, ok := after[id]
		if !ok {
			t.Fatalf("restart lost %s", id)
		}
		// Confirmation belongs to live Runtime; the comparison owns capture facts.
		expected.Observation.ConfirmationState = actual.Observation.ConfirmationState
		if !reflect.DeepEqual(expected, actual) {
			t.Fatalf("restart changed %s content/usage/order/provenance", id)
		}
		assertHistoryMCPViews(t, ctx, binary, project, env, second.url, actual.Observation, actual.Logs, []string{"READ", "logs"})
	}
	second.stop(t, ctx, binary, project, env)
}

func writeRecordingIntegrityJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeRecordingIntegrityMock(t *testing.T, project string) (string, string) {
	t.Helper()
	factory := filepath.Join(project, "factory")
	f := invokeArtifactFixture{project: project}
	writeSummaryArtifactFactory(t, f)
	path := filepath.Join(factory, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var definition map[string]any
	if err := json.Unmarshal(data, &definition); err != nil {
		t.Fatal(err)
	}
	delete(definition["workstations"].([]any)[0].(map[string]any), "expectedArtifacts")
	writeRecordingIntegrityJSON(t, path, definition)
	input, output, cached, reasoning := int64(17), int64(9), int64(0), int64(2)
	usage := &workers.MockWorkerUsageConfig{Provider: "codex", Model: "integrity-model", InputTokens: &input, OutputTokens: &output, CachedInputTokens: &cached, ReasoningOutputTokens: &reasoning}
	config := workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{
		{WorkstationName: "direct", RunType: workers.MockWorkerRunTypeAccept, ResultBody: json.RawMessage(`{"output":"DIRECT_CAPTURE_BETA"}`), Usage: usage},
		{WorkstationName: "omitted", RunType: workers.MockWorkerRunTypeAccept, ResultBody: json.RawMessage(`{"output":"DIRECT_CAPTURE_BETA"}`)},
		{WorkstationName: "process", RunType: workers.MockWorkerRunTypeAccept, ResultBody: json.RawMessage(`{"output":"FACTORY_CAPTURE_ALPHA COMPLETE"}`), Usage: usage},
	}}
	configPath := filepath.Join(project, "mock.json")
	writeRecordingIntegrityJSON(t, configPath, config)
	return factory, configPath
}

func readRecordingIntegritySnapshots(t *testing.T, ctx context.Context, binary, project string, env []string, server string) map[string]capturedSummaryArtifactSnapshot {
	t.Helper()
	var rows []api.WorkerSessionObservation
	deadline := time.NewTicker(20 * time.Millisecond)
	defer deadline.Stop()
	for {
		rows = historyRows(t, ctx, binary, project, env, server, "archived")
		ready := len(rows) == 3
		for _, row := range rows {
			ready = ready && row.RecordingHealth != nil && *row.RecordingHealth == "COMPLETE"
		}
		if ready {
			break
		}
		select {
		case <-deadline.C:
		case <-ctx.Done():
			t.Fatalf("recording integrity completion: %v rows=%+v", ctx.Err(), rows)
		}
	}
	result := make(map[string]capturedSummaryArtifactSnapshot)
	for _, row := range rows {
		var snapshot capturedSummaryArtifactSnapshot
		id := row.WorkerSessionId
		read := func(view string, into any) {
			args := []string{"--remote", "--server", server, "--json", "worker-sessions"}
			if view == "summary" {
				args = append(args, "show", "--worker-session-id", id)
			} else {
				args = append(args, "read", "--worker-session-id", id, "--view", view)
			}
			body := historyCLI(t, ctx, binary, project, env, args...)
			if err := json.Unmarshal(body, into); err != nil {
				t.Fatal(err)
			}
		}
		read("summary", &snapshot.Observation)
		read("logs", &snapshot.Logs)
		if id == "omitted" {
			// A mock without usage has no third-party association. Preserve the
			// existing explicit transcript-unavailable result; logs hold content.
			command := exec.CommandContext(ctx, binary, "--remote", "--server", server, "--json", "worker-sessions", "read", "--worker-session-id", id, "--view", "transcript")
			command.Dir, command.Env = project, env
			body, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(body), `"code":"WORKER_SESSION_TRANSCRIPT_UNAVAILABLE"`) {
				t.Fatalf("unassociated transcript: error=%v body=%s", err, body)
			}
		} else {
			read("transcript", &snapshot.Transcript)
		}
		if row.State != "COMPLETED" || (id != "omitted" && len(snapshot.Transcript.Entries) != 1) {
			t.Fatalf("single completed logical result: %+v", snapshot)
		}
		marker := "DIRECT_CAPTURE_BETA"
		if !row.Direct {
			marker = "FACTORY_CAPTURE_ALPHA COMPLETE"
		}
		if id != "omitted" && (snapshot.Transcript.Entries[0].Text == nil || *snapshot.Transcript.Entries[0].Text != `{"output":"`+marker+`"}`) {
			t.Fatalf("exact content lost: %+v", snapshot.Transcript)
		}
		encoded, err := json.Marshal(snapshot.Logs.Events)
		if err != nil || !strings.Contains(string(encoded), marker) {
			t.Fatalf("captured content lost: %v", err)
		}
		f := invokeArtifactFixture{ctx: ctx, binary: binary, project: project, env: env}
		var httpSummary api.WorkerSessionObservation
		var httpLogs api.WorkerSessionLogPage
		readSummaryArtifactHTTP(t, f, server, "/worker-sessions/"+id, &httpSummary)
		readSummaryArtifactHTTP(t, f, server, "/worker-sessions/"+id+"/logs", &httpLogs)
		if !reflect.DeepEqual(snapshot.Observation, httpSummary) || !reflect.DeepEqual(snapshot.Logs, httpLogs) {
			t.Fatal("CLI/HTTP capture facts differ")
		}
		if id != "omitted" {
			var httpTranscript api.WorkerSessionTranscriptResponse
			readSummaryArtifactHTTP(t, f, server, "/worker-sessions/"+id+"/transcript", &httpTranscript)
			if !reflect.DeepEqual(snapshot.Transcript, httpTranscript) {
				t.Fatal("CLI/HTTP transcript differs")
			}
		}
		assertRecordingIntegrityUsage(t, snapshot, id == "omitted")
		for i, frame := range snapshot.Logs.Events {
			if frame.Event.Position != int64(i+1) || frame.Event.CapturedAt == nil || frame.Event.SourceEventId == "" {
				t.Fatal("captured positions/identity/time lost")
			}
		}
		if snapshot.Logs.NextToken != nil || snapshot.Logs.CommittedPosition != int64(len(snapshot.Logs.Events)) {
			t.Fatal("incomplete capture snapshot")
		}
		result[id] = snapshot
	}
	return result
}

func assertRecordingIntegrityUsage(t *testing.T, snapshot capturedSummaryArtifactSnapshot, omitted bool) {
	t.Helper()
	usage := snapshot.Observation.TokenUsage
	data, err := json.Marshal(snapshot.Logs.Events)
	if err != nil {
		t.Fatal(err)
	}
	if omitted {
		if usage != nil || strings.Contains(string(data), `"origin":"SYNTHETIC"`) {
			t.Fatal("omitted usage was invented")
		}
		return
	}
	if usage == nil || usage.Origin == nil || *usage.Origin != api.ProviderSessionTokenUsageOriginSYNTHETIC ||
		usage.InputTokens == nil || *usage.InputTokens != 17 || usage.OutputTokens == nil || *usage.OutputTokens != 9 ||
		usage.CachedInputTokens == nil || *usage.CachedInputTokens != 0 || usage.ReasoningOutputTokens == nil || *usage.ReasoningOutputTokens != 2 ||
		usage.TotalTokens == nil || *usage.TotalTokens != 26 || !strings.Contains(string(data), `"origin":"SYNTHETIC"`) || !strings.Contains(string(data), `"delivery":"SYNTHESIZED"`) {
		t.Fatalf("synthetic counters/zero/origin changed: %+v", usage)
	}
	if snapshot.Observation.Provider == nil || !strings.EqualFold(*snapshot.Observation.Provider, "codex") || snapshot.Observation.Model == nil || *snapshot.Observation.Model != "integrity-model" {
		t.Fatal("mock usage lost configured provider/model")
	}
	count := 0
	for _, frame := range snapshot.Logs.Events {
		payload, err := json.Marshal(frame.Event.Payload)
		if err != nil {
			t.Fatal(err)
		}
		var draft workers.Draft
		if err := json.Unmarshal(payload, &draft); err != nil {
			t.Fatal(err)
		}
		if draft.Kind != workers.KindUsage {
			continue
		}
		count++
		var fact map[string]any
		if err := json.Unmarshal(draft.Payload, &fact); err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"origin": "SYNTHETIC", "model": "integrity-model", "inputTokens": float64(17), "outputTokens": float64(9), "cachedInputTokens": float64(0), "reasoningOutputTokens": float64(2), "totalTokens": float64(26)}
		if !reflect.DeepEqual(fact, want) || draft.Provenance.Delivery != workers.DeliverySynthesized {
			t.Fatalf("captured configured usage fact changed: %+v", fact)
		}
	}
	if count != 1 {
		t.Fatalf("configured usage facts = %d, want one", count)
	}
}

type capturedSummaryArtifactSnapshot struct {
	Observation api.WorkerSessionObservation
	Logs        api.WorkerSessionLogPage
	Transcript  api.WorkerSessionTranscriptResponse
}

// The upstream build supplies one CLI artifact. Two isolated cells cross real
// subprocess/activation boundaries with a PATH-owned provider shim and no paid
// calls. Factory execution exits idle with code zero despite failed Work.
func TestCapturedSummaryCleanIdleRestart(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"direct", "factory-artifact-rejection"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runCapturedSummaryCleanIdleRestart(t, name)
		})
	}
}

func runCapturedSummaryCleanIdleRestart(t *testing.T, name string) {
	f := newInvokeArtifactFixture(t)
	// This cell crosses two real host lifetimes plus a finite Factory run;
	// the fixture's single-invocation deadline does not own this whole journey.
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	t.Cleanup(cancel)
	f.ctx = ctx
	f.env = append(f.env, "T7_SHIM_COMPLETE=1")
	var first *historyHost
	record := filepath.Join(f.project, "factory-recording.json")
	if name == "direct" {
		first = startHistoryHost(t, f.ctx, f.binary, f.project, f.env)
		historyCLI(t, f.ctx, f.binary, f.project, f.env, "--remote", "--server", first.url, "--json", "worker-sessions", "invoke", "--execution", f.document)
	} else {
		writeSummaryArtifactFactory(t, f)
		output := historyCLI(t, f.ctx, f.binary, f.project, f.env, "run", "--dir", filepath.Join(f.project, "factory"), "--record", record, "--quiet")
		t.Logf("Factory natural idle exit=0 stdout=%s", output)
		removeHistorySeeds(t, filepath.Join(f.project, "factory"))
		first = startHistoryHost(t, f.ctx, f.binary, f.project, f.env)
	}
	before := readSummaryArtifactSnapshot(t, f, first.url, "")
	if name != "direct" && (before.Observation.State != "FAILED" || before.Observation.Failure == nil || before.Observation.WorkId == nil) {
		t.Fatalf("expected-artifact rejection lost captured failure: %+v", before.Observation)
	}
	first.stop(t, f.ctx, f.binary, f.project, f.env)
	t.Log("first idle host exit=0; same profile and invocation directory retained")
	for _, provider := range []string{".codex", ".cursor", ".claude"} {
		if err := os.WriteFile(filepath.Join(f.home, provider), []byte("provider files unavailable"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	args := []string{}
	if name != "direct" {
		args = []string{"--dir", filepath.Join(f.project, "factory"), "--record", record, "--continuously"}
	}
	second := startHistoryHost(t, f.ctx, f.binary, f.project, f.env, args...)
	after := readSummaryArtifactSnapshot(t, f, second.url, before.Observation.WorkerSessionId)
	// Runtime samples canonical Factory confirmation separately from Worker
	// capture. This witness compares capture facts, not that independent gate.
	before.Observation.ConfirmationState = after.Observation.ConfirmationState
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("restart changed captured facts/content: before=%+v after=%+v", before, after)
	}
	if name != "direct" {
		assertSummaryArtifactFailedWork(t, f, second.url, *after.Observation.WorkId)
	}
	second.stop(t, f.ctx, f.binary, f.project, f.env)
}

func writeSummaryArtifactFactory(t *testing.T, f invokeArtifactFixture) {
	t.Helper()
	factory := filepath.Join(f.project, "factory")
	for _, path := range []string{filepath.Join(factory, "workers", "worker"), filepath.Join(factory, "workstations", "process")} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	definition := `{"name":"summary-restart","workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],"workers":[{"name":"worker"}],"workstations":[{"name":"process","worker":"worker","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"done"}],"onFailure":[{"workType":"task","state":"failed"}],"expectedArtifacts":[{"name":"required-report","pattern":"missing-report.txt","nonEmpty":true}]}]}`
	dir, _ := json.Marshal(f.project)
	files := map[string]string{
		filepath.Join(factory, "factory.json"):                         definition,
		filepath.Join(factory, "workers", "worker", "AGENTS.md"):       "---\ntype: MODEL_WORKER\nexecutorProvider: CODEX\nmodel: t7-artifact-model\nmodelProvider: codex\nreasoningEffort: high\nstopToken: COMPLETE\n---\nReturn the controlled answer.\n",
		filepath.Join(factory, "workstations", "process", "AGENTS.md"): fmt.Sprintf("---\ntype: MODEL_WORKSTATION\nrunner: codex\nworkingDirectory: %s\n---\nReturn the controlled answer.\n", dir),
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	testutil.WriteSeedFile(t, factory, "task", []byte(`{"title":"expected artifact failure"}`))
}

func readSummaryArtifactSnapshot(t *testing.T, f invokeArtifactFixture, server, id string) capturedSummaryArtifactSnapshot {
	t.Helper()
	rows := historyRows(t, f.ctx, f.binary, f.project, f.env, server, "archived")
	if id == "" {
		if len(rows) != 1 {
			t.Fatalf("captured membership: %+v", rows)
		}
		id = rows[0].WorkerSessionId
	}
	var shown api.WorkerSessionObservation
	if err := json.Unmarshal(f.cli(t, server, "show", "--worker-session-id", id), &shown); err != nil {
		t.Fatal(err)
	}
	assertSummaryArtifactFacts(t, shown)
	for _, history := range []string{"archived", "all"} {
		listed := historyRows(t, f.ctx, f.binary, f.project, f.env, server, history)
		if len(listed) != 1 {
			t.Fatalf("%s membership disagrees: %+v", history, listed)
		}
		listed[0].ConfirmationState = shown.ConfirmationState
		if !reflect.DeepEqual(listed[0], shown) {
			t.Fatalf("%s summary disagrees: %+v %+v", history, listed, shown)
		}
	}
	assertSummaryArtifactHTTP(t, f, server, shown)
	if shown.FactorySessionId != nil {
		var scoped api.WorkerSessionObservation
		raw := f.cli(t, server, "show", "--session", *shown.FactorySessionId, "--worker-session-id", id)
		if err := json.Unmarshal(raw, &scoped); err != nil || !reflect.DeepEqual(scoped, shown) {
			t.Fatalf("original scope differs: %s %v", raw, err)
		}
	}
	var transcript api.WorkerSessionTranscriptResponse
	if err := json.Unmarshal(f.cli(t, server, "read", "--worker-session-id", id), &transcript); err != nil {
		t.Fatal(err)
	}
	var logs api.WorkerSessionLogPage
	if err := json.Unmarshal(f.cli(t, server, "read", "--worker-session-id", id, "--view", "logs"), &logs); err != nil {
		t.Fatal(err)
	}
	return capturedSummaryArtifactSnapshot{Observation: shown, Logs: logs, Transcript: transcript}
}

func assertSummaryArtifactHTTP(t *testing.T, f invokeArtifactFixture, server string, shown api.WorkerSessionObservation) {
	t.Helper()
	var detail api.WorkerSessionObservation
	readSummaryArtifactHTTP(t, f, server, "/worker-sessions/"+shown.WorkerSessionId, &detail)
	if !reflect.DeepEqual(detail, shown) {
		t.Fatalf("CLI/HTTP show differs: %+v %+v", shown, detail)
	}
	for _, history := range []string{"archived", "all"} {
		var list api.ListWorkerSessionsResponse
		readSummaryArtifactHTTP(t, f, server, "/worker-sessions?history="+history, &list)
		if len(list.Sessions) != 1 {
			t.Fatalf("HTTP %s membership: %+v", history, list)
		}
		list.Sessions[0].ConfirmationState = shown.ConfirmationState
		if !reflect.DeepEqual(list.Sessions[0], shown) {
			t.Fatalf("HTTP %s captured facts differ: %+v %+v", history, shown, list)
		}
	}
}

func readSummaryArtifactHTTP(t *testing.T, f invokeArtifactFixture, server, path string, result any) {
	t.Helper()
	request, err := http.NewRequestWithContext(f.ctx, http.MethodGet, server+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(result); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("HTTP %s: status=%d err=%v", path, response.StatusCode, err)
	}
}

func assertSummaryArtifactFacts(t *testing.T, shown api.WorkerSessionObservation) {
	t.Helper()
	if shown.ProviderSession == nil || shown.ProviderSession.Id != "t7-artifact-thread" || !shown.ProviderSessionAvailable || shown.Transcript != "AVAILABLE" || shown.TokenUsage == nil || shown.StartedAt == nil || shown.EndedAt == nil || shown.DurationMillis == nil {
		t.Fatalf("lost committed facts: %+v", shown)
	}
}

func assertSummaryArtifactFailedWork(t *testing.T, f invokeArtifactFixture, server, workID string) {
	t.Helper()
	request, err := http.NewRequestWithContext(f.ctx, http.MethodGet, server+"/factory-sessions/~default/work/"+workID, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var work api.Work
	if err := json.NewDecoder(response.Body).Decode(&work); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("restored Work: status=%d err=%v", response.StatusCode, err)
	}
	if work.FailureDetail == nil || work.FailureDetail.Reason != "EXPECTED_ARTIFACTS_UNSATISFIED" {
		t.Fatalf("independent Work failure lost: %+v", work)
	}
}
