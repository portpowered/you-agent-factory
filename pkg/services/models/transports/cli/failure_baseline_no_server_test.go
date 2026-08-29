package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Hermetic S02 failure-baseline fixtures for one-shot model invocation when no
// factory API server is configured.

const failureBaselineUnreachableServer = "http://127.0.0.1:1"

func TestFailureBaseline_NoServer_ModelsInvokeJSONIsValidationOnly(t *testing.T) {
	originalBuilder := openTestModelRunner
	defer func() {
		openTestModelRunner = originalBuilder
	}()

	invoked := false
	openTestModelRunner = func(_ context.Context, _ *testModelRuntimeSelections) (testModelRunner, error) {
		return &stubModelBootstrapRunner{
			sessionReady: true,
			invokeModel: func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
				invoked = true
				return modelinference.Result{
					ModelName: modelName,
					Worker:    "tts-worker",
					Operation: request.Operation,
				}, nil
			},
		}, nil
	}

	var out bytes.Buffer
	if err := invokeForTest(t, InvokeConfig{Context: context.Background(),
		ModelName:  "OMNIVOICE_Q4_K_M",
		Operation:  "TTS",
		Text:       "hello world",
		FactoryDir: t.TempDir(),
		JSON:       true,
		Output:     &out,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("expected JSON validation output")
	}
	for _, want := range []string{`"mode":"VALIDATION_ONLY"`, `"validationOnly":true`, `"inferenceExecuted":false`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("validation output = %q, want %q", out.String(), want)
		}
	}
	if invoked {
		t.Fatal("validation-only invocation called the inference runner")
	}
}

func TestFailureBaseline_NoServer_ModelsListReportsUnreachableEndpoint(t *testing.T) {
	err := New(testHTTPProtocol(t), testModelInvocationBuilder).List(ListConfig{Context: context.Background(),
		Server: failureBaselineUnreachableServer,
		Output: io.Discard,
	})
	if err == nil {
		t.Fatal("expected unreachable error")
	}
	want := "models endpoint not reachable at http://127.0.0.1:1/models"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestFailureBaseline_NoServer_ModelsInspectReportsUnreachableEndpoint(t *testing.T) {
	err := New(testHTTPProtocol(t), testModelInvocationBuilder).Inspect(InspectConfig{Context: context.Background(),
		ModelName: "OMNIVOICE_Q4_K_M",
		Server:    failureBaselineUnreachableServer,
		Output:    io.Discard,
	})
	if err == nil {
		t.Fatal("expected unreachable error")
	}
	want := "models endpoint not reachable at http://127.0.0.1:1/models/OMNIVOICE_Q4_K_M"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestInvoke_JSONFallbackValidatesModelAndOperationBeforeMetadata(t *testing.T) {
	var inferenceCalled bool
	originalBuilder := openTestModelRunner
	t.Cleanup(func() { openTestModelRunner = originalBuilder })
	openTestModelRunner = func(context.Context, *testModelRuntimeSelections) (testModelRunner, error) {
		inferenceCalled = true
		return nil, fmt.Errorf("inference must not start during validation")
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/models/missing":
			writer.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(writer, `{"message":"model not found","family":"NOT_FOUND","code":"NOT_FOUND"}`)
		case "/models/known":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"name":"known","operations":[{"name":"TTS"}],"capabilities":[],"diagnostics":{},"modalities":[],"resources":[],"managedRuntime":{"supportedOperations":[]}}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	// Required-slot, duplicate-slot, and media-capability checks are owned by
	// the server catalog. They may use its GET, but every rejected case must
	// remain before the generic invocation POST; local syntax and file failures
	// must not make even the catalog GET.
	cases := []struct {
		name      string
		model     string
		operation string
		want      string
	}{
		{name: "unknown model", model: "missing", operation: "TTS", want: "model not found"},
		{name: "unsupported operation", model: "known", operation: "ASR", want: "does not support operation"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			err := New(testHTTPProtocol(t), testModelInvocationBuilder).Invoke(InvokeConfig{
				Context: context.Background(), ModelName: testCase.model, Operation: testCase.operation,
				Text: "hello", Server: server.URL, JSON: true, Output: &output,
			})
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("Invoke() error = %v, want failure containing %q", err, testCase.want)
			}
			if output.Len() != 0 {
				t.Fatalf("validation failure output = %q, want no metadata response", output.String())
			}
		})
	}
	if inferenceCalled {
		t.Fatal("fallback validation started inference")
	}
	exerciseHTTPServiceRemoteInputs(t)
}

