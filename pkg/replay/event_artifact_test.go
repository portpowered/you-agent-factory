package replay

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestJavaScriptRecording_RoundTripsCompletedAndFailedSessionFacts(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name         string
		factoryState interfaces.FactoryState
		failure      string
		wantStatus   factoryapi.FactorySessionDurableLifecycleStatus
	}{
		{name: "completed", factoryState: interfaces.FactoryStateCompleted, wantStatus: factoryapi.FactorySessionDurableLifecycleStatusSucceeded},
		{name: "failed", factoryState: interfaces.FactoryStateFailed, failure: "child execution failed", wantStatus: factoryapi.FactorySessionDurableLifecycleStatusFailed},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recordedAt := time.Date(2026, time.July, 12, 16, 0, 0, 0, time.UTC)
			config := javascriptRecordingFactoryConfig()
			history := factoryevents.NewFactoryEventHistory(nil, func() time.Time { return recordedAt })
			history.RecordSessionLifecycleFromFactoryConfig("session-recorded-js", config, 0, recordedAt)
			if testCase.factoryState == interfaces.FactoryStateCompleted {
				history.RecordOrchestratorCheckpointWritten(factoryevents.OrchestratorCheckpointWrittenInput{
					SessionID:             "session-recorded-js",
					OrchestratorKind:      factoryapi.JAVASCRIPT,
					OrchestratorDialect:   "workflow-v1",
					CheckpointID:          "checkpoint-1",
					Source:                "runtime",
					Tick:                  1,
					Label:                 "after-plan",
					SourceHash:            "sha256:source",
					RuntimeSnapshotDigest: "sha256:checkpoint-state",
					ArtifactRef: &factoryapi.FactoryArtifactRef{
						Id:         "artifact-checkpoint-1",
						Kind:       factoryapi.FactoryArtifactKindCHECKPOINT,
						Visibility: factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT,
					},
					ResumabilityStatus: factoryapi.RESUMABLE,
				}, recordedAt.Add(time.Second))
			}
			history.RecordSessionLifecycleCompletion("session-recorded-js", config, 2, testCase.factoryState, testCase.failure, recordedAt.Add(2*time.Second))

			artifact, err := NewEventLogArtifactFromFactory(recordedAt, javascriptRecordingFactory(), nil, interfaces.ReplayDiagnostics{})
			if err != nil {
				t.Fatalf("NewEventLogArtifactFromFactory: %v", err)
			}
			artifact.Events = append(artifact.Events, history.Events()...)
			path := filepath.Join(t.TempDir(), testCase.name+".replay.json")
			if err := Save(path, artifact); err != nil {
				t.Fatalf("Save: %v", err)
			}
			loaded, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			assertRecordedJavaScriptLifecycle(t, loaded.Events, testCase.wantStatus, testCase.failure)
			assertRecordingOmitsRawJavaScriptInternals(t, loaded)
		})
	}
}

func javascriptRecordingFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{Name: "recorded-javascript-factory", Orchestrator: &interfaces.FactoryOrchestratorConfig{
		Kind: interfaces.OrchestratorKindJavaScript,
		JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
			Dialect: "workflow-v1", SourceRef: "workflows/main.js", SourceHash: "sha256:source",
			ArgsSchema: json.RawMessage(`{"type":"object"}`), DefaultPolicy: json.RawMessage(`{"maxAgents":4}`),
		},
	}}
}

func javascriptRecordingFactory() factoryapi.Factory {
	kind, dialect, sourceRef := factoryapi.JAVASCRIPT, "workflow-v1", "workflows/main.js"
	return factoryapi.Factory{Name: "recorded-javascript-factory", WorkTypes: &[]factoryapi.WorkType{{Name: "task"}}, Workers: &[]factoryapi.Worker{{Name: "worker-a"}}, Workstations: &[]factoryapi.Workstation{{Name: "process", Worker: "worker-a", Inputs: []factoryapi.WorkstationIO{}, Outputs: &[]factoryapi.WorkstationIO{}}}, Orchestrator: &factoryapi.FactoryOrchestrator{Kind: kind, Javascript: &factoryapi.FactoryOrchestratorJavaScriptConfig{Dialect: &dialect, SourceRef: &sourceRef, SourceHash: stringPointer("sha256:source")}}}
}

