package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

type invokeInvokerFake struct {
	invoke func(context.Context, string, models.Request) (models.Result, error)
}

func (fake invokeInvokerFake) InvokeModel(
	ctx context.Context,
	name string,
	request models.Request,
) (models.Result, error) {
	return fake.invoke(ctx, name, request)
}

func TestAdapter_InvokeModelDecodesBodyAndInvokesInvokerWithMappedRequest(t *testing.T) {
	t.Parallel()

	var invokedName string
	var invokedRequest models.Request
	invoker := invokeInvokerFake{
		invoke: func(_ context.Context, name string, request models.Request) (models.Result, error) {
			invokedName = name
			invokedRequest = request
			return models.Result{
				ModelName: name,
				Worker:    "voice-local",
				Operation: request.Operation,
			}, nil
		},
	}
	handler := NewHandlerFromRoot(RootBinding{
		Models:  &rootFake{},
		Invoker: invoker,
	}, zap.NewNop())
	recorder := httptest.NewRecorder()
	body := `{"operation":"TTS","content":[{"type":"TEXT","text":"hello"}]}`

	handler.InvokeModel(
		recorder,
		httptest.NewRequest(http.MethodPost, "/models/voice/invocations", strings.NewReader(body)),
		"voice",
	)

	if invokedName != "voice" {
		t.Fatalf("InvokeModel invoked with name = %q, want voice", invokedName)
	}
	if invokedRequest.Operation != "TTS" || len(invokedRequest.Content) != 1 {
		t.Fatalf("InvokeModel request = %#v, want decoded TTS invocation", invokedRequest)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body = %s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ModelInvocationResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ModelName != "voice" || response.Worker != "voice-local" || response.Operation != "TTS" {
		t.Fatalf("response = %#v, want encoded invoke success", response)
	}
}

func TestAdapter_InvokeModelRejectsInvalidPayloadBeforeInvoker(t *testing.T) {
	t.Parallel()

	invoker := invokeInvokerFake{
		invoke: func(context.Context, string, models.Request) (models.Result, error) {
			t.Fatal("InvokeModel must not be called for invalid request")
			return models.Result{}, nil
		},
	}
	handler := NewHandlerFromRoot(RootBinding{
		Models:  &rootFake{},
		Invoker: invoker,
	}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.InvokeModel(
		recorder,
		httptest.NewRequest(http.MethodPost, "/models/voice/invocations", strings.NewReader(`{"operation":"TTS","content":{}}`)),
		"voice",
	)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "content must be an array") {
		t.Fatalf("response = %d %s, want content validation error", recorder.Code, recorder.Body.String())
	}
}

func TestAdapter_InvokeModelRejectsMissingOperationBeforeInvoker(t *testing.T) {
	t.Parallel()

	invoker := invokeInvokerFake{
		invoke: func(context.Context, string, models.Request) (models.Result, error) {
			t.Fatal("InvokeModel must not be called for missing operation")
			return models.Result{}, nil
		},
	}
	handler := NewHandlerFromRoot(RootBinding{
		Models:  &rootFake{},
		Invoker: invoker,
	}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.InvokeModel(
		recorder,
		httptest.NewRequest(http.MethodPost, "/models/voice/invocations", strings.NewReader(`{"content":[{"type":"TEXT","text":"hello"}]}`)),
		"voice",
	)

	assertCatalogHTTPError(t, recorder, http.StatusBadRequest, "BAD_REQUEST", "operation is required")
}

func TestAdapter_InvokeModelServesStreamFileResponse(t *testing.T) {
	t.Parallel()

	streamFile := filepath.Join(t.TempDir(), "speech.wav")
	if err := os.WriteFile(streamFile, []byte("RIFF"), 0o600); err != nil {
		t.Fatalf("write stream file: %v", err)
	}
	invoker := invokeInvokerFake{
		invoke: func(_ context.Context, name string, request models.Request) (models.Result, error) {
			return models.Result{
				ModelName:         name,
				Operation:         request.Operation,
				StreamFile:        streamFile,
				StreamContentType: "audio/wav",
			}, nil
		},
	}
	handler := NewHandlerFromRoot(RootBinding{
		Models:  &rootFake{},
		Invoker: invoker,
	}, zap.NewNop())
	recorder := httptest.NewRecorder()
	body := `{"operation":"TTS","content":[{"type":"TEXT","text":"hello"}]}`

	handler.InvokeModel(
		recorder,
		httptest.NewRequest(http.MethodPost, "/models/voice/invocations", strings.NewReader(body)),
		"voice",
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "audio/wav" {
		t.Fatalf("content-type = %q, want audio/wav", got)
	}
	if body := recorder.Body.String(); body != "RIFF" {
		t.Fatalf("body = %q, want streamed file contents", body)
	}
}

var _ workers.ModelInvoker = invokeInvokerFake{}
