package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestModelsRemoteGenericInvokeCatalogFailuresAvoidPost(t *testing.T) {
	t.Parallel()
	t.Run("outage", testRemoteGenericCatalogOutage)
	t.Run("cancellation", testRemoteGenericCatalogCancellation)
}

func testRemoteGenericCatalogOutage(t *testing.T) {
	var catalogCalls atomic.Int32
	var postCalls atomic.Int32
	server := httptest.NewServer(remoteCatalogOutageHandler(&catalogCalls, &postCalls))
	defer server.Close()

	var output bytes.Buffer
	err := remoteHTTPService(t, remoteStaticInputReader([]byte("PNG"))).Invoke(remoteInvokeConfig(
		context.Background(), server.URL, []string{"prompt=hello", "image=@fixture.png"}, &output,
	))
	var apiErr *clihttp.APIError
	if err == nil || !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("catalog outage = %v, want typed service-unavailable error", err)
	}
	if catalogCalls.Load() != 1 || postCalls.Load() != 0 || output.Len() != 0 {
		t.Fatalf("catalog outage effects = GET:%d POST:%d output:%q, want 1/0/empty", catalogCalls.Load(), postCalls.Load(), output.String())
	}
}

func remoteCatalogOutageHandler(catalogCalls, postCalls *atomic.Int32) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			catalogCalls.Add(1)
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(writer).Encode(factoryapi.ErrorResponse{
				Code:   factoryapi.ErrorResponseCode("MODEL_BACKEND_NOT_READY"),
				Family: factoryapi.ErrorFamilyInternalServerError, Message: "catalog unavailable",
			})
			return
		}
		if request.Method == http.MethodPost {
			postCalls.Add(1)
		}
		http.NotFound(writer, request)
	}
}

func testRemoteGenericCatalogCancellation(t *testing.T) {
	started := make(chan struct{})
	done := make(chan struct{})
	server := httptest.NewServer(remoteCatalogCancellationHandler(started, done))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output bytes.Buffer
	result := make(chan error, 1)
	go func() {
		result <- remoteHTTPService(t, remoteStaticInputReader([]byte("PNG"))).Invoke(remoteInvokeConfig(
			ctx, server.URL, []string{"prompt=hello", "image=@fixture.png"}, &output,
		))
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("catalog request did not start")
	}
	cancel()
	select {
	case err := <-result:
		if err == nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("catalog cancellation = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("catalog cancellation did not return")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("catalog handler did not observe cancellation")
	}
	if output.Len() != 0 {
		t.Fatalf("catalog cancellation output = %q, want empty", output.String())
	}
}

func remoteCatalogCancellationHandler(started, done chan struct{}) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			http.NotFound(writer, request)
			return
		}
		close(started)
		<-request.Context().Done()
		close(done)
	}
}

func remoteASRModelDetail() factoryapi.ModelDetail {
	required := true
	return factoryapi.ModelDetail{
		Name: "asr",
		Operations: []factoryapi.ModelInvocationOperation{{
			Name: modelinference.OperationASR,
			Inputs: remotePointerSlice([]factoryapi.ModelInvocationSlot{{
				Name: "audio", Modality: remotePointer(factoryapi.ModelInvocationContentTypeAudio), Required: &required,
			}}),
			Outputs: remotePointerSlice([]factoryapi.ModelInvocationSlot{
				{Name: "transcript", Modality: remotePointer(factoryapi.ModelInvocationContentTypeText)},
				{Name: "segments", Modality: remotePointer(factoryapi.ModelInvocationContentTypeJSON)},
			}),
		}},
	}
}

func remoteParameterOutputModelDetail() factoryapi.ModelDetail {
	required := true
	return factoryapi.ModelDetail{
		Name: "llm",
		Operations: []factoryapi.ModelInvocationOperation{{
			Name: modelinference.OperationOMNI,
			Inputs: remotePointerSlice([]factoryapi.ModelInvocationSlot{
				{Name: "prompt", Modality: remotePointer(factoryapi.ModelInvocationContentTypeText), Required: &required, MediaTypes: remotePointerSlice([]string{"text/plain"})},
				{Name: "image", Modality: remotePointer(factoryapi.ModelInvocationContentTypeImage), MediaTypes: remotePointerSlice([]string{"image/*"})},
				{Name: "parameters", Modality: remotePointer(factoryapi.ModelInvocationContentTypeJSON), MediaTypes: remotePointerSlice([]string{"application/json"})},
			}),
			Outputs: remotePointerSlice([]factoryapi.ModelInvocationSlot{
				{Name: "text", Modality: remotePointer(factoryapi.ModelInvocationContentTypeText)},
				{Name: "usage", Modality: remotePointer(factoryapi.ModelInvocationContentTypeJSON)},
			}),
		}},
	}
}
