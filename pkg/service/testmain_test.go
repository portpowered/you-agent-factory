package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/api"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"go.uber.org/zap"
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
	if svc.modelResources == nil {
		t.Fatal("expected BuildFactoryService to initialize modelResources")
	}
	if svc.localModels == nil {
		t.Fatal("expected BuildFactoryService to initialize localModels")
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
	svc.modelAssets = staticModelAssetPuller{cache: localModelTestCacheLayout(t)}
	svc.localModels = newManagedLocalModelManager(svc.modelAssets, runtime)

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
			Command:       defaultOmniVoiceCommand,
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
