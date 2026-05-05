package replay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
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

	result, err := ArtifactFromEventStream(strings.NewReader(stream))
	if err != nil {
		t.Fatalf("ArtifactFromEventStream: %v", err)
	}

	if result.ParsedEvents != 1 {
		t.Fatalf("ParsedEvents = %d, want 1", result.ParsedEvents)
	}
	if result.SkippedTrailingBlocks != 1 {
		t.Fatalf("SkippedTrailingBlocks = %d, want 1", result.SkippedTrailingBlocks)
	}
	if got := result.Artifact.Factory.Workers; got == nil || len(*got) != 1 {
		t.Fatalf("artifact factory workers = %#v, want hydrated factory config", got)
	}
}

func TestArtifactFromEventStream_NormalizesLegacyCronPayloads(t *testing.T) {
	recordedAt := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	artifact, err := NewEventLogArtifactFromFactory(recordedAt, factoryapi.Factory{
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
			Outputs: []factoryapi.WorkstationIO{{
				WorkType: "task",
				State:    "complete",
			}},
		}},
	}, nil, interfaces.ReplayDiagnostics{})
	if err != nil {
		t.Fatalf("NewEventLogArtifactFromFactory: %v", err)
	}

	stream := marshalReplayEventStream(t, artifact.Events...)
	result, err := ArtifactFromEventStream(strings.NewReader(stream))
	if err != nil {
		t.Fatalf("ArtifactFromEventStream: %v", err)
	}

	if result.ParsedEvents != 1 {
		t.Fatalf("ParsedEvents = %d, want 1", result.ParsedEvents)
	}
	if result.Artifact.Factory.Workstations == nil || len(*result.Artifact.Factory.Workstations) != 1 {
		t.Fatalf("artifact workstations = %#v, want one normalized cron workstation", result.Artifact.Factory.Workstations)
	}
	workstation := (*result.Artifact.Factory.Workstations)[0]
	if workstation.Cron == nil || workstation.Cron.Schedule != legacyEventStreamCronPlaceholderSchedule {
		t.Fatalf("normalized cron = %#v, want placeholder schedule %q", workstation.Cron, legacyEventStreamCronPlaceholderSchedule)
	}
	runStartedPayload, err := result.Artifact.Events[0].Payload.AsRunRequestEventPayload()
	if err != nil {
		t.Fatalf("AsRunRequestEventPayload: %v", err)
	}
	if runStartedPayload.Factory.Workstations == nil || len(*runStartedPayload.Factory.Workstations) != 1 {
		t.Fatalf("run-started payload workstations = %#v, want one normalized workstation", runStartedPayload.Factory.Workstations)
	}
	if got := (*runStartedPayload.Factory.Workstations)[0].Cron; got == nil || got.Schedule != legacyEventStreamCronPlaceholderSchedule {
		t.Fatalf("run-started normalized cron = %#v, want placeholder schedule %q", got, legacyEventStreamCronPlaceholderSchedule)
	}
}