func assertRecordedJavaScriptLifecycle(t *testing.T, recorded []factoryapi.FactoryEvent, wantStatus factoryapi.FactorySessionDurableLifecycleStatus, wantFailure string) {
	t.Helper()
	started, completed := recordedJavaScriptSessionEvents(t, recorded)
	assertRecordedJavaScriptStart(t, started)
	assertRecordedJavaScriptCompletion(t, completed, wantStatus, wantFailure)
}

func recordedJavaScriptSessionEvents(t *testing.T, recorded []factoryapi.FactoryEvent) (*factoryapi.SessionStartedEventPayload, *factoryapi.SessionCompletedEventPayload) {
	t.Helper()
	var started *factoryapi.SessionStartedEventPayload
	var completed *factoryapi.SessionCompletedEventPayload
	for _, event := range recorded {
		switch event.Type {
		case factoryapi.FactoryEventTypeSessionStarted:
			payload, err := event.Payload.AsSessionStartedEventPayload()
			if err != nil {
				t.Fatalf("decode session started: %v", err)
			}
			started = &payload
		case factoryapi.FactoryEventTypeSessionCompleted:
			payload, err := event.Payload.AsSessionCompletedEventPayload()
			if err != nil {
				t.Fatalf("decode session completed: %v", err)
			}
			completed = &payload
		}
	}
	return started, completed
}

func assertRecordedJavaScriptStart(t *testing.T, started *factoryapi.SessionStartedEventPayload) {
	t.Helper()
	if started == nil || started.SourceRef == nil || *started.SourceRef != "workflows/main.js" || started.SourceHash == nil || *started.SourceHash != "sha256:source" {
		t.Fatalf("session start identity = %#v, want source ref and digest", started)
	}
	if started.PolicyHash == nil || *started.PolicyHash == "" || started.ArgsDigest == nil || *started.ArgsDigest == "" {
		t.Fatalf("session start digests = %#v, want policy and argument digests", started)
	}
}

func assertRecordedJavaScriptCompletion(t *testing.T, completed *factoryapi.SessionCompletedEventPayload, wantStatus factoryapi.FactorySessionDurableLifecycleStatus, wantFailure string) {
	t.Helper()
	if completed == nil || completed.FinalStatus != wantStatus || completed.ResultStatus == nil {
		t.Fatalf("session completion = %#v, want status %s and result availability", completed, wantStatus)
	}
	if wantFailure != "" && (completed.FailureDetail == nil || completed.FailureDetail.Message != wantFailure) {
		t.Fatalf("failure detail = %#v, want %q", completed.FailureDetail, wantFailure)
	}
}

func assertRecordingOmitsRawJavaScriptInternals(t *testing.T, artifact *interfaces.ReplayArtifact) {
	t.Helper()
	data, err := json.Marshal(artifact)
	if err != nil {
		t.Fatalf("marshal loaded artifact: %v", err)
	}
	for _, forbidden := range []string{"vmState", "checkpointState", "childDispatches", "providerTranscript"} {
		if strings.Contains(string(data), `"`+forbidden+`"`) {
			t.Fatalf("recording contains unsupported raw field %q", forbidden)
		}
	}
}

func stringPointer(value string) *string { return &value }

func TestRunStartedPayloadFromEvent_RejectsRetiredFactoryAliases(t *testing.T) {
	rawEvent := map[string]any{
		"id":            "factory-event/run-started",
		"schemaVersion": factoryapi.AgentFactoryEventV1,
		"type":          factoryapi.FactoryEventTypeRunRequest,
		"context": map[string]any{
			"eventTime": time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
			"tick":      0,
		},
		"payload": map[string]any{
			"recordedAt": time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
			"factory": map[string]any{
				"name": "retired-factory-alias-event",
				"workTypes": []map[string]any{
					{
						"name": "story",
						"states": []map[string]string{
							{"name": "ready", "type": "PROCESSING"},
							{"name": "complete", "type": "TERMINAL"},
						},
					},
				},
				"workers": []map[string]any{
					{
						"name":           "executor",
						"type":           "MODEL_WORKER",
						"modelProvider":  "CODEX",
						"model_provider": "anthropic",
					},
				},
				"workstations": []map[string]any{
					{
						"name":     "scheduled-story",
						"behavior": "CRON",
						"worker":   "executor",
						"cron": map[string]any{
							"schedule":         "*/5 * * * *",
							"triggerAtStart":   false,
							"trigger_at_start": true,
							"expiryWindow":     "30s",
							"expiry_window":    "45s",
						},
						"outputs": []map[string]string{
							{"workType": "story", "state": "complete"},
						},
					},
				},
			},
		},
	}

	data, err := json.Marshal(rawEvent)
	if err != nil {
		t.Fatalf("marshal raw event: %v", err)
	}

	var event factoryapi.FactoryEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("unmarshal factory event: %v", err)
	}

	_, err = runStartedPayloadFromEvent(event)
	if err == nil {
		t.Fatal("expected retired factory aliases to be rejected")
	}
	if want := "workers[0].model_provider is not supported; use modelProvider"; !strings.Contains(err.Error(), want) {
		t.Fatalf("expected model_provider retirement guidance, got %v", err)
	}
}

