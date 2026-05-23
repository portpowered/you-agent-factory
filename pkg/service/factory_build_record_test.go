package service

import (
	"context"
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
	"github.com/portpowered/infinite-you/pkg/replay"
	"go.uber.org/zap"
)

func TestBuildFactoryService_LoadsWorkersFromConfig(t *testing.T) {
	dir := t.TempDir()

	// Config with a "worker-a" worker entry.
	cfg := map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
	writeFactoryJSON(t, dir, cfg)
	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "process")

	ctx := context.Background()
	svc, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestBuildFactoryService_WorkerWithoutAgentsMD_SkippedSilently(t *testing.T) {
	dir := t.TempDir()

	// Config with a "worker-a" worker entry, but no AGENTS.md on disk.
	cfg := map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
	writeFactoryJSON(t, dir, cfg)
	writeWorkstationAgentsMD(t, dir, "process")
	// No worker AGENTS.md — worker should be silently skipped.

	ctx := context.Background()
	_, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService should succeed even with no AGENTS.md: %v", err)
	}
}

func TestBuildFactoryService_MissingFactoryJSON(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	_, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:    dir,
		Logger: zap.NewNop(),
	})
	if err == nil {
		t.Fatal("expected error when factory.json is missing")
	}
}

func TestBuildFactoryService_MockWorkersConfigPassedThrough(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	ctx := context.Background()
	svc, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	snap, err := svc.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snap.FactoryState != string(interfaces.FactoryStateIdle) {
		t.Errorf("expected IDLE state, got %s", snap.FactoryState)
	}
	if svc.cfg.MockWorkersConfig == nil {
		t.Fatal("expected mock-worker config to be preserved")
	}
	if len(svc.cfg.MockWorkersConfig.MockWorkers) != 0 {
		t.Fatalf("mock worker count = %d, want empty default accept config", len(svc.cfg.MockWorkersConfig.MockWorkers))
	}
}

func TestBuildFactoryService_RuntimeModePassedThrough(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	ctx := context.Background()
	svc, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("Run returned before cancellation: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode factory service to stop")
	}
}

func TestBuildFactoryService_RecordModeWritesInitialArtifact(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	recordPath := filepath.Join(t.TempDir(), "recording.json")
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		RecordPath:        recordPath,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	artifact, err := replay.Load(recordPath)
	if err != nil {
		t.Fatalf("Load(recording): %v", err)
	}
	if artifact.Factory.Workers == nil {
		t.Fatal("expected embedded factory config")
	}
	if artifact.Factory.FactoryDirectory == nil || *artifact.Factory.FactoryDirectory != dir {
		t.Fatalf("factory directory = %#v, want %q", artifact.Factory.FactoryDirectory, dir)
	}
}