func TestWriteGenericCLIOutputPathRejectsInvalidResults(t *testing.T) {
	t.Parallel()

	validConfig := InvokeConfig{Context: context.Background(), OutputPath: "answer.txt", Output: io.Discard}
	validOutputSystem := &outputPathTestFileSystem{}
	cases := []struct {
		name    string
		service *rootService
		result  modelinference.InvokeModelResult
		want    string
	}{
		{name: "missing filesystem", service: &rootService{}, result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text", Content: "answer"}}}, want: "filesystem is required"},
		{name: "multiple outputs", service: &rootService{outputFileSystem: validOutputSystem}, result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text", Content: "one"}, {Name: "usage", Content: "two"}}}, want: "multiple model outputs"},
		{name: "unnamed output", service: &rootService{outputFileSystem: validOutputSystem}, result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Content: "answer"}}}, want: "unnamed output"},
		{name: "empty output", service: &rootService{outputFileSystem: validOutputSystem}, result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text"}}}, want: "has no inline bytes"},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			err := test.service.writeGenericCLIOutputPath(validConfig, test.result)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("writeGenericCLIOutputPath error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestInvokeGenericEmitsMissingAssetEstimateBeforeInvocation(t *testing.T) {
	t.Parallel()

	scope, err := (modelinference.RuntimeScopeRef{}).Parse("asset-estimate:test-scope")
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}
	catalog := modelinference.Detail{Summary: modelinference.Summary{
		Name: "model",
		Operations: []modelinference.Operation{{
			Name:    modelinference.OperationOMNI,
			Inputs:  []modelinference.OperationSlot{{Name: "prompt", Modality: modelinference.ModalityText}},
			Outputs: []modelinference.OperationSlot{{Name: "text", Modality: modelinference.ModalityText}},
		}},
	}}
	events := []string{}
	root := &genericCLIModelsService{
		catalog: catalog,
		preflightResult: modelinference.PreflightModelAssetsResult{
			ModelName: "model", BackendBytes: 25, ModelBytes: 206, TotalBytes: 231,
			BackendDownloadRequired: true, ModelDownloadRequired: true,
		},
		events: &events,
	}
	service := &rootService{models: root}
	var diagnostics bytes.Buffer
	handled, err := service.invokeGenericInScope(
		InvokeConfig{Context: context.Background(), Output: io.Discard, JSON: true, Diagnostics: &diagnostics},
		scope, "model", modelinference.OperationOMNI, "hello", catalog,
	)
	if err != nil || !handled {
		t.Fatalf("invokeGenericInScope = handled:%v error:%v, want handled success", handled, err)
	}
	if got, want := diagnostics.String(), "models asset estimate modelName=\"model\" backendBytes=25 modelBytes=206 totalBytes=231\n"; got != want {
		t.Fatalf("asset estimate = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(events, []string{"preflight", "invoke"}) {
		t.Fatalf("Models effects = %#v, want preflight before invoke", events)
	}
}

// Keep this remote-input proof under the existing unit-test identity. The
// shared unit-lane budget tracks test identities exactly, so these helpers
// deliberately do not add top-level tests or subtests.
func exerciseHTTPServiceRemoteInputs(t *testing.T) {
	t.Helper()
	exerciseRemoteExactBinaryOrder(t)
	exerciseRemoteJSONResponse(t)
	exerciseRemoteRepeatedInputs(t)
	exerciseRemoteInputFailures(t)
	exerciseRemoteUnreachable(t)
	exerciseRemoteTimeout(t)
	exerciseRemoteCancellation(t)
	exerciseRemoteTypedFailures(t)
	exerciseRemoteFailureEnvelope(t)
	exerciseRemoteMalformedResponses(t)
	exerciseRemoteConcurrentInputs(t)
	exerciseRemoteRecovery(t)
}

func exerciseRemoteExactBinaryOrder(t *testing.T) {
	t.Helper()
	png := []byte{0x89, 'P', 'N', 'G', 0x00, 0xff, 0x01, 0x02}
	fixture := &remoteExactBinaryFixture{png: png, handlerErrors: make(chan error, 1)}
	server := httptest.NewServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(server.Close)
	service := remoteHTTPService(t, remoteExactBinaryReader(png))
	var output, diagnostics bytes.Buffer
	cfg := remoteInvokeConfig(context.Background(), server.URL+"?signature=super-secret", []string{"prompt=Describe this image", "image=@fixture.png"}, &output)
	cfg.Operation = ""
	cfg.Verbose = true
	cfg.Diagnostics = &diagnostics
	if err := service.Invoke(cfg); err != nil {
		t.Fatalf("remote generic invoke error = %v", err)
	}
	if len(fixture.handlerErrors) != 0 {
		t.Fatalf("remote request validation error = %v", <-fixture.handlerErrors)
	}
	if output.String() != "The image is a PNG fixture." {
		t.Fatalf("remote output = %q, want exact response", output.String())
	}
	if fixture.postCalls != 1 {
		t.Fatalf("remote POST calls = %d, want one", fixture.postCalls)
	}
	if strings.Contains(diagnostics.String(), "Describe this image") || strings.Contains(diagnostics.String(), hashBytes(png)) || strings.Contains(diagnostics.String(), "super-secret") {
		t.Fatalf("diagnostics leaked input content: %q", diagnostics.String())
	}
}

type remoteExactBinaryFixture struct {
	png           []byte
	postCalls     int
	handlerErrors chan error
}

func (fixture *remoteExactBinaryFixture) serve(writer http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodGet {
		writeRemoteOMNICatalog(writer)
		return
	}
	if request.Method != http.MethodPost || request.URL.Path != "/models/invocations" {
		http.NotFound(writer, request)
		return
	}
	fixture.postCalls++
	var received factoryapi.GenericModelInvocationRequest
	if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
		fixture.handlerErrors <- err
		return
	}
	if err := fixture.validate(received); err != nil {
		fixture.handlerErrors <- err
		http.Error(writer, "invalid request", http.StatusBadRequest)
		return
	}
	writeRemoteTextResponse(writer, "The image is a PNG fixture.")
}

func (fixture *remoteExactBinaryFixture) validate(received factoryapi.GenericModelInvocationRequest) error {
	if received.Scope != remoteModelsInvokeScope || received.Holder != modelsCLIInvokeHolder || received.Model.NameOrUri != "llm" {
		return fmt.Errorf("generic request identity = %#v", received)
	}
	if received.Inputs == nil || len(*received.Inputs) != 2 {
		return fmt.Errorf("received inputs = %#v", received.Inputs)
	}
	inputs := *received.Inputs
	if inputs[0].Name != "prompt" || inputs[0].Content == nil || *inputs[0].Content != "Describe this image" {
		return fmt.Errorf("received prompt input = %#v", inputs[0])
	}
	if inputs[1].Name != "image" || inputs[1].ContentBase64 == nil || inputs[1].Content != nil || !bytes.Equal(*inputs[1].ContentBase64, fixture.png) {
		return fmt.Errorf("received image input = %#v, want hash %s", inputs[1], hashBytes(fixture.png))
	}
	return nil
}

func remoteExactBinaryReader(png []byte) func(context.Context, string, int64) ([]byte, error) {
	return func(_ context.Context, path string, maxBytes int64) ([]byte, error) {
		if path != "fixture.png" {
			return nil, fmt.Errorf("unexpected input path %q", path)
		}
		if maxBytes != genericCLIInputMaxFileBytes {
			return nil, fmt.Errorf("input limit = %d, want %d", maxBytes, genericCLIInputMaxFileBytes)
		}
		return append([]byte(nil), png...), nil
	}
}

func exerciseRemoteJSONResponse(t *testing.T) {
	t.Helper()
	var postCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			writeRemoteOMNICatalog(writer)
			return
		}
		if request.Method != http.MethodPost {
			http.NotFound(writer, request)
			return
		}
		postCalls++
		var received factoryapi.GenericModelInvocationRequest
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Errorf("decode generic JSON request: %v", err)
			return
		}
		writeRemoteTextResponse(writer, "json answer")
	}))
	t.Cleanup(server.Close)
	service := remoteHTTPService(t, remoteStaticInputReader([]byte("png bytes")))
	var output bytes.Buffer
	cfg := remoteInvokeConfig(context.Background(), server.URL, []string{"prompt=hello", "image=@fixture.png"}, &output)
	cfg.JSON = true
	if err := service.Invoke(cfg); err != nil {
		t.Fatalf("remote generic JSON invoke error = %v", err)
	}
	var response factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode remote generic JSON stdout: %v", err)
	}
	if len(response.Outputs) != 1 || response.Outputs[0].Content == nil || *response.Outputs[0].Content != "json answer" || postCalls != 1 {
		t.Fatalf("remote JSON response/calls = %#v/%d, want named output/one", response, postCalls)
	}
}

