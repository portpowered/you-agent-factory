// pkgmaintcheck:ignore-file-lines consolidated same-package service tests remain on root-only runtime seams until dedicated service test seams are extracted.
// backendsizecheck:ignore-file consolidated same-package service tests remain on root-only runtime seams until dedicated service test seams are extracted.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/api"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"go.uber.org/zap"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	rootDir, err := os.MkdirTemp("", "pkg-service-test-env-*")
	if err != nil {
		panic(err)
	}

	homeDir := filepath.Join(rootDir, "home")
	tempDir := filepath.Join(homeDir, "tmp")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		panic(err)
	}

	for key, value := range map[string]string{
		"HOME":        homeDir,
		"USERPROFILE": homeDir,
		"HOMEDRIVE":   filepath.VolumeName(homeDir),
		"HOMEPATH":    homeDir,
		"TMPDIR":      tempDir,
		"TMP":         tempDir,
		"TEMP":        tempDir,
	} {
		if err := os.Setenv(key, value); err != nil {
			panic(err)
		}
	}

	code := m.Run()
	_ = os.RemoveAll(rootDir)
	os.Exit(code)
}

func init() {
	testHome := filepath.Join(os.TempDir(), "infinite-you-pkg-service-test-home")
	if err := os.MkdirAll(testHome, 0o755); err != nil {
		panic(err)
	}
	if err := os.Setenv("HOME", testHome); err != nil {
		panic(err)
	}
}

func TestInvokeModelHTTP_UsesManagedLocalModelRuntimePath(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtime := &fakeLocalModelRuntime{
		response: interfaces.InferenceResponse{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		},
	}

	dir := scaffoldLocalModelLongTestFactoryDir(t, localModelLongTestFactoryConfig())
	writeLocalModelLongTestWorkerConfig(t, dir)
	writeLocalModelLongTestWorkstationConfig(t, dir)
	server, shutdown := startLocalModelHTTPTestServer(t, dir, runtime)
	defer shutdown()
	defer server.Close()

	body, err := json.Marshal(factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Bindings: &[]factoryapi.WorkstationOperationBinding{{
			Slot: "text",
			Selector: &factoryapi.WorkstationOperationBindingSelector{
				Type: func() *factoryapi.ModelOperationContentType {
					value := factoryapi.ModelOperationContentTypeText
					return &value
				}(),
			},
		}},
		Content: &factoryapi.WorkContent{
			mustGeneratedLocalModelHTTPTextPart(t, "hello http local model"),
		},
	})
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}
	decoded := invokeLocalModelHTTP(t, server, body)
	assertLocalModelHTTPInvocationResponse(t, decoded, audioPath)
	if runtime.loadCount() != 1 || runtime.invocationCount() != 1 {
		t.Fatalf("runtime load/invoke counts = %d/%d, want 1/1", runtime.loadCount(), runtime.invocationCount())
	}
}