func TestRunStartedPayloadFromEvent_AllowsLegacyOnFailureObjectWhileNormalizingFactoryBoundary(t *testing.T) {
	recordedAt := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	rawEvent := map[string]any{
		"id":            "factory-event/run-started",
		"schemaVersion": factoryapi.AgentFactoryEventV1,
		"type":          factoryapi.FactoryEventTypeRunRequest,
		"context": map[string]any{
			"eventTime": recordedAt.Format(time.RFC3339),
			"tick":      0,
		},
		"payload": map[string]any{
			"recordedAt": recordedAt.Format(time.RFC3339),
			"factory": map[string]any{
				"name": "legacy-onfailure-shape",
				"workTypes": []map[string]any{
					{
						"name": "story",
						"states": []map[string]string{
							{"name": "ready", "type": "INITIAL"},
							{"name": "failed", "type": "FAILED"},
							{"name": "complete", "type": "TERMINAL"},
						},
					},
				},
				"workers": []map[string]any{
					{
						"name":          "executor",
						"type":          "MODEL_WORKER",
						"modelProvider": "CODEX",
					},
				},
				"workstations": []map[string]any{
					{
						"name":      "scheduled-story",
						"worker":    "executor",
						"inputs":    []map[string]string{{"workType": "story", "state": "ready"}},
						"outputs":   []map[string]string{{"workType": "story", "state": "complete"}},
						"onFailure": map[string]string{"workType": "story", "state": "failed"},
					},
				},
			},
		},
	}

	data, err := json.Marshal(rawEvent)
	if err != nil {
		t.Fatalf("marshal raw event: %v", err)
	}

	var event factoryapi.FactoryEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("unmarshal factory event: %v", err)
	}

	payload, err := runStartedPayloadFromEvent(event)
	if err != nil {
		t.Fatalf("runStartedPayloadFromEvent() error = %v", err)
	}
	if !payload.RecordedAt.Equal(recordedAt) {
		t.Fatalf("RecordedAt = %s, want %s", payload.RecordedAt, recordedAt)
	}
	if payload.Factory.Name != "legacy-onfailure-shape" {
		t.Fatalf("factory name = %q, want legacy-onfailure-shape", payload.Factory.Name)
	}
	if payload.Factory.Workstations == nil || len(*payload.Factory.Workstations) != 1 {
		t.Fatalf("factory workstations = %#v, want normalized workstation", payload.Factory.Workstations)
	}
	if got := (*payload.Factory.Workstations)[0].OnFailure; got == nil || !reflect.DeepEqual(*got, []factoryapi.WorkstationIO{{WorkType: "story", State: "failed"}}) {
		t.Fatalf("normalized onFailure = %#v, want [{WorkType:story State:failed}]", got)
	}
}

