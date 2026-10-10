package recording_sidecar_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/work"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// These two cells consume the invoking build's artifact. Declarative mocks
// cross real host lifetime and durable read boundaries without native providers.
// Serial children preserve one writer and keep the owned process budget small.
func TestScopedPhysicalRecordingFacts(t *testing.T) {
	for _, loss := range []bool{false, true} {
		name := "reopen"
		if loss {
			name = "loss"
		}
		t.Run(name, func(t *testing.T) { runScopedPhysicalRecordingFacts(t, loss) })
	}
}

func scopedPhysicalArtifactFixture(t *testing.T, loss bool) invokeArtifactFixture {
	t.Helper()
	project := t.TempDir()
	f := invokeArtifactFixture{binary: invokeArtifactBinary(t), project: project, home: filepath.Join(project, "home"), ctx: t.Context()}
	f.env = builtcliacceptance.ProcessEnvForIsolatedHome(f.home)
	factory := filepath.Join(project, "factory")
	for _, path := range []string{f.home, factory} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	definition := `{"name":"physical-reopen","workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],"workers":[{"name":"worker","type":"AGENT_WORKER","modelProvider":"CODEX","model":"fixture-model"}],"workstations":[{"name":"process","type":"AGENT_RUN","worker":"worker","outcomeFormat":"decision-envelope","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"done"}],"onFailure":[{"workType":"task","state":"failed"}]}]}`
	if err := os.WriteFile(filepath.Join(factory, "factory.json"), []byte(definition), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(testutil.MustRepoPath(t, "docs/examples/mock-workers-result-body-business-invalid.json"))
	if err != nil {
		t.Fatal(err)
	}
	var example map[string]any
	if err := json.Unmarshal(data, &example); err != nil {
		t.Fatal(err)
	}
	bad := example["mockWorkers"].([]any)[0].(map[string]any)["resultBody"]
	rows := []any{map[string]any{"workInputs": []any{map[string]string{"workId": "success"}}, "runType": "accept", "resultBody": map[string]string{"decision": "ACCEPTED", "output": "physical success"}}, map[string]any{"workInputs": []any{map[string]string{"workId": "business"}}, "runType": "accept", "resultBody": bad}}
	if loss {
		rows = []any{map[string]any{"runType": "accept", "gateConfig": map[string]string{"arrivedFile": filepath.Join(project, "arrived"), "releaseFile": filepath.Join(project, "release"), "timeout": "4m"}}}
	}
	data, err = json.Marshal(map[string]any{"mockWorkers": rows})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "mock.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

func runScopedPhysicalRecordingFacts(t *testing.T, loss bool) {
	f := scopedPhysicalArtifactFixture(t, loss)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	f.ctx = ctx
	factory, board := filepath.Join(f.project, "factory"), filepath.Join(f.project, "board.json")
	ids := []string{"success", "business"}
	if loss {
		ids = []string{"lost"}
	}
	for _, id := range ids {
		testutil.WriteSeedRequest(t, factory, work.SubmitRequest{WorkID: id, Name: id, WorkTypeID: "task", Payload: []byte(`{"title":"physical facts"}`)})
	}
	first := startHistoryHost(t, ctx, f.binary, f.project, f.env, "--dir", factory, "--record", board, "--continuously", "--with-mock-workers="+filepath.Join(f.project, "mock.json"))
	before := make(map[string]capturedSummaryArtifactSnapshot)
	for _, workID := range ids {
		row := awaitScopedPhysicalArtifact(t, f, first.url, workID, loss)
		before[workID] = readScopedPhysicalArtifact(t, f, first.url, "~default", row)
	}
	removeHistorySeeds(t, factory)
	if loss {
		if err := first.command.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-first.done:
			first.stopped = true
		case <-ctx.Done():
			t.Fatal("lost owner did not join")
		}
	} else {
		first.stop(t, ctx, f.binary, f.project, f.env)
	}
	second := startHistoryHost(t, ctx, f.binary, f.project, f.env, "--dir", factory, "--resume", board, "--continuously", "--with-mock-workers="+filepath.Join(f.project, "mock.json"))
	var sessions api.ListFactorySessionsResponse
	readSummaryArtifactHTTP(t, f, second.url, "/factory-sessions", &sessions)
	if len(sessions.Sessions) != 1 {
		t.Fatalf("unexpected reopened sessions: %+v", sessions)
	}
	for _, workID := range ids {
		prior := before[workID]
		if prior.Observation.FactorySessionId == nil || sessions.Sessions[0].Id == *prior.Observation.FactorySessionId {
			t.Fatal("reopen did not create a distinct selector UUID")
		}
		var row api.WorkerSessionObservation
		readSummaryArtifactHTTP(t, f, second.url, "/worker-sessions/"+prior.Observation.WorkerSessionId, &row)
		after := readScopedPhysicalArtifact(t, f, second.url, "~default", row)
		assertReopenedPhysicalArtifact(t, f, second.url, workID, loss, prior, after)
	}
	second.stop(t, ctx, f.binary, f.project, f.env)
}

func assertReopenedPhysicalArtifact(t *testing.T, f invokeArtifactFixture, server, workID string, loss bool, prior, after capturedSummaryArtifactSnapshot) {
	t.Helper()
	row := after.Observation
	if loss {
		if row.State == "RUNNING" || row.RecordingHealth == nil || *row.RecordingHealth != "INCOMPLETE" || row.RecordingHealthReason == nil || *row.RecordingHealthReason != "OWNER_LOST" || row.EndedAt != nil || !reflect.DeepEqual(prior.Logs.Events, after.Logs.Events) {
			t.Fatalf("owner loss erased prefix or invented completion: %+v", after)
		}
	} else {
		prior.Observation.ConfirmationState = after.Observation.ConfirmationState
		if !reflect.DeepEqual(prior, after) {
			t.Fatalf("reopen changed physical facts: before=%+v after=%+v", prior, after)
		}
		var item api.Work
		readSummaryArtifactHTTP(t, f, server, "/factory-sessions/~default/work/"+workID, &item)
		want := api.WorkStateTypeTERMINAL
		if workID == "business" {
			want = api.WorkStateTypeFAILED
		}
		if item.State == nil || item.State.Type != want {
			t.Fatalf("reopen changed independent Work outcome: %+v", item)
		}
	}
}

func awaitScopedPhysicalArtifact(t *testing.T, f invokeArtifactFixture, server, workID string, live bool) api.WorkerSessionObservation {
	t.Helper()
	// Host readiness precedes dispatch. Poll the public projection, since this
	// OS-process boundary cannot expose an in-process dispatch signal.
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		var list api.ListWorkerSessionsResponse
		readSummaryArtifactHTTP(t, f, server, "/factory-sessions/~default/worker-sessions?workId="+workID, &list)
		if len(list.Sessions) == 1 {
			row := list.Sessions[0]
			if live && row.State == "RUNNING" {
				if _, err := os.Stat(filepath.Join(f.project, "arrived")); err == nil {
					return row
				}
			}
			if !live && row.State == "COMPLETED" && row.RecordingHealth != nil && *row.RecordingHealth == "COMPLETE" {
				return row
			}
		}
		select {
		case <-ticker.C:
		case <-f.ctx.Done():
			t.Fatal("physical attempt did not reach expected public state")
		}
	}
}

