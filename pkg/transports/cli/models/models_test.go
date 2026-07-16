package models

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"
	"go.uber.org/zap"
)

func TestRenderList_WritesDiscoveredModelsTable(t *testing.T) {
	var out bytes.Buffer
	err := RenderList(factoryapi.ListModelsResponse{
		Results: []factoryapi.ModelSummary{{
			Name:             "OMNIVOICE_Q4_K_M",
			ProviderLocality: factoryapi.WorkerModelLocalityLocal,
			Status:           factoryapi.ModelStatusREADY,
			LoadState:        factoryapi.UNLOADED,
			ManagedRuntime: factoryapi.ManagedRuntime{
				Identity:       "OMNIVOICE_Q4_K_M",
				ReadinessState: factoryapi.ManagedRuntimeReadinessStateREADY,
				LifecycleState: factoryapi.ManagedRuntimeLifecycleStateINSTALLED,
				Locality:       factoryapi.WorkerModelLocalityLocal,
			},
			Operations: []factoryapi.ModelOperation{{Name: "TTS"}},
			Modalities: []factoryapi.ModelOperationContentType{factoryapi.ModelOperationContentTypeAudio, factoryapi.ModelOperationContentTypeText},
			Resources:  []factoryapi.ModelResourceSummary{{Name: "voice-cache", Type: factoryapi.ResourceTypeModel, Capacity: 1}},
		}},
	}, &out)
	if err != nil {
		t.Fatalf("RenderList: %v", err)
	}
	got := out.String()
	for _, want := range []string{"NAME", "READINESS", "LIFECYCLE", "OMNIVOICE_Q4_K_M", "LOCAL", "READY", "INSTALLED", "TTS", "AUDIO,TEXT"} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("rendered table missing %q:\n%s", want, got)
		}
	}
}

func TestRenderModel_WritesManagedRuntimeInspectFields(t *testing.T) {
	diagnostics := factoryapi.StringMap{
		"readinessState": "MISSING",
		"missingAssets":  "weights.bin",
	}
	var out bytes.Buffer
	err := RenderModel(factoryapi.ModelDetail{
		Name: "SECOND_RUNTIME",
		ManagedRuntime: factoryapi.ManagedRuntime{
			Identity:       "SECOND_RUNTIME",
			ReadinessState: factoryapi.ManagedRuntimeReadinessStateMISSING,
			LifecycleState: factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
			Locality:       factoryapi.WorkerModelLocalityLocal,
			Diagnostics:    &diagnostics,
		},
		ProviderLocality: factoryapi.WorkerModelLocalityLocal,
		Operations:       []factoryapi.ModelOperation{{Name: "EMBED"}},
	}, &out)
	if err != nil {
		t.Fatalf("RenderModel: %v", err)
	}
	got := out.String()
	for _, want := range []string{"Readiness:\tMISSING", "Lifecycle:\tNOT_INSTALLED", "missingAssets=weights.bin"} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered inspect output missing %q:\n%s", want, got)
		}
	}
}

