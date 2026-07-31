package replay

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestArtifactFromEventStream_ParsesCanonicalEventStreamAndSkipsTruncatedTail(t *testing.T) {
	artifact := testReplayArtifact(t,
		replayWorkRequestEvent(t, "request-1", 1, "api", []factoryapi.Work{{
			Name:         "task-1",
			TraceId:      stringPtrIfNotEmpty("trace-1"),
			WorkId:       stringPtrIfNotEmpty("work-1"),
			WorkTypeName: stringPtrIfNotEmpty("task"),
		}}, nil),
	)

	data, err := json.Marshal(artifact.Events[0])
	if err != nil {
		t.Fatalf("Marshal event: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	stream := "data: " + strings.Join(lines, "\n") + "\n\n" +
		`data: {"id":"truncated"` + "\n"

	result, err := ArtifactFromEventStream(strings.NewReader(stream), testFactorySnapshotDecoder)
	if err != nil {
		t.Fatalf("ArtifactFromEventStream: %v", err)
	}

	if result.ParsedEvents != 1 {
		t.Fatalf("ParsedEvents = %d, want 1", result.ParsedEvents)
	}
	if result.SkippedTrailingBlocks != 1 {
		t.Fatalf("SkippedTrailingBlocks = %d, want 1", result.SkippedTrailingBlocks)
	}
	factory := decodeReplayFactorySnapshot(t, result.Artifact.Factory)
	if got := factory.Workers; got == nil || len(*got) != 1 {
		t.Fatalf("artifact factory workers = %#v, want hydrated factory config", got)
	}
}

func TestArtifactFromEventStreamFileRejectsMissingFileOperations(t *testing.T) {
	t.Parallel()
	result, err := ArtifactFromEventStreamFile("events.jsonl", testFactorySnapshotDecoder, nil, nil, nil)
	if result != nil || err == nil || !strings.Contains(err.Error(), "file operations are required") {
		t.Fatalf("ArtifactFromEventStreamFile = (%#v, %v), want missing operations", result, err)
	}
}

func TestArtifactFromEventStream_NormalizesLegacyCronPayloads(t *testing.T) {
	recordedAt := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	artifact, err := NewEventLogArtifact(recordedAt, mustFactorySnapshot(t, factoryapi.Factory{
		Name: "legacy-cron-factory",
		WorkTypes: &[]factoryapi.WorkType{{
			Name: "task",
			States: []factoryapi.WorkState{{
				Name: "complete",
				Type: factoryapi.WorkStateType(interfaces.StateTypeTerminal),
			}},
		}},
		Workers: &[]factoryapi.Worker{{
			Name: "executor",
		}},
		Workstations: &[]factoryapi.Workstation{{
			Name:     "daily-refresh",
			Behavior: stringPtrIfNotEmpty(factoryapi.WorkstationKindCron),
			Worker:   "executor",
			Outputs: &[]factoryapi.WorkstationIO{{
				WorkType: "task",
				State:    "complete",
			}},
		}},
	}), nil, interfaces.ReplayDiagnostics{})
	if err != nil {
		t.Fatalf("NewEventLogArtifact: %v", err)
	}

	stream := marshalReplayEventStream(t, artifact.Events...)
	result, err := ArtifactFromEventStream(strings.NewReader(stream), testFactorySnapshotDecoder)
	if err != nil {
		t.Fatalf("ArtifactFromEventStream: %v", err)
	}

	if result.ParsedEvents != 1 {
		t.Fatalf("ParsedEvents = %d, want 1", result.ParsedEvents)
	}
	factory := decodeReplayFactorySnapshot(t, result.Artifact.Factory)
	if factory.Workstations == nil || len(*factory.Workstations) != 1 {
		t.Fatalf("artifact workstations = %#v, want one normalized cron workstation", factory.Workstations)
	}
	workstation := (*factory.Workstations)[0]
	if workstation.Cron == nil || stringValue(workstation.Cron.Schedule) != legacyEventStreamCronPlaceholderSchedule {
		t.Fatalf("normalized cron = %#v, want placeholder schedule %q", workstation.Cron, legacyEventStreamCronPlaceholderSchedule)
	}
	generatedRunStarted := mustGeneratedReplayEvent(t, result.Artifact.Events[0])
	runStartedPayload, err := generatedRunStarted.Payload.AsRunRequestEventPayload()
	if err != nil {
		t.Fatalf("AsRunRequestEventPayload: %v", err)
	}
	if runStartedPayload.Factory.Workstations == nil || len(*runStartedPayload.Factory.Workstations) != 1 {
		t.Fatalf("run-started payload workstations = %#v, want one normalized workstation", runStartedPayload.Factory.Workstations)
	}
	if got := (*runStartedPayload.Factory.Workstations)[0].Cron; got == nil || stringValue(got.Schedule) != legacyEventStreamCronPlaceholderSchedule {
		t.Fatalf("run-started normalized cron = %#v, want placeholder schedule %q", got, legacyEventStreamCronPlaceholderSchedule)
	}
}

func TestSaveArtifactFromEventStreamFile_HydratesAdjacentFactoryAndRewritesEmbeddedFactoryPayloads(t *testing.T) {
	factoryDir := t.TempDir()
	writeReplayArtifactHydrationFixture(t, factoryDir)
	recordedAt := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	eventStreamPath := writeReplayEventStreamFixture(t, factoryDir, recordedAt)

	artifactPath := filepath.Join(factoryDir, "runs", "runtime.replay.json")
	loadAdjacentFactory := scriptedHydratedFactorySnapshotDirectoryLoader(t, factoryDir)
	result, err := SaveArtifactFromEventStreamFile(
		testReplayStorage(),
		eventStreamPath,
		artifactPath,
		testFactorySnapshotDecoder,
		loadAdjacentFactory,
		func(path string) (io.ReadCloser, error) { return os.Open(path) },
		os.Stat,
	)
	if err != nil {
		t.Fatalf("SaveArtifactFromEventStreamFile: %v", err)
	}
	if result.ParsedEvents != 2 {
		t.Fatalf("ParsedEvents = %d, want 2", result.ParsedEvents)
	}

	loaded, err := Load(testReplayStorage(), artifactPath, testFactorySnapshotDecoder)
	if err != nil {
		t.Fatalf("Load(%s): %v", artifactPath, err)
	}
	assertReplayHydratedFactoryRuntime(t, decodeReplayFactorySnapshot(t, loaded.Factory))

	generatedRunStarted := mustGeneratedReplayEvent(t, loaded.Events[0])
	runStartedPayload, err := generatedRunStarted.Payload.AsRunRequestEventPayload()
	if err != nil {
		t.Fatalf("AsRunRequestEventPayload: %v", err)
	}
	assertReplayHydratedFactoryRuntime(t, runStartedPayload.Factory)

	generatedInitial := mustGeneratedReplayEvent(t, loaded.Events[1])
	initialPayload, err := generatedInitial.Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		t.Fatalf("AsInitialStructureRequestEventPayload: %v", err)
	}
	assertReplayHydratedFactoryRuntime(t, initialPayload.Factory)
}

func scriptedHydratedFactorySnapshotDirectoryLoader(
	t *testing.T,
	wantFactoryDir string,
) interfaces.FactorySnapshotDirectoryLoader {
	t.Helper()
	return func(factoryDir string) (*interfaces.FactorySnapshot, error) {
		if factoryDir != wantFactoryDir {
			t.Fatalf("adjacent Factory directory = %q, want %q", factoryDir, wantFactoryDir)
		}
		return interfaces.NewFactorySnapshot(map[string]any{
			"name": "customer-project",
			"id":   "customer-project",
			"workTypes": []map[string]any{{
				"name": "story",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
				},
			}},
			"workers": []map[string]any{{
				"name":    "executor",
				"type":    "SCRIPT_WORKER",
				"command": "go",
			}},
			"workstations": []map[string]any{{
				"name":   "execute-story",
				"worker": "executor",
				"type":   "SCRIPT_RUN",
				"body":   "Implement {{ .WorkID }}.",
				"inputs": []map[string]string{{
					"workType": "story",
					"state":    "init",
				}},
				"outputs": []map[string]string{{
					"workType": "story",
					"state":    "complete",
				}},
			}},
		})
	}
}