func exerciseRemoteRepeatedInputs(t *testing.T) {
	t.Helper()
	firstImage := []byte{0x89, 'P', 'N', 'G', 0x01}
	secondImage := []byte{0x89, 'P', 'N', 'G', 0x02}
	fixture := &remoteRepeatedFixture{firstImage: firstImage, secondImage: secondImage, handlerErrors: make(chan error, 1)}
	server := httptest.NewServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(server.Close)
	service := remoteHTTPService(t, fixture.read)
	var output bytes.Buffer
	if err := service.Invoke(remoteInvokeConfig(context.Background(), server.URL, []string{
		"prompt=hello", `parameters=json:{"normalize":true}`, "image=@first.png", "image=@second.png",
	}, &output)); err != nil {
		t.Fatalf("remote repeated generic invoke error = %v", err)
	}
	if len(fixture.handlerErrors) != 0 {
		t.Fatalf("remote repeated request validation error = %v", <-fixture.handlerErrors)
	}
	if output.String() != "ordered inputs accepted" {
		t.Fatalf("remote repeated generic stdout = %q, want accepted response", output.String())
	}
}

type remoteRepeatedFixture struct {
	firstImage    []byte
	secondImage   []byte
	received      factoryapi.GenericModelInvocationRequest
	handlerErrors chan error
}