func TestBuildFactoryService_InitializesManagedLocalModelFields(t *testing.T) {
	dir := scaffoldLocalModelLongTestFactoryDir(t, localModelLongTestFactoryConfig())
	writeLocalModelLongTestWorkerConfig(t, dir)
	writeLocalModelLongTestWorkstationConfig(t, dir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:                                     dir,
		RuntimeMode:                             interfaces.RuntimeModeService,
		Port:                                    1,
		Logger:                                  zap.NewNop(),
		LocalModelRuntimeOverride:               &fakeLocalModelRuntime{},
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc.modelAssets == nil {
		t.Fatal("expected BuildFactoryService to initialize modelAssets")
	}
	bundle := svc.currentRuntimeBundle()
	if bundle == nil {
		t.Fatal("expected startup runtime bundle")
	}
	if bundle.modelResources == nil {
		t.Fatal("expected startup bundle to initialize modelResources")
	}
	if bundle.localModels == nil {
		t.Fatal("expected startup bundle to initialize localModels")
	}
	if svc.sessions == nil {
		t.Fatal("expected BuildFactoryService to initialize sessions")
	}
	if svc.hostedWorkers.Logger == nil {
		t.Fatal("expected BuildFactoryService to initialize hostedWorkers logger")
	}
}

func TestBuildHostedWorkersConfig_DelegatesServiceConfigFields(t *testing.T) {
	t.Parallel()

	client := &http.Client{}
	resolver := hostedworkers.SecretResolver(func(context.Context, interfaces.RuntimeConfigLookup, string) (string, error) {
		return "token", nil
	})
	cfg := &FactoryServiceConfig{
		HostedPollerHTTPClient:     client,
		HostedPollerSecretResolver: resolver,
		HostedLinearEndpoint:       " https://linear.example/graphql ",
	}

	got := buildHostedWorkersConfig(cfg, zap.NewNop(), nil)
	if got.HTTPClient != client {
		t.Fatal("hosted workers HTTP client was not wired from FactoryServiceConfig")
	}
	if got.SecretResolver == nil {
		t.Fatal("hosted workers secret resolver was not wired from FactoryServiceConfig")
	}
	if got.LinearEndpoint != "https://linear.example/graphql" {
		t.Fatalf("LinearEndpoint = %q, want trimmed service config endpoint", got.LinearEndpoint)
	}
}

func startLocalModelHTTPTestServer(t *testing.T, dir string, runtime *fakeLocalModelRuntime) (*httptest.Server, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	svc, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:                                     dir,
		RuntimeMode:                             interfaces.RuntimeModeService,
		Port:                                    1,
		Logger:                                  zap.NewNop(),
		LocalModelRuntimeOverride:               runtime,
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	if err != nil {
		cancel()
		t.Fatalf("BuildFactoryService: %v", err)
	}
	puller := staticModelAssetPuller{cache: localModelTestCacheLayout(t)}
	svc.modelAssets = puller
	if bundle := svc.startupRuntimeBundle(); bundle != nil {
		bundle.modelAssets = puller
		bundle.localModels = newManagedLocalModelManager(puller, runtime)
	}

	runErrCh := make(chan error, 1)
	go func() { runErrCh <- svc.Run(ctx) }()
	waitForFactoryServiceRuntimeReady(t, svc)

	server := httptest.NewServer(api.NewServer(svc, 0, zap.NewNop()).Handler())
	shutdown := func() {
		cancel()
		if err := <-runErrCh; err != nil && err != context.Canceled {
			t.Fatalf("svc.Run: %v", err)
		}
	}
	return server, shutdown
}

func invokeLocalModelHTTP(t *testing.T, server *httptest.Server, body []byte) factoryapi.ModelInvocationResponse {
	t.Helper()
	resp, err := http.Post(server.URL+"/models/OMNIVOICE_Q4_K_M/invocations", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /models/.../invocations: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload := new(bytes.Buffer)
		_, _ = payload.ReadFrom(resp.Body)
		t.Fatalf("POST /models/.../invocations status = %d, want 200: %s", resp.StatusCode, payload.String())
	}

	var decoded factoryapi.ModelInvocationResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation response: %v", err)
	}
	return decoded
}

func assertLocalModelHTTPInvocationResponse(t *testing.T, decoded factoryapi.ModelInvocationResponse, audioPath string) {
	t.Helper()
	if decoded.ModelName != "OMNIVOICE_Q4_K_M" || decoded.Operation != "TTS" || decoded.ProviderLocality != factoryapi.WorkerModelLocalityLocal {
		t.Fatalf("invocation identity = %#v, want OMNIVOICE local TTS", decoded)
	}
	if len(decoded.Content) != 1 {
		t.Fatalf("response content count = %d, want 1", len(decoded.Content))
	}
	audioPart, err := decoded.Content[0].AsWorkAudioContentPart()
	if err != nil {
		t.Fatalf("decode response audio content: %v", err)
	}
	if audioPart.File != audioPath || stringValue(audioPart.ContentType) != "audio/wav" {
		t.Fatalf("response audio part = %#v, want file %q and audio/wav", audioPart, audioPath)
	}
}

func waitForFactoryServiceRuntimeReady(t *testing.T, svc *FactoryService) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := svc.GetEngineStateSnapshot(context.Background())
		if err == nil && snapshot.FactoryState == string(interfaces.FactoryStateRunning) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for factory service runtime readiness")
}