func readScopedPhysicalArtifact(t *testing.T, f invokeArtifactFixture, server, scope string, row api.WorkerSessionObservation) capturedSummaryArtifactSnapshot {
	t.Helper()
	var scoped api.WorkerSessionObservation
	raw := historyCLI(t, f.ctx, f.binary, f.project, f.env, "--server", server, "--json", "worker-sessions", "show", "--session", scope, "--worker-session-id", row.WorkerSessionId)
	if err := json.Unmarshal(raw, &scoped); err != nil {
		t.Fatal(err)
	}
	scoped.ConfirmationState = row.ConfirmationState
	if row.State == "RUNNING" {
		scoped.DurationMillis = row.DurationMillis
	}
	if !reflect.DeepEqual(scoped, row) {
		a, _ := json.Marshal(scoped)
		b, _ := json.Marshal(row)
		t.Fatalf("scoped physical summary differs: %s %s", a, b)
	}
	var page api.WorkerSessionLogPage
	readSummaryArtifactHTTP(t, f, server, "/worker-sessions/"+row.WorkerSessionId+"/logs?limit=1000", &page)
	if len(page.Events) == 0 || row.RecordingHealth == nil || string(page.Health) != string(*row.RecordingHealth) {
		t.Fatalf("capture/state disagrees: %+v %+v", page, row)
	}
	// Declarative mocks have no Provider Session reference. Their captured
	// prefix stays readable while transcript refusal distinguishes live/lost.
	response, err := http.Get(server + "/factory-sessions/" + scope + "/worker-sessions/" + row.WorkerSessionId + "/transcript")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var failure api.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&failure); err != nil {
		t.Fatal(err)
	}
	want := api.ErrorResponseCodeWORKERSESSIONTRANSCRIPTUNAVAILABLE
	if row.State == "RUNNING" {
		want = api.ErrorResponseCodeWORKERSESSIONTRANSCRIPTACTIVE
	}
	if response.StatusCode == http.StatusOK || failure.Code != want {
		t.Fatalf("no-reference transcript = %d %+v, want %s", response.StatusCode, failure, want)
	}
	return capturedSummaryArtifactSnapshot{Observation: row, Logs: page}
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
