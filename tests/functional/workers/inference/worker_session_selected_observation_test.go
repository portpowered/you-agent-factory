package inference_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/google/uuid"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The command edge holds one lifecycle phase while customer CLI reads traverse
// production session selection, HTTP binding, live registry and history joins.
func TestWorkerSessionSelectedSessionObservationConsistency(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"available", "missing-transcript", "no-provider-tuple"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()
			runSelectedObservationScenario(t, variant)
		})
	}
}

func runSelectedObservationScenario(t *testing.T, variant string) {
	t.Helper()
	group := sharedInferenceGroup
	group.ensure(t)
	dir := support.ScaffoldSingleStepFactory(t, "selected-observation-"+variant)
	support.WriteAgentConfig(t, dir, "processor", sharedInferenceWithExecutorProvider(
		"---\ntype: MODEL_WORKER\nmodel: gpt-5-codex\nmodelProvider: CODEX\nstopToken: COMPLETE\n---\nProcess the input task.\n", "CODEX"))
	providerID := uuid.NewString()
	runner := &selectedObservationRunner{providerID: providerID, ready: make(chan struct{}), release: make(chan struct{})}
	if variant == "no-provider-tuple" {
		runner.providerID = ""
	}
	if variant == "available" {
		writeSelectedObservationTranscript(t, group.homeDir, providerID)
	}
	if err := group.commands.set(dir, runner, nil, inferenceRouteContext{scenarioName: t.Name(), sessionID: "opening"}); err != nil {
		t.Fatal(err)
	}
	defer group.commands.clear(dir)
	sessionID := openSharedInferenceSession(t, group, dir)
	defer closeSharedInferenceSession(t, group, sessionID)
	defer runner.unblock()
	otherDir := support.ScaffoldSingleStepFactory(t, "selected-observation-other")
	otherID := openSharedInferenceSession(t, group, otherDir)
	defer closeSharedInferenceSession(t, group, otherID)
	submitted := support.SubmitSessionWorkAt(t, group.baseURL, sessionID, *sharedInferenceWork(t.Name()))
	if submitted.WorkId == nil {
		t.Fatal("Work submission omitted identity")
	}
	ctx, cancel := context.WithTimeout(t.Context(), sharedInferenceScenarioTimeout)
	defer cancel()
	select {
	case <-runner.ready:
	case <-ctx.Done():
		t.Fatal("command edge did not publish progress")
	}
	// The synchronous output observer has returned after association and tool
	// progress publication. The edge now holds execution until all reads finish.
	workID := *submitted.WorkId
	rows := selectedObservationList(t, group, dir, "list", "--session", sessionID, "--work-id", workID)
	if len(rows) != 1 {
		t.Fatalf("selected Work rows = %#v, want one owned attempt", rows)
	}
	workerID := rows[0].WorkerSessionId
	assertSelectedObservationParity(t, group, dir, sessionID, workID, workerID, runner.providerID, variant, "RUNNING")
	assertSelectedObservationErrors(t, group, dir, sessionID, otherID, workID, workerID)
	assertSelectedObservationParity(t, group, dir, sessionID, workID, workerID, runner.providerID, variant, "RUNNING")
	runner.unblock()
	support.WaitForSessionTerminalStatus(t, group.baseURL, sessionID, sharedInferenceScenarioTimeout)
	assertSelectedObservationParity(t, group, dir, sessionID, workID, workerID, runner.providerID, variant, "COMPLETED")
}

type selectedObservationRunner struct {
	providerID     string
	ready, release chan struct{}
	once           sync.Once
}

func (r *selectedObservationRunner) unblock() { r.once.Do(func() { close(r.release) }) }

func (r *selectedObservationRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return r.RunStreaming(ctx, request, nil)
}

func (r *selectedObservationRunner) RunStreaming(ctx context.Context, _ platformprocess.CommandRequest, observer platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	progress := ""
	if r.providerID != "" {
		progress = fmt.Sprintf("{\"type\":\"thread.started\",\"thread_id\":%q}\n", r.providerID)
	}
	progress += "{\"type\":\"turn.started\"}\n{\"type\":\"item.started\",\"item\":{\"id\":\"owned-tool\",\"type\":\"command_execution\",\"command\":\"echo selected-observation\",\"status\":\"in_progress\"}}\n"
	if observer != nil {
		observer(platformprocess.OutputStreamStdout, []byte(progress))
	}
	close(r.ready)
	select {
	case <-r.release:
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
	completion := support.CodexSuccessStdout("Selected observation COMPLETE")
	if observer != nil {
		observer(platformprocess.OutputStreamStdout, completion)
	}
	return platformprocess.CommandResult{Stdout: append([]byte(progress), completion...)}, nil
}

func writeSelectedObservationTranscript(t *testing.T, home, providerID string) {
	t.Helper()
	dir := filepath.Join(home, ".codex", "sessions", "2026", "10", "04")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rollout-"+providerID+".jsonl")
	contents := fmt.Sprintf("{\"type\":\"session_meta\",\"payload\":{\"id\":%q,\"timestamp\":\"2026-10-04T19:34:00Z\"}}\n{\"type\":\"event_msg\",\"payload\":{\"type\":\"agent_message\",\"message\":\"Owned transcript COMPLETE\"}}\n", providerID)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
}

func selectedObservationCLI(t *testing.T, group *inferenceProcessGroup, dir string, args ...string) ([]byte, error) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), append([]string{"you", "--server", group.baseURL, "--json", "worker-sessions"}, args...))
	inputs.Input.Env = sharedInferenceProcessEnvironment(group.homeDir)
	inputs.Input.WorkingDirectory = dir
	err := group.process.Execute(inputs.Input)
	if err != nil {
		return []byte(inputs.Stderr()), err
	}
	return []byte(inputs.Stdout()), nil
}