func scaffoldLocalModelLongTestFactoryDir(t *testing.T, cfg *interfaces.FactoryConfig) string {
	t.Helper()
	dir := t.TempDir()
	data, err := factoryconfig.MarshalCanonicalFactoryConfig(cfg)
	if err != nil {
		t.Fatalf("marshal factory config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("write factory config: %v", err)
	}
	return dir
}

func localModelLongTestFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		Name: "omnivoice-local-model-test",
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "speech",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Resources: []interfaces.ResourceConfig{{
			Name:       "omnivoice-cache",
			Type:       interfaces.ResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
		Workers: []interfaces.WorkerConfig{{
			Name:          "tts-worker",
			Type:          interfaces.WorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelProvider: interfaces.RunnerIDCodex,
			ModelLocality: interfaces.ModelLocalityLocal,
			Command:       localmodels.DefaultOmniVoiceCommand,
			Resources: []interfaces.ResourceConfig{{
				Name:     "omnivoice-cache",
				Capacity: 1,
			}},
			Operations: []interfaces.ModelOperation{{
				Name: "TTS",
				Inputs: []interfaces.ModelOperationSlot{{
					Name:         "text",
					ContentTypes: []string{interfaces.ModelOperationContentTypeText},
					Required:     true,
				}},
				Outputs: []interfaces.ModelOperationSlot{{
					Name:         "audio",
					ContentTypes: []string{interfaces.ModelOperationContentTypeAudio},
				}},
			}},
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "speak",
			Type:           interfaces.WorkstationTypeInvoke,
			WorkerTypeName: "tts-worker",
			Operation:      "TTS",
			OperationBindings: []interfaces.ModelOperationBinding{{
				Slot: "text",
				Selector: &interfaces.ModelOperationBindingSelector{
					Type: interfaces.ModelOperationContentTypeText,
				},
			}},
			Inputs:    []interfaces.IOConfig{{WorkTypeName: "speech", StateName: "init"}},
			Outputs:   []interfaces.IOConfig{{WorkTypeName: "speech", StateName: "complete"}},
			OnFailure: []interfaces.IOConfig{{WorkTypeName: "speech", StateName: "failed"}},
		}},
	}
}

func writeLocalModelLongTestWorkerConfig(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "workers", "tts-worker", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("Synthesize speech from the resolved text content.\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeLocalModelLongTestWorkstationConfig(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "workstations", "speak", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("Generate speech output.\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustGeneratedLocalModelHTTPTextPart(t *testing.T, text string) factoryapi.WorkContentPart {
	t.Helper()
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeTextUpper,
		Text: text,
	}); err != nil {
		t.Fatalf("build http text content part: %v", err)
	}
	return part
}

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
		MockWorkersConfig: factoryconfig.NewEmptyMockWorkersConfig(),
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
		MockWorkersConfig: factoryconfig.NewEmptyMockWorkersConfig(),
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
		MockWorkersConfig: factoryconfig.NewEmptyMockWorkersConfig(),
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
		MockWorkersConfig: factoryconfig.NewEmptyMockWorkersConfig(),
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
		MockWorkersConfig: factoryconfig.NewEmptyMockWorkersConfig(),
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
		MockWorkersConfig: factoryconfig.NewEmptyMockWorkersConfig(),
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
		MockWorkersConfig: factoryconfig.NewEmptyMockWorkersConfig(),
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
		MockWorkersConfig:   factoryconfig.NewEmptyMockWorkersConfig(),
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

type providerCallRecorder struct {
	mu        sync.Mutex
	calls     []interfaces.ProviderInferenceRequest
	responses []interfaces.InferenceResponse
}

func (p *providerCallRecorder) Infer(_ context.Context, req interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.calls = append(p.calls, interfaces.CloneProviderInferenceRequest(req))
	if len(p.responses) == 0 {
		return interfaces.InferenceResponse{Content: "ok"}, nil
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	return response, nil
}

func (p *providerCallRecorder) Calls() []interfaces.ProviderInferenceRequest {
	p.mu.Lock()
	defer p.mu.Unlock()

	calls := make([]interfaces.ProviderInferenceRequest, len(p.calls))
	for i, call := range p.calls {
		calls[i] = interfaces.CloneProviderInferenceRequest(call)
	}
	return calls
}

type providerCommandRunnerRecorder struct {
	mu       sync.Mutex
	requests []workers.CommandRequest
	result   workers.CommandResult
}

func (r *providerCommandRunnerRecorder) Run(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.requests = append(r.requests, workers.CommandRequest(interfaces.CloneSubprocessExecutionRequest(req)))
	return r.result, nil
}

func (r *providerCommandRunnerRecorder) Requests() []workers.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()

	requests := make([]workers.CommandRequest, len(r.requests))
	copy(requests, r.requests)
	return requests
}

func TestLoadWorkersFromConfig_PromptTemplateFromBody(t *testing.T) {
	dir := t.TempDir()

	expectedPrompt := "You are a design reviewer. Evaluate the design for {{ .Payload }}."
	writeWorkstationAgentsMDWithPrompt(t, dir, "review", expectedPrompt)
	writeWorkerAgentsMD(t, dir, "worker-a")

	factoryCfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	}
	cfg := newLoadedFactoryConfigForServiceTest(t, dir, factoryCfg,
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	opts, err := loadWorkersFromConfigForServiceTest(cfg.FactoryDir(), cfg.FactoryConfig(), "", cfg, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["worker-a"]
	if !ok {
		t.Fatal("expected worker-a executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
	wsDef, ok := wsExec.RuntimeConfig.Workstation("review")
	if !ok {
		t.Fatal("expected 'review' workstation in runtime config")
	}
	if wsDef.PromptTemplate != expectedPrompt {
		t.Errorf("expected prompt template %q, got %q", expectedPrompt, wsDef.PromptTemplate)
	}
}

func TestLoadWorkersFromConfig_PromptTemplateFromFile(t *testing.T) {
	dir := t.TempDir()

	expectedPrompt := "Custom prompt loaded from file: {{ .WorkID }}"
	writeWorkstationAgentsMDWithPromptFile(t, dir, "review", "prompt.md", expectedPrompt)
	writeWorkerAgentsMD(t, dir, "worker-a")

	factoryCfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	}
	cfg := newLoadedFactoryConfigForServiceTest(t, dir, factoryCfg,
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	opts, err := loadWorkersFromConfigForServiceTest(cfg.FactoryDir(), cfg.FactoryConfig(), "", cfg, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["worker-a"]
	if !ok {
		t.Fatal("expected worker-a executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
	wsDef, ok := wsExec.RuntimeConfig.Workstation("review")
	if !ok {
		t.Fatal("expected 'review' workstation in runtime config")
	}
	if wsDef.PromptTemplate != expectedPrompt {
		t.Errorf("expected prompt template %q, got %q", expectedPrompt, wsDef.PromptTemplate)
	}
}

func TestLoadWorkersFromConfig_ModelWorkerWithCanonicalExecutorProviderUsesAgentExecutorPath(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "worker-a", `---
type: MODEL_WORKER
model: gpt-5.4
executorProvider: script_wrap
modelProvider: codex
stopToken: COMPLETE
---
You are a helpful assistant.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	opts, err := loadWorkersFromConfigForServiceTest(cfg.FactoryDir(), cfg.FactoryConfig(), "", cfg, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["worker-a"]
	if !ok {
		t.Fatal("expected worker-a executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
	if _, ok := wsExec.Executor.(*workers.AgentExecutor); !ok {
		t.Fatalf("expected wrapped executor to be *workers.AgentExecutor, got %T", wsExec.Executor)
	}

	workerDef, ok := wsExec.RuntimeConfig.Worker("worker-a")
	if !ok {
		t.Fatal("expected worker-a in runtime config")
	}
	if workerDef.ExecutorProvider != "script_wrap" {
		t.Fatalf("executor provider = %q, want script_wrap", workerDef.ExecutorProvider)
	}
	if workerDef.ModelProvider != "codex" {
		t.Fatalf("model provider = %q, want codex", workerDef.ModelProvider)
	}
}

func TestLoadWorkersFromConfig_ModelWorkerUsesCanonicalProviderCommandRunnerAndRecordingProvider(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "worker-a", `---
type: MODEL_WORKER
model: gpt-5.4
executorProvider: script_wrap
modelProvider: codex
stopToken: COMPLETE
---
You are a helpful assistant.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	runner := &providerCommandRunnerRecorder{
		result: workers.CommandResult{
			Stdout: []byte("provider-output COMPLETE"),
			Stderr: []byte(`{"event":"session.created","session_id":"sess_codex_123"}`),
		},
	}
	recorded := make([]factoryapi.FactoryEvent, 0, 2)
	recorder := func(event factoryapi.FactoryEvent) {
		recorded = append(recorded, event)
	}

	opts, err := loadWorkersFromConfigForServiceTest(cfg.FactoryDir(), cfg.FactoryConfig(), "", cfg, nil, runner, nil, nil, recorder)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["worker-a"]
	if !ok {
		t.Fatal("expected worker-a executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}

	result, err := wsExec.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-model-worker-provider-command",
		TransitionID:    "t-model-worker-provider-command",
		WorkerType:      "worker-a",
		WorkstationName: "review",
		InputTokens: workers.InputTokens(interfaces.Token{
			ID: "tok-model-worker-provider-command",
			Color: interfaces.TokenColor{
				WorkID:  "work-model-worker-provider-command",
				Payload: []byte("helpful input"),
			},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertCanonicalModelWorkerExecutionResult(t, result)
	assertCanonicalProviderCommandRequests(t, runner.Requests())
	assertRecordedInferenceEvents(t, recorded)
}

// backendsizecheck:ignore-function this dual-locality integration test keeps the full model-invoke execution assertion path in one place.
func TestLoadWorkersFromConfig_ModelInvokeContractExecutesAcrossLocalAndCloudWorkers(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		model         string
		modelLocality string
	}{
		{
			name:          "local tts worker",
			model:         "OMNIVOICE_Q4_K_M",
			modelLocality: interfaces.ModelLocalityLocal,
		},
		{
			name:          "cloud tts worker",
			model:         "gpt-4o-mini-tts",
			modelLocality: interfaces.ModelLocalityCloud,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			provider, wsExec := modelInvokeExecutionFixture(t, tt.model, tt.modelLocality)
			result, err := wsExec.Execute(context.Background(), modelInvokeDispatch())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Outcome != interfaces.OutcomeAccepted {
				t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
			}
			assertModelInvokeProviderCall(t, provider.Calls(), tt.model, tt.modelLocality)
		})
	}
}

func modelInvokeExecutionFixture(t *testing.T, model string, modelLocality string) (*providerCallRecorder, *workers.WorkstationExecutor) {
	t.Helper()
	provider := &providerCallRecorder{responses: []interfaces.InferenceResponse{{Content: "audio-ready"}}}
	factoryCfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "speak", WorkerTypeName: "tts-worker"}},
		Workers:      []interfaces.WorkerConfig{{Name: "tts-worker"}},
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", factoryCfg,
		map[string]*interfaces.WorkerConfig{"tts-worker": modelInvokeRuntimeWorker(model, modelLocality)},
		map[string]*interfaces.FactoryWorkstationConfig{"speak": modelInvokeWorkstationConfig()},
	)
	opts, err := loadWorkersFromConfigForServiceTest("", factoryCfg, "", runtimeCfg, provider, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}
	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}
	exec, ok := fc.WorkerExecutors["tts-worker"]
	if !ok {
		t.Fatal("expected tts-worker executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
	return provider, wsExec
}

func modelInvokeRuntimeWorker(model string, modelLocality string) *interfaces.WorkerConfig {
	return &interfaces.WorkerConfig{
		Name:          "tts-worker",
		Type:          interfaces.WorkerTypeModel,
		Model:         model,
		ModelProvider: interfaces.RunnerIDCodex,
		ModelLocality: modelLocality,
		Body:          "You are a TTS worker.",
		Operations: []interfaces.ModelOperation{{
			Name: "TTS",
			Inputs: []interfaces.ModelOperationSlot{
				{Name: "text", ContentTypes: []string{interfaces.ModelOperationContentTypeText}, Required: true},
				{Name: "voice", ContentTypes: []string{interfaces.ModelOperationContentTypeJSON}},
			},
			Outputs: []interfaces.ModelOperationSlot{{Name: "audio", ContentTypes: []string{interfaces.ModelOperationContentTypeAudio}}},
		}},
	}
}

func modelInvokeWorkstationConfig() *interfaces.FactoryWorkstationConfig {
	return &interfaces.FactoryWorkstationConfig{
		Name:           "speak",
		Type:           interfaces.WorkstationTypeInvoke,
		WorkerTypeName: "tts-worker",
		Operation:      "TTS",
		PromptTemplate: "Synthesize {{ (index .Inputs 0).WorkID }}",
		OperationBindings: []interfaces.ModelOperationBinding{
			{
				Slot: "text",
				Selector: &interfaces.ModelOperationBindingSelector{
					Label: "utterance",
					Type:  interfaces.ModelOperationContentTypeText,
				},
			},
			{
				Slot: "voice",
				Config: []interfaces.WorkContentPart{{
					Type: interfaces.WorkContentPartTypeJSON,
					Role: "voice",
					JSON: []byte(`{"name":"alloy"}`),
				}},
			},
		},
	}
}

func modelInvokeDispatch() interfaces.WorkDispatch {
	return interfaces.WorkDispatch{
		DispatchID:      "dispatch-tts",
		TransitionID:    "transition-tts",
		WorkerType:      "tts-worker",
		WorkstationName: "speak",
		InputTokens: workers.InputTokens(interfaces.Token{
			ID: "token-tts",
			Color: interfaces.TokenColor{
				WorkID: "work-tts",
				Content: []interfaces.WorkContentPart{{
					Type:  interfaces.WorkContentPartTypeText,
					Label: "utterance",
					Text:  "hello world",
				}},
			},
		}),
	}
}

func assertModelInvokeProviderCall(t *testing.T, calls []interfaces.ProviderInferenceRequest, wantModel string, wantLocality string) {
	t.Helper()
	if len(calls) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(calls))
	}
	call := calls[0]
	if call.Model != wantModel {
		t.Fatalf("provider model = %q, want %q", call.Model, wantModel)
	}
	if call.ModelLocality != wantLocality {
		t.Fatalf("provider model locality = %q, want %q", call.ModelLocality, wantLocality)
	}
	if call.ModelOperation != "TTS" {
		t.Fatalf("provider model operation = %q, want TTS", call.ModelOperation)
	}
	if len(call.ModelBindings) != 2 {
		t.Fatalf("provider model bindings = %#v, want 2 entries", call.ModelBindings)
	}
	assertModelInvokeTextBinding(t, call.ModelBindings[0])
	assertModelInvokeVoiceBinding(t, call.ModelBindings[1])
}

func assertModelInvokeTextBinding(t *testing.T, binding interfaces.ResolvedModelOperationBinding) {
	t.Helper()
	if binding.Slot != "text" || binding.Source != interfaces.ModelOperationBindingSourceInput || binding.Content[0].Text != "hello world" {
		t.Fatalf("text model binding = %#v, want generic text slot from input", binding)
	}
}

func assertModelInvokeVoiceBinding(t *testing.T, binding interfaces.ResolvedModelOperationBinding) {
	t.Helper()
	if binding.Slot != "voice" || binding.Source != interfaces.ModelOperationBindingSourceConfig || string(binding.Content[0].JSON) != `{"name":"alloy"}` {
		t.Fatalf("voice model binding = %#v, want config voice binding", binding)
	}
}

func TestLoadWorkersFromConfig_ReplayEmbeddedRuntimeUsesCanonicalLookup(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "review")

	loaded := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	generated, err := replay.GeneratedFactoryFromRuntimeConfig(loaded.FactoryDir(), loaded.FactoryConfig(), loaded)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromRuntimeConfig: %v", err)
	}
	runtimeCfg, err := replay.RuntimeConfigFromGeneratedFactory(generated)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}

	opts, err := loadWorkersFromConfigForServiceTest(runtimeCfg.FactoryDir(), runtimeCfg.Factory, "", runtimeCfg, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["worker-a"]
	if !ok {
		t.Fatal("expected worker-a executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
	if got := wsExec.RuntimeConfig.FactoryDir(); got != dir {
		t.Fatalf("embedded runtime FactoryDir = %q, want %q", got, dir)
	}
	if got := wsExec.RuntimeConfig.RuntimeBaseDir(); got != dir {
		t.Fatalf("embedded runtime RuntimeBaseDir = %q, want %q", got, dir)
	}
	if _, ok := wsExec.RuntimeConfig.Worker("worker-a"); !ok {
		t.Fatal("expected replay runtime worker lookup for worker-a")
	}
	if _, ok := wsExec.RuntimeConfig.Workstation("review"); !ok {
		t.Fatal("expected replay runtime workstation lookup for review")
	}
}

func TestLoadWorkersFromConfig_LoadedRuntimeBaseDirOverrideFlowsThroughCanonicalLookup(t *testing.T) {
	dir := t.TempDir()
	runtimeBaseDir := t.TempDir()

	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "review")

	loaded := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)
	loaded.SetRuntimeBaseDir(runtimeBaseDir)

	opts, err := loadWorkersFromConfigForServiceTest(loaded.FactoryDir(), loaded.FactoryConfig(), "", loaded, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["worker-a"]
	if !ok {
		t.Fatal("expected worker-a executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
	if got := wsExec.RuntimeConfig.FactoryDir(); got != dir {
		t.Fatalf("loaded runtime FactoryDir = %q, want %q", got, dir)
	}
	if got := wsExec.RuntimeConfig.RuntimeBaseDir(); got != runtimeBaseDir {
		t.Fatalf("loaded runtime RuntimeBaseDir = %q, want %q", got, runtimeBaseDir)
	}
}

func TestLoadWorkersFromConfig_CanonicalRuntimeLookupDrivesScriptExecutionWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	runtimeBaseDir := t.TempDir()

	writeScriptWorkerAgentsMD(t, dir, "script-worker")
	writeRuntimeLookupWorkstationAgentsMD(t, dir, "run-script")

	loaded := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "run-script",
			WorkerTypeName: "script-worker",
		}},
		Workers: []interfaces.WorkerConfig{{Name: "script-worker"}},
	},
		map[string]*interfaces.WorkerConfig{
			"script-worker": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "script-worker")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"run-script": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "run-script")),
		},
	)
	loaded.SetRuntimeBaseDir(runtimeBaseDir)

	runner := &capturingCommandRunner{}
	opts, err := loadWorkersFromConfigForServiceTest(loaded.FactoryDir(), loaded.FactoryConfig(), "", loaded, nil, nil, runner, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["script-worker"]
	if !ok {
		t.Fatal("expected script-worker executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}

	result, err := wsExec.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-runtime-lookup-script",
		TransitionID:    "t-runtime-lookup-script",
		WorkerType:      "script-worker",
		WorkstationName: "run-script",
		ProjectID:       "agent-factory",
		InputTokens: workers.InputTokens(interfaces.Token{
			ID: "tok-runtime-lookup-script",
			Color: interfaces.TokenColor{
				WorkID: "work-runtime-lookup-script",
			},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if got := runner.request.WorkDir; got != filepath.Join(runtimeBaseDir, "workspace") {
		t.Fatalf("command working directory = %q, want %q", got, filepath.Join(runtimeBaseDir, "workspace"))
	}
}

func TestLoadWorkersFromConfig_CanonicalRuntimeLookupResolvesPortableFactoryScriptReferencesAgainstNamedFactoryDir(t *testing.T) {
	rootDir := t.TempDir()
	namedFactoryDir := filepath.Join(rootDir, "beta")

	writeScriptWorkerAgentsMDWithCommand(t, namedFactoryDir, "script-worker", "pwsh", []string{"-File", "factory/scripts/execute-story.ps1"})
	writeRuntimeLookupWorkstationAgentsMD(t, namedFactoryDir, "run-script")
	if err := os.MkdirAll(filepath.Join(namedFactoryDir, "scripts"), 0o755); err != nil {
		t.Fatalf("create scripts dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(namedFactoryDir, "scripts", "execute-story.ps1"), []byte("Write-Output 'ok'\n"), 0o644); err != nil {
		t.Fatalf("write portable script: %v", err)
	}

	loaded := newLoadedFactoryConfigForServiceTest(t, namedFactoryDir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "run-script",
			WorkerTypeName: "script-worker",
		}},
		Workers: []interfaces.WorkerConfig{{Name: "script-worker"}},
	},
		map[string]*interfaces.WorkerConfig{
			"script-worker": mustLoadWorkerConfig(t, filepath.Join(namedFactoryDir, "workers", "script-worker")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"run-script": mustLoadWorkstationConfig(t, filepath.Join(namedFactoryDir, "workstations", "run-script")),
		},
	)
	loaded.SetRuntimeBaseDir(rootDir)

	runner := &capturingCommandRunner{}
	opts, err := loadWorkersFromConfigForServiceTest(loaded.FactoryDir(), loaded.FactoryConfig(), "", loaded, nil, nil, runner, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["script-worker"]
	if !ok {
		t.Fatal("expected script-worker executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}

	result, err := wsExec.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-runtime-lookup-script-ref",
		TransitionID:    "t-runtime-lookup-script-ref",
		WorkerType:      "script-worker",
		WorkstationName: "run-script",
		ProjectID:       "agent-factory",
		InputTokens: workers.InputTokens(interfaces.Token{
			ID: "tok-runtime-lookup-script-ref",
			Color: interfaces.TokenColor{
				WorkID: "work-runtime-lookup-script-ref",
			},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if len(runner.request.Args) != 2 {
		t.Fatalf("command args = %#v, want 2 entries", runner.request.Args)
	}
	if got := runner.request.Args[1]; got != filepath.Join(namedFactoryDir, "scripts", "execute-story.ps1") {
		t.Fatalf("portable script arg = %q, want %q", got, filepath.Join(namedFactoryDir, "scripts", "execute-story.ps1"))
	}
	if got := runner.request.WorkDir; got != filepath.Join(rootDir, "workspace") {
		t.Fatalf("command working directory = %q, want %q", got, filepath.Join(rootDir, "workspace"))
	}
}

func TestLoadWorkersFromConfig_ReplayRuntimeLookupDrivesScriptExecutionWorkingDirectory(t *testing.T) {
	dir := t.TempDir()

	writeScriptWorkerAgentsMD(t, dir, "script-worker")
	writeRuntimeLookupWorkstationAgentsMD(t, dir, "run-script")

	loaded := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "run-script",
			WorkerTypeName: "script-worker",
		}},
		Workers: []interfaces.WorkerConfig{{Name: "script-worker"}},
	},
		map[string]*interfaces.WorkerConfig{
			"script-worker": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "script-worker")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"run-script": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "run-script")),
		},
	)

	generated, err := replay.GeneratedFactoryFromRuntimeConfig(loaded.FactoryDir(), loaded.FactoryConfig(), loaded)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromRuntimeConfig: %v", err)
	}
	runtimeCfg, err := replay.RuntimeConfigFromGeneratedFactory(generated)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}

	runner := &capturingCommandRunner{}
	opts, err := loadWorkersFromConfigForServiceTest(runtimeCfg.FactoryDir(), runtimeCfg.Factory, "", runtimeCfg, nil, nil, runner, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["script-worker"]
	if !ok {
		t.Fatal("expected script-worker executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}

	result, err := wsExec.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-replay-runtime-lookup-script",
		TransitionID:    "t-replay-runtime-lookup-script",
		WorkerType:      "script-worker",
		WorkstationName: "run-script",
		ProjectID:       "agent-factory",
		InputTokens: workers.InputTokens(interfaces.Token{
			ID: "tok-replay-runtime-lookup-script",
			Color: interfaces.TokenColor{
				WorkID: "work-replay-runtime-lookup-script",
			},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if got := runner.request.WorkDir; got != filepath.Join(dir, "workspace") {
		t.Fatalf("command working directory = %q, want %q", got, filepath.Join(dir, "workspace"))
	}
}

func TestLoadWorkersFromConfig_ScriptWorkerUsesWorkstationExecutor(t *testing.T) {
	dir := t.TempDir()
	scriptRecorder := func(factoryapi.FactoryEvent) {}

	writeScriptWorkerAgentsMD(t, dir, "script-worker")
	writeWorkstationAgentsMD(t, dir, "run-script")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "run-script"}},
		Workers:      []interfaces.WorkerConfig{{Name: "script-worker"}},
	},
		map[string]*interfaces.WorkerConfig{
			"script-worker": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "script-worker")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"run-script": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "run-script")),
		},
	)

	opts, err := loadWorkersFromConfigForServiceTest(cfg.FactoryDir(), cfg.FactoryConfig(), "", cfg, nil, nil, &stubCommandRunner{}, scriptRecorder, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["script-worker"]
	if !ok {
		t.Fatal("expected script-worker executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
	scriptExec, ok := wsExec.Executor.(*workers.ScriptExecutor)
	if !ok {
		t.Fatalf("expected wrapped executor to be *workers.ScriptExecutor, got %T", wsExec.Executor)
	}
	if recorder := reflect.ValueOf(scriptExec).Elem().FieldByName("recorder"); !recorder.IsValid() || recorder.IsNil() {
		t.Fatal("expected script executor to receive canonical script event recorder")
	}
}

func TestLoadWorkersFromConfig_RegistersWorkerlessLogicalWorkstationByName(t *testing.T) {
	cfg := newLoadedFactoryConfigForServiceTest(t, "", &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "review-loop-breaker",
			Type: interfaces.WorkstationTypeLogical,
			Inputs: []interfaces.IOConfig{{
				WorkTypeName: "story",
				StateName:    "init",
			}},
			Outputs: []interfaces.IOConfig{{
				WorkTypeName: "story",
				StateName:    "failed",
			}},
		}},
	}, nil, map[string]*interfaces.FactoryWorkstationConfig{
		"review-loop-breaker": {
			Name: "review-loop-breaker",
			Type: interfaces.WorkstationTypeLogical,
			Inputs: []interfaces.IOConfig{{
				WorkTypeName: "story",
				StateName:    "init",
			}},
			Outputs: []interfaces.IOConfig{{
				WorkTypeName: "story",
				StateName:    "failed",
			}},
		},
	})

	opts, err := loadWorkersFromConfigForServiceTest(cfg.FactoryDir(), cfg.FactoryConfig(), "", cfg, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["review-loop-breaker"]
	if !ok {
		t.Fatal("expected workerless logical workstation executor to be registered by workstation name")
	}
	if _, ok := exec.(*workers.WorkstationExecutor); !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
}

