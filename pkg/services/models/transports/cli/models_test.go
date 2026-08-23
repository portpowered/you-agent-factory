package cli

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
	"time"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type commandServiceFake struct {
	list    func(ListConfig) error
	inspect func(InspectConfig) error
	invoke  func(InvokeConfig) error
	pull    func(PullConfig) error
	remove  func(RemoveConfig) error
}
type modelsPullDoer func(*http.Request) (*http.Response, error)

func (doer modelsPullDoer) Do(request *http.Request) (*http.Response, error) {
	return doer(request)
}

type modelsPullRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper modelsPullRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTripper(request)
}

func (fake commandServiceFake) List(cfg ListConfig) error       { return fake.list(cfg) }
func (fake commandServiceFake) Inspect(cfg InspectConfig) error { return fake.inspect(cfg) }
func (fake commandServiceFake) Invoke(cfg InvokeConfig) error   { return fake.invoke(cfg) }
func (fake commandServiceFake) Pull(cfg PullConfig) error       { return fake.pull(cfg) }
func (fake commandServiceFake) Remove(cfg RemoveConfig) error {
	if fake.remove != nil {
		return fake.remove(cfg)
	}
	return nil
}
func TestCommandHandlerTransformsInvokeCommandState(t *testing.T) {
	server := "http://127.0.0.1:7437"
	logger := zap.NewNop()
	var diagnostics bytes.Buffer

	handler := NewCommandHandler(
		commandServiceFake{invoke: func(cfg InvokeConfig) error {
			if cfg.ModelName != "OMNIVOICE_Q4_K_M" || cfg.Operation != "TTS" || cfg.Text != "hello" || cfg.OutputPath != "speech.wav" {
				t.Fatalf("InvokeConfig command values = %#v", cfg)
			}
			if cfg.Server != server || !cfg.JSON || !cfg.Verbose || !cfg.Debug {
				t.Fatalf("InvokeConfig global values = %#v", cfg)
			}
			if cfg.FactoryDir != "" || cfg.WorkingDirectory != "/factory" || cfg.HomeDir != "/home/tester" || cfg.Logger != logger || cfg.Diagnostics != &diagnostics {
				t.Fatalf("InvokeConfig dependencies = %#v", cfg)
			}
			return nil
		}},
		func(*cobra.Command) io.Writer { return &diagnostics },
		func() (string, error) { return "/home/tester", nil },
		func(_ *cobra.Command, homeDir string) (operatorconfig.ResolvedDefaults, error) {
			if homeDir != "/home/tester" {
				t.Fatalf("operator defaults home = %q", homeDir)
			}
			return operatorconfig.ResolvedDefaults{}, nil
		},
		func() (*zap.Logger, error) { return logger, nil },
	)

	cmd := &cobra.Command{Use: "invoke"}
	cmd.SetContext(startupcli.WithWorkingDirectory(context.Background(), "/factory"))
	cmd.SetOut(io.Discard)
	invokeInputs, inherited := resolvedInvokeHandlerInputs(t, server)
	if err := handler.Invoke(cmd, invokeInputs, inherited); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
}
func resolvedInvokeHandlerInputs(
	t *testing.T,
	server string,
) (resolvedinput.Inputs, resolvedinput.Inputs) {
	t.Helper()
	local, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{
			{ID: modelsInvokeNameInputID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourcePositionalArgument}},
			{ID: modelsInvokeOperationID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
			{ID: modelsInvokeTextID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
			{ID: modelsInvokeInputID, Kind: resolvedinput.ValueKindStringArray, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
			{ID: modelsInvokeOutputID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
		},
		[]resolvedinput.Candidate{
			{InputID: modelsInvokeNameInputID, Source: resolvedinput.SourcePositionalArgument, Value: resolvedinput.StringValue("OMNIVOICE_Q4_K_M")},
			{InputID: modelsInvokeOperationID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringValue("TTS")},
			{InputID: modelsInvokeTextID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringValue("hello")},
			{InputID: modelsInvokeInputID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringArrayValue([]string{"audio=@meeting.wav", "prompt=hint"})},
			{InputID: modelsInvokeOutputID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringValue("speech.wav")},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, _, inherited := resolvedModelsHandlerInputs(t, server)
	return local, inherited
}
func TestCommandHandlerTransformsListInspectAndPullArguments(t *testing.T) {
	server := "http://127.0.0.1:7437"
	called := map[string]bool{}
	handler := NewCommandHandler(
		commandServiceFake{
			list: func(cfg ListConfig) error {
				called["list"] = true
				if cfg.Server != server || !cfg.JSON || !cfg.Verbose || !cfg.Debug {
					t.Fatalf("ListConfig = %#v", cfg)
				}
				return nil
			},
			inspect: func(cfg InspectConfig) error {
				called["inspect"] = true
				if cfg.ModelName != "model-a" || cfg.Server != server {
					t.Fatalf("InspectConfig = %#v", cfg)
				}
				return nil
			},
			pull: func(cfg PullConfig) error {
				called["pull"] = true
				if cfg.ModelName != "model-b" || cfg.Server != server || cfg.Context.Err() != context.Canceled {
					t.Fatalf("PullConfig = %#v", cfg)
				}
				return nil
			},
		},
		func(*cobra.Command) io.Writer { return io.Discard },
		nil,
		nil,
		nil,
	)
	cmd := &cobra.Command{Use: "models"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	inspectInputs, pullInputs, inherited := resolvedModelsHandlerInputs(t, server)
	if err := handler.List(cmd, resolvedinput.Inputs{}, inherited); err != nil {
		t.Fatal(err)
	}
	if err := handler.Inspect(cmd, inspectInputs, inherited); err != nil {
		t.Fatal(err)
	}
	if err := handler.Pull(cmd, pullInputs, inherited); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"list", "inspect", "pull"} {
		if !called[operation] {
			t.Fatalf("%s service operation was not called", operation)
		}
	}
}

func resolvedModelsHandlerInputs(
	t *testing.T,
	server string,
) (resolvedinput.Inputs, resolvedinput.Inputs, resolvedinput.Inputs) {
	t.Helper()
	inherited, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{
			{ID: serverInputID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
			{ID: jsonInputID, Kind: resolvedinput.ValueKindBool, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
			{ID: verboseInputID, Kind: resolvedinput.ValueKindBool, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
			{ID: debugInputID, Kind: resolvedinput.ValueKindBool, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
		},
		[]resolvedinput.Candidate{
			{InputID: serverInputID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringValue(server)},
			{InputID: jsonInputID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.BoolValue(true)},
			{InputID: verboseInputID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.BoolValue(true)},
			{InputID: debugInputID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.BoolValue(true)},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	inspectInputs, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{{
			ID: modelsInspectNameInputID, Kind: resolvedinput.ValueKindString,
			Precedence: []resolvedinput.Source{resolvedinput.SourcePositionalArgument},
		}},
		[]resolvedinput.Candidate{{
			InputID: modelsInspectNameInputID, Source: resolvedinput.SourcePositionalArgument,
			Value: resolvedinput.StringValue("model-a"),
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	pullInputs, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{{
			ID: modelsPullNameInputID, Kind: resolvedinput.ValueKindString,
			Precedence: []resolvedinput.Source{resolvedinput.SourcePositionalArgument},
		}},
		[]resolvedinput.Candidate{{
			InputID: modelsPullNameInputID, Source: resolvedinput.SourcePositionalArgument,
			Value: resolvedinput.StringValue("model-b"),
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	return inspectInputs, pullInputs, inherited
}

func TestRenderList_WritesDiscoveredModelsTable(t *testing.T) {
	cacheBytes := int64(1234)
	var out bytes.Buffer
	err := renderList(factoryapi.ListModelsResponse{
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
				CacheBytes:     &cacheBytes,
			},
			Operations: []factoryapi.ModelInvocationOperation{{Name: "TTS"}},
			Modalities: []factoryapi.ModelInvocationContentType{factoryapi.ModelInvocationContentTypeAudio, factoryapi.ModelInvocationContentTypeText},
			Resources:  []factoryapi.ModelResourceSummary{{Name: "voice-cache", Type: factoryapi.ResourceTypeModel, Capacity: 1}},
		}},
	}, &out)
	if err != nil {
		t.Fatalf("RenderList: %v", err)
	}
	got := out.String()
	for _, want := range []string{"NAME", "READINESS", "LIFECYCLE", "CACHE SIZE", "OMNIVOICE_Q4_K_M", "LOCAL", "READY", "INSTALLED", "TTS", "AUDIO,TEXT", "1.21 KiB (1234 bytes)"} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("rendered table missing %q:\n%s", want, got)
		}
	}
}

func TestManagedRuntimeMappingPreservesCacheFacts(t *testing.T) {
	revision := "rev-1"
	cachePath := "/tmp/models/OMNIVOICE_Q4_K_M/rev-1"
	cacheBytes := int64(1234)
	got := managedRuntimeToGenerated(modelinference.Runtime{
		Identity: "OMNIVOICE_Q4_K_M",
		Revision: &revision, CachePath: &cachePath, CacheBytes: &cacheBytes,
	})
	if got.Revision == nil || *got.Revision != revision ||
		got.CachePath == nil || *got.CachePath != cachePath ||
		got.CacheBytes == nil || *got.CacheBytes != cacheBytes {
		t.Fatalf("managed runtime cache facts = revision=%v path=%v bytes=%v, want rev-1/path/1234", got.Revision, got.CachePath, got.CacheBytes)
	}
}

func TestNewRequiresHTTPAndInvocationDependencies(t *testing.T) {
	if service := New(nil, testModelInvocationBuilder); service != nil {
		t.Fatalf("New(nil, invocation) = %T, want nil", service)
	}
	if service := New(testHTTPProtocol(t), nil); service != nil {
		t.Fatalf("New(protocol, nil) = %T, want nil", service)
	}
	if service := New(testHTTPProtocol(t), testModelInvocationBuilder); service == nil {
		t.Fatal("New(protocol, invocation) = nil, want Models CLI service")
	}
}

func TestRenderModel_WritesManagedRuntimeInspectFields(t *testing.T) {
	diagnostics := factoryapi.StringMap{
		"readinessState": "MISSING",
		"missingAssets":  "weights.bin",
	}
	var out bytes.Buffer
	err := renderModel(factoryapi.ModelDetail{
		Name: "SECOND_RUNTIME",
		ManagedRuntime: factoryapi.ManagedRuntime{
			Identity:       "SECOND_RUNTIME",
			ReadinessState: factoryapi.ManagedRuntimeReadinessStateMISSING,
			LifecycleState: factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
			Locality:       factoryapi.WorkerModelLocalityLocal,
			Diagnostics:    &diagnostics,
		},
		ProviderLocality: factoryapi.WorkerModelLocalityLocal,
		Operations:       []factoryapi.ModelInvocationOperation{{Name: "EMBED"}},
	}, &out)
	if err != nil {
		t.Fatalf("RenderModel: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"Readiness:\tMISSING", "Lifecycle:\tNOT_INSTALLED",
		"Revision:\tNOT_INSTALLED", "Cache Size:\tNOT_INSTALLED", "Cache Path:\tNOT_INSTALLED",
		"missingAssets=weights.bin",
	} {
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
	_, err := queryModelWithProtocol(context.Background(), testHTTPProtocol(t), serverBase, "missing")
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("error = %v, want ErrModelNotFound", err)
	}
	var apiErr *clihttp.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T, want decoded APIError", err)
	}
	if apiErr.CLIErrorCode() != "NOT_FOUND" || apiErr.CLIErrorFamily() != factoryapi.ErrorFamilyNotFound || apiErr.CLIErrorMessage() != "model not found" {
		t.Fatalf("APIError fields = code %q family %q message %q", apiErr.CLIErrorCode(), apiErr.CLIErrorFamily(), apiErr.CLIErrorMessage())
	}
}

func TestInvoke_JSONWritesMetadataResponse(t *testing.T) {
	installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
		return modelinference.Result{
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
	if err := invokeForTest(t, InvokeConfig{Context: context.Background(),
		ModelName:  "OMNIVOICE_Q4_K_M",
		Operation:  "TTS",
		Text:       "hello world",
		FactoryDir: t.TempDir(),
		Logger:     zap.NewNop(),
		JSON:       true,
		Output:     &out,
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
	installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
		if request.Options == nil || request.Options.ResponseMode == nil || *request.Options.ResponseMode != factoryapi.AUDIOSTREAM {
			t.Fatalf("request options = %#v, want AUDIO_STREAM mode", request.Options)
		}
		return modelinference.Result{
			ModelName:  modelName,
			Operation:  request.Operation,
			StreamFile: streamFile,
		}, nil
	}))

	outputPath := filepath.Join(t.TempDir(), "speech.wav")
	if err := invokeForTest(t, InvokeConfig{Context: context.Background(),
		ModelName:  "OMNIVOICE_Q4_K_M",
		Operation:  "TTS",
		Text:       "hello world",
		OutputPath: outputPath,
		FactoryDir: t.TempDir(),
		Logger:     zap.NewNop(),
		Output:     io.Discard,
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
	installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
		return modelinference.Result{
			ModelName:  modelName,
			Operation:  request.Operation,
			StreamFile: streamFile,
		}, nil
	}))

	outputPath := filepath.Join(t.TempDir(), "speech.wav")
	var diagnostics bytes.Buffer
	if err := invokeForTest(t, InvokeConfig{Context: context.Background(),
		ModelName:   "OMNIVOICE_Q4_K_M",
		Operation:   "TTS",
		Text:        "hello world",
		OutputPath:  outputPath,
		FactoryDir:  t.TempDir(),
		Logger:      zap.NewNop(),
		Output:      io.Discard,
		Verbose:     true,
		Diagnostics: &diagnostics,
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
	installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(_ context.Context, _ string, _ factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
		return modelinference.Result{}, fmt.Errorf("%w: model not found", modelinference.ErrNotFound)
	}))

	outputPath := filepath.Join(t.TempDir(), "speech.wav")
	err := invokeForTest(t, InvokeConfig{Context: context.Background(),
		ModelName:  "missing",
		Operation:  "TTS",
		Text:       "hello world",
		OutputPath: outputPath,
		FactoryDir: t.TempDir(),
		Logger:     zap.NewNop(),
		Output:     io.Discard,
	})
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("error = %v, want ErrModelNotFound", err)
	}
}

func TestInvoke_JSONSurfacesClassifiedLoadingFailureFromBootstrap(t *testing.T) {
	installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(_ context.Context, _ string, _ factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
		return modelinference.Result{}, fmt.Errorf("%w: wait for the managed runtime to finish loading and retry the invocation", modelinference.ErrLoading)
	}))

	err := invokeForTest(t, InvokeConfig{Context: context.Background(),
		ModelName:  "OMNIVOICE_Q4_K_M",
		Operation:  "TTS",
		Text:       "hello world",
		FactoryDir: t.TempDir(),
		Logger:     zap.NewNop(),
		JSON:       true,
		Output:     io.Discard,
	})
	if err == nil {
		t.Fatal("expected loading failure")
	}
	if !strings.Contains(err.Error(), "wait for the managed runtime to finish loading") {
		t.Fatalf("error = %q, want loading guidance", err.Error())
	}
}

func TestInvoke_RequiresInjectedInvocationOperation(t *testing.T) {
	if service := New(testHTTPProtocol(t), nil); service != nil {
		t.Fatalf("New(protocol, nil) = %T, want nil", service)
	}
}

func TestInvoke_AudioUnreachableUsesBootstrapInsteadOfTransportMessage(t *testing.T) {
	originalBuilder := openTestModelRunner
	defer func() {
		openTestModelRunner = originalBuilder
	}()

	openTestModelRunner = func(_ context.Context, _ *testModelRuntimeSelections) (testModelRunner, error) {
		return &stubModelBootstrapRunner{
			sessionReady: true,
			invokeModel: func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
				return modelinference.Result{
					ModelName: modelName,
					Operation: request.Operation,
				}, nil
			},
		}, nil
	}

	outputPath := filepath.Join(t.TempDir(), "speech.wav")
	err := invokeForTest(t, InvokeConfig{Context: context.Background(),
		ModelName:  "OMNIVOICE_Q4_K_M",
		Operation:  "TTS",
		Text:       "hello world",
		OutputPath: outputPath,
		FactoryDir: t.TempDir(),
		Server:     "http://127.0.0.1:1",
		Output:     io.Discard,
		Logger:     zap.NewNop(),
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
		_, _ = io.WriteString(w, `{"modelName":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","outcome":"PULLED","cachePath":"","revision":"","downloadedFiles":[],"managedRuntimePull":{"identity":"OMNIVOICE_Q4_K_M","pullOutcome":"SOURCE_FETCH_FAILED","readinessState":"FAILED"}}`)
	}))
	defer server.Close()

	var out bytes.Buffer
	err := New(testHTTPProtocol(t), testModelInvocationBuilder).Pull(PullConfig{Context: context.Background(),
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
	if response.Outcome != factoryapi.ModelPullOutcomeFAILED {
		t.Fatalf("pull JSON = %q, want top-level outcome FAILED", strings.TrimSpace(out.String()))
	}
}

func TestModelPullCompatibilityOutcomeProjectsManagedOutcomeTotally(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		managed factoryapi.ManagedRuntimePullOutcome
		want    factoryapi.ModelPullOutcome
	}{
		{name: "installed", managed: factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY, want: factoryapi.ModelPullOutcomePULLED},
		{name: "already present", managed: factoryapi.ManagedRuntimePullOutcomeALREADYPRESENT, want: factoryapi.ModelPullOutcomeALREADYPRESENT},
		{name: "already ready", managed: factoryapi.ManagedRuntimePullOutcomeALREADYREADY, want: factoryapi.ModelPullOutcomeALREADYPRESENT},
		{name: "still loading", managed: factoryapi.ManagedRuntimePullOutcomeSTILLLOADING, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "timed out", managed: factoryapi.ManagedRuntimePullOutcomeTIMEDOUT, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "cancelled", managed: factoryapi.ManagedRuntimePullOutcomeCANCELLED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "source fetch failed", managed: factoryapi.ManagedRuntimePullOutcomeSOURCEFETCHFAILED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "source resolution failed", managed: factoryapi.ManagedRuntimePullOutcomeSOURCERESOLUTIONFAILED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "integrity verification failed", managed: factoryapi.ManagedRuntimePullOutcomeINTEGRITYVERIFICATIONFAILED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "assembly failed", managed: factoryapi.ManagedRuntimePullOutcomeASSEMBLYFAILED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "cache installation failed", managed: factoryapi.ManagedRuntimePullOutcomeCACHEINSTALLATIONFAILED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "readiness evaluation failed", managed: factoryapi.ManagedRuntimePullOutcomeREADINESSEVALUATIONFAILED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "asset preparation failed", managed: factoryapi.ManagedRuntimePullOutcomeASSETPREPARATIONFAILED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "unsupported", managed: factoryapi.ManagedRuntimePullOutcomeUNSUPPORTEDRUNTIME, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "future outcome", managed: factoryapi.ManagedRuntimePullOutcome("FUTURE_OUTCOME"), want: factoryapi.ModelPullOutcomeFAILED},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := pullResultToGenerated(modelinference.PullResult{
				ModelName: "voice", Outcome: "PULLED", ManagedPullOutcome: string(tc.managed),
			})
			if result.Outcome != tc.want {
				t.Fatalf("managed outcome %q projected as %q, want %q", tc.managed, result.Outcome, tc.want)
			}
		})
	}
}

func TestModelsPullUsesDedicatedProtocolAndPreservesCallerCancellation(t *testing.T) {
	t.Parallel()
	var standardCalls atomic.Int32
	standard, err := clihttp.NewProtocol(modelsPullDoer(func(*http.Request) (*http.Response, error) {
		standardCalls.Add(1)
		return nil, errors.New("ordinary CLI protocol was used for pull")
	}), testHTTPClock{})
	if err != nil {
		t.Fatalf("standard protocol: %v", err)
	}
	pullStarted := make(chan struct{})
	var startOnce atomic.Bool
	pull, err := clihttp.NewProtocol(modelsPullDoer(func(request *http.Request) (*http.Response, error) {
		if _, ok := request.Context().Deadline(); ok {
			return nil, errors.New("pull request inherited a fixed client deadline")
		}
		if startOnce.CompareAndSwap(false, true) {
			close(pullStarted)
		}
		<-request.Context().Done()
		return nil, request.Context().Err()
	}), testHTTPClock{})
	if err != nil {
		t.Fatalf("pull protocol: %v", err)
	}
	service := NewWithOutputFileSystemAndPullProtocol(
		standard, pull, testModelInvocationBuilder, nil,
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- service.Pull(PullConfig{
			Context: ctx, ModelName: "OMNIVOICE_Q4_K_M", Server: "http://factory.test",
			Output: io.Discard,
		})
	}()
	select {
	case <-pullStarted:
	case <-time.After(time.Second):
		t.Fatal("dedicated pull protocol was not invoked")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("pull error = %v, want caller cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pull did not terminate after caller cancellation")
	}
	if standardCalls.Load() != 0 {
		t.Fatalf("ordinary protocol calls = %d, want 0", standardCalls.Load())
	}
}

func TestModelsPullWaitsForDedicatedProtocolTerminalResponse(t *testing.T) {
	t.Parallel()
	standard, err := clihttp.NewProtocol(modelsPullDoer(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("ordinary CLI protocol was used for pull")
	}), testHTTPClock{})
	if err != nil {
		t.Fatalf("standard protocol: %v", err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	pull, err := clihttp.NewProtocol(modelsPullDoer(func(request *http.Request) (*http.Response, error) {
		if _, ok := request.Context().Deadline(); ok {
			return nil, errors.New("pull request inherited a fixed client deadline")
		}
		close(started)
		<-release
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`{"modelName":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","outcome":"PULLED","managedRuntimePull":{"identity":"OMNIVOICE_Q4_K_M","pullOutcome":"INSTALLED_SUCCESSFULLY","readinessState":"READY"}}`,
			)),
		}, nil
	}), testHTTPClock{})
	if err != nil {
		t.Fatalf("pull protocol: %v", err)
	}
	service := NewWithOutputFileSystemAndPullProtocol(
		standard, pull, testModelInvocationBuilder, nil,
	)
	var output bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- service.Pull(PullConfig{
			Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M",
			Server: "http://factory.test", JSON: true, Output: &output,
		})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("dedicated pull protocol was not invoked")
	}
	select {
	case err := <-done:
		t.Fatalf("pull completed before terminal response: %v", err)
	default:
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("pull after terminal response: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pull did not finish after terminal response")
	}
	if !strings.Contains(output.String(), `"outcome":"PULLED"`) {
		t.Fatalf("pull output = %q, want terminal success", output.String())
	}
}