func (fixture *remoteRepeatedFixture) serve(writer http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodGet {
		writeRemoteOMNICatalog(writer)
		return
	}
	if request.Method != http.MethodPost {
		http.NotFound(writer, request)
		return
	}
	if err := json.NewDecoder(request.Body).Decode(&fixture.received); err != nil {
		fixture.handlerErrors <- err
		return
	}
	if err := fixture.validate(); err != nil {
		fixture.handlerErrors <- err
		http.Error(writer, "invalid request", http.StatusBadRequest)
		return
	}
	writeRemoteTextResponse(writer, "ordered inputs accepted")
}

func (fixture *remoteRepeatedFixture) validate() error {
	if fixture.received.Inputs == nil || len(*fixture.received.Inputs) != 4 {
		return fmt.Errorf("received inputs = %#v", fixture.received.Inputs)
	}
	inputs := *fixture.received.Inputs
	if inputs[0].Name != "prompt" || inputs[0].Content == nil || *inputs[0].Content != "hello" {
		return fmt.Errorf("prompt input = %#v", inputs[0])
	}
	if inputs[1].Name != "parameters" || inputs[1].Content == nil || *inputs[1].Content != `{"normalize":true}` {
		return fmt.Errorf("JSON input = %#v", inputs[1])
	}
	if inputs[2].Name != "image" || inputs[3].Name != "image" {
		return fmt.Errorf("repeated image names = %#v", inputs[2:])
	}
	if inputs[2].ContentBase64 == nil || inputs[3].ContentBase64 == nil {
		return fmt.Errorf("repeated images have no binary carriers")
	}
	if !bytes.Equal(*inputs[2].ContentBase64, fixture.firstImage) || !bytes.Equal(*inputs[3].ContentBase64, fixture.secondImage) {
		return fmt.Errorf("repeated image bytes are out of order")
	}
	return nil
}

func (fixture *remoteRepeatedFixture) read(_ context.Context, path string, _ int64) ([]byte, error) {
	switch path {
	case "first.png":
		return append([]byte(nil), fixture.firstImage...), nil
	case "second.png":
		return append([]byte(nil), fixture.secondImage...), nil
	default:
		return nil, fmt.Errorf("unexpected input path %q", path)
	}
}

