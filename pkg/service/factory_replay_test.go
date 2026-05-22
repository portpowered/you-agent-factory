package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type recordingDiagnosticsProvider struct{}

func (recordingDiagnosticsProvider) Infer(_ context.Context, _ interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	return interfaces.InferenceResponse{
		Content: "Done. COMPLETE",
		Diagnostics: &interfaces.WorkDiagnostics{
			Provider: &interfaces.ProviderDiagnostic{
				ResponseMetadata: map[string]string{"request_id": "provider-request-1"},
			},
		},
	}, nil
}

func TestBuildFactoryService_ReplayModeLoadsEmbeddedConfigWithoutFactoryFiles(t *testing.T) {
	sourceDir := t.TempDir()
	writeFactoryJSON(t, sourceDir, minimalFactoryConfig())
	writeScriptWorkerAgentsMD(t, sourceDir, "worker-a")
	writeWorkstationAgentsMD(t, sourceDir, "process")

	loaded, err := config.LoadRuntimeConfig(sourceDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	artifactPath := filepath.Join(t.TempDir(), "recording.json")
	artifact := newReplayArtifactFromLoadedFactory(t, time.Now().UTC(), loaded)
	if err := replay.Save(artifactPath, artifact); err != nil {
		t.Fatalf("Save artifact: %v", err)
	}

	replayDir := t.TempDir()
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               replayDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		ReplayPath:        artifactPath,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService replay: %v", err)
	}

	if svc.net == nil {
		t.Fatal("expected replay service to build net from embedded config")
	}
	if _, ok := svc.net.WorkTypes["task"]; !ok {
		t.Fatal("expected task work type from embedded config")
	}
	if _, err := os.Stat(filepath.Join(replayDir, interfaces.FactoryConfigFile)); !os.IsNotExist(err) {
		t.Fatalf("replay should not create or require local factory.json, stat err = %v", err)
	}
}

