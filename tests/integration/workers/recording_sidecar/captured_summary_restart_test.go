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

	"github.com/portpowered/infinite-you/internal/testutil"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

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