func selectedObservationList(t *testing.T, group *inferenceProcessGroup, dir string, args ...string) []factoryapi.WorkerSessionObservation {
	t.Helper()
	raw, err := selectedObservationCLI(t, group, dir, args...)
	if err != nil {
		t.Fatalf("CLI %v: %v %s", args, err, raw)
	}
	var list factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("decode list: %v %s", err, raw)
	}
	return list.Sessions
}

func assertSelectedObservationParity(t *testing.T, group *inferenceProcessGroup, dir, sessionID, workID, workerID, providerID, variant, state string) {
	t.Helper()
	workRows := selectedObservationList(t, group, dir, "list", "--session", sessionID, "--work-id", workID)
	if len(workRows) != 1 {
		t.Fatalf("Work rows = %#v", workRows)
	}
	args := []string{"list", "--session", sessionID, "--max-results", "1000"}
	if state == "RUNNING" {
		args = append(args, "--state", "RUNNING", "--state", "STARTING")
	}
	fleet := selectedObservationList(t, group, dir, args...)
	var matching []factoryapi.WorkerSessionObservation
	for _, row := range fleet {
		if row.WorkerSessionId == workerID {
			matching = append(matching, row)
		}
	}
	if len(matching) != 1 {
		t.Fatalf("matching fleet rows = %#v, want one %s", matching, workerID)
	}
	raw, err := selectedObservationCLI(t, group, dir, "show", "--session", sessionID, "--worker-session-id", workerID)
	if err != nil {
		t.Fatalf("selected show: %v %s", err, raw)
	}
	var shown factoryapi.WorkerSessionObservation
	if err := json.Unmarshal(raw, &shown); err != nil {
		t.Fatal(err)
	}
	for _, row := range []factoryapi.WorkerSessionObservation{matching[0], workRows[0], shown} {
		assertSelectedObservationFacts(t, row, matching[0], sessionID, workerID, providerID, variant, state)
	}
}

func assertSelectedObservationFacts(t *testing.T, row, fleet factoryapi.WorkerSessionObservation, sessionID, workerID, providerID, variant, state string) {
	t.Helper()
	if row.State != factoryapi.WorkerSessionObservationState(state) || row.FactorySessionId == nil || *row.FactorySessionId != sessionID || row.WorkerSessionId != workerID {
		t.Fatalf("live facts = %#v; want %s in %s", row, state, sessionID)
	}
	assertSelectedProviderFacts(t, row, providerID, variant)
	if state == "RUNNING" && row.ConfirmationState != factoryapi.UNCONFIRMED {
		t.Fatalf("active confirmation = %s", row.ConfirmationState)
	}
	if !reflect.DeepEqual(row.RecordingHealth, fleet.RecordingHealth) || !reflect.DeepEqual(row.RecordingHealthReason, fleet.RecordingHealthReason) || row.ConfirmationState != fleet.ConfirmationState {
		t.Fatalf("health/confirmation disagree: fleet=%#v scoped=%#v", fleet, row)
	}
}

func assertSelectedProviderFacts(t *testing.T, row factoryapi.WorkerSessionObservation, providerID, variant string) {
	t.Helper()
	wantProvider := (*factoryapi.WorkerSessionProviderSessionRef)(nil)
	if providerID != "" {
		wantProvider = &factoryapi.WorkerSessionProviderSessionRef{Provider: "codex", Kind: "session_id", Id: providerID}
	}
	if row.ProviderSessionAvailable != (providerID != "") || !reflect.DeepEqual(row.ProviderSession, wantProvider) {
		t.Fatalf("provider facts = %#v/%t, want controlled tuple %q", row.ProviderSession, row.ProviderSessionAvailable, providerID)
	}
	wantTranscript := factoryapi.WorkerSessionObservationTranscriptUNAVAILABLE
	if variant == "available" {
		wantTranscript = factoryapi.WorkerSessionObservationTranscriptAVAILABLE
	}
	wantHealth := factoryapi.WorkerSessionObservationRecordingHealthComplete
	if row.State == factoryapi.WorkerSessionObservationStateRunning {
		wantHealth = factoryapi.WorkerSessionObservationRecordingHealthIncomplete
	}
	if row.RecordingHealth == nil || *row.RecordingHealth != wantHealth || row.RecordingHealthReason != nil {
		t.Fatalf("controlled recording health = %#v/%v, want %s without interruption", row.RecordingHealth, row.RecordingHealthReason, wantHealth)
	}
	if row.Transcript != wantTranscript || row.Failure != nil {
		t.Fatalf("optional facts = %#v", row)
	}
}

func assertSelectedObservationErrors(t *testing.T, group *inferenceProcessGroup, dir, sessionID, otherID, workID, workerID string) {
	t.Helper()
	for _, args := range [][]string{
		{"show", "--session", otherID, "--worker-session-id", workerID},
		{"list", "--session", otherID, "--work-id", workID},
		{"show", "--session", sessionID, "--worker-session-id", uuid.NewString()},
	} {
		raw, err := selectedObservationCLI(t, group, dir, args...)
		var diagnostic struct {
			Code string `json:"code"`
		}
		if err == nil || json.Unmarshal(raw, &diagnostic) != nil || (diagnostic.Code != "WORKER_SESSION_NOT_FOUND" && diagnostic.Code != "WORK_NOT_FOUND") {
			t.Fatalf("out-of-scope read %v = %v %s, want typed not-found", args, err, raw)
		}
	}
}
