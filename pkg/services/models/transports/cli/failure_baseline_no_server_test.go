package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Hermetic S02 failure-baseline fixtures for one-shot model invocation when no
// factory API server is reachable on the configured loopback endpoint.

const failureBaselineUnreachableServer = "http://127.0.0.1:1"

func TestFailureBaseline_NoServer_ModelsInvokeJSONUsesBootstrapInsteadOfUnreachableEndpoint(t *testing.T) {
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
		Server:     failureBaselineUnreachableServer,
		FactoryDir: t.TempDir(),
		JSON:       true,
		Output:     &out,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("expected JSON invoke output from bootstrap path")
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