func TestMergeFactorySnapshotsMissingRuntimeFields_PreservesUnknownRecordedFields(t *testing.T) {
	recorded, err := interfaces.NewFactorySnapshot(map[string]any{
		"name":          "customer-project",
		"futureFactory": map[string]any{"enabled": true},
		"workers": []map[string]any{{
			"name":         "executor",
			"futureWorker": "retained",
		}},
		"workstations": []map[string]any{{
			"name":              "execute-story",
			"worker":            "",
			"futureWorkstation": []string{"retained"},
		}},
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot(recorded): %v", err)
	}
	authored, err := interfaces.NewFactorySnapshot(map[string]any{
		"name":             "customer-project",
		"factoryDirectory": "/factory",
		"workers": []map[string]any{{
			"name":    "executor",
			"type":    "SCRIPT_WORKER",
			"command": "go",
		}},
		"workstations": []map[string]any{{
			"name":       "execute-story",
			"worker":     "executor",
			"promptFile": "prompt.md",
		}},
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot(authored): %v", err)
	}

	merged, err := mergeFactorySnapshotsMissingRuntimeFields(recorded, authored)
	if err != nil {
		t.Fatalf("mergeFactorySnapshotsMissingRuntimeFields: %v", err)
	}
	var got map[string]any
	if err := merged.Decode(&got); err != nil {
		t.Fatalf("Decode merged snapshot: %v", err)
	}
	assertMergedFactorySnapshotFields(t, got)
}

func assertMergedFactorySnapshotFields(t *testing.T, got map[string]any) {
	t.Helper()
	if got["factoryDirectory"] != "/factory" {
		t.Fatalf("factoryDirectory = %#v, want /factory", got["factoryDirectory"])
	}
	if future, ok := got["futureFactory"].(map[string]any); !ok || future["enabled"] != true {
		t.Fatalf("futureFactory = %#v, want preserved object", got["futureFactory"])
	}
	workers := got["workers"].([]any)
	worker := workers[0].(map[string]any)
	if worker["type"] != "SCRIPT_WORKER" || worker["command"] != "go" || worker["futureWorker"] != "retained" {
		t.Fatalf("merged worker = %#v", worker)
	}
	workstations := got["workstations"].([]any)
	workstation := workstations[0].(map[string]any)
	if workstation["worker"] != "executor" || workstation["promptFile"] != "prompt.md" {
		t.Fatalf("merged workstation = %#v", workstation)
	}
	if future, ok := workstation["futureWorkstation"].([]any); !ok || len(future) != 1 || future[0] != "retained" {
		t.Fatalf("futureWorkstation = %#v, want preserved collection", workstation["futureWorkstation"])
	}
}

func decodeReplayFactorySnapshot(t *testing.T, snapshot *interfaces.FactorySnapshot) factoryapi.Factory {
	t.Helper()
	var factory factoryapi.Factory
	if err := snapshot.Decode(&factory); err != nil {
		t.Fatalf("decode replay Factory snapshot: %v", err)
	}
	return factory
}

func writeReplayArtifactHydrationFixture(t *testing.T, factoryDir string) {
	t.Helper()

	writeReplayFactoryJSON(t, factoryDir, map[string]any{
		"name": "customer-project",
		"id":   "customer-project",
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"workers":      []map[string]any{{"name": "executor"}},
		"workstations": []map[string]any{{"name": "execute-story", "worker": "executor", "inputs": []map[string]string{{"workType": "story", "state": "init"}}, "outputs": []map[string]string{{"workType": "story", "state": "complete"}}}},
	})
	writeReplayAgentsMD(t, filepath.Join(factoryDir, "workers", "executor"), `---
type: SCRIPT_WORKER
command: go
args: ["test", "./..."]
timeout: 30s
---
Run the test suite.
`)
	writeReplayAgentsMD(t, filepath.Join(factoryDir, "workstations", "execute-story"), `---
type: MODEL_WORKSTATION
worker: executor
promptFile: prompt.md
---
Fallback body.
`)
	if err := os.WriteFile(filepath.Join(factoryDir, "workstations", "execute-story", "prompt.md"), []byte("Implement {{ .WorkID }}."), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}
}

func writeReplayEventStreamFixture(t *testing.T, factoryDir string, recordedAt time.Time) string {
	t.Helper()

	recordedFactory := replayRecordedFactoryFixture()
	runStarted, err := runStartedEventFromSnapshot(recordedAt, mustFactorySnapshot(t, recordedFactory), nil, interfaces.ReplayDiagnostics{})
	if err != nil {
		t.Fatalf("runStartedEventFromSnapshot: %v", err)
	}
	initial, err := interfaces.NewFactoryEvent(replayInitialStructureEvent(t, recordedFactory, recordedAt))
	if err != nil {
		t.Fatalf("convert initial structure event: %v", err)
	}
	events := []interfaces.FactoryEvent{runStarted, initial}
	assignEventSequences(events)

	eventStreamPath := filepath.Join(factoryDir, "runs", "runtime.events")
	if err := os.MkdirAll(filepath.Dir(eventStreamPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(eventStreamPath), err)
	}
	if err := os.WriteFile(eventStreamPath, []byte(marshalReplayEventStream(t, events...)), 0o644); err != nil {
		t.Fatalf("write event stream: %v", err)
	}
	return eventStreamPath
}

func replayRecordedFactoryFixture() factoryapi.Factory {
	return factoryapi.Factory{
		Name: "customer-project",
		Id:   stringPtrIfNotEmpty("customer-project"),
		WorkTypes: &[]factoryapi.WorkType{{
			Name: "story",
			States: []factoryapi.WorkState{
				{Name: "init", Type: factoryapi.WorkStateType(interfaces.StateTypeInitial)},
				{Name: "complete", Type: factoryapi.WorkStateType(interfaces.StateTypeTerminal)},
			},
		}},
		Workers: &[]factoryapi.Worker{{Name: "executor"}},
		Workstations: &[]factoryapi.Workstation{{
			Name: "execute-story", Worker: "executor", Inputs: []factoryapi.WorkstationIO{{WorkType: "story", State: "init"}}, Outputs: &[]factoryapi.WorkstationIO{{WorkType: "story", State: "complete"}},
		}},
	}
}

func TestArtifactFromEventStream_MissingRequiredFieldsReturnsExplicitError(t *testing.T) {
	stream := `data: {"id":"factory-event/run-started","schemaVersion":"AGENT_FACTORY_EVENT_V1"}

`

	_, err := ArtifactFromEventStream(strings.NewReader(stream), testFactorySnapshotDecoder)
	if err == nil {
		t.Fatal("ArtifactFromEventStream() error = nil, want missing required replay event fields")
	}
	if !strings.Contains(err.Error(), "decode event stream block 1: required replay event fields missing") {
		t.Fatalf("ArtifactFromEventStream() error = %q, want explicit missing-field message", err)
	}
}

func replayInitialStructureEvent(t *testing.T, factory factoryapi.Factory, recordedAt time.Time) factoryapi.FactoryEvent {
	t.Helper()

	var union factoryapi.FactoryEvent_Payload
	if err := union.FromInitialStructureRequestEventPayload(factoryapi.InitialStructureRequestEventPayload{Factory: factory}); err != nil {
		t.Fatalf("encode initial structure event payload: %v", err)
	}
	return factoryapi.FactoryEvent{
		Id:            "factory-event/initial-structure/0",
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeInitialStructureRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime: recordedAt,
			Tick:      0,
		},
		Payload: union,
	}
}

func marshalReplayEventStream[T any](t *testing.T, events ...T) string {
	t.Helper()

	var builder strings.Builder
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("Marshal event stream payload: %v", err)
		}
		builder.WriteString("data: ")
		builder.Write(data)
		builder.WriteString("\n\n")
	}
	return builder.String()
}

