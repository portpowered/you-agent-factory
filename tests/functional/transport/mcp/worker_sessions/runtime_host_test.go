package workersessions_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The independently root-built host is the real selected HTTP boundary. Only
// provider execution is controlled; no provider session association is emitted.
type controlHostRunner struct{ started chan (<-chan struct{}) }

func (r controlHostRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	done := make(chan struct{})
	r.started <- done
	<-ctx.Done()
	close(done)
	return platformprocess.CommandResult{}, ctx.Err()
}

func runRealHostControls(t *testing.T, process support.Process) {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "mcp-control-host")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	runner := controlHostRunner{started: make(chan (<-chan struct{}), 4)}
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true, Edges: serviceedges.Edges{ProviderCommandRunner: runner},
	})
	session, ctx := startMCP(t, process, host.URL())
	sibling := admitControlWorker(t, ctx, host.URL(), "sibling", runner)
	for _, operation := range []string{"CANCEL", "TERMINATE"} {
		id := "mcp-" + strings.ToLower(operation)
		done := admitControlWorker(t, ctx, host.URL(), id, runner)
		listed := callWorker(t, ctx, session, "list", map[string]any{"scope": "direct"})["result"]
		assertRuntimeObservationParity(t, listed, getHost(t, host.URL()+"/worker-sessions?scope=direct"))
		before := callWorker(t, ctx, session, "read", map[string]any{"workerSessionId": id})["result"].(map[string]any)["session"]
		assertRuntimeObservationParity(t, before, getHost(t, host.URL()+"/worker-sessions/"+id))
		args := map[string]any{"workerSessionId": id, "operation": operation}
		applied := callWorker(t, ctx, session, "control", args)["result"].(map[string]any)
		assertOwnedControl(t, applied, id, operation, "APPLIED")
		waitControlSignal(t, done)
		retry := callWorker(t, ctx, session, "control", args)["result"]
		assertOwnedControl(t, retry.(map[string]any), id, operation, "NOOP")
		assertJSONEqual(t, retry, requestHost(t, http.MethodPost, host.URL()+"/worker-sessions/"+id+"/"+strings.ToLower(operation)))
		assertCLIControlParity(t, host, retry, id, strings.ToLower(operation))
		read := callWorker(t, ctx, session, "read", map[string]any{"workerSessionId": id, "view": "events", "limit": 1})["result"].(map[string]any)
		assertJSONEqual(t, read["session"], getHost(t, host.URL()+"/worker-sessions/"+id))
		page := read["events"].(map[string]any)
		if len(page["events"].([]any)) != 1 || page["truncated"] != true {
			t.Fatalf("real retained stream did not report bounded prefix: %v", page)
		}
		select {
		case <-sibling:
			t.Fatal("control stopped the sibling provider execution")
		default:
		}
		peer := getHost(t, host.URL()+"/worker-sessions/sibling").(map[string]any)
		if peer["state"] != "RUNNING" && peer["state"] != "STARTING" {
			t.Fatalf("sibling state: %v", peer)
		}
	}
	unknown := callTool(t, ctx, session, "you.worker_session.read", map[string]any{"workerSessionId": "missing-runtime-worker"})
	assertToolError(t, unknown, "worker_session.not_found", false)
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": "sibling", "operation": "TERMINATE"})
	waitControlSignal(t, sibling)
}

// ACTIVE_CLOCK duration advances between public reads. Check each sample's
// contract independently, then compare all remaining fields exactly.
func assertRuntimeObservationParity(t *testing.T, actual, expected any) {
	t.Helper()
	normalizeActiveDuration(t, actual)
	normalizeActiveDuration(t, expected)
	assertJSONEqual(t, actual, expected)
}

func normalizeActiveDuration(t *testing.T, value any) {
	t.Helper()
	switch value := value.(type) {
	case map[string]any:
		if value["durationBasis"] == "ACTIVE_CLOCK" {
			duration, ok := value["durationMillis"].(float64)
			if !ok || duration < 0 {
				t.Fatalf("invalid active duration: %v", value)
			}
			delete(value, "durationMillis")
		}
		for _, child := range value {
			normalizeActiveDuration(t, child)
		}
	case []any:
		for _, child := range value {
			normalizeActiveDuration(t, child)
		}
	}
}

func admitControlWorker(t *testing.T, ctx context.Context, host, id string, runner controlHostRunner) <-chan struct{} {
	t.Helper()
	payload := map[string]any{
		"requestId": "request-" + id, "workerSessionId": id,
		"execution": map[string]any{"workstationName": "process", "workerType": "processor", "dispatch": map[string]any{"dispatchId": "attempt-" + id, "workstationName": "process", "workerType": "processor"}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, host+"/worker-sessions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("start status %d: %s", response.StatusCode, body)
	}
	select {
	case done := <-runner.started:
		return done
	case <-ctx.Done():
		t.Fatal("provider boundary was not reached")
	}
	return nil
}

func waitControlSignal(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("owned provider execution did not stop")
	}
}

func assertOwnedControl(t *testing.T, result map[string]any, id, operation, outcome string) {
	t.Helper()
	if result["workerSessionId"] != id || result["dispatchId"] != "attempt-"+id || result["action"] != operation || result["outcome"] != outcome {
		t.Fatalf("exact target control result: %v", result)
	}
}

func assertCLIControlParity(t *testing.T, host *support.FunctionalAPIServer, expected any, id, operation string) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "worker-sessions", operation, id, "--server", host.URL(), "--json"})
	if err := host.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI control: %v, stderr=%s", err, inputs.Stderr())
	}
	var actual any
	if err := json.Unmarshal([]byte(inputs.Stdout()), &actual); err != nil {
		t.Fatalf("CLI JSON: %v, output=%s", err, inputs.Stdout())
	}
	assertJSONEqual(t, actual, expected)
}
