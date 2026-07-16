// pkgmaintcheck:ignore-file-lines consolidated same-package service tests remain on root-only runtime seams until dedicated service test seams are extracted.
// backendsizecheck:ignore-file consolidated same-package service tests remain on root-only runtime seams until dedicated service test seams are extracted.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/validationassert"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	"github.com/portpowered/infinite-you/pkg/factory/replay"
	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/runtimepersist"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysnapshot"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerapplication "github.com/portpowered/infinite-you/pkg/workers/application"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	workerexecutor "github.com/portpowered/infinite-you/pkg/workers/executor"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func generatedFactoryEventsForTest(t testing.TB, events []interfaces.FactoryEvent) []factoryapi.FactoryEvent {
	t.Helper()
	generated := make([]factoryapi.FactoryEvent, len(events))
	for index, event := range events {
		if err := event.Decode(&generated[index]); err != nil {
			t.Fatalf("decode canonical Factory event %q for compatibility assertion: %v", event.Id, err)
		}
	}
	return generated
}

func generatedFactoryFromRuntimeConfigForTest(factoryDir string, factoryCfg *interfaces.FactoryConfig, runtimeCfg interfaces.RuntimeDefinitionLookup) (factoryapi.Factory, error) {
	snapshot, err := replay.FactorySnapshotFromRuntimeConfig(factoryDir, factoryCfg, runtimeCfg)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	generated, err := factorysnapshot.ToAPI(snapshot)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return *generated, nil
}

func runtimeConfigFromGeneratedFactoryForTest(generated factoryapi.Factory) (*replay.EmbeddedRuntimeConfig, error) {
	snapshot, err := interfaces.NewFactorySnapshot(generated)
	if err != nil {
		return nil, err
	}
	return replay.RuntimeConfigFromFactorySnapshot(snapshot)
}

// recordModeServiceRunTimeout allows full runtime startup under Windows CI load.
const recordModeServiceRunTimeout = 5 * time.Second

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
		response: workerexecution.InferenceResponse{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		},
	}

	server, launcher, _, shutdown := startLocalModelInferenceTestServer(t, runtime)
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
	if launcher.startCount() != 1 {
		t.Fatalf("supervised process start count = %d, want 1 shared host runtime", launcher.startCount())
	}
	if runtime.loadCount() != 1 || runtime.invocationCount() != 1 {
		t.Fatalf("runtime load/invoke counts = %d/%d, want 1/1", runtime.loadCount(), runtime.invocationCount())
	}
	if got := runtime.lastLoadServingEndpoint(); got != launcher.healthEndpoint {
		t.Fatalf("serving endpoint = %q, want supervised lease endpoint %q", got, launcher.healthEndpoint)
	}
}