func TestMergeGeneratedWorkers_ReplacesExistingEntriesAndAppendsRuntimeOnlyInSortedOrder(t *testing.T) {
	factory := &factoryapi.Factory{
		Workers: &[]factoryapi.Worker{
			{
				Name:    "alpha",
				Type:    stringPtrIfNotEmpty(factoryapi.WorkerTypeScriptWorker),
				Command: stringPtrIfNotEmpty("stale-alpha"),
			},
			{
				Name:    "zeta",
				Type:    stringPtrIfNotEmpty(factoryapi.WorkerTypeScriptWorker),
				Command: stringPtrIfNotEmpty("keep-zeta"),
			},
		},
	}

	runtimeWorkers := map[string]interfaces.WorkerConfig{
		"charlie": {
			Type:      string(factoryapi.WorkerTypeScriptWorker),
			Command:   "charlie-command",
			Args:      []string{"charlie-arg"},
			StopToken: "DONE",
		},
		"alpha": {
			Type:      string(factoryapi.WorkerTypeScriptWorker),
			Command:   "fresh-alpha",
			Args:      []string{"alpha-arg"},
			StopToken: "COMPLETE",
		},
		"bravo": {
			Type:    string(factoryapi.WorkerTypeScriptWorker),
			Command: "bravo-command",
		},
	}

	if err := mergeGeneratedWorkers(factory, runtimeWorkers, nil); err != nil {
		t.Fatalf("mergeGeneratedWorkers() error = %v", err)
	}
	if factory.Workers == nil {
		t.Fatal("merged workers = nil, want generated worker list")
	}

	got := *factory.Workers
	if len(got) != 4 {
		t.Fatalf("merged workers count = %d, want 4", len(got))
	}
	if got[0].Name != "alpha" || stringValue(got[0].Command) != "fresh-alpha" || !reflect.DeepEqual(stringSliceValue(got[0].Args), []string{"alpha-arg"}) {
		t.Fatalf("merged alpha worker = %#v, want replaced runtime definition", got[0])
	}
	if stringValue(got[0].StopToken) != "COMPLETE" {
		t.Fatalf("merged alpha stop token = %q, want COMPLETE", stringValue(got[0].StopToken))
	}
	if got[1].Name != "zeta" || stringValue(got[1].Command) != "keep-zeta" {
		t.Fatalf("merged zeta worker = %#v, want untouched existing generated entry", got[1])
	}
	if got[2].Name != "bravo" || stringValue(got[2].Command) != "bravo-command" {
		t.Fatalf("merged bravo worker = %#v, want first sorted runtime-only append", got[2])
	}
	if got[3].Name != "charlie" || stringValue(got[3].Command) != "charlie-command" || !reflect.DeepEqual(stringSliceValue(got[3].Args), []string{"charlie-arg"}) {
		t.Fatalf("merged charlie worker = %#v, want second sorted runtime-only append", got[3])
	}
}

func TestMergeGeneratedWorkstations_ReplacesExistingEntriesAndAppendsRuntimeOnlyInSortedOrder(t *testing.T) {
	factory, runtimeWorkstations := mergeGeneratedWorkstationsFixture()
	if err := mergeGeneratedWorkstations(factory, runtimeWorkstations, nil); err != nil {
		t.Fatalf("mergeGeneratedWorkstations() error = %v", err)
	}
	assertMergedGeneratedWorkstations(t, factory)
}

func workstationKindPtr(value factoryapi.WorkstationKind) *factoryapi.WorkstationKind {
	return &value
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func mergeGeneratedWorkstationsFixture() (*factoryapi.Factory, map[string]interfaces.FactoryWorkstationConfig) {
	return &factoryapi.Factory{
			Workstations: &[]factoryapi.Workstation{
				{
					Name:     "alpha",
					Worker:   "stale-worker",
					Behavior: workstationKindPtr(factoryapi.WorkstationKindCron),
					Cron: &factoryapi.WorkstationCron{
						Schedule: "0 * * * *",
					},
					Inputs:  []factoryapi.WorkstationIO{{WorkType: "story", State: "stale"}},
					Outputs: &[]factoryapi.WorkstationIO{{WorkType: "story", State: "stale-done"}},
				},
				{
					Name:    "zeta",
					Worker:  "keep-worker",
					Inputs:  []factoryapi.WorkstationIO{{WorkType: "story", State: "ready"}},
					Outputs: &[]factoryapi.WorkstationIO{{WorkType: "story", State: "done"}},
				},
			},
		}, map[string]interfaces.FactoryWorkstationConfig{
			"charlie": {
				Name:           "charlie",
				Kind:           interfaces.WorkstationKindStandard,
				Type:           interfaces.WorkstationTypeLogical,
				WorkerTypeName: "charlie-worker",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "queued"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
			},
			"alpha": {
				Kind:             interfaces.WorkstationKindCron,
				Type:             interfaces.WorkstationTypeLogical,
				WorkerTypeName:   "fresh-worker",
				Cron:             &interfaces.CronConfig{Schedule: "*/5 * * * *", TriggerAtStart: true, ExpiryWindow: "30s"},
				Inputs:           []interfaces.IOConfig{{WorkTypeName: "story", StateName: "review"}},
				Outputs:          []interfaces.IOConfig{{WorkTypeName: "story", StateName: "complete"}},
				OnFailure:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "failed"}},
				Resources:        []interfaces.ResourceConfig{{Name: "agent-slot", Capacity: 2}},
				WorkingDirectory: "/repo/runtime",
			},
			"bravo": {
				Name:           "bravo",
				Kind:           interfaces.WorkstationKindStandard,
				Type:           interfaces.WorkstationTypeLogical,
				WorkerTypeName: "bravo-worker",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "ready"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
				Resources:      []interfaces.ResourceConfig{{Name: "gpu", Capacity: 1}},
			},
		}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this helper intentionally validates the merged generated workstation artifact contract in one place.
