package replay

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/config"
	"github.com/portpowered/agent-factory/pkg/interfaces"

	"github.com/portpowered/agent-factory/pkg/workers"
)

// portos:func-length-exception owner=agent-factory reason=replay-artifact-roundtrip-fixture review=2026-07-18 removal=split-artifact-fixture-event-builders-and-storage-assertions-before-next-replay-artifact-change
func TestSaveLoad_PreservesReplayArtifactFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.replay.json")
	recordedAt := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	generatedFactory := artifactTestFactory()
	runStarted, err := runStartedEventFromFactory(recordedAt, generatedFactory, &interfaces.ReplayWallClockMetadata{
		StartedAt:  recordedAt,
		FinishedAt: recordedAt.Add(time.Second),
	}, interfaces.ReplayDiagnostics{})
	if err != nil {
		t.Fatalf("runStartedEvent: %v", err)
	}
	submission := replayWorkRequestEvent(t, "request-1", 2, "api", []factoryapi.Work{{
		Name:         "story-1",
		WorkId:       stringPtrIfNotEmpty("work-1"),
		RequestId:    stringPtrIfNotEmpty("request-1"),
		WorkTypeName: stringPtrIfNotEmpty("story"),
		TraceId:      stringPtrIfNotEmpty("trace-1"),
		Payload:      map[string]any{"title": "first"},
	}}, nil)
	dispatch := replayDispatchCreatedEvent(t, interfaces.WorkDispatch{
		DispatchID:   "dispatch-1",
		TransitionID: "transition-1",
		WorkerType:   "executor",
		InputTokens: workers.InputTokens(interfaces.Token{
			ID: "token-1",
			Color: interfaces.TokenColor{
				WorkID:     "work-1",
				WorkTypeID: "story",
				DataType:   interfaces.DataTypeWork,
				TraceID:    "trace-1",
			},
		}),
		Execution: interfaces.ExecutionMetadata{
			ReplayKey: "transition-1/work-1",
			TraceID:   "trace-1",
			WorkIDs:   []string{"work-1"},
		},
	}, 3)
	inference := replayInferenceResponseEvent(
		t,
		interfaces.WorkDispatch{
			DispatchID: "dispatch-1",
			Execution: interfaces.ExecutionMetadata{
				RequestID: "request-1",
				TraceID:   "trace-1",
				WorkIDs:   []string{"work-1"},
			},
		},
		"dispatch-1/inference-request/1",
		1,
		4,
		"done",
		nil,
		&interfaces.WorkDiagnostics{
			Provider: &interfaces.ProviderDiagnostic{
				Provider: "mock",
				Model:    "mock-model",
				ResponseMetadata: map[string]string{
					"request_id": "provider-request-1",
				},
			},
		},
		"",
	)
	completion := replayDispatchCompletedEvent(t, "completion-1", interfaces.WorkResult{
		DispatchID:   "dispatch-1",
		TransitionID: "transition-1",
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "done",
	}, 5)
	events := []factoryapi.FactoryEvent{runStarted, submission, dispatch, inference, completion, runFinishedEvent(recordedAt.Add(time.Second), &interfaces.ReplayWallClockMetadata{
		StartedAt:  recordedAt,
		FinishedAt: recordedAt.Add(time.Second),
	}, interfaces.ReplayDiagnostics{})}
	assignEventSequences(events)
	artifact := &interfaces.ReplayArtifact{
		SchemaVersion: CurrentSchemaVersion,
		RecordedAt:    recordedAt,
		Events:        events,
		Factory:       generatedFactory,
		WallClock: &interfaces.ReplayWallClockMetadata{
			StartedAt:  recordedAt,
			FinishedAt: recordedAt.Add(time.Second),
		},
	}

	if err := Save(path, artifact); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	dispatchPayload := requireReplayDispatchCreated(t, loaded.Events, "dispatch-1")
	if dispatchPayload.TransitionId != "transition-1" {
		t.Fatalf("dispatch transition = %q, want transition-1", dispatchPayload.TransitionId)
	}
	completionPayload := requireReplayDispatchCompleted(t, loaded.Events, "dispatch-1")
	if stringValue(completionPayload.CompletionId) != "completion-1" {
		t.Fatalf("completion ID = %q, want completion-1", stringValue(completionPayload.CompletionId))
	}
	if loaded.Events[1].Context.Tick != 2 || loaded.Events[2].Context.Tick != 3 || loaded.Events[3].Context.Tick != 4 || loaded.Events[4].Context.Tick != 5 {
		t.Fatalf("logical ticks were not preserved: request=%d dispatch=%d inference=%d completion=%d",
			loaded.Events[1].Context.Tick,
			loaded.Events[2].Context.Tick,
			loaded.Events[3].Context.Tick,
			loaded.Events[4].Context.Tick)
	}
	inferencePayload := requireReplayInferenceResponse(t, loaded.Events, "dispatch-1/inference-request/1")
	if got := (*inferencePayload.Diagnostics.Provider.ResponseMetadata)["request_id"]; got != "provider-request-1" {
		t.Fatalf("provider diagnostic request_id = %q, want provider-request-1", got)
	}
	if inferencePayload.Diagnostics == nil {
		t.Fatalf("inference diagnostics = nil, want safe diagnostics")
	}
}