func TestBuildFactoryService_RecordModeResolvesGeneratedDefaultSessionPathAndCreatesParents(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	recordTemplate := filepath.Join(
		t.TempDir(),
		"recordings",
		"2026-05",
		"2026-05-23",
		"factory-session-__factory_session_id__-184512-uuid-1.json",
	)
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		RecordPath:        recordTemplate,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	resolvedPath := sessionScopedRecordPath(recordTemplate, defaultFactorySessionID)
	if _, err := os.Stat(filepath.Dir(resolvedPath)); err != nil {
		t.Fatalf("Stat(recording dir): %v", err)
	}
	artifact, err := replay.Load(resolvedPath)
	if err != nil {
		t.Fatalf("Load(recording): %v", err)
	}
	if artifact.Factory.FactoryDirectory == nil || *artifact.Factory.FactoryDirectory != dir {
		t.Fatalf("factory directory = %#v, want %q", artifact.Factory.FactoryDirectory, dir)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-runtime-log-fixture review=2026-07-18 removal=split-runtime-log-fixture-before-next-runtime-logging-change
func TestBuildFactoryService_RecordModeRecordsSubmittedWorkAtEngineTick(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "initial-work.json")
	work := interfaces.SubmitRequest{
		WorkTypeID: "task",
		Name:       "from-work-file",
		TraceID:    "trace-work-file",
		Payload:    []byte(`{"task":"record me"}`),
	}
	writeWorkRequestFile(t, workFile, work)

	recordPath := filepath.Join(t.TempDir(), "recording.json")
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		RecordPath:        recordPath,
		WorkFile:          workFile,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	artifact, err := replay.Load(recordPath)
	if err != nil {
		t.Fatalf("Load(recording): %v", err)
	}
	assertReplayArtifactStoresCanonicalEvents(t, recordPath, artifact, []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
		factoryapi.FactoryEventTypeWorkRequest,
		factoryapi.FactoryEventTypeDispatchRequest,
		factoryapi.FactoryEventTypeDispatchResponse,
		factoryapi.FactoryEventTypeRunResponse,
	})
	submissions := serviceReplayWorkRequestEvents(t, artifact)
	if len(submissions) != 1 {
		t.Fatalf("expected 1 recorded submission, got %d", len(submissions))
	}
	submission := submissions[0]
	if submission.Event.Context.Tick != 1 {
		t.Fatalf("submission observed tick = %d, want 1", submission.Event.Context.Tick)
	}
	if serviceFirstStringValue(submission.Event.Context.TraceIds) != "trace-work-file" {
		t.Fatalf("recorded trace ID = %q, want trace-work-file", serviceFirstStringValue(submission.Event.Context.TraceIds))
	}
	if serviceStringValue(submission.Payload.Source) != "external-submit" {
		t.Fatalf("recorded source = %q, want external-submit", serviceStringValue(submission.Payload.Source))
	}
	dispatches := serviceReplayDispatchCreatedEvents(t, artifact)
	if len(dispatches) != 1 {
		t.Fatalf("expected 1 recorded dispatch, got %d", len(dispatches))
	}
	dispatch := dispatches[0]
	if dispatch.Event.Context.Tick < submission.Event.Context.Tick {
		t.Fatalf("dispatch created tick = %d, want no earlier than submission tick %d", dispatch.Event.Context.Tick, submission.Event.Context.Tick)
	}
	dispatchID := serviceStringValue(dispatch.Event.Context.DispatchId)
	if dispatchID == "" {
		t.Fatal("expected dispatch context to carry dispatch ID")
	}
	completions := serviceReplayDispatchCompletedEvents(t, artifact)
	if len(completions) != 1 {
		t.Fatalf("expected 1 recorded completion, got %d", len(completions))
	}
	completion := completions[0]
	completionDispatchID := serviceStringValue(completion.Event.Context.DispatchId)
	if completionDispatchID != dispatchID {
		t.Fatalf("completion dispatch ID = %q, want %q", completionDispatchID, dispatchID)
	}
	if completion.Event.Context.Tick < dispatch.Event.Context.Tick {
		t.Fatalf("completion observed tick = %d, want no earlier than dispatch tick %d", completion.Event.Context.Tick, dispatch.Event.Context.Tick)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this helper intentionally validates the replay artifact event contract in one place.
func assertReplayArtifactStoresCanonicalEvents(t *testing.T, path string, artifact *interfaces.ReplayArtifact, wantSubsequence []factoryapi.FactoryEventType) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read recording: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("recording is not JSON: %v", err)
	}
	for _, key := range []string{"schemaVersion", "recordedAt", "events"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("recording missing top-level %s: %s", key, data)
		}
	}
	for _, legacyKey := range []string{"schema_version", "recorded_at", "work_requests", "submissions", "dispatches", "completions"} {
		if _, ok := raw[legacyKey]; ok {
			t.Fatalf("recording persisted legacy top-level key %q: %s", legacyKey, data)
		}
	}
	for _, legacyConfigKey := range forbiddenReplayConfigStorageKeys() {
		if strings.Contains(string(data), legacyConfigKey) {
			t.Fatalf("recording persisted legacy config key %q: %s", legacyConfigKey, data)
		}
	}
	if len(artifact.Events) == 0 {
		t.Fatal("recording has no canonical events")
	}
	for i, event := range artifact.Events {
		if event.Context.Sequence != i {
			t.Fatalf("event %d sequence = %d, want %d", i, event.Context.Sequence, i)
		}
	}
	types := make([]factoryapi.FactoryEventType, 0, len(artifact.Events))
	for _, event := range artifact.Events {
		types = append(types, event.Type)
	}
	next := 0
	for _, eventType := range types {
		if next < len(wantSubsequence) && eventType == wantSubsequence[next] {
			next++
		}
	}
	if next != len(wantSubsequence) {
		t.Fatalf("recording event types = %v, want subsequence %v", types, wantSubsequence)
	}
}