func TestModelsTransportErrorSummaryIdentifiesTimeoutAndCancellation(t *testing.T) {
	t.Parallel()
	if got := modelsTransportErrorSummary(context.DeadlineExceeded); got != "error=timeout" {
		t.Fatalf("deadline summary = %q, want error=timeout", got)
	}
	if got := modelsTransportErrorSummary(context.Canceled); got != "error=canceled" {
		t.Fatalf("cancellation summary = %q, want error=canceled", got)
	}
	if got := modelsTransportErrorSummary(errors.New("connection refused")); got != "error=unreachable" {
		t.Fatalf("transport summary = %q, want error=unreachable", got)
	}
}

func TestModelsPullDiagnosticsIdentifyOrdinaryClientTimeout(t *testing.T) {
	t.Parallel()
	protocol, err := clihttp.NewProtocol(&http.Client{
		Timeout: 5 * time.Millisecond,
		Transport: modelsPullRoundTripper(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			return nil, request.Context().Err()
		}),
	}, testHTTPClock{})
	if err != nil {
		t.Fatalf("timeout protocol: %v", err)
	}
	var diagnostics bytes.Buffer
	_, err = pullModel(pullOptions{
		Context: context.Background(), Server: "http://factory.test",
		ModelName: "OMNIVOICE_Q4_K_M", Diagnostics: &diagnostics, Verbose: true,
		HTTP: protocol,
	})
	if err == nil {
		t.Fatal("pull error = nil, want ordinary client timeout")
	}
	if !strings.Contains(diagnostics.String(), "error=timeout") {
		t.Fatalf("timeout diagnostics = %q, want error=timeout", diagnostics.String())
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
	if err := New(testHTTPProtocol(t), testModelInvocationBuilder).Pull(PullConfig{Context: context.Background(),
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
	if err := New(testHTTPProtocol(t), testModelInvocationBuilder).List(ListConfig{Context: context.Background(),
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

	installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
		return modelinference.Result{
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
	if err := New(testHTTPProtocol(t), testModelInvocationBuilder).Inspect(InspectConfig{Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Server: serverBase, Output: io.Discard, Verbose: true, Diagnostics: &diagnostics}); err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if err := invokeForTest(t, InvokeConfig{Context: context.Background(),
		ModelName:   "OMNIVOICE_Q4_K_M",
		Operation:   "TTS",
		Text:        "secret direct input",
		FactoryDir:  t.TempDir(),
		Logger:      zap.NewNop(),
		JSON:        true,
		Output:      io.Discard,
		Verbose:     true,
		Diagnostics: &diagnostics,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if err := New(testHTTPProtocol(t), testModelInvocationBuilder).Pull(PullConfig{Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Server: serverBase, Output: io.Discard, Verbose: true, Diagnostics: &diagnostics}); err != nil {
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

func TestModelsFailureOmitsNonJSONResponseBody(t *testing.T) {
	responseBody := "opaque-secret-response-marker"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, responseBody)
	}))
	defer server.Close()

	var diagnostics bytes.Buffer
	_, err := queryModel(queryOptions{
		Context:     context.Background(),
		HTTP:        testHTTPProtocol(t),
		Server:      strings.TrimSuffix(server.URL, "/"),
		ModelName:   "broken",
		Verbose:     true,
		Diagnostics: &diagnostics,
	})
	if err == nil {
		t.Fatal("expected queryModel to fail")
	}
	gotErr := err.Error()
	if !strings.Contains(gotErr, "models request failed (502): response body was not a structured API error") {
		t.Fatalf("error = %q, want safe non-JSON response summary", gotErr)
	}
	if strings.Contains(gotErr, responseBody) {
		t.Fatalf("error included raw response body")
	}
	diag := diagnostics.String()
	assertDiagnosticsContains(t, diag, []string{
		"models inspect response",
		"endpointPath=/models/broken",
		"status=502",
		"responseBytes=29",
	})
	if strings.Contains(diag, responseBody) {
		t.Fatalf("diagnostics leaked model input or response content:\n%s", diag)
	}
}

func TestManagedRuntimePullResponseErrorPreservesOutcomeDetails(t *testing.T) {
	err := managedRuntimePullResponseError(http.StatusUnprocessableEntity, []byte(`{
		"managedRuntimePull": {
			"identity": "OMNIVOICE_Q4_K_M",
			"pullOutcome": "SOURCE_FETCH_FAILED",
			"readinessState": "FAILED",
			"pullDiagnostics": {
				"modelName": "OMNIVOICE_Q4_K_M",
				"resolvedRepository": "owner/repo",
				"revision": "rev-1",
				"file": "weights.gguf",
				"operation": "download asset",
				"requestUrl": "https://assets.example.test/owner/repo/weights.gguf?download=true",
				"upstreamStatusCode": 502
			}
		}
	}`))
	if err == nil {
		t.Fatal("managedRuntimePullResponseError() = nil, want classified failure")
	}
	if got, want := err.Error(), "managed runtime pull failed (pullOutcome=SOURCE_FETCH_FAILED readinessState=FAILED)"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
	var coded interface {
		CLIErrorCode() string
		CLIErrorFamily() factoryapi.ErrorFamily
		CLIErrorMessage() string
	}
	if !errors.As(err, &coded) {
		t.Fatalf("error = %T, want coded model pull failure", err)
	}
	if coded.CLIErrorCode() != managedRuntimePullFailureCode ||
		coded.CLIErrorFamily() != factoryapi.ErrorFamilyBadRequest ||
		coded.CLIErrorMessage() != err.Error() {
		t.Fatalf("coded failure = (%q, %q, %q), want safe outcome diagnostic", coded.CLIErrorCode(), coded.CLIErrorFamily(), coded.CLIErrorMessage())
	}
	var diagnostics *modelinference.PullDiagnosticsError
	if !errors.As(err, &diagnostics) || diagnostics == nil {
		t.Fatalf("error = %T, want structured pull diagnostics cause", err)
	}
	if !strings.Contains(diagnostics.Error(), "repository=owner/repo") ||
		!strings.Contains(diagnostics.Error(), "status=502") ||
		!strings.Contains(diagnostics.Error(), "operation=download asset") {
		t.Fatalf("diagnostics = %q, want repository, operation, and status", diagnostics)
	}
}