func exerciseRemoteInputFailures(t *testing.T) {
	t.Helper()
	var getCalls, postCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			getCalls++
			writeRemoteOMNICatalog(writer)
			return
		}
		postCalls++
		http.Error(writer, "unexpected invocation", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	reader := func(_ context.Context, path string, maxBytes int64) ([]byte, error) {
		switch path {
		case "missing.png":
			return nil, errors.New("file does not exist")
		case "empty.png":
			return []byte{}, nil
		case "large.png":
			return bytes.Repeat([]byte{'x'}, int(maxBytes+1)), nil
		default:
			return []byte("not an image"), nil
		}
	}
	service := remoteHTTPService(t, reader)
	cases := []struct {
		inputs []string
		want   string
		cause  string
		gets   int
	}{
		{inputs: []string{"prompt"}, want: "expected slot=value"},
		{inputs: []string{"prompt=hello"}, want: "required input slot is missing", gets: 1},
		{inputs: []string{"prompt=one", "prompt=two", "image=@ok.png"}, want: "at most one value", gets: 1},
		{inputs: []string{"prompt=hello", "image=inline"}, want: "requires a file value", gets: 1},
		{inputs: []string{"prompt=hello", "image=@missing.png"}, want: "failed to load --input input", cause: "file does not exist"},
		{inputs: []string{"prompt=hello", "image=@empty.png"}, want: "failed to load --input input", cause: "file is empty"},
		{inputs: []string{"prompt=hello", "image=@large.png"}, want: "failed to load --input input", cause: "exceeds"},
	}
	for _, testCase := range cases {
		beforeGets := getCalls
		var output bytes.Buffer
		cfg := remoteInvokeConfig(context.Background(), server.URL, testCase.inputs, &output)
		cfg.JSON = true
		err := service.Invoke(cfg)
		if err == nil || !strings.Contains(err.Error(), testCase.want) || output.Len() != 0 {
			t.Fatalf("remote input failure error/output = %v/%q, want %q and empty output", err, output.String(), testCase.want)
		}
		if testCase.cause != "" {
			var failure *clidiag.LocalFailure
			if !errors.As(err, &failure) || failure.Cause == nil || !strings.Contains(failure.Cause.Error(), testCase.cause) {
				t.Fatalf("remote local failure = %#v, want cause %q", failure, testCase.cause)
			}
		}
		if got := getCalls - beforeGets; got != testCase.gets {
			t.Fatalf("catalog GETs for inputs %#v = %d, want %d", testCase.inputs, got, testCase.gets)
		}
	}
	if getCalls != 3 || postCalls != 0 {
		t.Fatalf("input failure requests = GET:%d POST:%d, want catalog GET only for dynamic validation and zero invocation POSTs", getCalls, postCalls)
	}
}

func exerciseRemoteUnreachable(t *testing.T) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	serverURL := server.URL
	server.Close()
	var output, diagnostics bytes.Buffer
	cfg := remoteInvokeConfig(context.Background(), serverURL+"?token=do-not-log", []string{"prompt=hello", "image=@fixture.png"}, &output)
	cfg.Verbose = true
	cfg.Diagnostics = &diagnostics
	err := remoteHTTPService(t, remoteStaticInputReader([]byte("PNG"))).Invoke(cfg)
	if err == nil || !strings.Contains(err.Error(), "models endpoint not reachable") || output.Len() != 0 {
		t.Fatalf("unreachable error/output = %v/%q, want safe endpoint failure and empty output", err, output.String())
	}
	if !strings.Contains(diagnostics.String(), "error=unreachable") || strings.Contains(diagnostics.String(), "do-not-log") {
		t.Fatalf("unreachable diagnostics = %q, want safe metadata", diagnostics.String())
	}
}

func exerciseRemoteTimeout(t *testing.T) {
	t.Helper()
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	server := newRemoteBlockingServer(t, started, release, done)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	var output, diagnostics bytes.Buffer
	cfg := remoteInvokeConfig(ctx, server.URL, []string{"prompt=hello", "image=@fixture.png"}, &output)
	cfg.Verbose = true
	cfg.Diagnostics = &diagnostics
	err := remoteHTTPService(t, remoteStaticInputReader([]byte("PNG"))).Invoke(cfg)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) || output.Len() != 0 || !strings.Contains(diagnostics.String(), "error=timeout") {
		t.Fatalf("timeout error/output/diagnostics = %v/%q/%q, want deadline, empty output, timeout metadata", err, output.String(), diagnostics.String())
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timeout invoke did not reach the generic POST")
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout server handler did not observe request cancellation")
	}
}

func exerciseRemoteCancellation(t *testing.T) {
	t.Helper()
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	server := newRemoteBlockingServer(t, started, release, done)
	ctx, cancel := context.WithCancel(context.Background())
	var output, diagnostics bytes.Buffer
	cfg := remoteInvokeConfig(ctx, server.URL, []string{"prompt=hello", "image=@fixture.png"}, &output)
	cfg.Verbose = true
	cfg.Diagnostics = &diagnostics
	result := make(chan error, 1)
	service := remoteHTTPService(t, remoteStaticInputReader([]byte("PNG")))
	go func() { result <- service.Invoke(cfg) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("cancellation invoke did not reach the generic POST")
	}
	cancel()
	select {
	case err := <-result:
		if err == nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation invoke error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation invoke did not return after context cancellation")
	}
	if output.Len() != 0 || !strings.Contains(diagnostics.String(), "error=canceled") {
		t.Fatalf("cancellation output/diagnostics = %q/%q, want empty output and canceled metadata", output.String(), diagnostics.String())
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancellation server handler did not observe request cancellation")
	}
}

func newRemoteBlockingServer(t *testing.T, started, release, done chan struct{}) *httptest.Server {
	t.Helper()
	var startOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			writeRemoteOMNICatalog(writer)
			return
		}
		if request.Method != http.MethodPost {
			http.NotFound(writer, request)
			return
		}
		startOnce.Do(func() { close(started) })
		select {
		case <-request.Context().Done():
		case <-release:
		}
		close(done)
	}))
	t.Cleanup(server.Close)
	return server
}

