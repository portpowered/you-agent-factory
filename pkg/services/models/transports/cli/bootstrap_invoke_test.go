package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/platform/metrics"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

type stubModelBootstrapRunner struct {
	invokeModel  func(context.Context, string, factoryapi.ModelInvocationRequest) (modelinference.Result, error)
	run          func(context.Context) error
	sessionReady bool
}

func (s *stubModelBootstrapRunner) Run(ctx context.Context) error {
	if s.run != nil {
		return s.run(ctx)
	}
	<-ctx.Done()
	return ctx.Err()
}

func (s *stubModelBootstrapRunner) InvokeModel(
	ctx context.Context,
	modelName string,
	request factoryapi.ModelInvocationRequest,
) (modelinference.Result, error) {
	if s.invokeModel != nil {
		return s.invokeModel(ctx, modelName, request)
	}
	return modelinference.Result{}, errors.New("unexpected InvokeModel call")
}

func (s *stubModelBootstrapRunner) GetCurrentFactoryForSession(context.Context, string) (factoryapi.Factory, error) {
	if s.sessionReady {
		return factoryapi.Factory{Name: "factory"}, nil
	}
	return factoryapi.Factory{}, apisurface.ErrFactorySessionNotFound
}

func (s *stubModelBootstrapRunner) CloseFactorySession(context.Context, string) error {
	return nil
}

func installStubModelBootstrapRunner(t *testing.T, runner *stubModelBootstrapRunner) {
	t.Helper()
	originalBuilder := openTestModelRunner
	t.Cleanup(func() {
		openTestModelRunner = originalBuilder
	})
	openTestModelRunner = func(_ context.Context, _ *testModelRuntimeSelections) (testModelRunner, error) {
		if runner == nil {
			t.Fatal("stub bootstrap runner is required")
		}
		return runner, nil
	}
}

func readyStubModelBootstrapRunner(invokeModel func(context.Context, string, factoryapi.ModelInvocationRequest) (modelinference.Result, error)) *stubModelBootstrapRunner {
	return &stubModelBootstrapRunner{
		sessionReady: true,
		invokeModel:  invokeModel,
	}
}

func TestInvoke_RoutesThroughSharedBootstrapWithoutHTTPEndpoint(t *testing.T) {
	originalBuilder := openTestModelRunner
	defer func() {
		openTestModelRunner = originalBuilder
	}()

	homeDir := t.TempDir()
	streamFile := filepath.Join(t.TempDir(), "stream.wav")
	if err := os.WriteFile(streamFile, []byte("RIFF....WAVE"), 0o644); err != nil {
		t.Fatalf("write stream file: %v", err)
	}
	var capturedModel string
	var capturedRequest factoryapi.ModelInvocationRequest
	openTestModelRunner = func(_ context.Context, cfg *testModelRuntimeSelections) (testModelRunner, error) {
		if cfg == nil || strings.TrimSpace(cfg.Dir) == "" {
			t.Fatal("expected bootstrap service config with factory dir")
		}
		if cfg.Port != 0 {
			t.Fatalf("bootstrap port = %d, want 0 for no-server invoke", cfg.Port)
		}
		if cfg.SystemConfigHomeDir != homeDir || cfg.RuntimeLogDir != logging.RuntimeLogsRoot(homeDir) || cfg.RuntimeMetricsDir != metrics.RuntimeMetricsRoot(homeDir) {
			t.Fatalf("bootstrap home paths = home %q logs %q metrics %q; want roots below %q", cfg.SystemConfigHomeDir, cfg.RuntimeLogDir, cfg.RuntimeMetricsDir, homeDir)
		}
		return &stubModelBootstrapRunner{
			sessionReady: true,
			invokeModel: func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
				capturedModel = modelName
				capturedRequest = request
				return modelinference.Result{
					ModelName:        modelName,
					Worker:           "tts-worker",
					Operation:        request.Operation,
					ProviderLocality: string(factoryapi.WorkerModelLocalityLocal),
					StreamFile:       streamFile,
				}, nil
			},
		}, nil
	}

	if err := invokeForTest(t, InvokeConfig{Context: context.Background(),
		ModelName:  "OMNIVOICE_Q4_K_M",
		Operation:  "TTS",
		Text:       "hello world",
		FactoryDir: t.TempDir(),
		HomeDir:    homeDir,
		Server:     failureBaselineUnreachableServer,
		OutputPath: filepath.Join(t.TempDir(), "speech.wav"),
		Logger:     zap.NewNop(),
		Output:     io.Discard,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if capturedModel != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("model = %q, want OMNIVOICE_Q4_K_M", capturedModel)
	}
	if capturedRequest.Operation != "TTS" {
		t.Fatalf("operation = %q, want TTS", capturedRequest.Operation)
	}
}

