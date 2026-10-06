package acceptance

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// F6-01 crosses each public Start boundary with a schema whose integer
// requirement distinguishes validated structured output from raw JSON text.
func TestT7InvokeOutputSchemaParity(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.worker-sessions.invoke", "rest/startWorkerSession", "rest/readWorkerSessionLogs")
	fixture := ensureInvokeContinuePackageFixture(t)
	for _, mode := range []string{"local", "remote", "http"} {
		for _, outcome := range []string{"valid", "invalid"} {
			t.Run(mode+"-"+outcome, func(t *testing.T) {
				t.Parallel()
				t7AssertOutputSchema(t, fixture, mode, outcome)
			})
		}
	}
}

func t7AssertOutputSchema(t *testing.T, fixture *invokeContinuePackageFixture, mode, outcome string) {
	t.Helper()
	scenario := fixture.scenario(t, "t7-schema-"+mode+"-"+outcome)
	defer scenario.close(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	id := scenarioScopedID(scenario, "schema-session")
	document := invokeContinueExecutionDocument(invokeContinueExecutionSpec{
		requestID: id + "-request", workerSessionID: id, dispatchID: id + "-attempt",
		workingDirectory: scenario.workingDirectory, userMessage: "synthetic structured output",
	})
	document["execution"].(map[string]any)["outputSchema"] = `{"type":"object","properties":{"answer":{"type":"string"},"count":{"type":"integer"}},"required":["answer","count"],"additionalProperties":false}`
	document["execution"].(map[string]any)["dispatch"].(map[string]any)["execution"] = map[string]any{"requestId": id + "-request"}
	document["retry"] = map[string]any{"maxAttempts": 1}
	path := filepath.Join(scenario.workingDirectory, "schema.json")
	writeInvokeContinueJSON(t, path, document)
	if mode == "http" {
		status, body := t7HTTP(t, ctx, http.MethodPost, fixture.baseURL+"/worker-sessions", document)
		if status != http.StatusAccepted || !strings.Contains(body, id) {
			t.Fatalf("HTTP schema admission = %d %s", status, body)
		}
	}
	args := []string{"you", "--json"}
	if mode != "local" {
		args = append(args, "--remote", "--server", fixture.baseURL)
	}
	args = append(args, "worker-sessions", "invoke", "--execution", path)
	inputs := support.FakeInputs(ctx, args)
	inputs.Input.Env = scenario.environment()
	inputs.Input.WorkingDirectory = scenario.workingDirectory
	err := fixture.process.Execute(inputs.Input)
	if outcome == "invalid" {
		if err == nil {
			t.Fatalf("schema violation succeeded: %s", inputs.Stdout())
		}
		assertDirectWorkerSessionCLIError(t, inputs, "WORKER_SESSION_FAILED")
	} else {
		if err != nil {
			t.Fatalf("schema invoke: %v\n%s", err, inputs.Stderr())
		}
		var result struct {
			directWorkerSessionCLIResult
			StructuredResult map[string]any `json:"structuredResult"`
		}
		decodeDirectWorkerSessionResult(t, inputs.Stdout(), &result)
		if !result.Accepted || result.WorkerSessionID != id || result.State != "COMPLETED" ||
			result.StructuredResult["answer"] != "schema validated answer" || result.StructuredResult["count"] != float64(7) {
			t.Fatalf("validated result = %s", inputs.Stdout())
		}
	}
	t7AssertSchemaCapture(t, ctx, fixture.baseURL, id, outcome)
	requests := scenario.providerRunner.Requests()
	if len(requests) == 0 || string(requests[0].Stdin) != "synthetic structured output" {
		t.Fatalf("schema command inputs = %#v", requests)
	}
}

func t7AssertSchemaCapture(t *testing.T, ctx context.Context, baseURL, id, outcome string) {
	t.Helper()
	status, body := t7HTTP(t, ctx, http.MethodGet, baseURL+"/worker-sessions/"+id+"/logs", nil)
	if status != http.StatusOK || !strings.Contains(body, `"health":"COMPLETE"`) {
		t.Fatalf("schema capture = %d %s", status, body)
	}
	var logs struct {
		Events []struct {
			Event struct {
				Position int64           `json:"position"`
				Payload  json.RawMessage `json:"payload"`
			} `json:"event"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(body), &logs); err != nil || len(logs.Events) == 0 {
		t.Fatalf("decode schema capture: %v: %s", err, body)
	}
	var previous int64
	for _, entry := range logs.Events {
		if entry.Event.Position <= previous {
			t.Fatalf("schema capture order: %s", body)
		}
		previous = entry.Event.Position
	}
	terminal := string(logs.Events[len(logs.Events)-1].Event.Payload)
	if outcome == "valid" {
		if !strings.Contains(body, `"structuredOutput":{"answer":"schema validated answer","count":7}`) || !strings.Contains(terminal, `"status":"COMPLETED"`) {
			t.Fatalf("validated structured capture: %s", body)
		}
	} else if !strings.Contains(terminal, `"status":"FAILED"`) || !strings.Contains(body, "structured_output_schema_violation") {
		t.Fatalf("schema failure capture: %s", body)
	}
}