func TestBuildFactoryService_ReplayModeDefaultsToDeterministicClock(t *testing.T) {
	sourceDir := t.TempDir()
	writeFactoryJSON(t, sourceDir, minimalFactoryConfig())
	writeScriptWorkerAgentsMD(t, sourceDir, "worker-a")
	writeWorkstationAgentsMD(t, sourceDir, "process")

	loaded, err := config.LoadRuntimeConfig(sourceDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	recordedAt := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	artifactPath := filepath.Join(t.TempDir(), "recording.json")
	artifact := newReplayArtifactFromLoadedFactory(t, recordedAt, loaded)
	if err := replay.Save(artifactPath, artifact); err != nil {
		t.Fatalf("Save artifact: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               t.TempDir(),
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		ReplayPath:        artifactPath,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService replay: %v", err)
	}

	if _, ok := svc.clock.(*replay.DeterministicClock); !ok {
		t.Fatalf("expected replay service to use deterministic clock, got %T", svc.clock)
	}
	if got := svc.clock.Now(); !got.Equal(recordedAt) {
		t.Fatalf("replay clock Now() = %s, want %s", got, recordedAt)
	}
}

func TestBuildFactoryService_ReplayModeUsesRecordedProviderSideEffects(t *testing.T) {
	sourceDir := t.TempDir()
	writeFactoryJSON(t, sourceDir, minimalFactoryConfig())
	writeWorkerAgentsMD(t, sourceDir, "worker-a")
	writeWorkstationAgentsMD(t, sourceDir, "process")

	artifactPath := filepath.Join(t.TempDir(), "recording.json")
	saveReplayBehaviorArtifact(t, sourceDir, artifactPath, interfaces.WorkDispatch{
		DispatchID:      "recorded-dispatch-provider",
		TransitionID:    "process",
		WorkerType:      "worker-a",
		WorkstationName: "process",
		Execution: interfaces.ExecutionMetadata{
			ReplayKey: "process/trace-replay-provider/work-replay-provider",
			TraceID:   "trace-replay-provider",
			WorkIDs:   []string{"work-replay-provider"},
		},
	}, interfaces.WorkResult{
		DispatchID:   "recorded-dispatch-provider",
		TransitionID: "process",
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "replayed provider output",
		Diagnostics: &interfaces.WorkDiagnostics{
			Provider: &interfaces.ProviderDiagnostic{
				ResponseMetadata: map[string]string{"request_id": "replay-provider-1"},
			},
		},
	})

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:        t.TempDir(),
		Logger:     zap.NewNop(),
		ReplayPath: artifactPath,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService replay: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run replay: %v", err)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-replay-fixture review=2026-07-18 removal=split-replay-fixture-before-next-replay-service-change
func TestBuildFactoryService_ReplayModeDeliversRecordedCompletionAtLogicalTick(t *testing.T) {
	sourceDir := t.TempDir()
	writeFactoryJSON(t, sourceDir, minimalFactoryConfig())
	writeWorkerAgentsMD(t, sourceDir, "worker-a")
	writeWorkstationAgentsMD(t, sourceDir, "process")

	loaded, err := config.LoadRuntimeConfig(sourceDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	artifactPath := filepath.Join(t.TempDir(), "recording.json")
	recordedDispatch := interfaces.WorkDispatch{
		DispatchID:      "recorded-dispatch-logical-tick",
		TransitionID:    "process",
		WorkerType:      "worker-a",
		WorkstationName: "process",
		Execution: interfaces.ExecutionMetadata{
			ReplayKey: "process/trace-logical-tick/work-logical-tick",
			TraceID:   "trace-logical-tick",
			WorkIDs:   []string{"work-logical-tick"},
		},
	}
	recordedResult := interfaces.WorkResult{
		DispatchID:                  recordedDispatch.DispatchID,
		TransitionID:                recordedDispatch.TransitionID,
		Outcome:                     interfaces.OutcomeAccepted,
		Output:                      "replayed provider output",
		SelectedClassificationLabel: "approved",
		Diagnostics: &interfaces.WorkDiagnostics{
			Provider: &interfaces.ProviderDiagnostic{
				ResponseMetadata: map[string]string{"request_id": "logical-tick-provider-1"},
			},
		},
	}
	recordedAt := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	artifact := newReplayArtifactFromLoadedFactory(t, recordedAt, loaded)
	artifact.Events = append(artifact.Events,
		serviceReplayWorkRequestEvent(t, "recorded-submission-logical-tick", 1, "recorded-artifact", []factoryapi.Work{{
			Name:         "work-logical-tick",
			WorkId:       serviceStringPtr("work-logical-tick"),
			WorkTypeName: serviceStringPtr("task"),
			TraceId:      serviceStringPtr("trace-logical-tick"),
			Payload:      map[string]any{"task": "replay logical tick"},
		}}, nil),
		serviceReplayDispatchCreatedEvent(t, recordedDispatch, 1),
		serviceReplayDispatchCompletedEvent(t, "recorded-completion-logical-tick", recordedResult, 4),
	)
	if err := replay.Save(artifactPath, artifact); err != nil {
		t.Fatalf("Save replay artifact: %v", err)
	}

	var completions []interfaces.FactoryCompletionRecord
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:        t.TempDir(),
		Logger:     zap.NewNop(),
		ReplayPath: artifactPath,
		ExtraOptions: []factory.FactoryOption{
			factory.WithCompletionRecorder(func(record interfaces.FactoryCompletionRecord) {
				completions = append(completions, record)
			}),
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService replay: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run replay: %v", err)
	}

	if len(completions) != 1 {
		t.Fatalf("expected 1 replay completion, got %d", len(completions))
	}
	if completions[0].ObservedTick != 4 {
		t.Fatalf("replay completion observed tick = %d, want 4", completions[0].ObservedTick)
	}
	if got := completions[0].Result.SelectedClassificationLabel; got != "approved" {
		t.Fatalf("replay completion selected classification label = %q, want approved", got)
	}

	state, err := svc.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(state.Marking.TokensInPlace("task:complete")) != 1 {
		t.Fatalf("expected replay token to reach task:complete after tick 4, marking = %#v", state.Marking.Tokens)
	}
}

func TestBuildFactoryService_ReplayModeUsesRecordedCommandRunnerSideEffects(t *testing.T) {
	sourceDir := t.TempDir()
	writeFactoryJSON(t, sourceDir, minimalFactoryConfig())
	writeScriptWorkerAgentsMDWithCommand(t, sourceDir, "worker-a", "replay-live-command-should-not-run", []string{"ok"})
	writeWorkstationAgentsMD(t, sourceDir, "process")

	artifactPath := filepath.Join(t.TempDir(), "recording.json")
	saveReplayBehaviorArtifact(t, sourceDir, artifactPath, interfaces.WorkDispatch{
		DispatchID:      "recorded-dispatch-command",
		TransitionID:    "process",
		WorkerType:      "worker-a",
		WorkstationName: "process",
		Execution: interfaces.ExecutionMetadata{
			ReplayKey: "process/trace-replay-command/work-replay-command",
			TraceID:   "trace-replay-command",
			WorkIDs:   []string{"work-replay-command"},
		},
	}, interfaces.WorkResult{
		DispatchID:   "recorded-dispatch-command",
		TransitionID: "process",
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "replayed command output",
		Diagnostics: &interfaces.WorkDiagnostics{
			Command: &interfaces.CommandDiagnostic{
				Command:  "replay-live-command-should-not-run",
				Args:     []string{"ok"},
				Stdout:   "replayed command output\n",
				Stderr:   "recorded command details\n",
				ExitCode: 0,
			},
		},
	})

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:        t.TempDir(),
		Logger:     zap.NewNop(),
		ReplayPath: artifactPath,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService replay: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run replay: %v", err)
	}
}

func TestBuildFactoryService_ReplayModeStopsOnDispatchDivergence(t *testing.T) {
	sourceDir := t.TempDir()
	writeFactoryJSON(t, sourceDir, minimalFactoryConfig())
	writeScriptWorkerAgentsMD(t, sourceDir, "worker-a")
	writeWorkstationAgentsMD(t, sourceDir, "process")

	artifactPath := filepath.Join(t.TempDir(), "recording.json")
	saveReplayBehaviorArtifact(t, sourceDir, artifactPath, interfaces.WorkDispatch{
		DispatchID:      "recorded-dispatch-mismatch",
		TransitionID:    "review",
		WorkerType:      "worker-a",
		WorkstationName: "review",
		Execution: interfaces.ExecutionMetadata{
			ReplayKey: "review/trace-divergence/work-divergence",
			TraceID:   "trace-divergence",
			WorkIDs:   []string{"work-divergence"},
		},
	}, interfaces.WorkResult{
		DispatchID:   "recorded-dispatch-mismatch",
		TransitionID: "review",
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "recorded output",
		Diagnostics: &interfaces.WorkDiagnostics{
			Command: &interfaces.CommandDiagnostic{
				Command:  "echo",
				Args:     []string{"ok"},
				Stdout:   "recorded output\n",
				ExitCode: 0,
			},
		},
	})

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:        t.TempDir(),
		Logger:     zap.NewNop(),
		ReplayPath: artifactPath,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService replay: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = svc.Run(ctx)
	if err == nil {
		t.Fatal("expected replay divergence error")
	}
	var divergence *replay.DivergenceError
	if !errors.As(err, &divergence) {
		t.Fatalf("Run error is not replay divergence: %T %v", err, err)
	}
	if divergence.Report.Category != replay.DivergenceCategoryDispatchMismatch {
		t.Fatalf("divergence category = %q, want %q", divergence.Report.Category, replay.DivergenceCategoryDispatchMismatch)
	}
	if !strings.Contains(divergence.Report.Expected, "transition=review") {
		t.Fatalf("expected divergence report to include recorded transition: %#v", divergence.Report)
	}
}

func TestBuildFactoryService_ReplayModeWarnsOnCurrentConfigHashMismatch(t *testing.T) {
	sourceDir := t.TempDir()
	writeFactoryJSON(t, sourceDir, minimalFactoryConfig())
	writeScriptWorkerAgentsMD(t, sourceDir, "worker-a")
	writeWorkstationAgentsMD(t, sourceDir, "process")

	artifactPath := filepath.Join(t.TempDir(), "recording.json")
	saveReplayBehaviorArtifact(t, sourceDir, artifactPath, interfaces.WorkDispatch{
		DispatchID:      "recorded-dispatch-warning",
		TransitionID:    "process",
		WorkerType:      "worker-a",
		WorkstationName: "process",
		Execution: interfaces.ExecutionMetadata{
			ReplayKey: "process/trace-warning/work-warning",
			TraceID:   "trace-warning",
			WorkIDs:   []string{"work-warning"},
		},
	}, interfaces.WorkResult{
		DispatchID:   "recorded-dispatch-warning",
		TransitionID: "process",
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "recorded output",
		Diagnostics: &interfaces.WorkDiagnostics{
			Command: &interfaces.CommandDiagnostic{
				Command:  "echo",
				Args:     []string{"ok"},
				Stdout:   "recorded output\n",
				ExitCode: 0,
			},
		},
	})

	mismatchedConfig := minimalFactoryConfig()
	mismatchedConfig["workers"] = []map[string]string{{"name": "worker-a"}, {"name": "worker-b"}}
	writeFactoryJSON(t, sourceDir, mismatchedConfig)

	core, observedLogs := observer.New(zap.WarnLevel)
	_, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:        sourceDir,
		Logger:     zap.New(core),
		ReplayPath: artifactPath,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService replay: %v", err)
	}

	warnings := observedLogs.FilterMessage("replay artifact metadata differs from current checkout").All()
	if len(warnings) == 0 {
		t.Fatal("expected replay metadata mismatch warning")
	}
}

func saveReplayBehaviorArtifact(t *testing.T, sourceDir, artifactPath string, dispatch interfaces.WorkDispatch, result interfaces.WorkResult) {
	t.Helper()

	createdTick := dispatch.Execution.DispatchCreatedTick
	if createdTick == 0 {
		createdTick = 1
		dispatch.Execution.DispatchCreatedTick = createdTick
	}

	loaded, err := config.LoadRuntimeConfig(sourceDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	recordedAt := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	artifact := newReplayArtifactFromLoadedFactory(t, recordedAt, loaded)
	artifact.Events = append(artifact.Events,
		serviceReplayWorkRequestEvent(t, "recorded-submission", 1, "recorded-artifact", serviceReplayWorksFromDispatch(dispatch), nil),
		serviceReplayDispatchCreatedEvent(t, dispatch, createdTick),
		serviceReplayDispatchCompletedEvent(t, "recorded-completion", result, 3),
	)
	if err := replay.Save(artifactPath, artifact); err != nil {
		t.Fatalf("Save replay artifact: %v", err)
	}
}

func newReplayArtifactFromLoadedFactory(t *testing.T, recordedAt time.Time, loaded *config.LoadedFactoryConfig) *interfaces.ReplayArtifact {
	t.Helper()

	generatedFactory, err := replay.GeneratedFactoryFromLoadedConfig(
		loaded,
		replay.WithGeneratedFactorySourceDirectory(loaded.FactoryDir()),
	)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromLoadedConfig: %v", err)
	}
	artifact, err := replay.NewEventLogArtifactFromFactory(recordedAt, generatedFactory, nil, interfaces.ReplayDiagnostics{})
	if err != nil {
		t.Fatalf("NewEventLogArtifactFromFactory: %v", err)
	}
	return artifact
}

func serviceReplayDispatchCreatedEvent(t *testing.T, dispatch interfaces.WorkDispatch, tick int) factoryapi.FactoryEvent {
	t.Helper()
	metadata := map[string]string{}
	if dispatch.Execution.ReplayKey != "" {
		metadata["replayKey"] = dispatch.Execution.ReplayKey
	}
	payload := factoryapi.DispatchRequestEventPayload{
		TransitionId: dispatch.TransitionID,
		Inputs:       serviceReplayDispatchInputRefsFromDispatch(dispatch),
		Resources:    serviceReplayResourcesFromDispatch(dispatch),
		Metadata:     serviceDispatchRequestMetadata(metadata),
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchRequestEventPayload(payload); err != nil {
		t.Fatalf("encode dispatch created event: %v", err)
	}
	return factoryapi.FactoryEvent{
		Id:            "factory-event/dispatch-created/" + dispatch.DispatchID,
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeDispatchRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime:  time.Date(2026, time.April, 10, 12, 0, tick, 0, time.UTC),
			Tick:       tick,
			DispatchId: serviceStringPtr(dispatch.DispatchID),
			RequestId:  serviceStringPtr(dispatch.Execution.RequestID),
			TraceIds:   serviceStringSlicePtr([]string{dispatch.Execution.TraceID}),
			WorkIds:    serviceStringSlicePtr(dispatch.Execution.WorkIDs),
		},
		Payload: union,
	}
}

func serviceReplayWorkRequestEvent(t *testing.T, requestID string, tick int, source string, works []factoryapi.Work, relations []factoryapi.Relation) factoryapi.FactoryEvent {
	t.Helper()
	payload := factoryapi.WorkRequestEventPayload{
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works:     serviceSlicePtr(works),
		Relations: serviceSlicePtr(relations),
		Source:    serviceStringPtr(source),
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromWorkRequestEventPayload(payload); err != nil {
		t.Fatalf("encode work request event: %v", err)
	}
	var traceIDs []string
	var workIDs []string
	for _, work := range works {
		traceIDs = append(traceIDs, serviceStringValue(work.TraceId))
		workIDs = append(workIDs, serviceStringValue(work.WorkId))
	}
	return factoryapi.FactoryEvent{
		Id:            "factory-event/work-request/" + requestID,
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeWorkRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime: time.Date(2026, time.April, 10, 12, 0, tick, 0, time.UTC),
			Tick:      tick,
			RequestId: serviceStringPtr(requestID),
			Source:    serviceStringPtr(source),
			TraceIds:  serviceStringSlicePtr(serviceUniqueNonEmpty(traceIDs)),
			WorkIds:   serviceStringSlicePtr(serviceUniqueNonEmpty(workIDs)),
		},
		Payload: union,
	}
}

func serviceReplayDispatchCompletedEvent(t *testing.T, completionID string, result interfaces.WorkResult, tick int) factoryapi.FactoryEvent {
	t.Helper()
	payload := factoryapi.DispatchResponseEventPayload{
		CompletionId:                serviceStringPtr(completionID),
		TransitionId:                result.TransitionID,
		Outcome:                     factoryapi.WorkOutcome(result.Outcome),
		Output:                      serviceStringPtr(result.Output),
		Error:                       serviceStringPtr(result.Error),
		Feedback:                    serviceStringPtr(result.Feedback),
		SelectedClassificationLabel: serviceStringPtr(result.SelectedClassificationLabel),
		ProviderFailure:             serviceProviderFailurePtr(result.ProviderFailure),
		Metrics:                     serviceWorkMetricsPtr(result.Metrics),
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchResponseEventPayload(payload); err != nil {
		t.Fatalf("encode dispatch completed event: %v", err)
	}
	return factoryapi.FactoryEvent{
		Id:            "factory-event/dispatch-completed/" + result.DispatchID,
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeDispatchResponse,
		Context: factoryapi.FactoryEventContext{
			EventTime:  time.Date(2026, time.April, 10, 12, 0, tick, 0, time.UTC),
			Tick:       tick,
			DispatchId: serviceStringPtr(result.DispatchID),
		},
		Payload: union,
	}
}

func serviceReplayWorkRequestEvents(t *testing.T, artifact *interfaces.ReplayArtifact) []serviceReplayWorkRequestRecord {
	t.Helper()
	var out []serviceReplayWorkRequestRecord
	for _, event := range artifact.Events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil {
			t.Fatalf("decode work request event %q: %v", event.Id, err)
		}
		out = append(out, serviceReplayWorkRequestRecord{Event: event, Payload: payload})
	}
	return out
}

type serviceReplayWorkRequestRecord struct {
	Event   factoryapi.FactoryEvent
	Payload factoryapi.WorkRequestEventPayload
}

func serviceReplayDispatchCreatedEvents(t *testing.T, artifact *interfaces.ReplayArtifact) []serviceReplayDispatchCreatedRecord {
	t.Helper()
	var out []serviceReplayDispatchCreatedRecord
	for _, event := range artifact.Events {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch created event %q: %v", event.Id, err)
		}
		out = append(out, serviceReplayDispatchCreatedRecord{Event: event, Payload: payload})
	}
	return out
}

type serviceReplayDispatchCreatedRecord struct {
	Event   factoryapi.FactoryEvent
	Payload factoryapi.DispatchRequestEventPayload
}

func serviceReplayDispatchCompletedEvents(t *testing.T, artifact *interfaces.ReplayArtifact) []serviceReplayDispatchCompletedRecord {
	t.Helper()
	var out []serviceReplayDispatchCompletedRecord
	for _, event := range artifact.Events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch completed event %q: %v", event.Id, err)
		}
		out = append(out, serviceReplayDispatchCompletedRecord{Event: event, Payload: payload})
	}
	return out
}

