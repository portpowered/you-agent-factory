package events

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const phaseCheckpointWorkflow = `phase("plan");
workflow.checkpoint({ label: "plan-ready", state: { ready: true } });
phase("execute");
return args.prompt;`

func TestJavaScriptInvocationEmitsCanonicalPhaseAndCheckpointEvents(t *testing.T) {
	stdout := runJavaScriptResponseStream(t)
	records := decodeNDJSONRecords(t, stdout)

	var lifecycle []factorydefinitions.FactoryEvent
	previousSessionSequence := -1
	for index, record := range records {
		if record.RecordType != factoryEventRecordType {
			continue
		}
		var event factorydefinitions.FactoryEvent
		if err := json.Unmarshal(record.Payload, &event); err != nil {
			t.Fatalf("decode Factory Event record %d: %v", index, err)
		}
		if event.Context.SessionSequence == nil {
			continue
		}
		if *event.Context.SessionSequence <= previousSessionSequence {
			t.Fatalf("Factory Session sequence %d follows %d", *event.Context.SessionSequence, previousSessionSequence)
		}
		previousSessionSequence = *event.Context.SessionSequence
		if event.Type == factorydefinitions.FactoryEventTypeOrchestratorPhaseChanged ||
			event.Type == factorydefinitions.FactoryEventTypeOrchestratorCheckpointWritten {
			lifecycle = append(lifecycle, event)
		}
	}

	assertJavaScriptLifecycle(t, lifecycle)
}

// TestJavaScriptInvocationWithServerJoinsListenerAfterTerminalResult proves hosted JavaScript cleanup at the CLI boundary.
func TestJavaScriptInvocationWithServerJoinsListenerAfterTerminalResult(t *testing.T) {
	var starts, stops, browsers atomic.Int32
	stdout := runJavaScriptResponseStreamWithOptions(
		t,
		serviceedges.Edges{
			APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
				starts.Add(1)
				request.OnBound(platformhttpserver.Binding{Port: request.Port})
				<-ctx.Done()
				stops.Add(1)
				return ctx.Err()
			},
			BrowserOpener: func(context.Context, string) error {
				browsers.Add(1)
				return nil
			},
		},
		"--with-server",
	)
	if stdout == "" {
		t.Fatal("JavaScript run omitted its canonical terminal response")
	}
	if starts.Load() != 1 || stops.Load() != 1 || browsers.Load() != 0 {
		t.Fatalf(
			"JavaScript server lifecycle = starts:%d stops:%d browsers:%d",
			starts.Load(),
			stops.Load(),
			browsers.Load(),
		)
	}
}

func assertJavaScriptLifecycle(t *testing.T, events []factorydefinitions.FactoryEvent) {
	t.Helper()
	want := []struct {
		eventType factorydefinitions.FactoryEventType
		phaseName string
		status    factorydefinitions.OrchestratorPhaseStatus
	}{
		{factorydefinitions.FactoryEventTypeOrchestratorPhaseChanged, "plan", factorydefinitions.OrchestratorPhaseStatusActive},
		{factorydefinitions.FactoryEventTypeOrchestratorCheckpointWritten, "plan", ""},
		{factorydefinitions.FactoryEventTypeOrchestratorPhaseChanged, "plan", factorydefinitions.OrchestratorPhaseStatusCompleted},
		{factorydefinitions.FactoryEventTypeOrchestratorPhaseChanged, "execute", factorydefinitions.OrchestratorPhaseStatusActive},
		{factorydefinitions.FactoryEventTypeOrchestratorPhaseChanged, "execute", factorydefinitions.OrchestratorPhaseStatusCompleted},
	}
	if len(events) != len(want) {
		t.Fatalf("JavaScript lifecycle event count = %d, want %d: %#v", len(events), len(want), events)
	}
	for index, expected := range want {
		event := events[index]
		if event.Type != expected.eventType || event.Context.PhaseName == nil || *event.Context.PhaseName != expected.phaseName {
			t.Fatalf("JavaScript lifecycle event %d = %#v, want %s phase %q", index, event, expected.eventType, expected.phaseName)
		}
		switch event.Type {
		case factorydefinitions.FactoryEventTypeOrchestratorPhaseChanged:
			var payload factorydefinitions.OrchestratorPhaseChangedEventPayload
			if err := event.DecodePayload(&payload); err != nil {
				t.Fatalf("decode phase event %d payload: %v", index, err)
			}
			if payload.PhaseStatus != expected.status {
				t.Fatalf("phase event %d status = %q, want %q", index, payload.PhaseStatus, expected.status)
			}
		case factorydefinitions.FactoryEventTypeOrchestratorCheckpointWritten:
			var payload factorydefinitions.OrchestratorCheckpointWrittenEventPayload
			if err := event.DecodePayload(&payload); err != nil {
				t.Fatalf("decode checkpoint event payload: %v", err)
			}
			if payload.Label != "plan-ready" || payload.ResumabilityStatus == "" || event.Context.CheckpointID == nil {
				t.Fatalf("checkpoint event = %#v payload = %#v, want public plan-ready resumable checkpoint", event, payload)
			}
		}
	}
}

func runJavaScriptResponseStream(t *testing.T) string {
	return runJavaScriptResponseStreamWithOptions(t, serviceedges.Edges{})
}

func runJavaScriptResponseStreamWithOptions(
	t *testing.T,
	edges serviceedges.Edges,
	extraArgs ...string,
) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "javascript-response-stream",
		"invocationSignature": map[string]any{
			"parameters": []any{map[string]any{
				"name": "prompt", "required": true,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			}},
		},
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"sourceRef": "workflow.js",
				"argsSchema": map[string]any{
					"type": "object", "required": []any{"prompt"},
					"properties":           map[string]any{"prompt": map[string]any{"type": "string"}},
					"additionalProperties": false,
				},
			},
		},
	})
	if err := os.WriteFile(filepath.Join(dir, "workflow.js"), []byte(phaseCheckpointWorkflow), 0o600); err != nil {
		t.Fatalf("write JavaScript workflow: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mock-workers.json"), []byte(`{"mockWorkers":[]}`), 0o600); err != nil {
		t.Fatalf("write mock-workers config: %v", err)
	}
	args := []string{
		"you", "--json", "run", "--factory", filepath.Join(dir, "factory.json"), "--output", "response-stream",
		"--no-record", "--with-mock-workers", filepath.Join(dir, "mock-workers.json"),
	}
	args = append(args, extraArgs...)
	args = append(args, "hello")
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.WorkingDirectory = dir
	home := t.TempDir()
	inputs.Input.Env = append(inputs.Input.Env, "HOME="+home, "USERPROFILE="+home)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	edges.ProviderCommandRunner = runner
	if err := support.BuildProcess(t, edges).Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stderr() != "" {
		t.Fatalf("stderr = %q, want empty successful-run stderr", inputs.Stderr())
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0", runner.CallCount())
	}
	return inputs.Stdout()
}