// portos:func-length-exception owner=agent-factory reason=replay-safe-diagnostics-regression-fixture review=2026-07-21 removal=extract-unsafe-diagnostics-fixture-builder-before-next-replay-artifact-expansion
func TestSaveLoad_StripsUnsafeCompletionDiagnosticsFromStoredReplayEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "safe-diagnostics.replay.json")
	artifact := testReplayArtifact(
		t,
		replayInferenceResponseEvent(
			t,
			interfaces.WorkDispatch{DispatchID: "dispatch-safe"},
			"dispatch-safe/inference-request/1",
			1,
			2,
			"completed",
			&interfaces.ProviderSessionMetadata{
				Provider: "codex",
				Kind:     "response_id",
				ID:       "resp-safe-123",
			},
			&interfaces.WorkDiagnostics{
				RenderedPrompt: &interfaces.RenderedPromptDiagnostic{
					SystemPromptHash: "system-hash-123",
					UserMessageHash:  "user-hash-456",
					Variables: map[string]string{
						"prompt_source":  "factory-renderer",
						"work_type_name": "story",
						"system_prompt":  "raw rendered system prompt must stay private",
						"user_message":   "raw rendered user message must stay private",
						"stdin":          "raw rendered stdin must stay private",
						"env":            "raw rendered environment must stay private",
					},
				},
				Provider: &interfaces.ProviderDiagnostic{
					Provider: "codex",
					Model:    "gpt-5.4",
					RequestMetadata: map[string]string{
						"prompt_source":      "provider-renderer",
						"worker_type":        "builder",
						"system_prompt_body": "raw prompt body must stay private",
						"stdin_payload":      "raw stdin payload must stay private",
						"env_secret":         "raw env secret must stay private",
					},
					ResponseMetadata: map[string]string{
						"retry_count":         "1",
						"provider_session_id": "resp-safe-123",
						"system_prompt_body":  "raw response prompt body must stay private",
						"stdin_payload":       "raw response stdin payload must stay private",
						"env_secret":          "raw response env secret must stay private",
					},
				},
				Command: &interfaces.CommandDiagnostic{
					Command: "echo",
					Stdin:   "raw command stdin must stay private",
					Env: map[string]string{
						"AGENT_FACTORY_AUTH_TOKEN": "raw environment value must stay private",
					},
				},
				Panic: &interfaces.PanicDiagnostic{Stack: "panic stack should not be stored"},
			},
			"",
		),
		replayDispatchCompletedEvent(t, "completion-safe", interfaces.WorkResult{
			DispatchID:   "dispatch-safe",
			TransitionID: "transition-safe",
			Outcome:      interfaces.OutcomeAccepted,
			Output:       "completed",
			ProviderFailure: &interfaces.ProviderFailureMetadata{
				Family: interfaces.ProviderErrorFamilyRetryable,
				Type:   interfaces.ProviderErrorTypeThrottled,
			},
		}, 3),
	)

	if err := Save(path, artifact); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	body := string(data)
	for _, unsafe := range replayUnsafeDiagnosticValues() {
		if strings.Contains(body, unsafe) {
			t.Fatalf("stored replay artifact leaked unsafe diagnostic value %q: %s", unsafe, body)
		}
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	inferencePayload := requireReplayInferenceResponse(t, loaded.Events, "dispatch-safe/inference-request/1")
	if inferencePayload.Diagnostics == nil || inferencePayload.Diagnostics.Provider == nil || inferencePayload.Diagnostics.RenderedPrompt == nil {
		t.Fatalf("inference diagnostics = %#v, want provider and rendered prompt", inferencePayload.Diagnostics)
	}
	if got := (*inferencePayload.Diagnostics.Provider.RequestMetadata)["worker_type"]; got != "builder" {
		t.Fatalf("request metadata worker_type = %q, want builder", got)
	}
	if got := (*inferencePayload.Diagnostics.Provider.ResponseMetadata)["provider_session_id"]; got != "resp-safe-123" {
		t.Fatalf("response metadata provider_session_id = %q, want resp-safe-123", got)
	}
	if got := (*inferencePayload.Diagnostics.RenderedPrompt.Variables)["work_type_name"]; got != "story" {
		t.Fatalf("rendered prompt work_type_name = %q, want story", got)
	}
	if got := stringValue(inferencePayload.ProviderSession.Id); got != "resp-safe-123" {
		t.Fatalf("provider session id = %q, want resp-safe-123", got)
	}
	completionPayload := requireReplayDispatchCompleted(t, loaded.Events, "dispatch-safe")
	if got := stringValue(completionPayload.ProviderFailure.Type); got != string(interfaces.ProviderErrorTypeThrottled) {
		t.Fatalf("provider failure type = %q, want throttled", got)
	}
}