type serviceReplayDispatchCompletedRecord struct {
	Event   factoryapi.FactoryEvent
	Payload factoryapi.DispatchResponseEventPayload
}

func serviceReplayInferenceResponseEvents(t *testing.T, artifact *interfaces.ReplayArtifact) []serviceReplayInferenceResponseRecord {
	t.Helper()
	var out []serviceReplayInferenceResponseRecord
	for _, event := range artifact.Events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response event %q: %v", event.Id, err)
		}
		out = append(out, serviceReplayInferenceResponseRecord{Event: event, Payload: payload})
	}
	return out
}

type serviceReplayInferenceResponseRecord struct {
	Event   factoryapi.FactoryEvent
	Payload factoryapi.InferenceResponseEventPayload
}

func serviceReplayWorksFromDispatch(dispatch interfaces.WorkDispatch) []factoryapi.Work {
	tokens := workers.WorkDispatchInputTokens(dispatch)
	works := make([]factoryapi.Work, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == interfaces.DataTypeResource {
			continue
		}
		workID := firstNonEmpty(token.Color.WorkID, token.ID)
		works = append(works, factoryapi.Work{
			Name:         firstNonEmpty(token.Color.Name, workID),
			WorkId:       serviceStringPtr(workID),
			WorkTypeName: serviceStringPtr(token.Color.WorkTypeID),
			TraceId:      serviceStringPtr(token.Color.TraceID),
			Tags:         serviceStringMapPtr(token.Color.Tags),
		})
	}
	if len(works) == 0 {
		for _, workID := range dispatch.Execution.WorkIDs {
			works = append(works, factoryapi.Work{
				Name:         workID,
				WorkId:       serviceStringPtr(workID),
				WorkTypeName: serviceStringPtr("task"),
				TraceId:      serviceStringPtr(dispatch.Execution.TraceID),
			})
		}
	}
	return works
}