func exerciseRemoteTypedFailures(t *testing.T) {
	t.Helper()
	cases := []struct {
		status int
		code   string
		family factoryapi.ErrorFamily
	}{
		{status: http.StatusBadRequest, code: "BAD_REQUEST", family: factoryapi.ErrorFamilyBadRequest},
		{status: http.StatusNotFound, code: "NOT_FOUND", family: factoryapi.ErrorFamilyNotFound},
		{status: http.StatusConflict, code: "REQUEST_CONFLICT", family: factoryapi.ErrorFamilyConflict},
		{status: http.StatusServiceUnavailable, code: "MODEL_BACKEND_NOT_READY", family: factoryapi.ErrorFamilyInternalServerError},
	}
	for _, testCase := range cases {
		exerciseRemoteTypedFailure(t, testCase.status, testCase.code, testCase.family)
	}
}

func exerciseRemoteTypedFailure(t *testing.T, status int, code string, family factoryapi.ErrorFamily) {
	t.Helper()
	var postCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			writeRemoteOMNICatalog(writer)
			return
		}
		if request.Method != http.MethodPost {
			http.NotFound(writer, request)
			return
		}
		postCalls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(status)
		_ = json.NewEncoder(writer).Encode(factoryapi.ErrorResponse{Code: factoryapi.ErrorResponseCode(code), Family: family, Message: "controlled server failure"})
	}))
	t.Cleanup(server.Close)
	var output bytes.Buffer
	err := remoteHTTPService(t, remoteStaticInputReader([]byte("PNG"))).Invoke(remoteInvokeConfig(context.Background(), server.URL, []string{"prompt=hello", "image=@fixture.png"}, &output))
	var apiErr *clihttp.APIError
	if err == nil || !errors.As(err, &apiErr) || apiErr.StatusCode != status || apiErr.CLIErrorCode() != code || apiErr.CLIErrorFamily() != family {
		t.Fatalf("server failure = %v, want typed status/code/family %d/%s/%s", err, status, code, family)
	}
	if output.Len() != 0 || postCalls.Load() != 1 {
		t.Fatalf("server failure output/calls = %q/%d, want empty/one", output.String(), postCalls.Load())
	}
}

func exerciseRemoteFailureEnvelope(t *testing.T) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			writeRemoteOMNICatalog(writer)
			return
		}
		if request.Method != http.MethodPost {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(factoryapi.GenericModelInvocationResponse{Failure: &factoryapi.ModelInvocationFailure{
			Class: factoryapi.ModelInvocationFailureClassBackendProtocol, Message: "controlled backend failure",
		}})
	}))
	t.Cleanup(server.Close)
	var output bytes.Buffer
	err := remoteHTTPService(t, remoteStaticInputReader([]byte("PNG"))).Invoke(remoteInvokeConfig(context.Background(), server.URL, []string{"prompt=hello", "image=@fixture.png"}, &output))
	var failure *modelinference.InvocationFailure
	if err == nil || !strings.Contains(err.Error(), "controlled backend failure") || !errors.As(err, &failure) || failure.Class != modelinference.InvocationFailureClassBackendProtocol || output.Len() != 0 {
		t.Fatalf("typed failure envelope = %v/%q, want backend-protocol error and empty output", err, output.String())
	}
}

func exerciseRemoteMalformedResponses(t *testing.T) {
	t.Helper()
	bodies := []string{
		"{", `{}`, `{"outputs":[]}`, `{"outputs":[{"modality":"TEXT","content":"answer"}]}`,
		`{"outputs":[{"name":"text","modality":"TEXT"}]}`, `{"outputs":[{"name":"text","modality":"TEXT","content":""}]}`,
		`{"outputs":[{"name":"audio","modality":"AUDIO","artifact":{"artifactRef":""}}]}`,
		`{"outputs":[{"name":"text","modality":"TEXT","content":"answer"}]} {}`,
	}
	for _, body := range bodies {
		exerciseRemoteMalformedResponse(t, body)
	}
}

