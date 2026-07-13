package models

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// Hermetic S02 failure-baseline fixtures for one-shot model invocation when no
// factory API server is reachable on the configured loopback endpoint.

const failureBaselineUnreachableServer = "http://127.0.0.1:1"

func TestFailureBaseline_NoServer_ModelsInvokeJSONUsesBootstrapInsteadOfUnreachableEndpoint(t *testing.T) {
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
					Worker:    "tts-worker",
					Operation: request.Operation,
				}, nil
			},
		}, nil
	}

	var out bytes.Buffer
	if err := Invoke(InvokeConfig{
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
	err := List(ListConfig{
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
	err := Inspect(InspectConfig{
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