func TestInvoke_UnreachableServerDoesNotFailWithTransportUnreachableMessage(t *testing.T) {
	originalBuilder := openTestModelRunner
	defer func() {
		openTestModelRunner = originalBuilder
	}()

	streamFile := filepath.Join(t.TempDir(), "stream.wav")
	if err := os.WriteFile(streamFile, []byte("RIFF....WAVE"), 0o644); err != nil {
		t.Fatalf("write stream file: %v", err)
	}
	openTestModelRunner = func(_ context.Context, _ *testModelRuntimeSelections) (testModelRunner, error) {
		return &stubModelBootstrapRunner{
			sessionReady: true,
			invokeModel: func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
				return modelinference.Result{
					ModelName:  modelName,
					Worker:     "tts-worker",
					Operation:  request.Operation,
					StreamFile: streamFile,
				}, nil
			},
		}, nil
	}

	if err := invokeForTest(t, InvokeConfig{Context: context.Background(),
		ModelName:  "OMNIVOICE_Q4_K_M",
		Operation:  "TTS",
		Text:       "hello world",
		FactoryDir: t.TempDir(),
		Server:     failureBaselineUnreachableServer,
		OutputPath: filepath.Join(t.TempDir(), "speech.wav"),
		Output:     io.Discard,
		Logger:     zap.NewNop(),
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
}

// capturingModelInvocationOperation records exactly what the CLI hands to the
// composition-root invocation operation, so the target field mapping can be
// asserted directly instead of being inferred from downstream runtime effects.
type capturingModelInvocationOperation struct {
	factoryDir       string
	homeDir          string
	operatorDefaults operatorconfig.ResolvedDefaults
	verbose          bool
	modelName        string
	request          modelinference.Request
	result           modelinference.Result
	err              error
}

type workingDirectoryResolverInvocation struct {
	capturingModelInvocationOperation
	resolvedFactoryDir string
	gotExplicit        string
	gotWorkingDir      string
}

func (operation *workingDirectoryResolverInvocation) ResolveModelInvocationFactoryDirForWorkingDirectory(
	explicit string,
	workingDirectory string,
) (string, error) {
	operation.gotExplicit = explicit
	operation.gotWorkingDir = workingDirectory
	return operation.resolvedFactoryDir, nil
}

func (c *capturingModelInvocationOperation) ResolveModelInvocationFactoryDir(explicit string) (string, error) {
	return explicit, nil
}

func (c *capturingModelInvocationOperation) ExportModelInvocationArtifact(string, string) error {
	return nil
}

func (c *capturingModelInvocationOperation) InvokeModel(
	_ context.Context,
	target InvocationTarget,
	modelName string,
	request modelinference.Request,
) (modelinference.Result, error) {
	c.factoryDir = target.FactoryDir
	c.homeDir = target.HomeDir
	c.operatorDefaults = target.OperatorDefaults
	c.verbose = target.Verbose
	c.modelName = modelName
	c.request = request
	return c.result, c.err
}

// TestRunBootstrapModelInvocation_CarriesResolvedInvocationInputs pins the four
// invocation inputs the bootstrap CLI path actually populates on its way to the
// composition-root operation, plus the model name and mapped request. This is
// the observable contract of the boundary: a later change to how the target is
// carried must keep every one of these values arriving unchanged.
func TestRunBootstrapModelInvocation_CarriesResolvedInvocationInputs(t *testing.T) {
	t.Parallel()

	operation := &capturingModelInvocationOperation{
		result: modelinference.Result{ModelName: "voice", Operation: "TTS", Worker: "narrator"},
	}
	defaults := operatorconfig.ResolvedDefaults{
		WorkerModelProvider: "codex",
		WorkerModel:         "gpt-5.6",
	}
	cfg := InvocationRequest{
		FactoryDir:       filepath.Join("tmp", "factory"),
		HomeDir:          filepath.Join("tmp", "home"),
		OperatorDefaults: defaults,
		Logger:           zap.NewNop(),
		Verbose:          true,
	}

	result, err := runBootstrapModelInvocation(
		context.Background(),
		operation,
		cfg,
		"voice",
		factoryapi.ModelInvocationRequest{
			Operation: "TTS",
			Content:   &factoryapi.WorkContent{mustGeneratedTextContentPart("speak this")},
		},
	)
	if err != nil {
		t.Fatalf("runBootstrapModelInvocation() error = %v, want success", err)
	}
	if result.ModelName != "voice" || result.Operation != "TTS" || result.Worker != "narrator" {
		t.Fatalf("result = %#v, want the operation result returned unchanged", result)
	}
	if operation.factoryDir != cfg.FactoryDir || operation.homeDir != cfg.HomeDir {
		t.Fatalf("target dirs = factory %q home %q, want %q and %q",
			operation.factoryDir, operation.homeDir, cfg.FactoryDir, cfg.HomeDir)
	}
	if operation.operatorDefaults != defaults {
		t.Fatalf("target operator defaults = %#v, want %#v", operation.operatorDefaults, defaults)
	}
	if !operation.verbose {
		t.Fatal("target verbose = false, want the resolved verbose flag carried through")
	}
	if operation.modelName != "voice" {
		t.Fatalf("model name = %q, want voice", operation.modelName)
	}
	if operation.request.Operation != "TTS" || len(operation.request.Content) != 1 {
		t.Fatalf("mapped request = %#v, want the TTS request with one content part", operation.request)
	}
}

// TestRunBootstrapModelInvocation_RejectsMissingInputs pins the two guard
// branches that produce a CLI error before any invocation is attempted.
func TestRunBootstrapModelInvocation_RejectsMissingInputs(t *testing.T) {
	t.Parallel()

	// The guard under test is specifically the nil-context branch, so an
	// explicitly nil Context value is the input being pinned here.
	var absentContext context.Context
	if _, err := runBootstrapModelInvocation(
		absentContext, &capturingModelInvocationOperation{}, InvocationRequest{}, "voice", factoryapi.ModelInvocationRequest{},
	); err == nil || !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("runBootstrapModelInvocation(nil ctx) error = %v, want a context requirement", err)
	}
	if _, err := runBootstrapModelInvocation(
		context.Background(), nil, InvocationRequest{}, "voice", factoryapi.ModelInvocationRequest{},
	); err == nil || !strings.Contains(err.Error(), "models invoke operation is required") {
		t.Fatalf("runBootstrapModelInvocation(nil operation) error = %v, want an operation requirement", err)
	}
}