func assertMergedGeneratedWorkstations(t *testing.T, factory *factoryapi.Factory) {
	t.Helper()
	if factory.Workstations == nil {
		t.Fatal("merged workstations = nil, want generated workstation list")
	}

	got := *factory.Workstations
	if len(got) != 4 {
		t.Fatalf("merged workstations count = %d, want 4", len(got))
	}
	if got[0].Name != "alpha" || got[0].Worker != "fresh-worker" {
		t.Fatalf("merged alpha workstation = %#v, want replaced runtime definition", got[0])
	}
	if got[0].Behavior == nil || *got[0].Behavior != factoryapi.WorkstationKindCron {
		t.Fatalf("merged alpha behavior = %#v, want CRON", got[0].Behavior)
	}
	if got[0].Cron == nil || got[0].Cron.Schedule != "*/5 * * * *" || !boolValue(got[0].Cron.TriggerAtStart) || stringValue(got[0].Cron.ExpiryWindow) != "30s" {
		t.Fatalf("merged alpha cron = %#v, want runtime cron fields", got[0].Cron)
	}
	if !reflect.DeepEqual(got[0].Inputs, []factoryapi.WorkstationIO{{WorkType: "story", State: "review"}}) {
		t.Fatalf("merged alpha inputs = %#v, want runtime inputs", got[0].Inputs)
	}
	if got[0].Outputs == nil || !reflect.DeepEqual(*got[0].Outputs, []factoryapi.WorkstationIO{{WorkType: "story", State: "complete"}}) {
		t.Fatalf("merged alpha outputs = %#v, want runtime outputs", got[0].Outputs)
	}
	if got[0].OnFailure == nil || !reflect.DeepEqual(*got[0].OnFailure, []factoryapi.WorkstationIO{{WorkType: "story", State: "failed"}}) {
		t.Fatalf("merged alpha onFailure = %#v, want runtime onFailure", got[0].OnFailure)
	}
	if stringValue(got[0].WorkingDirectory) != "/repo/runtime" {
		t.Fatalf("merged alpha working directory = %q, want /repo/runtime", stringValue(got[0].WorkingDirectory))
	}
	if got[0].Resources == nil || !reflect.DeepEqual(*got[0].Resources, []factoryapi.ResourceRequirement{{Name: "agent-slot", Capacity: 2}}) {
		t.Fatalf("merged alpha resources = %#v, want runtime resources", got[0].Resources)
	}
	if got[1].Name != "zeta" || got[1].Worker != "keep-worker" {
		t.Fatalf("merged zeta workstation = %#v, want untouched existing generated entry", got[1])
	}
	if got[2].Name != "bravo" || got[2].Worker != "bravo-worker" {
		t.Fatalf("merged bravo workstation = %#v, want first sorted runtime-only append", got[2])
	}
	if got[2].Resources == nil || !reflect.DeepEqual(*got[2].Resources, []factoryapi.ResourceRequirement{{Name: "gpu", Capacity: 1}}) {
		t.Fatalf("merged bravo resources = %#v, want appended runtime resources", got[2].Resources)
	}
	if got[3].Name != "charlie" || got[3].Worker != "charlie-worker" {
		t.Fatalf("merged charlie workstation = %#v, want second sorted runtime-only append", got[3])
	}
	if got[3].Behavior == nil || *got[3].Behavior != factoryapi.WorkstationKindStandard {
		t.Fatalf("merged charlie behavior = %#v, want STANDARD", got[3].Behavior)
	}
}