func TestSaveArtifactFromEventStreamFile_HydratesAdjacentFactoryAndRewritesEmbeddedFactoryPayloads(t *testing.T) {
	factoryDir := t.TempDir()
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
		"workers": []map[string]any{{
			"name": "executor",
		}},
		"workstations": []map[string]any{{
			"name":    "execute-story",
			"worker":  "executor",
			"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
			"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
		}},
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

	recordedAt := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	recordedFactory := factoryapi.Factory{
		Name: "customer-project",
		Id:   stringPtrIfNotEmpty("customer-project"),
		WorkTypes: &[]factoryapi.WorkType{{
			Name: "story",
			States: []factoryapi.WorkState{
				{Name: "init", Type: factoryapi.WorkStateType(interfaces.StateTypeInitial)},
				{Name: "complete", Type: factoryapi.WorkStateType(interfaces.StateTypeTerminal)},
			},
		}},
		Workers: &[]factoryapi.Worker{{
			Name: "executor",
		}},
		Workstations: &[]factoryapi.Workstation{{
			Name:    "execute-story",
			Worker:  "executor",
			Inputs:  []factoryapi.WorkstationIO{{WorkType: "story", State: "init"}},
			Outputs: []factoryapi.WorkstationIO{{WorkType: "story", State: "complete"}},
		}},
	}
	runStarted, err := runStartedEventFromFactory(recordedAt, recordedFactory, nil, interfaces.ReplayDiagnostics{})
	if err != nil {
		t.Fatalf("runStartedEventFromFactory: %v", err)
	}
	initialStructure := replayInitialStructureEvent(t, recordedFactory, recordedAt)
	events := []factoryapi.FactoryEvent{runStarted, initialStructure}
	assignEventSequences(events)

	eventStreamPath := filepath.Join(factoryDir, "runs", "runtime.events")
	if err := os.MkdirAll(filepath.Dir(eventStreamPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(eventStreamPath), err)
	}
	if err := os.WriteFile(eventStreamPath, []byte(marshalReplayEventStream(t, events...)), 0o644); err != nil {
		t.Fatalf("write event stream: %v", err)
	}

	artifactPath := filepath.Join(factoryDir, "runs", "runtime.replay.json")
	result, err := saveArtifactFromEventStreamFile(eventStreamPath, artifactPath)
	if err != nil {
		t.Fatalf("saveArtifactFromEventStreamFile: %v", err)
	}
	if result.ParsedEvents != 2 {
		t.Fatalf("ParsedEvents = %d, want 2", result.ParsedEvents)
	}

	loaded, err := Load(artifactPath)
	if err != nil {
		t.Fatalf("Load(%s): %v", artifactPath, err)
	}
	assertReplayHydratedFactoryRuntime(t, loaded.Factory)

	runStartedPayload, err := loaded.Events[0].Payload.AsRunRequestEventPayload()
	if err != nil {
		t.Fatalf("AsRunRequestEventPayload: %v", err)
	}
	assertReplayHydratedFactoryRuntime(t, runStartedPayload.Factory)

	initialPayload, err := loaded.Events[1].Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		t.Fatalf("AsInitialStructureRequestEventPayload: %v", err)
	}
	assertReplayHydratedFactoryRuntime(t, initialPayload.Factory)
}

func TestArtifactFromEventStream_MissingRequiredFieldsReturnsExplicitError(t *testing.T) {
	stream := `data: {"id":"factory-event/run-started","schemaVersion":"AGENT_FACTORY_EVENT_V1"}

`

	_, err := ArtifactFromEventStream(strings.NewReader(stream))
	if err == nil {
		t.Fatal("ArtifactFromEventStream() error = nil, want missing required replay event fields")
	}
	if !strings.Contains(err.Error(), "decode event stream block 1: required replay event fields missing") {
		t.Fatalf("ArtifactFromEventStream() error = %q, want explicit missing-field message", err)
	}
}

func saveArtifactFromEventStreamFile(eventStreamPath string, artifactPath string) (*EventStreamArtifactResult, error) {
	result, err := artifactFromEventStreamFile(eventStreamPath)
	if err != nil {
		return nil, err
	}
	if err := Save(artifactPath, result.Artifact); err != nil {
		return nil, fmt.Errorf("save replay artifact from event stream %q: %w", eventStreamPath, err)
	}
	return result, nil
}

func artifactFromEventStreamFile(eventStreamPath string) (*EventStreamArtifactResult, error) {
	file, err := os.Open(eventStreamPath)
	if err != nil {
		return nil, fmt.Errorf("open event stream %q: %w", eventStreamPath, err)
	}
	defer file.Close()

	result, err := ArtifactFromEventStream(file)
	if err != nil {
		return nil, fmt.Errorf("parse event stream %q: %w", eventStreamPath, err)
	}
	if err := hydrateArtifactFromAdjacentFactoryForTest(eventStreamPath, result.Artifact); err != nil {
		return nil, fmt.Errorf("hydrate replay artifact from adjacent factory for %q: %w", eventStreamPath, err)
	}
	return result, nil
}

