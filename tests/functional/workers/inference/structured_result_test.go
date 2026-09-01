package inference_test

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestDetachedStructuredResultReachesDispatchResponse(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		schema string
		output string
		want   any
	}{
		{
			name:   "object",
			schema: `{"type":"object","properties":{"verdict":{"type":"string"}},"required":["verdict"]}`,
			output: `{"verdict":"pass"}`,
			want:   map[string]any{"verdict": "pass"},
		},
		{
			name:   "explicit_null",
			schema: `{"type":"null"}`,
			output: "null",
			want:   nil,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			fixture := runDetachedStructuredResult(t, test.schema, test.output)
			if fixture.payload.Outcome != workerexecution.OutcomeAccepted {
				t.Fatalf("dispatch response outcome = %q, want accepted", fixture.payload.Outcome)
			}
			if !fixture.payload.StructuredResultPresent {
				t.Fatal("dispatch response structured result present = false, want true")
			}
			if !reflect.DeepEqual(fixture.payload.StructuredResult, test.want) {
				t.Fatalf("dispatch response structured result = %#v, want %#v", fixture.payload.StructuredResult, test.want)
			}

			publicPayload, err := fixture.publicEvent.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode public dispatch response payload: %v", err)
			}
			if !reflect.DeepEqual(publicPayload.StructuredResult, test.want) {
				t.Fatalf("public dispatch response structured result = %#v, want %#v", publicPayload.StructuredResult, test.want)
			}
			if test.name == "explicit_null" {
				raw := marshalStructuredResultEventToRawObject(t, fixture.publicEvent)
				payload, ok := raw["payload"].(map[string]any)
				if !ok {
					t.Fatalf("public dispatch response payload = %#v, want object", raw["payload"])
				}
				value, present := payload["structuredResult"]
				if !present || value != nil {
					t.Fatalf("public dispatch response structuredResult = %#v, want explicit null", value)
				}
			}
		})
	}
}

type detachedStructuredResultFixture struct {
	payload     workerexecution.DispatchResponseEventPayload
	publicEvent factoryapi.FactoryEvent
}

func runDetachedStructuredResult(
	t *testing.T,
	schema string,
	output string,
) detachedStructuredResultFixture {
	t.Helper()

	newFactory := func() string {
		factoryDir := support.ScaffoldFactory(t, map[string]any{
			"workTypes": []map[string]any{{
				"name":             "task",
				"handlingBehavior": []string{"DEFAULT"},
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			}},
			"workers": []map[string]string{{"name": "worker-a"}},
			"workstations": []map[string]any{{
				"name":         "process",
				"worker":       "worker-a",
				"outputSchema": schema,
				"inputs":       []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":      []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure":    []map[string]string{{"workType": "task", "state": "failed"}},
			}},
		})
		support.WriteAgentConfig(t, factoryDir, "worker-a", detachedStructuredWorkerConfig())
		testutil.WriteSeedRequest(t, factoryDir, workSubmitRequest())
		return factoryDir
	}

	commandResult := platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout(output),
	}
	runner := testutil.NewProviderCommandRunner(commandResult, commandResult)
	recordPath := filepath.Join(t.TempDir(), "structured-result.replay.json")
	recordedDir := newFactory()
	recordedArtifact := runDetachedStructuredResultRecording(t, recordedDir, recordPath, runner)
	recordedPayload := recordedStructuredResultPayload(t, recordedArtifact)

	liveDir := newFactory()
	_, _, publicEvents := runSharedInferenceFactoryToCompletion(t, liveDir, sharedInferenceScenario{
		commandRunner: runner,
	}, sharedInferenceScenarioTimeout)
	publicEvent := findDispatchResponseEvent(t, publicEvents)
	return detachedStructuredResultFixture{payload: recordedPayload, publicEvent: publicEvent}
}

func runDetachedStructuredResultRecording(
	t *testing.T,
	dir string,
	recordPath string,
	runner platformprocess.CommandRunner,
) *interfaces.ReplayArtifact {
	t.Helper()
	withSharedInferenceProcessAt(t, dir, sharedInferenceScenario{
		commandRunner:        runner,
		scenarioName:         "structured-result-recording",
		stopDaemonForExecute: true,
	}, func(process support.ApplicationProcess) {
		inputs := support.FakeInputs(t.Context(), []string{
			"you", "run", "--dir", dir, "--quiet", "--record", recordPath,
		})
		inputs.Input.Env = sharedInferenceProcessEnvironment(sharedInferenceGroup.homeDir)
		inputs.Input.WorkingDirectory = dir
		if err := process.Execute(inputs.Input); err != nil {
			t.Fatalf("recorded structured-result Process.Execute: %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
		}
	})
	return testutil.LoadReplayArtifact(t, recordPath)
}

func recordedStructuredResultPayload(
	t *testing.T,
	artifact *interfaces.ReplayArtifact,
) workerexecution.DispatchResponseEventPayload {
	t.Helper()
	var payload workerexecution.DispatchResponseEventPayload
	count := 0
	for _, event := range artifact.Events {
		if event.Type != interfaces.FactoryEventTypeDispatchResponse {
			continue
		}
		count++
		if err := event.DecodePayload(&payload); err != nil {
			t.Fatalf("decode recorded dispatch response: %v", err)
		}
	}
	if count != 1 {
		t.Fatalf("recorded dispatch response count = %d, want one", count)
	}
	return payload
}

func findDispatchResponseEvent(t *testing.T, events []factoryapi.FactoryEvent) factoryapi.FactoryEvent {
	t.Helper()
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeDispatchResponse {
			return event
		}
	}
	t.Fatalf("events = %#v, want one DISPATCH_RESPONSE", events)
	return factoryapi.FactoryEvent{}
}

func marshalStructuredResultEventToRawObject(t *testing.T, event factoryapi.FactoryEvent) map[string]any {
	t.Helper()

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event %s: %v", event.Id, err)
	}
	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatalf("unmarshal event %s: %v", event.Id, err)
	}
	return raw
}

func workSubmitRequest() work.SubmitRequest {
	return work.SubmitRequest{
		WorkID:     "structured-result-work",
		WorkTypeID: "task",
		Payload:    []byte(`{"title":"structured result"}`),
	}
}

func detachedStructuredWorkerConfig() string {
	return "---\n" +
		"executorProvider: CODEX\n" +
		"type: MODEL_WORKER\n" +
		"model: gpt-5-codex\n" +
		"modelProvider: CODEX\n" +
		"---\n" +
		"Process the input task.\n"
}