func TestLoad_UnsupportedSchemaVersion_ReturnsClearError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unsupported.replay.json")
	data := `{
		"schemaVersion": "agent-factory.replay.v99",
		"recordedAt": "2026-04-10T12:00:00Z",
		"events": []
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want unsupported schema error")
	}
	if !strings.Contains(err.Error(), "unsupported replay artifact schemaVersion") {
		t.Fatalf("Load() error = %q, want unsupported schema message", err)
	}
}

func TestLoad_LegacyReplayArrays_FailsValidationBeforeReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "malformed.replay.json")
	data := `{
		"schema_version": "agent-factory.replay.v1",
		"recorded_at": "2026-04-10T12:00:00Z",
		"dispatches": [{"dispatch_id": "dispatch-1", "created_tick": 1, "dispatch": {"dispatch_id": "other", "transition_id": "transition-1"}}]
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "schemaVersion is required") {
		t.Fatalf("Load() error = %q, want missing schemaVersion validation", err)
	}
}

func TestLoad_AcceptsInferenceEventsInCanonicalEventsArray(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inference-events.replay.json")
	request := replayInferenceDispatch()
	const inferenceRequestID = "inference-request-dispatch-1-attempt-1"
	artifact := testReplayArtifact(
		t,
		replayWorkRequestEvent(t, "request-1", 1, "api", []factoryapi.Work{{
			Name:         "story",
			WorkId:       stringPtrIfNotEmpty("work-1"),
			RequestId:    stringPtrIfNotEmpty("request-1"),
			WorkTypeName: stringPtrIfNotEmpty("task"),
			TraceId:      stringPtrIfNotEmpty("trace-1"),
		}}, nil),
		replayDispatchCreatedEvent(t, request.Dispatch, 2),
		replayInferenceRequestEvent(t, request, inferenceRequestID, 1, 2),
		replayInferenceResponseEvent(t, request.Dispatch, inferenceRequestID, 1, 2, "provider completed", nil, nil, ""),
		replayDispatchCompletedEvent(t, "completion-1", interfaces.WorkResult{
			DispatchID:   request.Dispatch.DispatchID,
			TransitionID: request.Dispatch.TransitionID,
			Outcome:      interfaces.OutcomeAccepted,
			Output:       "provider completed",
		}, 3),
	)

	if err := Save(path, artifact); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	assertReplayInferencePair(t, loaded, inferenceRequestID)
	if _, err := NewSideEffects(loaded); err != nil {
		t.Fatalf("NewSideEffects() with inference events: %v", err)
	}
	if _, err := NewCompletionDeliveryPlan(loaded); err != nil {
		t.Fatalf("NewCompletionDeliveryPlan() with inference events: %v", err)
	}
}