func hydrateArtifactFromAdjacentFactoryForTest(eventStreamPath string, artifact *interfaces.ReplayArtifact) error {
	if artifact == nil {
		return nil
	}
	factoryDir, ok := adjacentFactoryDirForTest(eventStreamPath)
	if !ok {
		return nil
	}
	loaded, err := config.LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		return nil
	}
	generated, err := GeneratedFactoryFromRuntimeConfig(
		loaded.FactoryDir(),
		loaded.FactoryConfig(),
		loaded,
		WithGeneratedFactorySourceDirectory(loaded.FactoryDir()),
	)
	if err != nil {
		return nil
	}
	merged := mergeGeneratedFactoryMissingRuntimeFieldsForTest(artifact.Factory, generated)
	if err := rewriteArtifactFactoryEventsForTest(artifact, merged); err != nil {
		return err
	}
	artifact.Factory = merged
	return nil
}

func adjacentFactoryDirForTest(eventStreamPath string) (string, bool) {
	candidates := []string{
		filepath.Dir(eventStreamPath),
		filepath.Dir(filepath.Dir(eventStreamPath)),
	}
	for _, dir := range candidates {
		if dir == "" || dir == "." {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, interfaces.FactoryConfigFile)); err == nil {
			return dir, true
		}
	}
	return "", false
}

func mergeGeneratedFactoryMissingRuntimeFieldsForTest(recorded factoryapi.Factory, authored factoryapi.Factory) factoryapi.Factory {
	merged := recorded
	if merged.FactoryDirectory == nil {
		merged.FactoryDirectory = authored.FactoryDirectory
	}
	if merged.SourceDirectory == nil {
		merged.SourceDirectory = authored.SourceDirectory
	}
	if merged.Id == nil {
		merged.Id = authored.Id
	}
	if merged.Metadata == nil || len(*merged.Metadata) == 0 {
		merged.Metadata = authored.Metadata
	}
	if merged.InputTypes == nil || len(*merged.InputTypes) == 0 {
		merged.InputTypes = authored.InputTypes
	}
	if merged.Workers != nil && authored.Workers != nil {
		authoredByName := make(map[string]factoryapi.Worker, len(*authored.Workers))
		for _, worker := range *authored.Workers {
			authoredByName[worker.Name] = worker
		}
		for i := range *merged.Workers {
			worker := &(*merged.Workers)[i]
			authoredWorker, ok := authoredByName[worker.Name]
			if !ok {
				continue
			}
			if worker.Type == nil {
				worker.Type = authoredWorker.Type
			}
			if worker.Command == nil {
				worker.Command = authoredWorker.Command
			}
			if worker.Args == nil || len(*worker.Args) == 0 {
				worker.Args = authoredWorker.Args
			}
			if worker.ModelProvider == nil {
				worker.ModelProvider = authoredWorker.ModelProvider
			}
			if worker.ExecutorProvider == nil {
				worker.ExecutorProvider = authoredWorker.ExecutorProvider
			}
			if worker.Timeout == nil {
				worker.Timeout = authoredWorker.Timeout
			}
			if worker.StopToken == nil {
				worker.StopToken = authoredWorker.StopToken
			}
			if worker.SkipPermissions == nil {
				worker.SkipPermissions = authoredWorker.SkipPermissions
			}
			if worker.Body == nil {
				worker.Body = authoredWorker.Body
			}
			if worker.Resources == nil || len(*worker.Resources) == 0 {
				worker.Resources = authoredWorker.Resources
			}
		}
	}
	if merged.Workstations != nil && authored.Workstations != nil {
		authoredByName := make(map[string]factoryapi.Workstation, len(*authored.Workstations))
		for _, workstation := range *authored.Workstations {
			authoredByName[workstation.Name] = workstation
		}
		for i := range *merged.Workstations {
			workstation := &(*merged.Workstations)[i]
			authoredWorkstation, ok := authoredByName[workstation.Name]
			if !ok {
				continue
			}
			if workstation.Id == nil {
				workstation.Id = authoredWorkstation.Id
			}
			if workstation.Behavior == nil {
				workstation.Behavior = authoredWorkstation.Behavior
			}
			if workstation.Type == nil {
				workstation.Type = authoredWorkstation.Type
			}
			if workstation.Worker == "" {
				workstation.Worker = authoredWorkstation.Worker
			}
			if len(workstation.Inputs) == 0 {
				workstation.Inputs = authoredWorkstation.Inputs
			}
			if len(workstation.Outputs) == 0 {
				workstation.Outputs = authoredWorkstation.Outputs
			}
			if workstation.OnFailure == nil {
				workstation.OnFailure = authoredWorkstation.OnFailure
			}
			if workstation.OnContinue == nil {
				workstation.OnContinue = authoredWorkstation.OnContinue
			}
			if workstation.OnRejection == nil {
				workstation.OnRejection = authoredWorkstation.OnRejection
			}
			if workstation.Resources == nil || len(*workstation.Resources) == 0 {
				workstation.Resources = authoredWorkstation.Resources
			}
			if workstation.Cron == nil {
				workstation.Cron = authoredWorkstation.Cron
			}
			if workstation.Guards == nil || len(*workstation.Guards) == 0 {
				workstation.Guards = authoredWorkstation.Guards
			}
			if workstation.Limits == nil {
				workstation.Limits = authoredWorkstation.Limits
			}
			if workstation.Worktree == nil {
				workstation.Worktree = authoredWorkstation.Worktree
			}
			if workstation.WorkingDirectory == nil {
				workstation.WorkingDirectory = authoredWorkstation.WorkingDirectory
			}
			if workstation.PromptFile == nil {
				workstation.PromptFile = authoredWorkstation.PromptFile
			}
			if workstation.Body == nil {
				workstation.Body = authoredWorkstation.Body
			}
			if workstation.StopWords == nil || len(*workstation.StopWords) == 0 {
				workstation.StopWords = authoredWorkstation.StopWords
			}
		}
	}
	return merged
}