func TestResolveBootstrapInvokeConfigUsesWorkingDirectoryResolver(t *testing.T) {
	t.Parallel()

	operation := &workingDirectoryResolverInvocation{resolvedFactoryDir: "project/factory"}
	defaults := operatorconfig.ResolvedDefaults{WorkerModelProvider: "codex"}
	config, err := resolveBootstrapInvokeConfig(invokeOptions{
		Invocation:       operation,
		FactoryDir:       "",
		WorkingDirectory: "project",
		HomeDir:          "home",
		OperatorDefaults: defaults,
		Verbose:          true,
	})
	if err != nil {
		t.Fatalf("resolveBootstrapInvokeConfig() error = %v", err)
	}
	if config.FactoryDir != "project/factory" || operation.gotExplicit != "" || operation.gotWorkingDir != "project" {
		t.Fatalf("resolved factory target = %#v, explicit=%q workingDirectory=%q", config, operation.gotExplicit, operation.gotWorkingDir)
	}
	if config.HomeDir != "home" || config.OperatorDefaults != defaults || !config.Verbose || config.Logger == nil {
		t.Fatalf("resolved invoke config = %#v, want defaults and a no-op logger", config)
	}
}

func TestBootstrapProjectionHelpersHandleEmptyAndAuthoredValues(t *testing.T) {
	t.Parallel()

	if got := derefGeneratedWorkContent(nil); got != nil {
		t.Fatalf("derefGeneratedWorkContent(nil) = %#v, want nil", got)
	}
	if got := generatedResolvedModelInvocationBindings(nil); got == nil || len(got) != 0 {
		t.Fatalf("empty generated bindings = %#v, want a non-nil empty slice", got)
	}
	bindings := generatedResolvedModelInvocationBindings([]modelinference.ResolvedModelOperationBinding{{
		Slot: "text", Source: "INPUT", Content: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText, Text: "hello",
		}},
	}})
	if len(bindings) != 1 || bindings[0].Slot != "text" || bindings[0].Content == nil {
		t.Fatalf("generated bindings = %#v, want one projected content binding", bindings)
	}

	label := "voice"
	selector := &factoryapi.WorkstationOperationBindingSelector{Label: &label}
	request := invocationRequestFromGenerated(factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Options: &factoryapi.ModelInvocationOptions{
			ResponseMode: func() *factoryapi.ModelInvocationResponseMode {
				mode := factoryapi.AUDIOSTREAM
				return &mode
			}(),
		},
		Bindings: &[]factoryapi.WorkstationOperationBinding{{Slot: "text", Selector: selector}},
	})
	if request.Options == nil || request.Options.ResponseMode == "" || len(request.Bindings) != 1 || request.Bindings[0].Selector == nil || request.Bindings[0].Selector.Label != label {
		t.Fatalf("mapped invocation request = %#v, want options and selector binding", request)
	}
}

func TestMapBootstrapModelInvokeErrorPreservesSentinelTaxonomy(t *testing.T) {
	t.Parallel()

	if mapBootstrapModelInvokeError(nil) != nil {
		t.Fatal("mapBootstrapModelInvokeError(nil) = non-nil")
	}
	for _, test := range []struct {
		name string
		err  error
		want error
	}{
		{name: "not found", err: modelinference.ErrNotFound, want: ErrModelNotFound},
		{name: "missing", err: modelinference.ErrMissing, want: modelinference.ErrMissing},
		{name: "unsupported response", err: modelinference.ErrUnsupportedResponseMode, want: modelinference.ErrUnsupportedResponseMode},
		{name: "unknown", err: errors.New("runtime failed"), want: nil},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			mapped := mapBootstrapModelInvokeError(test.err)
			if test.want == nil {
				if !errors.Is(mapped, test.err) {
					t.Fatalf("mapped error = %v, want original error", mapped)
				}
				return
			}
			if !errors.Is(mapped, test.want) {
				t.Fatalf("mapped error = %v, want errors.Is %v", mapped, test.want)
			}
		})
	}
}