func TestQueryModel_NotFoundUsesFriendlyError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"model not found","family":"NOT_FOUND","code":"NOT_FOUND"}`)
	}))
	defer server.Close()

	serverBase := strings.TrimSuffix(server.URL, "/")
	_, err := QueryModel(serverBase, "missing")
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("error = %v, want ErrModelNotFound", err)
	}
}

func TestInvoke_JSONWritesMetadataResponse(t *testing.T) {
	installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
		return apisurface.ModelInvocationResult{
			ModelName:        modelName,
			Worker:           "tts-worker",
			Operation:        request.Operation,
			ProviderLocality: string(factoryapi.WorkerModelLocalityLocal),
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeAudio,
				File: "artifacts/output.wav",
			}},
		}, nil
	}))

	var out bytes.Buffer
	if err := Invoke(InvokeConfig{
		BuildInvocation: testModelInvocationBuilder,
		ModelName:       "OMNIVOICE_Q4_K_M",
		Operation:       "TTS",
		Text:            "hello world",
		FactoryDir:      t.TempDir(),
		Logger:          zap.NewNop(),
		JSON:            true,
		Output:          &out,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	for _, want := range []string{"OMNIVOICE_Q4_K_M", `"operation":"TTS"`} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestInvoke_AudioWritesOutputFile(t *testing.T) {
	audioBytes := []byte("RIFF....WAVE")
	streamFile := filepath.Join(t.TempDir(), "stream.wav")
	if err := os.WriteFile(streamFile, audioBytes, 0o644); err != nil {
		t.Fatalf("write stream file: %v", err)
	}
	installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
		if request.Options == nil || request.Options.ResponseMode == nil || *request.Options.ResponseMode != factoryapi.AUDIOSTREAM {
			t.Fatalf("request options = %#v, want AUDIO_STREAM mode", request.Options)
		}
		return apisurface.ModelInvocationResult{
			ModelName:  modelName,
			Operation:  request.Operation,
			StreamFile: streamFile,
		}, nil
	}))

	outputPath := filepath.Join(t.TempDir(), "speech.wav")
	if err := Invoke(InvokeConfig{
		BuildInvocation: testModelInvocationBuilder,
		ModelName:       "OMNIVOICE_Q4_K_M",
		Operation:       "TTS",
		Text:            "hello world",
		OutputPath:      outputPath,
		FactoryDir:      t.TempDir(),
		Logger:          zap.NewNop(),
		Output:          io.Discard,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if !bytes.Equal(got, audioBytes) {
		t.Fatalf("output bytes = %q, want %q", got, audioBytes)
	}
}

func TestInvoke_AudioVerboseLogsOutputPath(t *testing.T) {
	audioBytes := []byte("RIFF....WAVE")
	streamFile := filepath.Join(t.TempDir(), "stream.wav")
	if err := os.WriteFile(streamFile, audioBytes, 0o644); err != nil {
		t.Fatalf("write stream file: %v", err)
	}
	installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
		return apisurface.ModelInvocationResult{
			ModelName:  modelName,
			Operation:  request.Operation,
			StreamFile: streamFile,
		}, nil
	}))

	outputPath := filepath.Join(t.TempDir(), "speech.wav")
	var diagnostics bytes.Buffer
	if err := Invoke(InvokeConfig{
		BuildInvocation: testModelInvocationBuilder,
		ModelName:       "OMNIVOICE_Q4_K_M",
		Operation:       "TTS",
		Text:            "hello world",
		OutputPath:      outputPath,
		FactoryDir:      t.TempDir(),
		Logger:          zap.NewNop(),
		Output:          io.Discard,
		Verbose:         true,
		Diagnostics:     &diagnostics,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	assertDiagnosticsContains(t, diagnostics.String(), []string{
		"models invoke bootstrap request",
		"outputPath=" + outputPath,
		"models invoke bootstrap response",
		"outputPath=" + outputPath,
	})
}

func TestInvoke_AudioNotFoundUsesFriendlyError(t *testing.T) {
	installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(_ context.Context, _ string, _ factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
		return apisurface.ModelInvocationResult{}, fmt.Errorf("%w: model not found", apisurface.ErrModelNotFound)
	}))

	outputPath := filepath.Join(t.TempDir(), "speech.wav")
	err := Invoke(InvokeConfig{
		BuildInvocation: testModelInvocationBuilder,
		ModelName:       "missing",
		Operation:       "TTS",
		Text:            "hello world",
		OutputPath:      outputPath,
		FactoryDir:      t.TempDir(),
		Logger:          zap.NewNop(),
		Output:          io.Discard,
	})
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("error = %v, want ErrModelNotFound", err)
	}
}

func TestInvoke_JSONSurfacesClassifiedLoadingFailureFromBootstrap(t *testing.T) {
	installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(_ context.Context, _ string, _ factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
		return apisurface.ModelInvocationResult{}, fmt.Errorf("%w: wait for the managed runtime to finish loading and retry the invocation", apisurface.ErrManagedRuntimeLoading)
	}))

	err := Invoke(InvokeConfig{
		BuildInvocation: testModelInvocationBuilder,
		ModelName:       "OMNIVOICE_Q4_K_M",
		Operation:       "TTS",
		Text:            "hello world",
		FactoryDir:      t.TempDir(),
		Logger:          zap.NewNop(),
		JSON:            true,
		Output:          io.Discard,
	})
	if err == nil {
		t.Fatal("expected loading failure")
	}
	if !strings.Contains(err.Error(), "wait for the managed runtime to finish loading") {
		t.Fatalf("error = %q, want loading guidance", err.Error())
	}
}

func TestInvoke_RequiresInjectedInvocationBuilder(t *testing.T) {
	err := Invoke(InvokeConfig{
		ModelName: "OMNIVOICE_Q4_K_M", Operation: "TTS", Text: "hello",
		FactoryDir: t.TempDir(), JSON: true, Output: io.Discard,
	})
	if err == nil || !strings.Contains(err.Error(), "collaborator builder is required") {
		t.Fatalf("Invoke() error = %v, want missing collaborator builder", err)
	}
}

func TestInvoke_AudioUnreachableUsesBootstrapInsteadOfTransportMessage(t *testing.T) {
	originalBuilder := buildModelInvocationBootstrap
	defer func() {
		buildModelInvocationBootstrap = originalBuilder
	}()

	buildModelInvocationBootstrap = func(_ context.Context, _ *service.FactoryServiceConfig) (modelBootstrapRunner, error) {
		return &stubModelBootstrapRunner{
			sessionReady: true,
			invokeModel: func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
				return apisurface.ModelInvocationResult{
					ModelName: modelName,
					Operation: request.Operation,
				}, nil
			},
		}, nil
	}

	outputPath := filepath.Join(t.TempDir(), "speech.wav")
	err := Invoke(InvokeConfig{
		BuildInvocation: testModelInvocationBuilder,
		ModelName:       "OMNIVOICE_Q4_K_M",
		Operation:       "TTS",
		Text:            "hello world",
		OutputPath:      outputPath,
		FactoryDir:      t.TempDir(),
		Server:          "http://127.0.0.1:1",
		Output:          io.Discard,
		Logger:          zap.NewNop(),
	})
	if err == nil {
		t.Fatal("expected bootstrap audio invoke without stream file to fail")
	}
	if strings.Contains(err.Error(), "models endpoint not reachable at http://127.0.0.1:1/models/OMNIVOICE_Q4_K_M/invocations") {
		t.Fatalf("error = %q, want bootstrap failure instead of unreachable transport message", err.Error())
	}
}

func TestPull_ClassifiedFailureReturnsManagedRuntimeOutcome(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"modelName":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","outcome":"","cachePath":"","revision":"","downloadedFiles":[],"managedRuntimePull":{"identity":"OMNIVOICE_Q4_K_M","pullOutcome":"SOURCE_FETCH_FAILED","readinessState":"FAILED"}}`)
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Pull(PullConfig{
		ModelName: "OMNIVOICE_Q4_K_M",
		Server:    strings.TrimSuffix(server.URL, "/"),
		JSON:      true,
		Output:    &out,
	})
	if err == nil {
		t.Fatal("expected classified pull failure error")
	}
	if !strings.Contains(err.Error(), "SOURCE_FETCH_FAILED") {
		t.Fatalf("error = %q, want classified pull outcome", err.Error())
	}
	var response factoryapi.ModelPullResponse
	if decodeErr := json.Unmarshal(out.Bytes(), &response); decodeErr != nil {
		t.Fatalf("json output is invalid: %v\n%s", decodeErr, out.String())
	}
	if response.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeSOURCEFETCHFAILED {
		t.Fatalf("pull outcome = %s, want SOURCE_FETCH_FAILED", response.ManagedRuntimePull.PullOutcome)
	}
}

func TestPull_JSONWritesPullMetadataResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/OMNIVOICE_Q4_K_M/pull" {
			t.Fatalf("path = %q, want pull path", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		_, _ = io.WriteString(w, `{"modelName":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","outcome":"PULLED","cachePath":"/tmp/models/OMNIVOICE_Q4_K_M/rev1","revision":"rev1","downloadedFiles":[{"path":"omnivoice-base-Q4_K_M.gguf","bytes":407}]}`)
	}))
	defer server.Close()

	serverBase := strings.TrimSuffix(server.URL, "/")
	var out bytes.Buffer
	if err := Pull(PullConfig{
		ModelName: "OMNIVOICE_Q4_K_M",
		Server:    serverBase,
		JSON:      true,
		Output:    &out,
	}); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	for _, want := range []string{"OMNIVOICE_Q4_K_M", `"outcome":"PULLED"`} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestModelsList_JSONVerboseKeepsStdoutParseableAndDiagnosticsSeparate(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/models" {
			t.Fatalf("path = %q, want /models", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"results":[{"name":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","status":"READY","loadState":"UNLOADED","operations":[{"name":"TTS"}],"modalities":["TEXT"],"resources":[]}]}`)
	}))
	defer server.Close()

	var out bytes.Buffer
	var diagnostics bytes.Buffer
	if err := List(ListConfig{
		Server:      strings.TrimSuffix(server.URL, "/"),
		JSON:        true,
		Verbose:     true,
		Output:      &out,
		Diagnostics: &diagnostics,
	}); err != nil {
		t.Fatalf("List: %v", err)
	}
	var response factoryapi.ListModelsResponse
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("json output is invalid: %v\n%s", err, out.String())
	}
	assertDiagnosticsContains(t, diagnostics.String(), []string{
		"models list request",
		"endpointPath=/models",
		"server=",
		"models list response",
		"status=200",
		"resultCount=1",
	})
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
}