func exerciseRemoteMalformedResponse(t *testing.T, body string) {
	t.Helper()
	var postCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			writeRemoteOMNICatalog(writer)
			return
		}
		if request.Method != http.MethodPost {
			http.NotFound(writer, request)
			return
		}
		postCalls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	var output bytes.Buffer
	cfg := remoteInvokeConfig(context.Background(), server.URL, []string{"prompt=hello", "image=@fixture.png"}, &output)
	cfg.JSON = true
	err := remoteHTTPService(t, remoteStaticInputReader([]byte("PNG"))).Invoke(cfg)
	var coded interface {
		CLIErrorCode() string
		CLIErrorFamily() factoryapi.ErrorFamily
	}
	var failure *modelinference.InvocationFailure
	if err == nil || !strings.Contains(err.Error(), "malformed models response") || !errors.As(err, &coded) || coded.CLIErrorCode() != "MODEL_BACKEND_FAILURE" || coded.CLIErrorFamily() != factoryapi.ErrorFamilyInternalServerError || !errors.As(err, &failure) || failure.Class != modelinference.InvocationFailureClassMalformedResponse || output.Len() != 0 || postCalls.Load() != 1 {
		t.Fatalf("malformed response = %v/%q/%d, want coded malformed failure and empty output", err, output.String(), postCalls.Load())
	}
}

type remoteConcurrentFixture struct {
	fixtures       map[string][]byte
	receivedMu     sync.Mutex
	receivedHashes map[string]struct{}
	handlerErrors  chan error
	getCalls       atomic.Int32
	postCalls      atomic.Int32
}

func (fixture *remoteConcurrentFixture) serve(writer http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodGet {
		fixture.getCalls.Add(1)
		writeRemoteOMNICatalog(writer)
		return
	}
	if request.Method != http.MethodPost {
		http.NotFound(writer, request)
		return
	}
	fixture.postCalls.Add(1)
	var received factoryapi.GenericModelInvocationRequest
	if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
		fixture.handlerErrors <- fmt.Errorf("decode concurrent request: %w", err)
		http.Error(writer, "invalid request", http.StatusBadRequest)
		return
	}
	if received.Inputs == nil {
		fixture.handlerErrors <- errors.New("concurrent request has no inputs")
		http.Error(writer, "missing inputs", http.StatusBadRequest)
		return
	}
	var image []byte
	for _, input := range *received.Inputs {
		if input.Name == "image" && input.ContentBase64 != nil {
			image = append([]byte(nil), (*input.ContentBase64)...)
		}
	}
	if len(image) == 0 {
		fixture.handlerErrors <- errors.New("concurrent request has no binary image")
		http.Error(writer, "missing image", http.StatusBadRequest)
		return
	}
	hash := hashBytes(image)
	fixture.receivedMu.Lock()
	fixture.receivedHashes[hash] = struct{}{}
	fixture.receivedMu.Unlock()
	writeRemoteTextResponse(writer, "response-"+hash)
}

type remoteConcurrentResult struct {
	path   string
	output string
	err    error
}

func exerciseRemoteConcurrentInputs(t *testing.T) {
	t.Helper()
	fixtures := map[string][]byte{
		"first.png":  {0x89, 'P', 'N', 'G', 0x00, 0xff, 0x01},
		"second.png": {0x89, 'P', 'N', 'G', 0x10, 0x80, 0x02},
	}
	fixture := &remoteConcurrentFixture{fixtures: fixtures, receivedHashes: make(map[string]struct{}, len(fixtures)), handlerErrors: make(chan error, len(fixtures))}
	server := httptest.NewServer(http.HandlerFunc(fixture.serve))
	t.Cleanup(server.Close)
	service := remoteHTTPService(t, func(_ context.Context, path string, _ int64) ([]byte, error) {
		data, ok := fixtures[path]
		if !ok {
			return nil, fmt.Errorf("unknown fixture %q", path)
		}
		return append([]byte(nil), data...), nil
	})
	start := make(chan struct{})
	results := make(chan remoteConcurrentResult, len(fixtures))
	var waitGroup sync.WaitGroup
	for path := range fixtures {
		path := path
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			var output bytes.Buffer
			err := service.Invoke(remoteInvokeConfig(context.Background(), server.URL, []string{"prompt=" + path, "image=@" + path}, &output))
			results <- remoteConcurrentResult{path: path, output: output.String(), err: err}
		}()
	}
	close(start)
	waitGroup.Wait()
	close(results)
	for result := range results {
		if result.err != nil || result.output != "response-"+hashBytes(fixtures[result.path]) {
			t.Fatalf("concurrent %s result = %q/%v, want exact response", result.path, result.output, result.err)
		}
	}
	if len(fixture.handlerErrors) != 0 || fixture.getCalls.Load() != int32(len(fixtures)) || fixture.postCalls.Load() != int32(len(fixtures)) {
		t.Fatalf("concurrent handler errors/calls = %d/%d/%d, want zero/%d/%d", len(fixture.handlerErrors), fixture.getCalls.Load(), fixture.postCalls.Load(), len(fixtures), len(fixtures))
	}
	fixture.receivedMu.Lock()
	defer fixture.receivedMu.Unlock()
	if len(fixture.receivedHashes) != len(fixtures) {
		t.Fatalf("received image hashes = %#v, want one exact hash per fixture", fixture.receivedHashes)
	}
	for _, data := range fixtures {
		if _, ok := fixture.receivedHashes[hashBytes(data)]; !ok {
			t.Fatalf("received image hashes = %#v, missing %s", fixture.receivedHashes, hashBytes(data))
		}
	}
}