func TestMapBootstrapModelInvokeError_PreservesInferenceFailureCauseChain(t *testing.T) {
	readinessErr := (modelinference.Runtime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: modelinference.ReadinessStateLoading,
		LifecycleState: modelinference.LifecycleStateLoading,
	}).InvocationError()
	failure := &modelinference.InferenceFailure{
		Class: modelinference.InferenceFailureClassLoadingModel, Message: "managed runtime is loading",
		ModelName: "OMNIVOICE_Q4_K_M", Operation: "TTS", Cause: readinessErr,
	}

	mapped := mapBootstrapModelInvokeError(failure)
	if !errors.Is(mapped, modelinference.ErrLoading) {
		t.Fatalf("mapped error = %v, want ErrManagedRuntimeLoading in chain", mapped)
	}
	var classified *modelinference.InferenceFailure
	if !errors.As(mapped, &classified) || classified.Class != modelinference.InferenceFailureClassLoadingModel {
		t.Fatalf("mapped error = %T, want loading_model InferenceFailure", mapped)
	}
}

func TestInvoke_AudioBootstrapCopiesStreamFile(t *testing.T) {
	originalBuilder := openTestModelRunner
	defer func() {
		openTestModelRunner = originalBuilder
	}()

	streamFile := filepath.Join(t.TempDir(), "stream.wav")
	if err := os.WriteFile(streamFile, []byte("RIFF....WAVE"), 0o644); err != nil {
		t.Fatalf("write stream file: %v", err)
	}

	openTestModelRunner = func(_ context.Context, _ *testModelRuntimeSelections) (testModelRunner, error) {
		return &stubModelBootstrapRunner{
			sessionReady: true,
			invokeModel: func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
				return modelinference.Result{
					ModelName:  modelName,
					Operation:  request.Operation,
					StreamFile: streamFile,
				}, nil
			},
		}, nil
	}

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
		t.Fatalf("read output: %v", err)
	}
	if string(got) != "RIFF....WAVE" {
		t.Fatalf("output = %q, want streamed audio bytes", string(got))
	}
}

func TestModelsCLIMissingCacheMessageKeepsThePullHint(t *testing.T) {
	t.Parallel()

	if got := modelCacheNotFoundMessage(modelinference.ErrModelCacheNotFound); got != "model cache is not installed; run you models pull <model> first" {
		t.Fatalf("modelCacheNotFoundMessage() = %q, want actionable pull hint", got)
	}
}

func TestReadModelsInvokeInputsClearsOnlyManifestOperationDefaultForNamedInputs(t *testing.T) {
	t.Parallel()

	inputs, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{
			{ID: modelsInvokeNameInputID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourcePositionalArgument}},
			{ID: modelsInvokeOperationID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault, resolvedinput.SourceCLIFlag}},
			{ID: modelsInvokeTextID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault}},
			{ID: modelsInvokeInputID, Kind: resolvedinput.ValueKindStringArray, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
			{ID: modelsInvokeOutputID, Kind: resolvedinput.ValueKindStringArray, Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault}},
		},
		[]resolvedinput.Candidate{
			{InputID: modelsInvokeNameInputID, Source: resolvedinput.SourcePositionalArgument, Value: resolvedinput.StringValue("embed")},
			{InputID: modelsInvokeOperationID, Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.StringValue("TTS")},
			{InputID: modelsInvokeTextID, Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.StringValue("")},
			{InputID: modelsInvokeInputID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringArrayValue([]string{"text=hello"})},
			{InputID: modelsInvokeOutputID, Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.StringArrayValue(nil)},
		},
	)
	if err != nil {
		t.Fatalf("resolve invoke inputs: %v", err)
	}
	got, err := readModelsInvokeInputs(inputs)
	if err != nil {
		t.Fatalf("readModelsInvokeInputs() error = %v", err)
	}
	if got.modelName != "embed" || got.operation != "" || len(got.inputMappings) != 1 || got.inputMappings[0] != "text=hello" {
		t.Fatalf("readModelsInvokeInputs() = %#v, want cleared default operation and named input", got)
	}
}

func TestCompositionServiceRecognizesDirectTTSAliasForOwnedOutputPath(t *testing.T) {
	t.Parallel()

	if !isDirectTTSAlias(InvokeConfig{ModelName: " tTs ", Operation: " tTs "}) {
		t.Fatal("isDirectTTSAlias() = false, want true for the built-in alias")
	}
	if isDirectTTSAlias(InvokeConfig{ModelName: "embed", Operation: modelinference.OperationEMBED}) {
		t.Fatal("isDirectTTSAlias() = true, want false for EMBED")
	}
}