func serviceReplayDispatchInputRefsFromDispatch(dispatch interfaces.WorkDispatch) []factoryapi.DispatchConsumedWorkRef {
	tokens := workers.WorkDispatchInputTokens(dispatch)
	refs := make([]factoryapi.DispatchConsumedWorkRef, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == interfaces.DataTypeResource {
			continue
		}
		workID := firstNonEmpty(token.Color.WorkID, token.ID)
		if workID == "" {
			continue
		}
		refs = append(refs, factoryapi.DispatchConsumedWorkRef{WorkId: workID})
	}
	if len(refs) == 0 {
		for _, workID := range dispatch.Execution.WorkIDs {
			if workID == "" {
				continue
			}
			refs = append(refs, factoryapi.DispatchConsumedWorkRef{WorkId: workID})
		}
	}
	return refs
}

func serviceReplayResourcesFromDispatch(dispatch interfaces.WorkDispatch) *[]factoryapi.Resource {
	tokens := workers.WorkDispatchInputTokens(dispatch)
	resources := make([]factoryapi.Resource, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType != interfaces.DataTypeResource {
			continue
		}
		resources = append(resources, factoryapi.Resource{Name: firstNonEmpty(token.Color.WorkTypeID, token.Color.Name)})
	}
	return serviceSlicePtr(resources)
}