func TestLoad_RunStartedFactoryBoundaryMatchesFileBoundaryDecode(t *testing.T) {
	recordedAt := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	factoryJSON := []byte(`{
		"id": "customer-project",
		"workTypes": [
			{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]},
			{"name":"page","states":[{"name":"complete","type":"TERMINAL"}]}
		],
		"resources": [{"name":"agent-slot","capacity":2}],
		"workers": [{"name":"executor","type":"MODEL_WORKER","modelProvider":"CLAUDE","stopToken":"COMPLETE"}],
		"workstations": [{
			"id":"execute-story-id",
			"name":"execute-story",
			"behavior":"REPEATER",
			"worker":"executor",
			"type":"MODEL_WORKSTATION",
			"promptTemplate":"Finish {{ .WorkID }}.",
			"inputs":[
				{"workType":"story","state":"init"},
				{"workType":"page","state":"complete","guards":[{"type":"ALL_CHILDREN_COMPLETE","parentInput":"story","spawnedBy":"chapter-parser"}]}
			],
			"outputs":[{"workType":"story","state":"complete"}],
			"resources":[{"name":"agent-slot","capacity":2}]
		}]
	}`)

	want, err := config.GeneratedFactoryFromOpenAPIJSON(factoryJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}

	path := filepath.Join(t.TempDir(), "camel-case.replay.json")
	writeReplayArtifactWithFactoryJSON(t, path, recordedAt, factoryJSON)

	artifact, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(artifact.Factory, want) {
		t.Fatalf("run-started factory mismatch\n got: %#v\nwant: %#v", artifact.Factory, want)
	}
}

func TestLoad_RunStartedFactoryBoundaryRejectsRetiredFanInField(t *testing.T) {
	recordedAt := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	factoryJSON := []byte(`{
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}],
			"join":{"waitFor":"story","waitState":"complete","require":"all"}
		}]
	}`)

	path := filepath.Join(t.TempDir(), "retired-join.replay.json")
	writeReplayArtifactWithFactoryJSON(t, path, recordedAt, factoryJSON)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want retired join boundary error")
	}
	if !strings.Contains(err.Error(), "decode factory generated-schema boundary") {
		t.Fatalf("Load() error = %q, want generated boundary context", err)
	}
	if !strings.Contains(err.Error(), "workstations[0].join is not supported") {
		t.Fatalf("Load() error = %q, want retired join message", err)
	}
}

func TestLoad_RunStartedFactoryBoundaryRejectsRetiredExhaustionRulesField(t *testing.T) {
	recordedAt := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	factoryJSON := []byte(`{
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"failed","type":"FAILED"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"failed"}]
		}],
		"exhaustionRules": [{
			"name":"execute-story-loop-breaker",
			"watchWorkstation":"execute-story",
			"maxVisits":3,
			"source":{"workType":"story","state":"init"},
			"target":{"workType":"story","state":"failed"}
		}]
	}`)

	path := filepath.Join(t.TempDir(), "retired-exhaustion-rules.replay.json")
	writeReplayArtifactWithFactoryJSON(t, path, recordedAt, factoryJSON)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want retired exhaustion_rules boundary error")
	}
	if !strings.Contains(err.Error(), "decode factory generated-schema boundary") {
		t.Fatalf("Load() error = %q, want generated boundary context", err)
	}
	if !strings.Contains(err.Error(), "exhaustion_rules is retired") {
		t.Fatalf("Load() error = %q, want retired exhaustion_rules message", err)
	}
}

func TestLoad_RunStartedFactoryBoundaryRejectsRetiredCronIntervalField(t *testing.T) {
	recordedAt := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	factoryJSON := []byte(`{
		"workTypes": [{"name":"task","states":[{"name":"ready","type":"PROCESSING"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"daily-refresh",
			"behavior":"CRON",
			"worker":"executor",
			"outputs":[{"workType":"task","state":"complete"}],
			"cron":{"interval":"5m"}
		}]
	}`)

	path := filepath.Join(t.TempDir(), "retired-cron-interval.replay.json")
	writeReplayArtifactWithFactoryJSON(t, path, recordedAt, factoryJSON)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want retired cron interval boundary error")
	}
	if !strings.Contains(err.Error(), "decode factory generated-schema boundary") {
		t.Fatalf("Load() error = %q, want generated boundary context", err)
	}
	if !strings.Contains(err.Error(), "workstations[0].cron.interval is not supported; use cron.schedule") {
		t.Fatalf("Load() error = %q, want retired cron interval message", err)
	}
}

func TestLoad_RunStartedFactoryBoundaryRejectsUnsupportedGeneratedField(t *testing.T) {
	recordedAt := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	factoryJSON := []byte(`{
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}],
			"unsupportedField": true
		}]
	}`)

	path := filepath.Join(t.TempDir(), "unsupported-field.replay.json")
	writeReplayArtifactWithFactoryJSON(t, path, recordedAt, factoryJSON)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want strict run-started factory boundary error")
	}
	if !strings.Contains(err.Error(), "decode factory generated-schema boundary") {
		t.Fatalf("Load() error = %q, want generated boundary context", err)
	}
	if !strings.Contains(err.Error(), `json: unknown field "unsupportedField"`) {
		t.Fatalf("Load() error = %q, want unknown-field rejection", err)
	}
}