func TestModelsVerboseLogsInspectInvokeAndPullMetadataWithoutInputText(t *testing.T) {
	var inspectRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models/OMNIVOICE_Q4_K_M":
			inspectRequests.Add(1)
			_, _ = io.WriteString(w, `{"name":"OMNIVOICE_Q4_K_M","managedRuntime":{"identity":"OMNIVOICE_Q4_K_M","readinessState":"READY","lifecycleState":"NOT_INSTALLED","locality":"LOCAL","supportedOperations":[{"name":"TTS"}],"diagnostics":{}},"providerLocality":"LOCAL","status":"READY","loadState":"UNLOADED","operations":[{"name":"TTS"}],"modalities":["TEXT"],"resources":[],"capabilities":[],"diagnostics":{}}`)
		case "/models/OMNIVOICE_Q4_K_M/pull":
			_, _ = io.WriteString(w, `{"modelName":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","outcome":"PULLED","cachePath":"/tmp/models/ghp_successResponseToken1234567890/rev1","revision":"rev1","downloadedFiles":[{"path":"omnivoice-base-Q4_K_M.gguf","bytes":407}],"managedRuntimePull":{"identity":"OMNIVOICE_Q4_K_M","pullOutcome":"INSTALLED_SUCCESSFULLY","readinessState":"READY"}}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
		return apisurface.ModelInvocationResult{
			ModelName: modelName,
			Worker:    "tts-worker",
			Operation: request.Operation,
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeAudio,
				File: "artifacts/sensitive-generated-output.wav",
			}},
		}, nil
	}))

	serverBase := strings.TrimSuffix(server.URL, "/")
	var diagnostics bytes.Buffer
	if err := Inspect(InspectConfig{ModelName: "OMNIVOICE_Q4_K_M", Server: serverBase, Output: io.Discard, Verbose: true, Diagnostics: &diagnostics}); err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if err := Invoke(InvokeConfig{
		BuildInvocation: testModelInvocationBuilder,
		ModelName:       "OMNIVOICE_Q4_K_M",
		Operation:       "TTS",
		Text:            "secret direct input",
		FactoryDir:      t.TempDir(),
		Logger:          zap.NewNop(),
		JSON:            true,
		Output:          io.Discard,
		Verbose:         true,
		Diagnostics:     &diagnostics,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if err := Pull(PullConfig{ModelName: "OMNIVOICE_Q4_K_M", Server: serverBase, Output: io.Discard, Verbose: true, Diagnostics: &diagnostics}); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	diag := diagnostics.String()
	assertDiagnosticsContains(t, diag, []string{
		"models inspect request",
		"modelName=\"OMNIVOICE_Q4_K_M\"",
		"readiness=READY",
		"models invoke bootstrap request",
		"operation=\"TTS\"",
		"models invoke bootstrap response",
		"worker=tts-worker",
		"models pull request",
		"pullOutcome=INSTALLED_SUCCESSFULLY",
		"readiness=READY",
		"downloadedFiles=1",
	})
	for _, forbidden := range []string{"secret direct input", "sensitive-generated-output.wav", "ghp_successResponseToken1234567890"} {
		if strings.Contains(diag, forbidden) {
			t.Fatalf("diagnostics leaked model input, response content, or token %q:\n%s", forbidden, diag)
		}
	}
	if got := inspectRequests.Load(); got != 1 {
		t.Fatalf("inspect requests = %d, want 1", got)
	}
}

func TestModelsVerboseFailureUsesBoundedNonJSONErrorPreview(t *testing.T) {
	longFailureBody := strings.Repeat("x", modelsErrorBodyPreviewSize+30)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, longFailureBody)
	}))
	defer server.Close()

	var diagnostics bytes.Buffer
	_, err := queryModel(queryOptions{
		Server:      strings.TrimSuffix(server.URL, "/"),
		ModelName:   "broken",
		Verbose:     true,
		Diagnostics: &diagnostics,
	})
	if err == nil {
		t.Fatal("expected queryModel to fail")
	}
	gotErr := err.Error()
	wantPreview := longFailureBody[:modelsErrorBodyPreviewSize] + "..."
	if !strings.Contains(gotErr, wantPreview) {
		t.Fatalf("error = %q, want bounded preview %q", gotErr, wantPreview)
	}
	if strings.Contains(gotErr, longFailureBody) {
		t.Fatalf("error included full response body")
	}
	diag := diagnostics.String()
	assertDiagnosticsContains(t, diag, []string{
		"models inspect response",
		"endpointPath=/models/broken",
		"status=502",
		"responseBytes=230",
	})
	if strings.Contains(diag, longFailureBody[:40]) {
		t.Fatalf("diagnostics leaked model input or response content:\n%s", diag)
	}
}

func TestModelsVerboseLogsFailureStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"model not found","family":"NOT_FOUND","code":"NOT_FOUND"}`)
	}))
	defer server.Close()

	var diagnostics bytes.Buffer
	_, err := queryModel(queryOptions{
		Server:      strings.TrimSuffix(server.URL, "/"),
		ModelName:   "missing",
		Verbose:     true,
		Diagnostics: &diagnostics,
	})
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("queryModel error = %v, want ErrModelNotFound", err)
	}
	assertDiagnosticsContains(t, diagnostics.String(), []string{
		"models inspect response",
		"endpointPath=/models/missing",
		"status=404",
	})
}

func assertDiagnosticsContains(t *testing.T, got string, wants []string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, got)
		}
	}
}