func TestModelsCLIRemoteRemovePublishesTheDeleteResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete || request.URL.Path != "/models/embed" {
			t.Fatalf("remove request = %s %s, want DELETE /models/embed", request.Method, request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"modelName":"embed","outcome":"REMOVED","revision":"rev-1","cachePath":"/cache/embed","bytesRemoved":3}`))
	}))
	defer server.Close()

	service := NewService(Config{Models: ownedCoverageModelsRoot{}, HTTP: ownedCoverageHTTPProtocol(t)})
	var output bytes.Buffer
	if err := service.Remove(RemoveConfig{Context: context.Background(), Server: server.URL, ModelName: "embed", JSON: true, Output: &output}); err != nil {
		t.Fatalf("remote Remove() error = %v", err)
	}
	var response factoryapi.ModelRemoveResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode remote remove response: %v\n%s", err, output.String())
	}
	if response.ModelName != "embed" || response.Revision != "rev-1" || response.BytesRemoved != 3 {
		t.Fatalf("remote remove response = %#v, want detached delete response", response)
	}
}

func TestModelsCLIInvocationDiagnosticsCoverEveryPublicFailureFamily(t *testing.T) {
	t.Parallel()

	tests := []struct {
		class modelinference.InvocationFailureClass
		code  string
	}{
		{class: modelinference.InvocationFailureClassInvalidModelReference, code: modelsRootModelUnavailableCode},
		{class: modelinference.InvocationFailureClassRevisionResolution, code: modelsRootModelUnavailableCode},
		{class: modelinference.InvocationFailureClassInvalidOperation, code: modelsRootBadRequestCode},
		{class: modelinference.InvocationFailureClassInvalidSlot, code: modelsRootBadRequestCode},
		{class: modelinference.InvocationFailureClassSlotArity, code: modelsRootBadRequestCode},
		{class: modelinference.InvocationFailureClassInvalidParameter, code: modelsRootBadRequestCode},
		{class: modelinference.InvocationFailureClassMediaCapability, code: modelsRootBadRequestCode},
		{class: modelinference.InvocationFailureClassArtifact, code: modelsRootBadRequestCode},
		{class: modelinference.InvocationFailureClassOfflineCache, code: "MODEL_OFFLINE_CACHE_UNAVAILABLE"},
		{class: modelinference.InvocationFailureClassBackendReadiness, code: "MODEL_BACKEND_NOT_READY"},
		{class: modelinference.InvocationFailureClassBackendProtocol, code: "MODEL_BACKEND_FAILURE"},
		{class: modelinference.InvocationFailureClassMalformedResponse, code: "MODEL_BACKEND_FAILURE"},
		{class: modelinference.InvocationFailureClassCancellation, code: "MODEL_INFERENCE_TIMEOUT"},
		{class: modelinference.InvocationFailureClassTimeout, code: "MODEL_INFERENCE_TIMEOUT"},
		{class: modelinference.InvocationFailureClassConfiguration, code: "MODEL_CONFIGURATION_FAILURE"},
	}
	for _, test := range tests {
		t.Run(string(test.class), func(t *testing.T) {
			mapped, ok := mapModelsInvocationError(&modelinference.InvocationFailure{Class: test.class, Message: "failure"})
			if !ok || mapped == nil {
				t.Fatalf("mapModelsInvocationError(%q) = (%v, %v), want mapped error", test.class, mapped, ok)
			}
			rootError, ok := mapped.(*modelsRootError)
			if !ok || rootError.CLIErrorCode() != test.code {
				t.Fatalf("mapped %q = %#v, want code %q", test.class, mapped, test.code)
			}
		})
	}
}

func TestRootInvokeValidatesGenericCLIInputForms(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  InvokeConfig
		want string
	}{
		{name: "context", cfg: InvokeConfig{Output: io.Discard}, want: "context is required"},
		{name: "output", cfg: InvokeConfig{Context: context.Background()}, want: "output writer is required"},
		{name: "model", cfg: InvokeConfig{Context: context.Background(), Output: io.Discard}, want: "model name is required"},
		{name: "custom named input needs operation", cfg: InvokeConfig{Context: context.Background(), ModelName: "custom", InputMappings: []string{"prompt=hello"}, Output: io.Discard}, want: "--operation is required"},
		{name: "text needs operation", cfg: InvokeConfig{Context: context.Background(), ModelName: "custom", Text: "hello", Output: io.Discard}, want: "--operation is required"},
		{name: "text is required", cfg: InvokeConfig{Context: context.Background(), ModelName: "custom", Operation: modelinference.OperationOMNI, Output: io.Discard}, want: "--text is required"},
		{name: "text and named input conflict", cfg: InvokeConfig{Context: context.Background(), ModelName: "llm", InputMappings: []string{"prompt=hello"}, Text: "hello", Output: io.Discard}, want: "--text cannot be used with --input"},
		{name: "remote server", cfg: InvokeConfig{Context: context.Background(), ModelName: "custom", Operation: modelinference.OperationOMNI, Text: "hello", Server: "http://remote", Output: io.Discard}, want: "composition-stable HTTP service"},
		{name: "scope opener", cfg: InvokeConfig{Context: context.Background(), ModelName: "custom", Operation: modelinference.OperationOMNI, Text: "hello", Output: io.Discard}, want: "runtime scope opener is required"},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			service := &rootService{}
			err := service.Invoke(test.cfg)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Invoke() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestWriteGenericCLIOutputSelectsDeclaredInlineOutput(t *testing.T) {
	t.Parallel()

	required := true
	optional := false
	catalog := modelinference.Detail{Summary: modelinference.Summary{Operations: []modelinference.Operation{{
		Name: modelinference.OperationOMNI,
		Outputs: []modelinference.OperationSlot{
			{Name: "usage", Modality: modelinference.ModalityJSON, Required: &optional},
			{Name: "text", Modality: modelinference.ModalityText, Required: &required},
		},
	}}}}
	var out bytes.Buffer
	err := writeGenericCLIOutputWithCatalog(&out, modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{
		{Name: "usage", Modality: modelinference.ModalityJSON, Content: `{"tokens":3}`},
		{Name: "text", Modality: modelinference.ModalityText, Content: "answer"},
	}}, catalog, modelinference.OperationOMNI)
	if err != nil {
		t.Fatalf("writeGenericCLIOutput: %v", err)
	}
	if out.String() != "answer" {
		t.Fatalf("inline output = %q, want required text", out.String())
	}
}

func TestWriteGenericCLIOutputRejectsAmbiguousAndInvalidResults(t *testing.T) {
	t.Parallel()

	required := true
	catalog := modelinference.Detail{Summary: modelinference.Summary{Operations: []modelinference.Operation{{
		Name: modelinference.OperationOMNI,
		Outputs: []modelinference.OperationSlot{
			{Name: "text", Modality: modelinference.ModalityText, Required: &required},
			{Name: "summary", Modality: modelinference.ModalityText, Required: &required},
		},
	}}}}
	cases := []struct {
		name   string
		result modelinference.InvokeModelResult
		cat    modelinference.Detail
		want   string
	}{
		{name: "ambiguous", cat: catalog, result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text", Content: "one"}, {Name: "summary", Content: "two"}}}, want: "multiple model outputs"},
		{name: "unknown operation", result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text", Content: "one"}, {Name: "summary", Content: "two"}}}, want: "multiple model outputs"},
		{name: "empty inline value", result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text"}}}, want: "no inline output"},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			err := writeGenericCLIOutputWithCatalog(&out, test.result, test.cat, modelinference.OperationOMNI)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("writeGenericCLIOutput error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestWriteGenericCLIOutputMappingsRejectsInvalidResults(t *testing.T) {
	t.Parallel()

	service := &rootService{outputFileSystem: &outputPathTestFileSystem{}}
	cfg := InvokeConfig{Context: context.Background(), OutputMappings: []string{"text=answer.txt"}, Output: io.Discard}
	cases := []struct {
		name    string
		service *rootService
		cfg     InvokeConfig
		result  modelinference.InvokeModelResult
		want    string
	}{
		{name: "missing filesystem", service: &rootService{}, cfg: cfg, result: modelinference.InvokeModelResult{}, want: "filesystem is required"},
		{name: "invalid mapping", service: service, cfg: InvokeConfig{Context: context.Background(), OutputMappings: []string{"text"}, Output: io.Discard}, result: modelinference.InvokeModelResult{}, want: "expected slot=path"},
		{name: "output count", service: service, cfg: cfg, result: modelinference.InvokeModelResult{}, want: "returned 0 outputs"},
		{name: "unmapped slot", service: service, cfg: cfg, result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "usage", Content: "3"}}}, want: "unmapped output slot"},
		{name: "empty content", service: service, cfg: cfg, result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text"}}}, want: "has no inline bytes"},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			err := test.service.writeGenericCLIOutputMappings(test.cfg, test.result)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("writeGenericCLIOutputMappings error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestGenericCLIJSONResultHonorsJSONAndDeclaredShape(t *testing.T) {
	t.Parallel()

	catalog := modelinference.Detail{Summary: modelinference.Summary{Operations: []modelinference.Operation{{
		Name:    modelinference.OperationOMNI,
		Outputs: []modelinference.OperationSlot{{Name: "text", Modality: modelinference.ModalityText}},
	}}}}
	result := modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text", Content: "answer"}}}
	cases := []struct {
		name string
		cfg  InvokeConfig
		res  modelinference.InvokeModelResult
		cat  modelinference.Detail
		want bool
	}{
		{name: "not JSON", cfg: InvokeConfig{}, res: result, cat: catalog},
		{name: "empty outputs", cfg: InvokeConfig{JSON: true}, res: modelinference.InvokeModelResult{}, cat: catalog},
		{name: "multiple outputs", cfg: InvokeConfig{JSON: true}, res: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text"}, {Name: "usage"}}}, cat: catalog, want: true},
		{name: "declared inline output", cfg: InvokeConfig{JSON: true}, res: result, cat: catalog, want: true},
		{name: "unknown operation", cfg: InvokeConfig{JSON: true}, res: result, cat: catalog, want: false},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			operation := modelinference.OperationOMNI
			if test.name == "unknown operation" {
				operation = "missing"
			}
			if got := genericCLIJSONResult(test.cfg, test.cat, operation, test.res); got != test.want {
				t.Fatalf("genericCLIJSONResult() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestParseGenericCLIOutputMappingsRejectsAmbiguousMappings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		values []string
		want   string
	}{
		{name: "missing equals", values: []string{"text"}, want: "expected slot=path"},
		{name: "empty slot", values: []string{"=answer.txt"}, want: "slot and path are required"},
		{name: "empty path", values: []string{"text="}, want: "slot and path are required"},
		{name: "stdout path", values: []string{"text=-"}, want: "path '-' is not supported"},
		{name: "duplicate slot", values: []string{"text=one.txt", "text=two.txt"}, want: "duplicate output mapping"},
		{name: "duplicate path", values: []string{"text=answer.txt", "usage=answer.txt"}, want: "same path"},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseGenericCLIOutputMappings(test.values); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("parseGenericCLIOutputMappings error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateGenericCLIOutputMappingsRequiresExactDeclaredSlots(t *testing.T) {
	t.Parallel()

	operation := modelinference.Operation{
		Name:    modelinference.OperationOMNI,
		Outputs: []modelinference.OperationSlot{{Name: "text"}, {Name: "usage"}},
	}
	cases := []struct {
		name   string
		values []string
		found  bool
		want   string
	}{
		{name: "unknown operation", values: []string{"text=text.txt", "usage=usage.json"}, want: "unknown operation"},
		{name: "missing slot", values: []string{"text=text.txt"}, found: true, want: "cover every output slot"},
		{name: "unknown slot", values: []string{"text=text.txt", "other=other.txt"}, found: true, want: "unknown slot"},
		{name: "valid", values: []string{"text=text.txt", "usage=usage.json"}, found: true},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			err := validateGenericCLIOutputMappings(test.values, operation, test.found)
			if test.want == "" {
				if err != nil {
					t.Fatalf("validateGenericCLIOutputMappings() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateGenericCLIOutputMappings() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRefreshInvokeReadinessUsesObservedRuntimeAndMapsFailures(t *testing.T) {
	t.Parallel()

	catalog := modelinference.Detail{Summary: modelinference.Summary{ManagedRuntime: modelinference.Runtime{Identity: "catalog"}}}
	readiness := modelinference.Runtime{Identity: "observed", ReadinessState: modelinference.ReadinessStateReady}
	cases := []struct {
		name      string
		result    modelinference.GetModelReadinessResult
		err       error
		wantID    string
		wantError string
	}{
		{name: "observed readiness", result: modelinference.GetModelReadinessResult{Readiness: readiness}, wantID: "observed"},
		{name: "unsupported fallback", err: modelinference.ErrUnsupportedOperation, wantID: "catalog"},
		{name: "mapped failure", err: errors.New("readiness fixture failed"), wantError: "readiness fixture failed"},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			root := &rootService{models: readinessModelRoot{result: test.result, err: test.err}}
			got, err := root.refreshInvokeReadiness(InvokeConfig{Context: context.Background()}, modelinference.RuntimeScopeRef{}, "model", modelinference.OperationOMNI, catalog)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("refreshInvokeReadiness error = %v, want %q", err, test.wantError)
				}
				return
			}
			if err != nil || got.ManagedRuntime.Identity != test.wantID {
				t.Fatalf("refreshInvokeReadiness = %#v, error %v, want identity %q", got.ManagedRuntime, err, test.wantID)
			}
		})
	}
}

type readinessModelRoot struct {
	modelinference.Service
	result modelinference.GetModelReadinessResult
	err    error
}

func (root readinessModelRoot) GetModelReadiness(context.Context, modelinference.GetModelReadinessRequest) (modelinference.GetModelReadinessResult, error) {
	return root.result, root.err
}

func TestStageGenericCLIOutputFileHandlesFilesystemFailures(t *testing.T) {
	t.Parallel()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	closeCancelled, cancelAfterClose := context.WithCancel(context.Background())
	cases := []struct {
		name string
		ctx  context.Context
		fs   *scriptedOutputFileSystem
		want string
	}{
		{name: "cancelled before create", ctx: cancelled, fs: &scriptedOutputFileSystem{}, want: context.Canceled.Error()},
		{name: "create failure", ctx: context.Background(), fs: &scriptedOutputFileSystem{createErr: errors.New("create failed")}, want: "create failed"},
		{name: "nil temporary", ctx: context.Background(), fs: &scriptedOutputFileSystem{createNil: true}, want: "no handle"},
		{name: "write failure", ctx: context.Background(), fs: &scriptedOutputFileSystem{temp: &scriptedOutputTemporaryFile{name: "tmp", writeErr: errors.New("write failed")}}, want: "write failed"},
		{name: "short write", ctx: context.Background(), fs: &scriptedOutputFileSystem{temp: &scriptedOutputTemporaryFile{name: "tmp", shortWrite: true}}, want: io.ErrShortWrite.Error()},
		{name: "close failure", ctx: context.Background(), fs: &scriptedOutputFileSystem{temp: &scriptedOutputTemporaryFile{name: "tmp", closeErr: errors.New("close failed")}}, want: "close failed"},
		{name: "cancelled after close", ctx: closeCancelled, fs: &scriptedOutputFileSystem{temp: &scriptedOutputTemporaryFile{name: "tmp", onClose: cancelAfterClose}}, want: context.Canceled.Error()},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			_, err := stageGenericCLIOutputFile(test.ctx, test.fs, "answer.txt", []byte("answer"))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("stageGenericCLIOutputFile error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestPublishAndBackupGenericCLIOutputsHandleHostFailures(t *testing.T) {
	t.Parallel()

	t.Run("publish cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		published, err := publishGenericCLIOutputs(ctx, &scriptedOutputFileSystem{}, modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text"}}}, []genericCLIOutputStage{{temporary: "tmp", targetPath: "answer.txt"}})
		if published != 0 || !errors.Is(err, context.Canceled) {
			t.Fatalf("publish cancellation = count:%d error:%v", published, err)
		}
	})
	t.Run("publish rename failure", func(t *testing.T) {
		published, err := publishGenericCLIOutputs(context.Background(), &scriptedOutputFileSystem{renameErr: errors.New("rename failed")}, modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text"}}}, []genericCLIOutputStage{{temporary: "tmp", targetPath: "answer.txt"}})
		if published != 0 || err == nil || !strings.Contains(err.Error(), "rename failed") {
			t.Fatalf("publish rename = count:%d error:%v", published, err)
		}
	})
	t.Run("backup inspection failure", func(t *testing.T) {
		_, err := backupGenericCLIOutputTargets(context.Background(), &scriptedOutputFileSystem{inspectErr: errors.New("inspect failed")}, []genericCLIOutputMapping{{slot: "text", path: "answer.txt"}})
		if err == nil || !strings.Contains(err.Error(), "inspect failed") {
			t.Fatalf("backup inspect = %v, want inspect failure", err)
		}
	})
	t.Run("backup reservation failure", func(t *testing.T) {
		_, err := backupGenericCLIOutputTargets(context.Background(), &scriptedOutputFileSystem{inspectExists: true, createErr: errors.New("reserve failed")}, []genericCLIOutputMapping{{slot: "text", path: "answer.txt"}})
		if err == nil || !strings.Contains(err.Error(), "reserve failed") {
			t.Fatalf("backup reserve = %v, want reserve failure", err)
		}
	})
	t.Run("backup rename failure", func(t *testing.T) {
		_, err := backupGenericCLIOutputTargets(context.Background(), &scriptedOutputFileSystem{
			inspectExists: true,
			temp:          &scriptedOutputTemporaryFile{name: "backup.tmp"},
			renameErr:     errors.New("backup rename failed"),
		}, []genericCLIOutputMapping{{slot: "text", path: "answer.txt"}})
		if err == nil || !strings.Contains(err.Error(), "backup rename failed") {
			t.Fatalf("backup rename = %v, want rename failure", err)
		}
	})
}

type scriptedOutputFileSystem struct {
	temp          *scriptedOutputTemporaryFile
	createErr     error
	createNil     bool
	inspectErr    error
	inspectExists bool
	removeErr     error
	renameErr     error
}

func (fs *scriptedOutputFileSystem) CreateTemp(string, string) (OutputTemporaryFile, error) {
	if fs.createErr != nil {
		return nil, fs.createErr
	}
	if fs.createNil {
		return nil, nil
	}
	if fs.temp == nil {
		fs.temp = &scriptedOutputTemporaryFile{name: "temporary.tmp"}
	}
	return fs.temp, nil
}

func (fs *scriptedOutputFileSystem) Inspect(string) (os.FileInfo, error) {
	if fs.inspectErr != nil {
		return nil, fs.inspectErr
	}
	if fs.inspectExists {
		return scriptedFileInfo{}, nil
	}
	return nil, os.ErrNotExist
}

func (fs *scriptedOutputFileSystem) Remove(string) error { return fs.removeErr }

func (fs *scriptedOutputFileSystem) Rename(string, string) error { return fs.renameErr }

type scriptedOutputTemporaryFile struct {
	name       string
	writeErr   error
	shortWrite bool
	closeErr   error
	onClose    func()
}

func (file *scriptedOutputTemporaryFile) Write(data []byte) (int, error) {
	if file.writeErr != nil {
		return 0, file.writeErr
	}
	if file.shortWrite {
		return len(data) - 1, nil
	}
	return len(data), nil
}

func (file *scriptedOutputTemporaryFile) Close() error {
	if file.onClose != nil {
		file.onClose()
	}
	return file.closeErr
}

func (file *scriptedOutputTemporaryFile) Name() string { return file.name }

type scriptedFileInfo struct{}

func (scriptedFileInfo) Name() string       { return "answer.txt" }
func (scriptedFileInfo) Size() int64        { return 1 }
func (scriptedFileInfo) Mode() os.FileMode  { return 0o600 }
func (scriptedFileInfo) ModTime() time.Time { return time.Time{} }
func (scriptedFileInfo) IsDir() bool        { return false }
func (scriptedFileInfo) Sys() any           { return nil }

type outputPathTestFileSystem struct{}

func (*outputPathTestFileSystem) CreateTemp(string, string) (OutputTemporaryFile, error) {
	return nil, errors.New("unexpected CreateTemp")
}

func (*outputPathTestFileSystem) Inspect(string) (os.FileInfo, error) {
	return nil, errors.New("unexpected Inspect")
}

func (*outputPathTestFileSystem) Remove(string) error { return errors.New("unexpected Remove") }

func (*outputPathTestFileSystem) Rename(string, string) error { return errors.New("unexpected Rename") }