func TestLoad_CheckedInInferenceEventFixtureAccepted(t *testing.T) {
	artifact, err := Load(filepath.FromSlash("testdata/inference-events.replay.json"))
	if err != nil {
		t.Fatalf("Load() checked-in inference fixture: %v", err)
	}
	assertReplayInferencePair(t, artifact, "inference-request-fixture-1")
	if _, err := NewSideEffects(artifact); err != nil {
		t.Fatalf("NewSideEffects() checked-in inference fixture: %v", err)
	}
}

func TestSave_ReplacesArtifactThroughRecoverableTempFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.replay.json")
	recordedAt := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	artifact := minimalValidArtifact(recordedAt)

	if err := Save(path, artifact); err != nil {
		t.Fatalf("initial Save() error = %v", err)
	}

	artifact.Events = append(artifact.Events, replayWorkRequestEvent(t, "submission-1", 1, "api", []factoryapi.Work{{
		Name:         "story",
		WorkTypeName: stringPtrIfNotEmpty("story"),
		TraceId:      stringPtrIfNotEmpty("trace-1"),
	}}, nil))
	assignEventSequences(artifact.Events)
	if err := Save(path, artifact); err != nil {
		t.Fatalf("replacement Save() error = %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := countReplayEvents(loaded.Events, factoryapi.FactoryEventTypeWorkRequest); got != 1 {
		t.Fatalf("work request events = %d, want 1", got)
	}
	tmpMatches, err := filepath.Glob(path + ".*.tmp")
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(tmpMatches) != 0 {
		t.Fatalf("unexpected leftover temp artifacts: %v", tmpMatches)
	}
}

func minimalValidArtifact(recordedAt time.Time) *interfaces.ReplayArtifact {
	artifact, err := NewEventLogArtifactFromFactory(recordedAt, factoryapi.Factory{
		WorkTypes:    &[]factoryapi.WorkType{},
		Resources:    &[]factoryapi.Resource{},
		Workers:      &[]factoryapi.Worker{},
		Workstations: &[]factoryapi.Workstation{},
	}, nil, interfaces.ReplayDiagnostics{})
	if err != nil {
		panic(err)
	}
	return artifact
}

func artifactTestFactory() factoryapi.Factory {
	return factoryapi.Factory{
		FactoryDirectory: stringPtrIfNotEmpty("fixtures/customer-run"),
		WorkTypes: &[]factoryapi.WorkType{{
			Name: "story",
			States: []factoryapi.WorkState{{
				Name: "init",
				Type: factoryapi.WorkStateType(interfaces.StateTypeInitial),
			}},
		}},
		Resources: &[]factoryapi.Resource{},
		Workers: &[]factoryapi.Worker{{
			Name:    "executor",
			Type:    stringPtrIfNotEmpty(factoryapi.WorkerTypeScriptWorker),
			Command: stringPtrIfNotEmpty("echo"),
			Args:    &[]string{"ok"},
		}},
		Workstations: &[]factoryapi.Workstation{{
			Id:      stringPtrIfNotEmpty("ws-1"),
			Name:    "execute",
			Worker:  "executor",
			Type:    stringPtrIfNotEmpty(factoryapi.WorkstationTypeLogicalMove),
			Inputs:  []factoryapi.WorkstationIO{},
			Outputs: []factoryapi.WorkstationIO{},
		}},
		Metadata: generatedStringMapPtr(map[string]string{"factory_hash": "sha256:abc"}),
	}
}

func replayInferenceDispatch() interfaces.ProviderInferenceRequest {
	dispatch := interfaces.WorkDispatch{
		DispatchID:   "dispatch-1",
		TransitionID: "process",
		WorkerType:   "worker-a",
		InputTokens: workers.InputTokens(interfaces.Token{
			ID: "token-1",
			Color: interfaces.TokenColor{
				WorkID:     "work-1",
				WorkTypeID: "task",
				DataType:   interfaces.DataTypeWork,
				TraceID:    "trace-1",
			},
		}),
		Execution: interfaces.ExecutionMetadata{
			RequestID: "request-1",
			ReplayKey: "process/work-1",
			TraceID:   "trace-1",
			WorkIDs:   []string{"work-1"},
		},
	}
	return interfaces.ProviderInferenceRequest{
		Dispatch:         dispatch,
		WorkerType:       dispatch.WorkerType,
		WorkingDirectory: "/workspace/project",
		Worktree:         "/workspace/project/.worktrees/story-1",
		UserMessage:      "Process work-1.",
	}
}

