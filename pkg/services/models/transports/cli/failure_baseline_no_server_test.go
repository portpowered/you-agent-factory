package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
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