func loadWorkersFromConfigForServiceTest(
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	factoryRunnerID string,
	runtimeCfg interfaces.RuntimeConfigLookup,
	providerOverride workers.Provider,
	providerCommandRunner workers.CommandRunner,
	commandRunner workers.CommandRunner,
	scriptRecorder workers.ScriptEventRecorder,
	inferenceRecorder workerprovider.InferenceEventRecorder,
) ([]factory.FactoryOption, error) {
	return loadWorkersFromConfig(
		factoryDir,
		factoryCfg,
		factoryRunnerID,
		runtimeCfg,
		logging.NoopLogger{},
		false,
		providerOverride,
		providerCommandRunner,
		commandRunner,
		scriptRecorder,
		inferenceRecorder,
		nil,
		nil,
		nil,
		nil,
	)
}

func assertCanonicalModelWorkerExecutionResult(t *testing.T, result interfaces.WorkResult) {
	t.Helper()

	if result.Outcome != interfaces.OutcomeAccepted || result.Output != "provider-output COMPLETE" {
		t.Fatalf("result = %#v, want accepted provider output", result)
	}
	if result.ProviderSession == nil || result.ProviderSession.Provider != "codex" || result.ProviderSession.ID != "sess_codex_123" {
		t.Fatalf("provider session = %#v, want canonical codex session metadata", result.ProviderSession)
	}
}

func assertCanonicalProviderCommandRequests(t *testing.T, requests []workers.CommandRequest) {
	t.Helper()

	if len(requests) != 1 {
		t.Fatalf("provider command runner request count = %d, want 1", len(requests))
	}
	if requests[0].Command != string(interfaces.ModelProviderCodex) {
		t.Fatalf("provider command = %q, want %q", requests[0].Command, interfaces.ModelProviderCodex)
	}
}

func assertRecordedInferenceEvents(t *testing.T, recorded []factoryapi.FactoryEvent) {
	t.Helper()

	if len(recorded) != 2 {
		t.Fatalf("recorded event count = %d, want 2", len(recorded))
	}
	if recorded[0].Type != factoryapi.FactoryEventTypeInferenceRequest {
		t.Fatalf("first event type = %s, want %s", recorded[0].Type, factoryapi.FactoryEventTypeInferenceRequest)
	}
	if recorded[1].Type != factoryapi.FactoryEventTypeInferenceResponse {
		t.Fatalf("second event type = %s, want %s", recorded[1].Type, factoryapi.FactoryEventTypeInferenceResponse)
	}
}