func writeReplayFactoryJSON(t *testing.T, factoryDir string, cfg map[string]any) {
	t.Helper()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent(factory.json): %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func writeReplayAgentsMD(t *testing.T, dir, content string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
}

func assertReplayHydratedFactoryRuntime(t *testing.T, factory factoryapi.Factory) {
	t.Helper()

	if factory.Workers == nil || len(*factory.Workers) != 1 {
		t.Fatalf("factory workers = %#v, want one hydrated worker", factory.Workers)
	}
	worker := (*factory.Workers)[0]
	if worker.Command == nil || *worker.Command != "go" {
		t.Fatalf("worker command = %#v, want go", worker.Command)
	}
	if worker.Type == nil || *worker.Type != factoryapi.WorkerTypeScriptWorker {
		t.Fatalf("worker type = %#v, want SCRIPT_WORKER", worker.Type)
	}

	if factory.Workstations == nil || len(*factory.Workstations) != 1 {
		t.Fatalf("factory workstations = %#v, want one hydrated workstation", factory.Workstations)
	}
	workstation := (*factory.Workstations)[0]
	if workstation.Body == nil || *workstation.Body != "Implement {{ .WorkID }}." {
		t.Fatalf("workstation body = %#v, want prompt file content", workstation.Body)
	}
	if workstation.Type == nil || *workstation.Type != factoryapi.WorkstationTypeScriptRun {
		t.Fatalf("workstation type = %#v, want SCRIPT_RUN", workstation.Type)
	}
}