func serviceDispatchRequestMetadata(values map[string]string) *factoryapi.DispatchRequestEventMetadata {
	if len(values) == 0 {
		return nil
	}
	return &factoryapi.DispatchRequestEventMetadata{
		ReplayKey: serviceStringPtr(values["replayKey"]),
	}
}

func serviceProviderFailurePtr(failure *interfaces.ProviderFailureMetadata) *factoryapi.ProviderFailureMetadata {
	return interfaces.GeneratedProviderFailureMetadata(failure)
}

func serviceWorkMetricsPtr(metrics interfaces.WorkMetrics) *factoryapi.WorkMetrics {
	if metrics.Duration == 0 && metrics.Cost == 0 && metrics.RetryCount == 0 {
		return nil
	}
	return &factoryapi.WorkMetrics{
		DurationNanos: serviceInt64Ptr(metrics.Duration.Nanoseconds()),
		Cost:          serviceFloat64Ptr(metrics.Cost),
		RetryCount:    serviceIntPtr(metrics.RetryCount),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func serviceStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func serviceFirstStringValue(values *[]string) string {
	if values == nil {
		return ""
	}
	for _, value := range *values {
		if value != "" {
			return value
		}
	}
	return ""
}

func serviceUniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func serviceStringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func serviceEnumPtr[T ~string](value T) *T {
	if value == "" {
		return nil
	}
	return &value
}

func serviceIntPtr(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func serviceInt64Ptr(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

func serviceFloat64Ptr(value float64) *float64 {
	if value == 0 {
		return nil
	}
	return &value
}

func serviceStringSlicePtr(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return &out
}

func serviceStringMapPtr(values map[string]string) *factoryapi.StringMap {
	if len(values) == 0 {
		return nil
	}
	converted := factoryapi.StringMap{}
	for key, value := range values {
		if value != "" {
			converted[key] = value
		}
	}
	if len(converted) == 0 {
		return nil
	}
	return &converted
}

func serviceSlicePtr[T any](values []T) *[]T {
	if len(values) == 0 {
		return nil
	}
	out := append([]T(nil), values...)
	return &out
}

func assertServiceFactoryEventsContainTypes(t *testing.T, events []factoryapi.FactoryEvent, wantTypes []factoryapi.FactoryEventType) {
	t.Helper()
	seen := make(map[factoryapi.FactoryEventType]bool, len(events))
	for _, event := range events {
		seen[event.Type] = true
	}
	for _, wantType := range wantTypes {
		if !seen[wantType] {
			t.Fatalf("factory event types = %v, want %s", serviceFactoryEventTypes(events), wantType)
		}
	}
}

func serviceFactoryEventTypes(events []factoryapi.FactoryEvent) []factoryapi.FactoryEventType {
	types := make([]factoryapi.FactoryEventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}