func forbiddenReplayConfigStorageKeys() []string {
	return []string{
		strings.Join([]string{"effective", "Config"}, ""),
		strings.Join([]string{"__replay", "Effective", "Config"}, ""),
		strings.Join([]string{"runtime", "Worker", "Config"}, ""),
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this record-mode runtime test keeps late artifact streaming assertions together on the service seam.
func TestBuildFactoryService_RecordModeStreamsReadableArtifactBeforeShutdown(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "initial-work.json")
	writeWorkRequestFile(t, workFile, interfaces.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    "trace-streamed-recording",
		Payload:    []byte(`{"task":"record before shutdown"}`),
	})

	recordPath := filepath.Join(t.TempDir(), "recording.json")
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                 dir,
		RuntimeMode:         interfaces.RuntimeModeService,
		MockWorkersConfig:   config.NewEmptyMockWorkersConfig(),
		Logger:              zap.NewNop(),
		RecordPath:          recordPath,
		RecordFlushInterval: 10 * time.Millisecond,
		WorkFile:            workFile,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()
	defer func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("Run after cancellation: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for service-mode factory service to stop")
		}
	}()

	deadline := time.Now().Add(serviceStreamedRecordingTimeout)
	lastArtifactSummary := "artifact not readable yet"
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			t.Fatalf("Run returned before shutdown: %v", err)
		default:
		}

		artifact, err := replay.Load(recordPath)
		if err == nil &&
			len(serviceReplayWorkRequestEvents(t, artifact)) == 1 &&
			len(serviceReplayDispatchCreatedEvents(t, artifact)) == 1 &&
			len(serviceReplayDispatchCompletedEvents(t, artifact)) == 1 {
			if artifact.WallClock != nil && !artifact.WallClock.FinishedAt.IsZero() {
				t.Fatal("streamed artifact should not have final wall-clock metadata before shutdown")
			}
			return
		}
		if err != nil {
			lastArtifactSummary = err.Error()
		} else {
			lastArtifactSummary = fmt.Sprintf(
				"work_requests=%d dispatch_created=%d dispatch_completed=%d",
				len(serviceReplayWorkRequestEvents(t, artifact)),
				len(serviceReplayDispatchCreatedEvents(t, artifact)),
				len(serviceReplayDispatchCompletedEvents(t, artifact)),
			)
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf(
		"record mode did not stream a readable artifact before shutdown within %s: %s",
		serviceStreamedRecordingTimeout,
		lastArtifactSummary,
	)
}

func TestBuildFactoryService_RecordModeCopiesWorkerDiagnosticsToArtifact(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "initial-work.json")
	work := interfaces.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    "trace-diagnostics",
		Payload:    []byte(`{"task":"record diagnostics"}`),
	}
	writeWorkRequestFile(t, workFile, work)

	recordPath := filepath.Join(t.TempDir(), "recording.json")
	provider := &recordingDiagnosticsProvider{}
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:              dir,
		Logger:           zap.NewNop(),
		RecordPath:       recordPath,
		WorkFile:         workFile,
		ProviderOverride: provider,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	artifact, err := replay.Load(recordPath)
	if err != nil {
		t.Fatalf("Load(recording): %v", err)
	}
	completions := serviceReplayDispatchCompletedEvents(t, artifact)
	if len(completions) != 1 {
		t.Fatalf("expected 1 recorded completion, got %d", len(completions))
	}
	inferenceResponses := serviceReplayInferenceResponseEvents(t, artifact)
	if len(inferenceResponses) != 1 {
		t.Fatalf("expected 1 recorded inference response, got %d", len(inferenceResponses))
	}
	diagnostics := inferenceResponses[0].Payload.Diagnostics
	if diagnostics == nil || diagnostics.Provider == nil {
		t.Fatal("expected provider diagnostics on recorded inference response")
	}
	if diagnostics.Provider.ResponseMetadata == nil || (*diagnostics.Provider.ResponseMetadata)["request_id"] != "provider-request-1" {
		t.Fatalf("recorded inference response metadata = %#v", diagnostics.Provider.ResponseMetadata)
	}
	if diagnostics.RenderedPrompt == nil || serviceStringValue(diagnostics.RenderedPrompt.UserMessageHash) == "" {
		t.Fatal("expected rendered prompt metadata on recorded inference response")
	}
}

func TestBuildFactoryService_RecordModeCopiesScriptDiagnosticsToArtifact(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeScriptWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "initial-work.json")
	writeWorkRequestFile(t, workFile, interfaces.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    "trace-script-diagnostics",
		Payload:    []byte(`{"task":"record script diagnostics"}`),
	})

	recordPath := filepath.Join(t.TempDir(), "recording.json")
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                   dir,
		Logger:                zap.NewNop(),
		RecordPath:            recordPath,
		WorkFile:              workFile,
		CommandRunnerOverride: recordingDiagnosticsCommandRunner{},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	artifact, err := replay.Load(recordPath)
	if err != nil {
		t.Fatalf("Load(recording): %v", err)
	}
	completions := serviceReplayDispatchCompletedEvents(t, artifact)
	if len(completions) != 1 {
		t.Fatalf("expected 1 recorded completion, got %d", len(completions))
	}
	inferenceResponses := serviceReplayInferenceResponseEvents(t, artifact)
	if len(inferenceResponses) != 0 {
		t.Fatalf("script workers should not record inference responses, got %d", len(inferenceResponses))
	}
	completion := completions[0].Payload
	if serviceStringValue(completion.Output) != "script done" {
		t.Fatalf("recorded script output = %q", serviceStringValue(completion.Output))
	}
}
