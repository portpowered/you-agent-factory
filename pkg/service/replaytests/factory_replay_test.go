package replaytests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
	service "github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

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
	svc, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
		Dir:               replayDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		ReplayPath:        artifactPath,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService replay: %v", err)
	}

	if _, err := svc.GetEngineStateSnapshot(context.Background()); err != nil {
		t.Fatalf("expected replay service to build an inspectable runtime from embedded config: %v", err)
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

	svc, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
		Dir:               t.TempDir(),
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		ReplayPath:        artifactPath,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService replay: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run replay with deterministic default clock: %v", err)
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

	svc, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
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
	svc, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
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

	svc, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
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

	svc, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
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
	_, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
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
		DurationMillis: serviceInt64Ptr(metrics.Duration.Milliseconds()),
		Cost:           serviceFloat64Ptr(metrics.Cost),
		RetryCount:     serviceIntPtr(metrics.RetryCount),
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

func minimalFactoryConfig() map[string]any {
	return map[string]any{
		"name": "test-factory",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":    "process",
			"worker":  "worker-a",
			"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
			"outputs": []map[string]string{{"workType": "task", "state": "complete"}},
		}},
	}
}

func writeFactoryJSON(t *testing.T, dir string, cfg map[string]any) {
	t.Helper()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func writeWorkerAgentsMD(t *testing.T, factoryDir, workerName string) {
	t.Helper()
	writeWorkerAgentsMDWithContent(t, factoryDir, workerName, "---\ntype: MODEL_WORKER\nmodel: claude-3-5-haiku-20241022\n---\nYou are a helpful assistant.\n")
}

func writeScriptWorkerAgentsMD(t *testing.T, factoryDir, workerName string) {
	t.Helper()
	writeScriptWorkerAgentsMDWithCommand(t, factoryDir, workerName, "echo", []string{"ok"})
}

func writeScriptWorkerAgentsMDWithCommand(t *testing.T, factoryDir, workerName, command string, args []string) {
	t.Helper()
	var argsYAML strings.Builder
	for _, arg := range args {
		argsYAML.WriteString("  - ")
		argsYAML.WriteString(strconv.Quote(arg))
		argsYAML.WriteString("\n")
	}
	writeWorkerAgentsMDWithContent(t, factoryDir, workerName, fmt.Sprintf("---\ntype: SCRIPT_WORKER\ncommand: %s\nargs:\n%s---\n", command, argsYAML.String()))
}

func writeWorkerAgentsMDWithContent(t *testing.T, factoryDir, workerName, content string) {
	t.Helper()
	workerDir := filepath.Join(factoryDir, "workers", workerName)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("create worker dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
}

func writeWorkstationAgentsMD(t *testing.T, factoryDir, workstationName string) {
	t.Helper()
	workstationDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(workstationDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	agentsMD := "---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"
	if err := os.WriteFile(filepath.Join(workstationDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}
