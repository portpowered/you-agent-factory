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
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func invocationTestCatalog() models.Detail {
	required := true
	return models.Detail{
		Summary: models.Summary{
			Name:             "voice",
			ProviderLocality: models.LocalityLocal,
		},
		Capabilities: []models.Capability{{
			Worker:           "voice-local",
			ProviderLocality: models.LocalityLocal,
			Operations: []models.Operation{{
				Name:   "TTS",
				Inputs: []models.OperationSlot{{Name: "text", Required: &required}},
			}},
		}},
	}
}

func invocationTestLease(scope models.RuntimeScopeRef) models.ModelLease {
	lease, err := (models.ModelLeaseRef{}).Parse("models-lease:http-test")
	if err != nil {
		panic(err)
	}
	return models.ModelLease{
		Lease: lease, Scope: scope, ModelName: "voice", Holder: modelsHTTPInvokeHolder,
		Status: models.ModelLeaseStatusActive,
	}
}

func newInvocationRoot(t *testing.T, invoke func(context.Context, models.InvokeModelRequest) (models.InvokeModelResult, error)) *rootFake {
	t.Helper()
	binding := testRootBinding(&rootFake{})
	root := binding.Models.(*rootFake)
	root.getCatalog = func(_ context.Context, request models.GetModelRequest) (models.GetModelResult, error) {
		if request.Name != "voice" || request.Operation != "TTS" || request.Scope != binding.Scope {
			t.Fatalf("GetCatalogModel request = %#v, want voice/TTS/test scope", request)
		}
		return models.GetModelResult{Model: invocationTestCatalog()}, nil
	}
	root.acquireLease = func(_ context.Context, request models.AcquireModelLeaseRequest) (models.AcquireModelLeaseResult, error) {
		if request.Name != "voice" || request.Holder != modelsHTTPInvokeHolder || request.Scope != binding.Scope {
			t.Fatalf("AcquireModelLease request = %#v, want HTTP holder and test scope", request)
		}
		return models.AcquireModelLeaseResult{Lease: invocationTestLease(binding.Scope)}, nil
	}
	root.invoke = invoke
	return root
}

func TestAdapter_InvokeModelUsesSameModelsRootForCatalogLeaseAndInvocation(t *testing.T) {
	t.Parallel()

	var invoked models.InvokeModelRequest
	root := newInvocationRoot(t, func(_ context.Context, request models.InvokeModelRequest) (models.InvokeModelResult, error) {
		invoked = request
		return models.InvokeModelResult{
			ModelName: "voice", Operation: "TTS", Status: models.ModelInvocationStatusCompleted,
			Content:          []models.InferenceContent{{ContentType: "text/plain", Content: "hello"}},
			LeaseDisposition: models.InvocationLeaseReleased,
		}, nil
	})
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.InvokeModel(
		recorder,
		httptest.NewRequest(http.MethodPost, "/models/voice/invocations", strings.NewReader(`{"operation":"TTS","content":[{"type":"TEXT","text":"hello"}]}`)),
		"voice",
	)

	if invoked.ModelName != "voice" || invoked.Operation != "TTS" || invoked.Input.Content != "hello" {
		t.Fatalf("InvokeModelWithLease request = %#v, want root-scoped text invocation", invoked)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body = %s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ModelInvocationResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ModelName != "voice" || response.Worker != "voice-local" || response.Operation != "TTS" {
		t.Fatalf("response = %#v, want Models-root invocation metadata", response)
	}
}

func TestAdapter_InvokeModelReleasesLeaseWhenRootFailsBeforeDisposition(t *testing.T) {
	t.Parallel()

	released := 0
	root := newInvocationRoot(t, func(context.Context, models.InvokeModelRequest) (models.InvokeModelResult, error) {
		return models.InvokeModelResult{}, models.ErrInferenceFailed
	})
	root.releaseLease = func(_ context.Context, request models.ReleaseModelLeaseRequest) (models.ReleaseModelLeaseResult, error) {
		if request.Scope != testRootBinding(root).Scope || request.Lease.IsZero() {
			t.Fatalf("ReleaseModelLease request = %#v, want issued test lease and scope", request)
		}
		released++
		return models.ReleaseModelLeaseResult{Outcome: models.ModelLeaseReleased}, nil
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.InvokeModel(
		recorder,
		httptest.NewRequest(http.MethodPost, "/models/voice/invocations", strings.NewReader(`{"operation":"TTS","content":[{"type":"TEXT","text":"hello"}]}`)),
		"voice",
	)

	if released != 1 {
		t.Fatalf("ReleaseModelLease calls = %d, want one cleanup release", released)
	}
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestAdapter_InvokeModelRejectsInvalidPayloadBeforeModelsRoot(t *testing.T) {
	t.Parallel()

	root := &rootFake{
		getCatalog: func(context.Context, models.GetModelRequest) (models.GetModelResult, error) {
			t.Fatal("Models root must not be called for invalid request")
			return models.GetModelResult{}, nil
		},
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
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

func TestAdapter_InvokeModelRejectsMissingOperationBeforeModelsRoot(t *testing.T) {
	t.Parallel()

	root := &rootFake{
		getCatalog: func(context.Context, models.GetModelRequest) (models.GetModelResult, error) {
			t.Fatal("Models root must not be called for missing operation")
			return models.GetModelResult{}, nil
		},
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.InvokeModel(
		recorder,
		httptest.NewRequest(http.MethodPost, "/models/voice/invocations", strings.NewReader(`{"content":[{"type":"TEXT","text":"hello"}]}`)),
		"voice",
	)

	assertCatalogHTTPError(t, recorder, http.StatusBadRequest, "BAD_REQUEST", "operation is required")
}

func TestAdapter_InvokeModelServesModelsRootArtifact(t *testing.T) {
	t.Parallel()

	streamFile := filepath.Join(t.TempDir(), "speech.wav")
	if err := os.WriteFile(streamFile, []byte("RIFF"), 0o600); err != nil {
		t.Fatalf("write stream file: %v", err)
	}
	artifact, err := (models.InferenceArtifactRef{}).Parse(streamFile)
	if err != nil {
		t.Fatalf("parse artifact ref: %v", err)
	}
	root := newInvocationRoot(t, func(_ context.Context, request models.InvokeModelRequest) (models.InvokeModelResult, error) {
		if request.ResponseMode != models.ResponseModeAudioStream {
			t.Fatalf("ResponseMode = %q, want AUDIO_STREAM", request.ResponseMode)
		}
		return models.InvokeModelResult{
			ModelName: "voice", Operation: "TTS", Status: models.ModelInvocationStatusCompleted,
			Artifacts:        []models.InferenceArtifact{{Artifact: artifact, MediaType: "audio/wav"}},
			LeaseDisposition: models.InvocationLeaseReleased,
		}, nil
	})
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	recorder := httptest.NewRecorder()
	body := `{"operation":"TTS","content":[{"type":"TEXT","text":"hello"}],"options":{"responseMode":"AUDIO_STREAM"}}`
	handler.InvokeModel(recorder, httptest.NewRequest(http.MethodPost, "/models/voice/invocations", strings.NewReader(body)), "voice")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "audio/wav" {
		t.Fatalf("content-type = %q, want audio/wav", got)
	}
	if body := recorder.Body.String(); body != "RIFF" {
		t.Fatalf("body = %q, want streamed artifact contents", body)
	}
}