func TestBuildFactoryService_InitializesManagedLocalModelFields(t *testing.T) {
	dir := scaffoldLocalModelLongTestFactoryDir(t, localModelLongTestFactoryConfigWithHealthEndpoint("http://127.0.0.1:1"))
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
	if bundle.ModelResources == nil {
		t.Fatal("expected startup bundle to initialize modelResources")
	}
	if bundle.LocalModels == nil {
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
	cfg := serviceTestConfigWithWorkerEdges(t, &FactoryServiceConfig{}, workerapplication.Edges{
		HostedHTTPClient: client, HostedSecretResolver: resolver,
		HostedLinearEndpoint: " https://linear.example/graphql ",
	})

	got := buildHostedWorkersConfigForServiceTest(cfg, zap.NewNop(), nil)
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

func startLocalModelInferenceTestServer(
	t *testing.T,
	runtime *fakeLocalModelRuntime,
) (*httptest.Server, *serviceTestFakeProcessLauncher, *FactoryService, func()) {
	t.Helper()
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	launcher := &serviceTestFakeProcessLauncher{healthEndpoint: healthServer.URL}
	dir := scaffoldLocalModelLongTestFactoryDir(t, localModelLongTestFactoryConfigWithHealthEndpoint(healthServer.URL))
	writeLocalModelLongTestWorkerConfig(t, dir)
	writeLocalModelLongTestWorkstationConfig(t, dir)

	puller := staticModelAssetPuller{cache: localModelTestCacheLayout(t)}
	domain := modelHostBackedLocalModelDomain(t, puller, launcher, runtime)

	ctx, cancel := context.WithCancel(context.Background())
	cfg := &FactoryServiceConfig{
		Dir:                                     dir,
		RuntimeMode:                             interfaces.RuntimeModeService,
		Port:                                    1,
		Logger:                                  zap.NewNop(),
		LocalModelRuntimeOverride:               runtime,
		ModelAssets:                             puller,
		SkipBuiltInRunnerPrerequisiteValidation: true,
	}
	cfg = serviceTestConfigWithWorkerApplication(t, cfg)
	root, err := ResolveFactoryServiceRoot(cfg)
	if err != nil {
		cleanupLocalModelInferenceSetup(cancel, healthServer)
		t.Fatalf("ResolveFactoryServiceRoot: %v", err)
	}
	load, err := LoadFactoryConfigForCompose(cfg, root)
	if err != nil {
		cleanupLocalModelInferenceSetup(cancel, healthServer)
		t.Fatalf("LoadFactoryConfigForCompose: %v", err)
	}
	clock := ServiceClockForCompose(cfg, load)
	sessions := NewFactorySessionsRegistry()
	startupLocalModels := domain
	runtimeBuild, err := newRuntimeBuildService(
		cfg,
		clock,
		root.BaseLogger,
		&startupLocalModels,
		newInferenceProgressPublisherFactory(sessions, root.BaseLogger),
		newSessionDispatchCompletionObserverFactory(sessions),
	)
	if err != nil {
		cleanupLocalModelInferenceSetup(cancel, healthServer)
		t.Fatalf("newRuntimeBuildService: %v", err)
	}
	collaborators := FactoryServiceCollaborators{
		Sessions:         sessions,
		LocalModels:      domain,
		RuntimeBuild:     runtimeBuild,
		WorkersScheduler: workersservice.NewWorkersSchedulerService(workersSchedulerServiceConfig(cfg, clock, root.BaseLogger, buildHostedWorkersConfigForServiceTest(cfg, root.BaseLogger, clock))),
	}
	shell, err := ComposeFactoryService(
		ctx,
		cfg,
		root,
		collaborators,
		load,
		clock,
		buildHostedWorkersConfigForServiceTest(cfg, root.BaseLogger, clock),
	)
	if err != nil {
		cleanupLocalModelInferenceSetup(cancel, healthServer)
		t.Fatalf("ComposeFactoryService: %v", err)
	}
	modelAPI, err := newTestModelService(shell)
	if err != nil {
		cancel()
		healthServer.Close()
		t.Fatalf("construct model service: %v", err)
	}
	svc := AttachModelServiceCollaborator(shell, AdaptModelService(modelAPI))
	svc = AttachFactorySaveCollaborator(
		FactoryServiceShell{Service: svc},
		ProvideFactorySaveCollaborator(FactoryServiceShell{Service: svc}, cfg),
	)

	runErrCh := make(chan error, 1)
	go func() { runErrCh <- svc.Run(ctx) }()
	waitForFactoryServiceRuntimeReady(t, svc)

	server := httptest.NewServer(api.NewServer(svc, 0, zap.NewNop()).Handler())
	shutdown := func() {
		cancel()
		if err := <-runErrCh; err != nil && err != context.Canceled {
			t.Fatalf("svc.Run: %v", err)
		}
		healthServer.Close()
	}
	return server, launcher, svc, shutdown
}

func cleanupLocalModelInferenceSetup(cancel context.CancelFunc, healthServer *httptest.Server) {
	cancel()
	healthServer.Close()
}

func newTestModelService(shell FactoryServiceShell) (*modelsservice.Service, error) {
	modelDeps, err := ModelServiceDependencies(shell)
	if err != nil {
		return nil, err
	}
	return modelsservice.NewService(modelDeps)
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
	if generatedAudioFileValue(audioPart.File) != audioPath || stringValue(audioPart.ContentType) != "audio/wav" {
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

func localModelLongTestFactoryConfigWithHealthEndpoint(healthEndpoint string) *interfaces.FactoryConfig {
	cfg := &interfaces.FactoryConfig{
		Name: "omnivoice-local-model-test",
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name:             "speech",
			HandlingBehavior: []string{interfaces.WorkTypeHandlingBehaviorDefault},
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Resources: []factoryresource.Config{{
			Name:       "omnivoice-cache",
			Type:       factoryresource.TypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
		Workers: []workerconfig.Config{{
			Name:          "tts-worker",
			Type:          interfaces.WorkerTypeInference,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelProvider: workerexecution.RunnerIDCodex,
			ModelLocality: workerconfig.ModelLocalityLocal,
			Command:       localmodels.DefaultOmniVoiceCommand,
			Resources: []factoryresource.Config{{
				Name:     "omnivoice-cache",
				Capacity: 1,
			}},
			Operations: []workerconfig.ModelOperation{{
				Name: "TTS",
				Inputs: []workerconfig.ModelOperationSlot{{
					Name:         "text",
					ContentTypes: []string{workerconfig.ModelOperationContentTypeText},
					Required:     true,
				}},
				Outputs: []workerconfig.ModelOperationSlot{{
					Name:         "audio",
					ContentTypes: []string{workerconfig.ModelOperationContentTypeAudio},
				}},
			}},
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "speak",
			Type:           interfaces.WorkstationTypeInference,
			WorkerTypeName: "tts-worker",
			Operation:      "TTS",
			OperationBindings: []interfaces.ModelOperationBinding{{
				Slot: "text",
				Selector: &interfaces.ModelOperationBindingSelector{
					Type: workerconfig.ModelOperationContentTypeText,
				},
			}},
			Inputs:    []interfaces.IOConfig{{WorkTypeName: "speech", StateName: "init"}},
			Outputs:   []interfaces.IOConfig{{WorkTypeName: "speech", StateName: "complete"}},
			OnFailure: []interfaces.IOConfig{{WorkTypeName: "speech", StateName: "failed"}},
		}},
	}
	if strings.TrimSpace(healthEndpoint) != "" {
		cfg.Workers[0].Args = []string{"--health-endpoint", healthEndpoint}
	}
	return cfg
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
	if svc.coordinatorPolicy().mockWorkersConfig == nil {
		t.Fatal("expected mock-worker config to be preserved")
	}
	if len(svc.coordinatorPolicy().mockWorkersConfig.MockWorkers) != 0 {
		t.Fatalf("mock worker count = %d, want empty default accept config", len(svc.coordinatorPolicy().mockWorkersConfig.MockWorkers))
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

	ctx, cancel := context.WithTimeout(context.Background(), recordModeServiceRunTimeout)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	artifact, err := replay.Load(recordPath)
	if err != nil {
		t.Fatalf("Load(recording): %v", err)
	}
	generatedFactory := decodeRecordedFactorySnapshot(t, artifact.Factory)
	if generatedFactory.Workers == nil {
		t.Fatal("expected embedded factory config")
	}
	if generatedFactory.FactoryDirectory == nil || *generatedFactory.FactoryDirectory != dir {
		t.Fatalf("factory directory = %#v, want %q", generatedFactory.FactoryDirectory, dir)
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

	ctx, cancel := context.WithTimeout(context.Background(), recordModeServiceRunTimeout)
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
	generatedFactory := decodeRecordedFactorySnapshot(t, artifact.Factory)
	if generatedFactory.FactoryDirectory == nil || *generatedFactory.FactoryDirectory != dir {
		t.Fatalf("factory directory = %#v, want %q", generatedFactory.FactoryDirectory, dir)
	}
}

func decodeRecordedFactorySnapshot(t *testing.T, snapshot *interfaces.FactorySnapshot) factoryapi.Factory {
	t.Helper()
	var generated factoryapi.Factory
	if err := snapshot.Decode(&generated); err != nil {
		t.Fatalf("decode recorded Factory snapshot: %v", err)
	}
	return generated
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
	work := work.SubmitRequest{
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

	ctx, cancel := context.WithTimeout(context.Background(), recordModeServiceRunTimeout)
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

func TestBuildFactoryService_PetriMutationsReloadThroughComposedDurableOwner(t *testing.T) {
	dir := t.TempDir()
	executionDir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	workFile := filepath.Join(dir, "petri-owner-work.json")
	writeWorkRequestFile(t, workFile, work.SubmitRequest{
		WorkTypeID: "task", Name: "petri-owner", TraceID: "trace-petri-owner",
	})
	cfg := &FactoryServiceConfig{
		Dir: dir, ExecutionBaseDir: executionDir, WorkFile: workFile,
		MockWorkersConfig: factoryconfig.NewEmptyMockWorkersConfig(), Logger: zap.NewNop(),
	}
	svc, err := BuildFactoryService(context.Background(), cfg)
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := svc.durableExecution.GetSession(context.Background(), defaultFactorySessionID); err != nil {
		t.Fatalf("live durable GetSession: %v", err)
	}
	if entries, err := os.ReadDir(runtimepersist.DirForProjectRoot(executionDir)); err != nil || len(entries) == 0 {
		t.Fatalf("persisted Factory Session entries = %v, err = %v", entries, err)
	}

	reloaded, err := composeDurableExecution(cfg, FactoryServiceRoot{FactoryRootDir: dir}, svc.clock)
	if err != nil {
		t.Fatalf("compose reloaded durable execution: %v", err)
	}
	session, err := reloaded.GetSession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetSession after production restart: %v", err)
	}
	if session.OrchestratorKind != interfaces.OrchestratorKindPetri || session.Status != factorysessionexecution.LifecycleStatusRunning {
		t.Fatalf("reloaded Petri session = %#v", session)
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
		types = append(types, factoryapi.FactoryEventType(event.Type))
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
	writeWorkRequestFile(t, workFile, work.SubmitRequest{
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
	work := work.SubmitRequest{
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
	writeWorkRequestFile(t, workFile, work.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    "trace-script-diagnostics",
		Payload:    []byte(`{"task":"record script diagnostics"}`),
	})

	recordPath := filepath.Join(t.TempDir(), "recording.json")
	svc, err := BuildFactoryService(context.Background(), serviceTestConfigWithWorkerEdges(t, &FactoryServiceConfig{
		Dir: dir, RuntimeMode: interfaces.RuntimeModeService, Logger: zap.NewNop(),
		RecordPath: recordPath, WorkFile: workFile,
	}, workerapplication.Edges{ScriptCommandRunner: recordingDiagnosticsCommandRunner{}}))
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runServiceUntilDispatchCompletion(t, svc)

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

func runServiceUntilDispatchCompletion(t *testing.T, svc *FactoryService) {
	t.Helper()
	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	deadline := time.NewTimer(serviceStreamedRecordingTimeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		select {
		case runErr := <-errCh:
			t.Fatalf("Run returned before dispatch completion: %v", runErr)
		case <-deadline.C:
			t.Fatalf("worker did not publish a dispatch completion within %s", serviceStreamedRecordingTimeout)
		case <-ticker.C:
			events, err := svc.GetFactoryEvents(context.Background())
			if err != nil {
				t.Fatalf("GetFactoryEvents: %v", err)
			}
			if slices.ContainsFunc(events, func(event factoryapi.FactoryEvent) bool {
				return event.Type == factoryapi.FactoryEventTypeDispatchResponse
			}) {
				cancelRun()
				select {
				case runErr := <-errCh:
					if runErr != nil {
						t.Fatalf("Run after cancellation: %v", runErr)
					}
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for service-mode factory service to stop")
				}
				return
			}
		}
	}
}

type providerCallRecorder struct {
	mu        sync.Mutex
	calls     []workerexecution.ProviderInferenceRequest
	responses []workerexecution.InferenceResponse
}

func (p *providerCallRecorder) Infer(_ context.Context, req workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.calls = append(p.calls, workerexecution.CloneProviderInferenceRequest(req))
	if len(p.responses) == 0 {
		return workerexecution.InferenceResponse{Content: "ok"}, nil
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	return response, nil
}

func (p *providerCallRecorder) Calls() []workerexecution.ProviderInferenceRequest {
	p.mu.Lock()
	defer p.mu.Unlock()

	calls := make([]workerexecution.ProviderInferenceRequest, len(p.calls))
	for i, call := range p.calls {
		calls[i] = workerexecution.CloneProviderInferenceRequest(call)
	}
	return calls
}

type providerCommandRunnerRecorder struct {
	mu       sync.Mutex
	requests []workers.CommandRequest
	result   workers.CommandResult
	err      error
}

func (r *providerCommandRunnerRecorder) Run(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.requests = append(r.requests, workers.CommandRequest(workerexecution.CloneSubprocessExecutionRequest(req)))
	return r.result, r.err
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
		Workers:      []workerconfig.Config{{Name: "worker-a"}},
	}
	cfg := newLoadedFactoryConfigForServiceTest(t, dir, factoryCfg,
		map[string]*workerconfig.Config{
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
		Workers:      []workerconfig.Config{{Name: "worker-a"}},
	}
	cfg := newLoadedFactoryConfigForServiceTest(t, dir, factoryCfg,
		map[string]*workerconfig.Config{
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
		Workers:      []workerconfig.Config{{Name: "worker-a"}},
	},
		map[string]*workerconfig.Config{
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
	router, ok := wsExec.Executor.(*workerexecutor.WorkstationBehaviorRouter)
	if !ok {
		t.Fatalf("expected wrapped executor to be *executor.WorkstationBehaviorRouter, got %T", wsExec.Executor)
	}
	if _, ok := router.InferenceExecutor.(*workers.AgentExecutor); !ok {
		t.Fatalf("expected inference executor to be *workers.AgentExecutor, got %T", router.InferenceExecutor)
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
		Workers:      []workerconfig.Config{{Name: "worker-a"}},
	},
		map[string]*workerconfig.Config{
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
	recorded := make([]workerexecution.InferenceEvent, 0, 2)
	recorder := func(event workerexecution.InferenceEvent) {
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

	result, err := wsExec.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-model-worker-provider-command",
		TransitionID:    "t-model-worker-provider-command",
		WorkerType:      "worker-a",
		WorkstationName: "review",
		InputTokens: workers.InputTokens(factorytoken.Token{
			ID: "tok-model-worker-provider-command",
			Color: factorytoken.Color{
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

func TestLoadWorkersFromConfig_ModelWorkerProgressPublisherLeavesCanonicalEventsUnchanged(t *testing.T) {
	var published []workerprovider.InferenceProgressFragment
	result, err, recorded := executeModelWorkerProgressPublisherServiceTest(t, modelWorkerProgressPublisherServiceTestOptions{
		commandResult: workers.CommandResult{
			Stdout: []byte("provider-output COMPLETE"),
			Stderr: []byte(`{"event":"session.created","session_id":"sess_codex_123"}`),
		},
		progressPublisher: func(fragment workerprovider.InferenceProgressFragment) {
			published = append(published, fragment)
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertCanonicalModelWorkerExecutionResult(t, result)
	assertRecordedInferenceEvents(t, recorded)
	if len(published) != 1 {
		t.Fatalf("published fragments = %#v, want one terminal completion marker", published)
	}
	if published[0].Kind != workerprovider.CompletedFragmentKind {
		t.Fatalf("published kind = %q, want %q", published[0].Kind, workerprovider.CompletedFragmentKind)
	}
	if published[0].DispatchID != "d-model-worker-provider-progress" {
		t.Fatalf("dispatch id = %q, want d-model-worker-provider-progress", published[0].DispatchID)
	}
	if published[0].ProviderSessionRef == nil || published[0].ProviderSessionRef.ID != "sess_codex_123" {
		t.Fatalf("provider session ref = %#v, want sess_codex_123", published[0].ProviderSessionRef)
	}
}

func TestLoadWorkersFromConfig_ModelWorkerProgressPublisherPanicLeavesSuccessfulOutcomeUnchanged(t *testing.T) {
	core, observed := observer.New(zap.WarnLevel)
	result, err, recorded := executeModelWorkerProgressPublisherServiceTest(t, modelWorkerProgressPublisherServiceTestOptions{
		commandResult: workers.CommandResult{
			Stdout: []byte("provider-output COMPLETE"),
			Stderr: []byte(`{"event":"session.created","session_id":"sess_codex_123"}`),
		},
		newSessionResponseStream: func() *factorysessions.SessionResponseStream {
			panic("stream build boom")
		},
		logger: zap.New(core),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertCanonicalModelWorkerExecutionResult(t, result)
	assertRecordedInferenceEvents(t, recorded)
	entry := findObservedLog(t, observed, "internal provider progress publication degraded")
	assertLogField(t, entry, "reason", "PUBLISH_PANIC")
	assertLogField(t, entry, "dispatch_id", "d-model-worker-provider-progress")
}

func TestLoadWorkersFromConfig_ModelWorkerProgressPublisherPanicLeavesProviderFailureUnchanged(t *testing.T) {
	core, observed := observer.New(zap.WarnLevel)
	result, err, recorded := executeModelWorkerProgressPublisherServiceTest(t, modelWorkerProgressPublisherServiceTestOptions{
		commandErr: errors.New("provider subprocess failed"),
		newSessionResponseStream: func() *factorysessions.SessionResponseStream {
			panic("stream build boom")
		},
		logger: zap.New(core),
	})
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("result outcome = %q, want failed provider result", result.Outcome)
	}
	if strings.TrimSpace(result.Error) == "" || !strings.Contains(result.Error, "provider subprocess failed") {
		t.Fatalf("result error = %q, want provider failure message", result.Error)
	}
	assertRecordedInferenceEvents(t, recorded)

	entry := findObservedLog(t, observed, "internal provider progress publication degraded")
	assertLogField(t, entry, "reason", "PUBLISH_PANIC")
	assertLogField(t, entry, "dispatch_id", "d-model-worker-provider-progress")
}

type modelWorkerProgressPublisherServiceTestOptions struct {
	commandResult            workers.CommandResult
	commandErr               error
	newSessionResponseStream func() *factorysessions.SessionResponseStream
	progressPublisher        workerprovider.InferenceProgressPublisher
	logger                   *zap.Logger
}

func executeModelWorkerProgressPublisherServiceTest(
	t *testing.T,
	options modelWorkerProgressPublisherServiceTestOptions,
) (workerexecution.WorkResult, error, []workerexecution.InferenceEvent) {
	t.Helper()

	runner := &providerCommandRunnerRecorder{
		result: options.commandResult,
		err:    options.commandErr,
	}
	recorded := make([]workerexecution.InferenceEvent, 0, 2)
	recorder := func(event workerexecution.InferenceEvent) {
		recorded = append(recorded, event)
	}

	dir, cfg := newModelWorkerProgressPublisherServiceFixture(t)
	sessions := newModelWorkerProgressPublisherSessions(dir)
	svc := &FactoryService{
		sessions:                 sessions,
		newSessionResponseStream: options.newSessionResponseStream,
	}

	logger := options.logger
	if logger == nil {
		logger = zap.NewNop()
	}
	progressPublisher := options.progressPublisher
	if progressPublisher == nil {
		progressPublisher = svc.inferenceProgressPublisher(modelWorkerProgressPublisherSessionID, logger)
	}

	opts, err := loadWorkersFromConfig(
		cfg.FactoryDir(),
		cfg.FactoryConfig(),
		"",
		cfg,
		nil,
		logging.NoopLogger{},
		false,
		nil,
		nil,
		progressPublisher,
		runner,
		nil,
		nil,
		recorder,
		nil,
		nil,
		time.Now,
		LocalModelDomain{},
	)
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

	result, execErr := wsExec.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      modelWorkerProgressPublisherDispatchID,
		TransitionID:    "t-model-worker-provider-progress",
		WorkerType:      "worker-a",
		WorkstationName: "review",
		InputTokens:     workers.InputTokens(modelWorkerProgressPublisherToken()),
	})
	assertCanonicalProviderCommandRequests(t, runner.Requests())
	return result, execErr, recorded
}

const (
	modelWorkerProgressPublisherSessionID  = "session-provider-progress-publication"
	modelWorkerProgressPublisherDispatchID = "d-model-worker-provider-progress"
)

func newModelWorkerProgressPublisherServiceFixture(t *testing.T) (string, *factoryconfig.LoadedFactoryConfig) {
	t.Helper()

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
		Workers:      []workerconfig.Config{{Name: "worker-a"}},
	},
		map[string]*workerconfig.Config{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)
	return dir, cfg
}

func newModelWorkerProgressPublisherSessions(dir string) *factorysessions.Registry {
	sessions := factorysessions.NewRegistry()
	sessions.Upsert(factorysessions.NewLiveSession(
		modelWorkerProgressPublisherSessionID,
		dir,
		dir,
		dir,
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		&liveSessionState{},
		false,
		filepath.Base(dir),
	), true)
	return sessions
}

func modelWorkerProgressPublisherToken() factorytoken.Token {
	return factorytoken.Token{
		ID: "tok-model-worker-provider-progress",
		Color: factorytoken.Color{
			WorkID:  "work-model-worker-provider-progress",
			Payload: []byte("helpful input"),
		},
	}
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
			modelLocality: workerconfig.ModelLocalityLocal,
		},
		{
			name:          "cloud tts worker",
			model:         "gpt-4o-mini-tts",
			modelLocality: workerconfig.ModelLocalityCloud,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			provider, wsExec := modelInvokeExecutionFixture(t, tt.model, tt.modelLocality)
			result, err := wsExec.Execute(context.Background(), modelInvokeDispatch())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Outcome != workerexecution.OutcomeAccepted {
				t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
			}
			assertModelInvokeProviderCall(t, provider.Calls(), tt.model, tt.modelLocality)
		})
	}
}

// backendsizecheck:ignore-function this managed-runtime integration test keeps the local model-invoke execution assertion path in one place.
func TestLoadWorkersFromConfig_ModelInvokeWorkstationExecutesThroughLocalManagedRuntimeEdge(t *testing.T) {
	t.Parallel()

	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	provider := &providerCallRecorder{}
	runtime := &fakeLocalModelRuntime{
		response: workerexecution.InferenceResponse{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		},
	}

	wsExec := modelInvokeWorkstationExecutorForLocalManagedRuntime(t, runtime, provider)
	result, err := wsExec.Execute(context.Background(), modelInvokeDispatch())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	assertModelInvokeAcceptedAudioOutput(t, result.Output, audioPath)
	if runtime.invocationCount() != 1 {
		t.Fatalf("managed runtime invocation count = %d, want 1", runtime.invocationCount())
	}
	if calls := provider.Calls(); len(calls) != 0 {
		t.Fatalf("provider calls = %#v, want local managed runtime to bypass provider path", calls)
	}
	assertManagedRuntimeModelInvokeInvocation(t, runtime.invocationRequests(), workerconfig.ModelLocalityLocal)
}

func modelInvokeWorkstationExecutorForLocalManagedRuntime(
	t *testing.T,
	runtime *fakeLocalModelRuntime,
	provider *providerCallRecorder,
) *workers.WorkstationExecutor {
	t.Helper()

	factoryCfg := localModelFactoryConfig()
	cache := localModelTestCacheLayout(t)
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", factoryCfg, map[string]*workerconfig.Config{
		"tts-worker": modelInvokeLocalManagedRuntimeWorker(),
	}, map[string]*interfaces.FactoryWorkstationConfig{
		"speak": modelInvokeWorkstationConfig(),
	})

	opts, err := loadWorkersFromConfig(
		"",
		factoryCfg,
		"",
		runtimeCfg,
		nil,
		logging.NoopLogger{},
		true,
		nil,
		provider,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		LocalModelDomain{
			Resources: newLocalModelResourceLimiter(),
			Assets:    staticModelAssetPuller{cache: cache},
			Runtime:   runtime,
			Manager:   newManagedLocalModelManager(staticModelAssetPuller{cache: cache}, runtime),
		},
	)
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
	return wsExec
}

func modelInvokeLocalManagedRuntimeWorker() *workerconfig.Config {
	worker := modelInvokeRuntimeWorker("OMNIVOICE_Q4_K_M", workerconfig.ModelLocalityLocal)
	worker.Resources = []factoryresource.Config{{
		Name:     "omnivoice-cache",
		Capacity: 1,
	}}
	return worker
}

func (r *fakeLocalModelRuntime) invocationRequests() []localModelInvocationRequest {
	r.mu.Lock()
	defer r.mu.Unlock()

	requests := make([]localModelInvocationRequest, len(r.invocations))
	copy(requests, r.invocations)
	return requests
}

func assertManagedRuntimeModelInvokeInvocation(
	t *testing.T,
	invocations []localModelInvocationRequest,
	wantLocality string,
) {
	t.Helper()

	if len(invocations) != 1 {
		t.Fatalf("managed runtime invocations = %d, want 1", len(invocations))
	}
	request := invocations[0].Request
	if request.ModelOperation != "TTS" {
		t.Fatalf("managed runtime model operation = %q, want TTS", request.ModelOperation)
	}
	if request.ModelLocality != wantLocality {
		t.Fatalf("managed runtime model locality = %q, want %q", request.ModelLocality, wantLocality)
	}
	if request.Model != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("managed runtime model = %q, want OMNIVOICE_Q4_K_M", request.Model)
	}
	if len(request.ModelBindings) != 2 {
		t.Fatalf("managed runtime bindings = %#v, want 2 entries", request.ModelBindings)
	}
	assertModelInvokeTextBinding(t, request.ModelBindings[0])
	assertModelInvokeVoiceBinding(t, request.ModelBindings[1])
}

func assertModelInvokeAcceptedAudioOutput(t *testing.T, output string, audioPath string) {
	t.Helper()

	var content []work.WorkContentPart
	if err := json.Unmarshal([]byte(output), &content); err != nil {
		t.Fatalf("decode accepted model invoke output: %v", err)
	}
	if len(content) != 1 || content[0].Type != work.WorkContentPartTypeAudio || content[0].File != audioPath {
		t.Fatalf("accepted output = %#v, want one audio part at %q", content, audioPath)
	}
}

func modelInvokeExecutionFixture(t *testing.T, model string, modelLocality string) (*providerCallRecorder, *workers.WorkstationExecutor) {
	t.Helper()
	provider := &providerCallRecorder{responses: []workerexecution.InferenceResponse{{Content: mustMarshalAudioContentResponse(t, filepath.Join(t.TempDir(), "speech.wav"))}}}
	factoryCfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "speak", WorkerTypeName: "tts-worker"}},
		Workers:      []workerconfig.Config{{Name: "tts-worker"}},
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", factoryCfg,
		map[string]*workerconfig.Config{"tts-worker": modelInvokeRuntimeWorker(model, modelLocality)},
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

func modelInvokeRuntimeWorker(model string, modelLocality string) *workerconfig.Config {
	return &workerconfig.Config{
		Name:          "tts-worker",
		Type:          interfaces.WorkerTypeModel,
		Model:         model,
		ModelProvider: workerexecution.RunnerIDCodex,
		ModelLocality: modelLocality,
		Body:          "You are a TTS worker.",
		Operations: []workerconfig.ModelOperation{{
			Name: "TTS",
			Inputs: []workerconfig.ModelOperationSlot{
				{Name: "text", ContentTypes: []string{workerconfig.ModelOperationContentTypeText}, Required: true},
				{Name: "voice", ContentTypes: []string{workerconfig.ModelOperationContentTypeJSON}},
			},
			Outputs: []workerconfig.ModelOperationSlot{{Name: "audio", ContentTypes: []string{workerconfig.ModelOperationContentTypeAudio}}},
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
					Type:  workerconfig.ModelOperationContentTypeText,
				},
			},
			{
				Slot: "voice",
				Config: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeJSON,
					Role: "voice",
					JSON: []byte(`{"name":"alloy"}`),
				}},
			},
		},
	}
}

func modelInvokeDispatch() work.WorkDispatch {
	return work.WorkDispatch{
		DispatchID:      "dispatch-tts",
		TransitionID:    "transition-tts",
		WorkerType:      "tts-worker",
		WorkstationName: "speak",
		InputTokens: workers.InputTokens(factorytoken.Token{
			ID: "token-tts",
			Color: factorytoken.Color{
				WorkID: "work-tts",
				Content: []work.WorkContentPart{{
					Type:  work.WorkContentPartTypeText,
					Label: "utterance",
					Text:  "hello world",
				}},
			},
		}),
	}
}

func assertModelInvokeProviderCall(t *testing.T, calls []workerexecution.ProviderInferenceRequest, wantModel string, wantLocality string) {
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

func assertModelInvokeTextBinding(t *testing.T, binding workerexecution.ResolvedModelOperationBinding) {
	t.Helper()
	if binding.Slot != "text" || binding.Source != workerexecution.ModelOperationBindingSourceInput || binding.Content[0].Text != "hello world" {
		t.Fatalf("text model binding = %#v, want generic text slot from input", binding)
	}
}

func assertModelInvokeVoiceBinding(t *testing.T, binding workerexecution.ResolvedModelOperationBinding) {
	t.Helper()
	if binding.Slot != "voice" || binding.Source != workerexecution.ModelOperationBindingSourceConfig || string(binding.Content[0].JSON) != `{"name":"alloy"}` {
		t.Fatalf("voice model binding = %#v, want config voice binding", binding)
	}
}

func TestLoadWorkersFromConfig_ReplayEmbeddedRuntimeUsesCanonicalLookup(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "review")

	loaded := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []workerconfig.Config{{Name: "worker-a"}},
	},
		map[string]*workerconfig.Config{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	generated, err := generatedFactoryFromRuntimeConfigForTest(loaded.FactoryDir(), loaded.FactoryConfig(), loaded)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromRuntimeConfig: %v", err)
	}
	runtimeCfg, err := runtimeConfigFromGeneratedFactoryForTest(generated)
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
		Workers:      []workerconfig.Config{{Name: "worker-a"}},
	},
		map[string]*workerconfig.Config{
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
		Workers: []workerconfig.Config{{Name: "script-worker"}},
	},
		map[string]*workerconfig.Config{
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

	result, err := wsExec.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-runtime-lookup-script",
		TransitionID:    "t-runtime-lookup-script",
		WorkerType:      "script-worker",
		WorkstationName: "run-script",
		ProjectID:       "agent-factory",
		InputTokens: workers.InputTokens(factorytoken.Token{
			ID: "tok-runtime-lookup-script",
			Color: factorytoken.Color{
				WorkID: "work-runtime-lookup-script",
			},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
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
		Workers: []workerconfig.Config{{Name: "script-worker"}},
	},
		map[string]*workerconfig.Config{
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

	result, err := wsExec.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-runtime-lookup-script-ref",
		TransitionID:    "t-runtime-lookup-script-ref",
		WorkerType:      "script-worker",
		WorkstationName: "run-script",
		ProjectID:       "agent-factory",
		InputTokens: workers.InputTokens(factorytoken.Token{
			ID: "tok-runtime-lookup-script-ref",
			Color: factorytoken.Color{
				WorkID: "work-runtime-lookup-script-ref",
			},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
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
		Workers: []workerconfig.Config{{Name: "script-worker"}},
	},
		map[string]*workerconfig.Config{
			"script-worker": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "script-worker")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"run-script": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "run-script")),
		},
	)

	generated, err := generatedFactoryFromRuntimeConfigForTest(loaded.FactoryDir(), loaded.FactoryConfig(), loaded)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromRuntimeConfig: %v", err)
	}
	runtimeCfg, err := runtimeConfigFromGeneratedFactoryForTest(generated)
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

	result, err := wsExec.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-replay-runtime-lookup-script",
		TransitionID:    "t-replay-runtime-lookup-script",
		WorkerType:      "script-worker",
		WorkstationName: "run-script",
		ProjectID:       "agent-factory",
		InputTokens: workers.InputTokens(factorytoken.Token{
			ID: "tok-replay-runtime-lookup-script",
			Color: factorytoken.Color{
				WorkID: "work-replay-runtime-lookup-script",
			},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	if got := runner.request.WorkDir; got != filepath.Join(dir, "workspace") {
		t.Fatalf("command working directory = %q, want %q", got, filepath.Join(dir, "workspace"))
	}
}

func TestLoadWorkersFromConfig_ScriptWorkerUsesWorkstationExecutor(t *testing.T) {
	dir := t.TempDir()
	scriptRecorder := func(workerexecution.ScriptEvent) {}

	writeScriptWorkerAgentsMD(t, dir, "script-worker")
	writeWorkstationAgentsMD(t, dir, "run-script")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "run-script"}},
		Workers:      []workerconfig.Config{{Name: "script-worker"}},
	},
		map[string]*workerconfig.Config{
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
		nil,
		logging.NoopLogger{},
		false,
		nil,
		providerOverride,
		nil,
		providerCommandRunner,
		commandRunner,
		scriptRecorder,
		inferenceRecorder,
		nil,
		nil,
		nil,
		LocalModelDomain{},
	)
}

func assertCanonicalModelWorkerExecutionResult(t *testing.T, result workerexecution.WorkResult) {
	t.Helper()

	if result.Outcome != workerexecution.OutcomeAccepted || result.Output != "provider-output COMPLETE" {
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
	if requests[0].Command != string(modelprovider.Codex) {
		t.Fatalf("provider command = %q, want %q", requests[0].Command, modelprovider.Codex)
	}
}

func assertRecordedInferenceEvents(t *testing.T, recorded []workerexecution.InferenceEvent) {
	t.Helper()

	if len(recorded) != 2 {
		t.Fatalf("recorded event count = %d, want 2", len(recorded))
	}
	if recorded[0].Kind != workerexecution.InferenceEventKindRequest || recorded[0].Request == nil || recorded[0].Response != nil {
		t.Fatalf("first event = %#v, want inference request", recorded[0])
	}
	if recorded[1].Kind != workerexecution.InferenceEventKindResponse || recorded[1].Response == nil || recorded[1].Request != nil {
		t.Fatalf("second event = %#v, want inference response", recorded[1])
	}
}

func generatedAudioFileValue(file *factoryapi.WorkContentDeprecatedFileProperty) string {
	if file == nil {
		return ""
	}
	return string(*file)
}
func TestWorkerWorkstationTaxonomyRuntime_InferencePairingExecutesLikeLegacyModelInvoke(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		publicWorkerType       string
		publicWorkstationType  string
		wantRuntimeWorkerType  string
		wantRuntimeWorkstation string
	}{
		{
			name:                   "inference taxonomy",
			publicWorkerType:       interfaces.WorkerTypeInference,
			publicWorkstationType:  interfaces.WorkstationTypeInference,
			wantRuntimeWorkerType:  interfaces.WorkerTypeModel,
			wantRuntimeWorkstation: interfaces.WorkstationTypeInvoke,
		},
		{
			name:                   "legacy model invoke",
			publicWorkerType:       interfaces.WorkerTypeModel,
			publicWorkstationType:  interfaces.WorkstationTypeInvoke,
			wantRuntimeWorkerType:  interfaces.WorkerTypeModel,
			wantRuntimeWorkstation: interfaces.WorkstationTypeInvoke,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := mustTaxonomyRuntimeFactoryConfigFromOpenAPI(
				t,
				tt.publicWorkerType,
				tt.publicWorkstationType,
				tt.wantRuntimeWorkerType,
				tt.wantRuntimeWorkstation,
			)
			provider, wsExec := taxonomyModelInvokeExecutionFixtureFromRuntimeConfig(t, cfg)

			result, err := wsExec.Execute(context.Background(), modelInvokeDispatch())
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if result.Outcome != workerexecution.OutcomeAccepted {
				t.Fatalf("outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
			}
			assertModelInvokeProviderCall(t, provider.Calls(), "gpt-4o-mini-tts", workerconfig.ModelLocalityCloud)
		})
	}
}

func TestWorkerWorkstationTaxonomyRuntime_OmniVoiceInferenceExecutesWithoutAgentLoopFields(t *testing.T) {
	t.Parallel()

	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtime := &fakeLocalModelRuntime{
		response: workerexecution.InferenceResponse{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		},
	}
	cfg := mustTaxonomyRuntimeFactoryConfigFromOpenAPI(
		t,
		interfaces.WorkerTypeInference,
		interfaces.WorkstationTypeInference,
		interfaces.WorkerTypeModel,
		interfaces.WorkstationTypeInvoke,
	)
	wsExec := taxonomyOmniVoiceInferenceWorkstationExecutor(t, runtime, cfg)

	result, err := wsExec.Execute(context.Background(), modelInvokeDispatch())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	assertModelInvokeAcceptedAudioOutput(t, result.Output, audioPath)
	if runtime.invocationCount() != 1 {
		t.Fatalf("managed runtime invocation count = %d, want 1 bounded inference operation", runtime.invocationCount())
	}
	invocations := runtime.invocationRequests()
	if len(invocations) != 1 {
		t.Fatalf("invocation requests = %d, want 1", len(invocations))
	}
	request := invocations[0].Request
	if request.ModelOperation != "TTS" {
		t.Fatalf("model operation = %q, want TTS inference operation", request.ModelOperation)
	}
	if strings.TrimSpace(request.OpenCodeAgent) != "" {
		t.Fatalf("open code agent = %q, want empty for inference-run execution", request.OpenCodeAgent)
	}
	if request.Model != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("model = %q, want OMNIVOICE_Q4_K_M inference model identity", request.Model)
	}
	if request.ModelLocality != workerconfig.ModelLocalityLocal {
		t.Fatalf("model locality = %q, want %q", request.ModelLocality, workerconfig.ModelLocalityLocal)
	}
	if len(request.ModelBindings) != 1 || request.ModelBindings[0].Slot != "text" {
		t.Fatalf("model bindings = %#v, want one resolved text input binding", request.ModelBindings)
	}
}

func TestWorkerWorkstationTaxonomyRuntime_InferenceTaxonomyRecordsModelExecutionEvents(t *testing.T) {
	t.Parallel()

	eventTime := taxonomyRuntimeEventTime()
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtime := &fakeLocalModelRuntime{
		response: workerexecution.InferenceResponse{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		},
	}
	cfg := mustTaxonomyRuntimeFactoryConfigFromOpenAPI(
		t,
		interfaces.WorkerTypeInference,
		interfaces.WorkstationTypeInference,
		interfaces.WorkerTypeModel,
		interfaces.WorkstationTypeInvoke,
	)
	wsExec, history := taxonomyOmniVoiceInferenceWorkstationExecutorWithEvents(t, runtime, cfg, eventTime)

	result, err := wsExec.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "dispatch-taxonomy-inference",
		TransitionID:    "transition-taxonomy-inference",
		WorkerType:      "tts-worker",
		WorkstationName: "speak",
		Execution: work.ExecutionMetadata{
			CurrentTick: 2,
			RequestID:   "request-taxonomy-inference",
			TraceID:     "trace-taxonomy-inference",
			WorkIDs:     []string{"work-taxonomy-inference"},
		},
		InputTokens: modelInvokeDispatch().InputTokens,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	assertRecordedLocalModelExecutionEvents(t, generatedFactoryEventsForTest(t, history.CanonicalEvents()), audioPath)
}

func TestWorkerWorkstationTaxonomyRuntime_AgentRunWithInferenceWorkerFailsValidationBeforeDispatch(t *testing.T) {
	t.Parallel()

	generated := taxonomyRuntimeIncompatibleInferenceWorkerAgentRunFactory()

	result, err := validationentry.ValidateFactoryAPI(context.Background(), generated, factoryvalidation.Options{})
	if err != nil {
		t.Fatalf("ValidateFactoryAPI: %v", err)
	}
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkerWorkstationBehaviorCompatibility)
	target := taxonomyRuntimeFindTargetByCode(t, result.Targets, factoryvalidation.CodeWorkerWorkstationBehaviorCompatibility)
	if !strings.Contains(target.Message, interfaces.WorkstationTypeAgent) {
		t.Fatalf("message %q missing workstation type %q", target.Message, interfaces.WorkstationTypeAgent)
	}
	if !strings.Contains(target.Message, interfaces.WorkerTypeInference) {
		t.Fatalf("message %q missing worker type %q", target.Message, interfaces.WorkerTypeInference)
	}
}

func mustTaxonomyRuntimeFactoryConfigFromOpenAPI(
	t *testing.T,
	publicWorkerType string,
	publicWorkstationType string,
	wantRuntimeWorkerType string,
	wantRuntimeWorkstationType string,
) *interfaces.FactoryConfig {
	t.Helper()

	generated, err := factoryconfig.GeneratedFactoryFromOpenAPIJSON(
		taxonomyRuntimeModelInvokeFactoryJSON(publicWorkerType, publicWorkstationType),
	)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	cfg, err := factoryconfig.FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	if len(cfg.Workers) != 1 {
		t.Fatalf("workers = %#v, want one worker", cfg.Workers)
	}
	if cfg.Workers[0].Type != wantRuntimeWorkerType {
		t.Fatalf("runtime worker type = %q, want %q", cfg.Workers[0].Type, wantRuntimeWorkerType)
	}
	if len(cfg.Workstations) != 1 {
		t.Fatalf("workstations = %#v, want one workstation", cfg.Workstations)
	}
	if cfg.Workstations[0].Type != wantRuntimeWorkstationType {
		t.Fatalf("runtime workstation type = %q, want %q", cfg.Workstations[0].Type, wantRuntimeWorkstationType)
	}
	return &cfg
}

func taxonomyModelInvokeExecutionFixtureFromRuntimeConfig(
	t *testing.T,
	cfg *interfaces.FactoryConfig,
) (*providerCallRecorder, *workers.WorkstationExecutor) {
	t.Helper()

	provider := &providerCallRecorder{responses: []workerexecution.InferenceResponse{{Content: mustMarshalAudioContentResponse(t, filepath.Join(t.TempDir(), "speech.wav"))}}}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(
		t,
		"",
		cfg,
		map[string]*workerconfig.Config{"tts-worker": taxonomyRuntimeModelInvokeWorker(cfg.Workers[0].Type)},
		map[string]*interfaces.FactoryWorkstationConfig{"speak": taxonomyRuntimeModelInvokeWorkstation(cfg.Workstations[0].Type)},
	)
	opts, err := loadWorkersFromConfigForServiceTest("", cfg, "", runtimeCfg, provider, nil, nil, nil, nil)
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

func taxonomyOmniVoiceInferenceWorkstationExecutor(
	t *testing.T,
	runtime *fakeLocalModelRuntime,
	cfg *interfaces.FactoryConfig,
) *workers.WorkstationExecutor {
	t.Helper()

	wsExec, _ := taxonomyOmniVoiceInferenceWorkstationExecutorWithEvents(t, runtime, cfg, taxonomyRuntimeEventTime())
	return wsExec
}

func taxonomyOmniVoiceInferenceWorkstationExecutorWithEvents(
	t *testing.T,
	runtime *fakeLocalModelRuntime,
	cfg *interfaces.FactoryConfig,
	eventTime time.Time,
) (*workers.WorkstationExecutor, *factoryevents.FactoryEventHistory) {
	t.Helper()

	cache := localModelTestCacheLayout(t)
	factoryCfg := localModelFactoryConfig()
	runtimeWorkers := localModelRuntimeWorkers()
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", factoryCfg, runtimeWorkers, map[string]*interfaces.FactoryWorkstationConfig{
		"speak": taxonomyRuntimeModelInvokeWorkstation(cfg.Workstations[0].Type),
	})
	history := factoryevents.NewFactoryEventHistory(nil, func() time.Time { return eventTime }, runtimeCfg)
	opts, err := loadWorkersFromConfig(
		"",
		factoryCfg,
		"",
		runtimeCfg,
		nil,
		logging.NoopLogger{},
		true,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		history.RecordModelEvent,
		nil,
		func() time.Time { return eventTime },
		LocalModelDomain{
			Resources: newLocalModelResourceLimiter(),
			Assets:    staticModelAssetPuller{cache: cache},
			Runtime:   runtime,
			Manager:   newManagedLocalModelManager(staticModelAssetPuller{cache: cache}, runtime),
		},
	)
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
	return wsExec, history
}

func taxonomyRuntimeModelInvokeFactoryJSON(workerType, workstationType string) []byte {
	payload := map[string]any{
		"name": "taxonomy-runtime-factory",
		"workTypes": []map[string]any{{
			"name": "speech",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":          "tts-worker",
			"type":          workerType,
			"model":         "gpt-4o-mini-tts",
			"modelProvider": "CODEX",
			"modelLocality": workerconfig.ModelLocalityCloud,
			"operations": []map[string]any{{
				"name": "TTS",
				"inputs": []map[string]any{
					{
						"name":         "text",
						"contentTypes": []string{workerconfig.ModelOperationContentTypeText},
						"required":     true,
					},
					{
						"name":         "voice",
						"contentTypes": []string{workerconfig.ModelOperationContentTypeJSON},
					},
				},
				"outputs": []map[string]any{{
					"name":         "audio",
					"contentTypes": []string{workerconfig.ModelOperationContentTypeAudio},
				}},
			}},
		}},
		"workstations": []map[string]any{{
			"name":      "speak",
			"type":      workstationType,
			"worker":    "tts-worker",
			"operation": "TTS",
			"operationBindings": []map[string]any{
				{
					"slot": "text",
					"selector": map[string]any{
						"label": "utterance",
						"type":  workerconfig.ModelOperationContentTypeText,
					},
				},
				{
					"slot": "voice",
					"config": []map[string]any{{
						"type": work.WorkContentPartTypeJSON,
						"role": "voice",
						"json": map[string]string{"name": "alloy"},
					}},
				},
			},
			"inputs":    []map[string]string{{"workType": "speech", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "speech", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "speech", "state": "failed"}},
		}},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return data
}

func taxonomyRuntimeModelInvokeWorker(runtimeWorkerType string) *workerconfig.Config {
	worker := modelInvokeRuntimeWorker("gpt-4o-mini-tts", workerconfig.ModelLocalityCloud)
	worker.Type = runtimeWorkerType
	return worker
}

func taxonomyRuntimeModelInvokeWorkstation(runtimeWorkstationType string) *interfaces.FactoryWorkstationConfig {
	workstation := modelInvokeWorkstationConfig()
	workstation.Type = runtimeWorkstationType
	return workstation
}

func taxonomyRuntimeEventTime() time.Time {
	return time.Date(2026, time.June, 11, 18, 0, 0, 0, time.UTC)
}

func taxonomyRuntimeIncompatibleInferenceWorkerAgentRunFactory() factoryapi.Factory {
	workerType := factoryapi.WorkerTypeInferenceWorker
	workstationType := factoryapi.WorkstationTypeAgentRun
	modelProvider := factoryapi.WorkerModelProviderCodex
	return factoryapi.Factory{
		Name: "taxonomy-runtime-factory",
		WorkTypes: &[]factoryapi.WorkType{{
			Name: "speech",
			States: []factoryapi.WorkState{
				{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
				{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
				{Name: "failed", Type: factoryapi.WorkStateTypeFAILED},
			},
		}},
		Workers: &[]factoryapi.Worker{{
			Name:          "tts-worker",
			Type:          &workerType,
			Model:         stringPtr("gpt-4o-mini-tts"),
			ModelProvider: &modelProvider,
		}},
		Workstations: &[]factoryapi.Workstation{{
			Name:   "speak",
			Type:   &workstationType,
			Worker: "tts-worker",
			Inputs: []factoryapi.WorkstationIO{{
				WorkType: "speech",
				State:    "init",
			}},
			Outputs: &[]factoryapi.WorkstationIO{{
				WorkType: "speech",
				State:    "complete",
			}},
			OnFailure: &[]factoryapi.WorkstationIO{{
				WorkType: "speech",
				State:    "failed",
			}},
		}},
	}
}

func taxonomyRuntimeFindTargetByCode(t *testing.T, targets []factoryvalidation.Target, code string) factoryvalidation.Target {
	t.Helper()
	for _, target := range targets {
		if target.Code == code {
			return target
		}
	}
	t.Fatalf("target with code %q not found in %#v", code, targets)
	return factoryvalidation.Target{}
}
func TestLoadWorkersFromConfig_InferenceWorkerUsesModelHostLeases(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtime := &fakeLocalModelRuntime{
		response: workerexecution.InferenceResponse{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		},
	}
	cache := localModelTestCacheLayout(t)
	puller := staticModelAssetPuller{cache: cache}
	launcher := &serviceTestFakeProcessLauncher{healthEndpoint: healthServer.URL}
	domain := modelHostBackedLocalModelDomain(t, puller, launcher, runtime)

	factoryCfg := inferenceModelHostFactoryConfig()
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", factoryCfg, inferenceModelHostRuntimeWorkers(healthServer.URL), map[string]*interfaces.FactoryWorkstationConfig{
		"speak": inferenceModelHostWorkstation(),
	})

	opts, err := loadWorkersFromConfig(
		"",
		factoryCfg,
		"",
		runtimeCfg,
		nil,
		logging.NoopLogger{},
		true,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		domain,
	)
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

	result, err := wsExec.Execute(context.Background(), inferenceModelHostDispatch())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	assertModelInvokeAcceptedAudioOutput(t, result.Output, audioPath)
	if launcher.startCount() != 1 {
		t.Fatalf("supervised process start count = %d, want 1 shared host runtime", launcher.startCount())
	}
	if runtime.loadCount() != 1 || runtime.invocationCount() != 1 {
		t.Fatalf("runtime load/invoke counts = %d/%d, want 1/1", runtime.loadCount(), runtime.invocationCount())
	}
	if got := runtime.lastLoadServingEndpoint(); got != healthServer.URL {
		t.Fatalf("serving endpoint = %q, want supervised lease endpoint %q", got, healthServer.URL)
	}
}

func TestFactorySessionInvocation_LocalLlamaCppInferenceUsesModelHostLeases(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtime := &fakeLocalModelRuntime{
		response: workerexecution.InferenceResponse{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		},
	}

	server, launcher, svc, shutdown := startLocalModelInferenceTestServer(t, runtime)
	defer shutdown()
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sourceKind := factoryapi.InvocationInputSourceKindText
	content := factoryapi.WorkContent{
		mustGeneratedLocalModelHTTPTextPart(t, "hello factory session inference"),
	}

	result, err := svc.InvokeFactorySession(ctx, factorysessions.DefaultSessionID, factoryapi.InvocationRequest{
		SourceKind: &sourceKind,
		Content:    &content,
	})
	if err != nil {
		t.Fatalf("InvokeFactorySession: %v", err)
	}
	if result.Status != "COMPLETED" {
		t.Fatalf("invocation status = %q, want COMPLETED (error=%q message=%q work=%q state=%q)", result.Status, result.ErrorCode, result.Message, result.WorkName, result.WorkState)
	}
	if len(result.PrimaryResult) == 0 {
		t.Fatalf("primaryResult = %#v, want invocation to participate in primary-result selection", result.PrimaryResult)
	}
	events, err := svc.GetFactoryEvents(ctx)
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	assertModelHostInferenceEventsInHistory(t, events, audioPath)
	if launcher.startCount() != 1 {
		t.Fatalf("supervised process start count = %d, want 1 shared host runtime", launcher.startCount())
	}
	if runtime.loadCount() != 1 || runtime.invocationCount() != 1 {
		t.Fatalf("runtime load/invoke counts = %d/%d, want 1/1", runtime.loadCount(), runtime.invocationCount())
	}
	if got := runtime.lastLoadServingEndpoint(); got != launcher.healthEndpoint {
		t.Fatalf("serving endpoint = %q, want supervised lease endpoint %q", got, launcher.healthEndpoint)
	}
}

func TestWorkerWorkstationTaxonomyRuntime_InferenceWorkerUsesModelHostLeases(t *testing.T) {
	t.Parallel()

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtime := &fakeLocalModelRuntime{
		response: workerexecution.InferenceResponse{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		},
	}
	cfg := mustTaxonomyRuntimeFactoryConfigFromOpenAPI(
		t,
		interfaces.WorkerTypeInference,
		interfaces.WorkstationTypeInference,
		interfaces.WorkerTypeModel,
		interfaces.WorkstationTypeInvoke,
	)
	wsExec, launcher := taxonomyOmniVoiceInferenceWorkstationExecutorWithModelHost(t, runtime, cfg, healthServer.URL)

	result, err := wsExec.Execute(context.Background(), modelInvokeDispatch())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	assertModelInvokeAcceptedAudioOutput(t, result.Output, audioPath)
	if launcher.startCount() != 1 {
		t.Fatalf("supervised process start count = %d, want 1 shared host runtime", launcher.startCount())
	}
	if got := runtime.lastLoadServingEndpoint(); got != healthServer.URL {
		t.Fatalf("serving endpoint = %q, want supervised lease endpoint %q", got, healthServer.URL)
	}
}

func modelHostBackedLocalModelDomain(
	t *testing.T,
	puller modelAssetPuller,
	launcher *serviceTestFakeProcessLauncher,
	runtime *fakeLocalModelRuntime,
) localModelDomain {
	t.Helper()
	host := newServiceTestSupervisedModelHost(t, puller, launcher)
	leaseExec := modelhost.NewLeaseExecution(host, puller, runtime, localModelHooks())
	return LocalModelDomain{
		Resources:      newLocalModelResourceLimiter(),
		Assets:         puller,
		Runtime:        runtime,
		Manager:        newManagedLocalModelManager(puller, runtime),
		Host:           host,
		LeaseExecution: leaseExec,
	}
}

func inferenceModelHostFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		Resources: []factoryresource.Config{{
			Name:       "omnivoice-cache",
			Type:       factoryresource.TypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "speak",
			WorkerTypeName: "tts-worker",
		}},
		Workers: []workerconfig.Config{{
			Name: "tts-worker",
		}},
	}
}

func inferenceModelHostRuntimeWorkers(healthEndpoint string) map[string]*workerconfig.Config {
	worker := &workerconfig.Config{
		Name:          "tts-worker",
		Type:          interfaces.WorkerTypeInference,
		Model:         "OMNIVOICE_Q4_K_M",
		ModelProvider: workerexecution.RunnerIDCodex,
		ModelLocality: workerconfig.ModelLocalityLocal,
		Resources: []factoryresource.Config{{
			Name:     "omnivoice-cache",
			Capacity: 1,
		}},
		Operations: []workerconfig.ModelOperation{{
			Name: "TTS",
			Inputs: []workerconfig.ModelOperationSlot{{
				Name:         "text",
				ContentTypes: []string{workerconfig.ModelOperationContentTypeText},
				Required:     true,
			}},
			Outputs: []workerconfig.ModelOperationSlot{{
				Name:         "audio",
				ContentTypes: []string{workerconfig.ModelOperationContentTypeAudio},
			}},
		}},
	}
	if strings.TrimSpace(healthEndpoint) != "" {
		worker.Args = []string{"--health-endpoint", healthEndpoint}
	}
	return map[string]*workerconfig.Config{"tts-worker": worker}
}

func inferenceModelHostWorkstation() *interfaces.FactoryWorkstationConfig {
	return &interfaces.FactoryWorkstationConfig{
		Name:           "speak",
		Type:           interfaces.WorkstationTypeInference,
		WorkerTypeName: "tts-worker",
		Operation:      "TTS",
		OperationBindings: []interfaces.ModelOperationBinding{{
			Slot: "text",
			Selector: &interfaces.ModelOperationBindingSelector{
				Label: "utterance",
				Type:  workerconfig.ModelOperationContentTypeText,
			},
		}},
	}
}

func inferenceModelHostDispatch() work.WorkDispatch {
	return work.WorkDispatch{
		DispatchID:      "dispatch-inference-lease",
		TransitionID:    "transition-inference-lease",
		WorkerType:      "tts-worker",
		WorkstationName: "speak",
		InputTokens: workers.InputTokens(factorytoken.Token{
			ID: "token-inference-lease",
			Color: factorytoken.Color{
				WorkID: "work-inference-lease",
				Content: []work.WorkContentPart{{
					Type:  work.WorkContentPartTypeText,
					Label: "utterance",
					Text:  "hello inference lease",
				}},
			},
		}),
	}
}

func taxonomyOmniVoiceInferenceWorkstationExecutorWithModelHost(
	t *testing.T,
	runtime *fakeLocalModelRuntime,
	cfg *interfaces.FactoryConfig,
	healthEndpoint string,
) (*workers.WorkstationExecutor, *serviceTestFakeProcessLauncher) {
	t.Helper()

	cache := localModelTestCacheLayout(t)
	puller := staticModelAssetPuller{cache: cache}
	launcher := &serviceTestFakeProcessLauncher{healthEndpoint: healthEndpoint}
	domain := modelHostBackedLocalModelDomain(t, puller, launcher, runtime)
	factoryCfg := localModelFactoryConfig()
	runtimeWorkers := inferenceModelHostRuntimeWorkers(healthEndpoint)
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", factoryCfg, runtimeWorkers, map[string]*interfaces.FactoryWorkstationConfig{
		"speak": taxonomyRuntimeModelInvokeWorkstation(cfg.Workstations[0].Type),
	})
	eventTime := taxonomyRuntimeEventTime()
	history := factoryevents.NewFactoryEventHistory(nil, func() time.Time { return eventTime }, runtimeCfg)
	opts, err := loadWorkersFromConfig(
		"",
		factoryCfg,
		"",
		runtimeCfg,
		nil,
		logging.NoopLogger{},
		true,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		history.RecordModelEvent,
		nil,
		func() time.Time { return eventTime },
		domain,
	)
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
	return wsExec, launcher
}

func (r *fakeLocalModelRuntime) lastLoadServingEndpoint() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.loads) == 0 {
		return ""
	}
	return strings.TrimSpace(r.loads[0].ServingEndpoint)
}

func assertModelHostInferenceEventsInHistory(t *testing.T, events []factoryapi.FactoryEvent, audioPath string) {
	t.Helper()
	modelEvents := make([]factoryapi.FactoryEvent, 0, 2)
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeModelRequest || event.Type == factoryapi.FactoryEventTypeModelResponse {
			modelEvents = append(modelEvents, event)
		}
	}
	assertRecordedLocalModelExecutionEvents(t, modelEvents, audioPath)
}