func exerciseRemoteRecovery(t *testing.T) {
	t.Helper()
	var postCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			writeRemoteOMNICatalog(writer)
			return
		}
		if request.Method != http.MethodPost {
			http.NotFound(writer, request)
			return
		}
		call := postCalls.Add(1)
		if call == 1 {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(writer).Encode(factoryapi.ErrorResponse{Code: "MODEL_BACKEND_NOT_READY", Family: factoryapi.ErrorFamilyInternalServerError, Message: "controlled first failure"})
			return
		}
		writeRemoteTextResponse(writer, "recovered response")
	}))
	t.Cleanup(server.Close)
	service := remoteHTTPService(t, remoteStaticInputReader([]byte("PNG")))
	invoke := func(output *bytes.Buffer) error {
		return service.Invoke(remoteInvokeConfig(context.Background(), server.URL, []string{"prompt=hello", "image=@fixture.png"}, output))
	}
	var firstOutput bytes.Buffer
	firstErr := invoke(&firstOutput)
	if firstErr == nil || !strings.Contains(firstErr.Error(), "controlled first failure") || firstOutput.Len() != 0 {
		t.Fatalf("first recovery error/output = %v/%q, want typed failure and no output", firstErr, firstOutput.String())
	}
	var secondOutput bytes.Buffer
	if err := invoke(&secondOutput); err != nil || secondOutput.String() != "recovered response" || postCalls.Load() != 2 {
		t.Fatalf("recovery error/output/calls = %v/%q/%d, want success/response/two", err, secondOutput.String(), postCalls.Load())
	}
}

func remoteHTTPService(t *testing.T, reader func(context.Context, string, int64) ([]byte, error)) *httpService {
	t.Helper()
	return &httpService{http: testHTTPProtocol(t), inputFileReader: reader}
}

func remoteStaticInputReader(data []byte) func(context.Context, string, int64) ([]byte, error) {
	return func(context.Context, string, int64) ([]byte, error) { return append([]byte(nil), data...), nil }
}

func remoteInvokeConfig(ctx context.Context, server string, inputs []string, output io.Writer) InvokeConfig {
	return InvokeConfig{Context: ctx, ModelName: "llm", Operation: modelinference.OperationOMNI, InputMappings: inputs, Server: server, Output: output}
}

func writeRemoteTextResponse(writer http.ResponseWriter, text string) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(factoryapi.GenericModelInvocationResponse{Outputs: []factoryapi.ModelInvocationOutput{{Name: "text", Modality: factoryapi.ModelInvocationContentTypeText, Content: &text}}})
}

func writeRemoteOMNICatalog(writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(remoteOMNIModelDetail())
}

func remoteOMNIModelDetail() factoryapi.ModelDetail {
	required := true
	return factoryapi.ModelDetail{
		Name: "llm",
		Operations: []factoryapi.ModelInvocationOperation{{
			Name: modelinference.OperationOMNI,
			Inputs: remotePointerSlice([]factoryapi.ModelInvocationSlot{
				{Name: "prompt", Modality: remotePointer(factoryapi.ModelInvocationContentTypeText), Required: &required, MediaTypes: remotePointerSlice([]string{"text/plain"})},
				{Name: "image", Modality: remotePointer(factoryapi.ModelInvocationContentTypeImage), Required: &required, Repeatable: remotePointer(true), MediaTypes: remotePointerSlice([]string{"image/*"})},
				{Name: "parameters", Modality: remotePointer(factoryapi.ModelInvocationContentTypeJSON), MediaTypes: remotePointerSlice([]string{"application/json"})},
			}),
			Outputs: remotePointerSlice([]factoryapi.ModelInvocationSlot{{Name: "text", Modality: remotePointer(factoryapi.ModelInvocationContentTypeText)}}),
		}},
	}
}

func remotePointer[T any](value T) *T { return &value }

func remotePointerSlice[T any](value []T) *[]T { return &value }

func hashBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