func assertReplayInferencePair(t *testing.T, artifact *interfaces.ReplayArtifact, inferenceRequestID string) {
	t.Helper()

	var request *factoryapi.InferenceRequestEventPayload
	var response *factoryapi.InferenceResponseEventPayload
	for _, event := range artifact.Events {
		switch event.Type {
		case factoryapi.FactoryEventTypeInferenceRequest:
			payload, err := event.Payload.AsInferenceRequestEventPayload()
			if err != nil {
				t.Fatalf("decode inference request event %q: %v", event.Id, err)
			}
			if payload.InferenceRequestId == inferenceRequestID {
				request = &payload
			}
		case factoryapi.FactoryEventTypeInferenceResponse:
			payload, err := event.Payload.AsInferenceResponseEventPayload()
			if err != nil {
				t.Fatalf("decode inference response event %q: %v", event.Id, err)
			}
			if payload.InferenceRequestId == inferenceRequestID {
				response = &payload
			}
		}
	}
	if request == nil || response == nil {
		t.Fatalf("inference pair %q missing request=%v response=%v", inferenceRequestID, request != nil, response != nil)
	}
	if request.Attempt != response.Attempt {
		t.Fatalf("inference pair correlation mismatch: request=%#v response=%#v", *request, *response)
	}
	if request.Prompt == "" {
		t.Fatalf("inference request %q prompt is empty", inferenceRequestID)
	}
}

func countReplayEvents(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func requireReplayDispatchCreated(t *testing.T, events []factoryapi.FactoryEvent, dispatchID string) factoryapi.DispatchRequestEventPayload {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest || stringValue(event.Context.DispatchId) != dispatchID {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch created event: %v", err)
		}
		return payload
	}
	t.Fatalf("missing DISPATCH_REQUEST for %s", dispatchID)
	return factoryapi.DispatchRequestEventPayload{}
}

func requireReplayDispatchCompleted(t *testing.T, events []factoryapi.FactoryEvent, dispatchID string) factoryapi.DispatchResponseEventPayload {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse || stringValue(event.Context.DispatchId) != dispatchID {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch completed event: %v", err)
		}
		return payload
	}
	t.Fatalf("missing DISPATCH_RESPONSE for %s", dispatchID)
	return factoryapi.DispatchResponseEventPayload{}
}

func requireReplayInferenceResponse(t *testing.T, events []factoryapi.FactoryEvent, inferenceRequestID string) factoryapi.InferenceResponseEventPayload {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response event: %v", err)
		}
		if payload.InferenceRequestId == inferenceRequestID {
			return payload
		}
	}
	t.Fatalf("missing INFERENCE_RESPONSE for %s", inferenceRequestID)
	return factoryapi.InferenceResponseEventPayload{}
}

func writeReplayArtifactWithFactoryJSON(t *testing.T, path string, recordedAt time.Time, factoryJSON []byte) {
	t.Helper()

	payload := map[string]any{
		"recordedAt": recordedAt.UTC().Format(time.RFC3339),
		"factory":    json.RawMessage(factoryJSON),
	}
	event := map[string]any{
		"id":            replayRunStartedEventID,
		"schemaVersion": string(factoryapi.AgentFactoryEventV1),
		"type":          string(factoryapi.FactoryEventTypeRunRequest),
		"context": map[string]any{
			"eventTime": recordedAt.UTC().Format(time.RFC3339),
			"sequence":  0,
			"tick":      0,
		},
		"payload": payload,
	}
	artifact := map[string]any{
		"schemaVersion": CurrentSchemaVersion,
		"recordedAt":    recordedAt.UTC().Format(time.RFC3339),
		"events":        []any{event},
	}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
}

func replayUnsafeDiagnosticValues() []string {
	return []string{
		"raw prompt body must stay private",
		"raw response prompt body must stay private",
		"raw stdin payload must stay private",
		"raw response stdin payload must stay private",
		"raw env secret must stay private",
		"raw response env secret must stay private",
		"raw rendered system prompt must stay private",
		"raw rendered user message must stay private",
		"raw rendered stdin must stay private",
		"raw rendered environment must stay private",
		"raw command stdin must stay private",
		"raw environment value must stay private",
		"AGENT_FACTORY_AUTH_TOKEN",
		"panic stack should not be stored",
	}
}