func rewriteArtifactFactoryEventsForTest(artifact *interfaces.ReplayArtifact, factory factoryapi.Factory) error {
	if artifact == nil {
		return nil
	}
	for index := range artifact.Events {
		event := &artifact.Events[index]
		switch event.Type {
		case factoryapi.FactoryEventTypeRunRequest:
			payload, err := runStartedPayloadFromEvent(*event)
			if err != nil {
				return err
			}
			payload.Factory = factory
			var union factoryapi.FactoryEvent_Payload
			if err := union.FromRunRequestEventPayload(payload); err != nil {
				return fmt.Errorf("rewrite run request factory payload: %w", err)
			}
			event.Payload = union
		case factoryapi.FactoryEventTypeInitialStructureRequest:
			payload, err := event.Payload.AsInitialStructureRequestEventPayload()
			if err != nil {
				return fmt.Errorf("decode initial structure event %q: %w", event.Id, err)
			}
			payload.Factory = factory
			var union factoryapi.FactoryEvent_Payload
			if err := union.FromInitialStructureRequestEventPayload(payload); err != nil {
				return fmt.Errorf("rewrite initial structure factory payload: %w", err)
			}
			event.Payload = union
		}
	}
	return nil
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

func marshalReplayEventStream(t *testing.T, events ...factoryapi.FactoryEvent) string {
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
	if workstation.Type == nil || *workstation.Type != factoryapi.WorkstationTypeModelWorkstation {
		t.Fatalf("workstation type = %#v, want MODEL_WORKSTATION", workstation.Type)
	}
}
