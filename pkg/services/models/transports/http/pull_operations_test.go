package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestAdapter_PullModelInvokesFakeRootWithDecodedName(t *testing.T) {
	t.Parallel()

	var invokedName string
	root := &rootFake{
		pull: func(_ context.Context, name string) (models.PullResult, error) {
			invokedName = name
			return models.PullResult{
				ModelName: name,
				Outcome:   "PULLED",
			}, nil
		},
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.PullModel(recorder, httptest.NewRequest(http.MethodPost, "/models/voice/pull", nil), "voice")

	if invokedName != "voice" {
		t.Fatalf("PullModel invoked root with name = %q, want voice", invokedName)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var response factoryapi.ModelPullResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ModelName != "voice" || response.Outcome != factoryapi.ModelPullOutcomePULLED {
		t.Fatalf("response = %#v, want encoded voice pull success", response)
	}
}

func TestAdapter_PullModelRejectsEmptyNameBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	root := &rootFake{
		pull: func(context.Context, string) (models.PullResult, error) {
			t.Fatal("fake root must not be invoked for empty model name")
			return models.PullResult{}, nil
		},
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.PullModel(recorder, httptest.NewRequest(http.MethodPost, "/models//pull", nil), "   ")

	assertCatalogHTTPError(t, recorder, http.StatusNotFound, "NOT_FOUND", "model not found")
}

func TestAdapter_PullModelEncodesPullErrorOutcome(t *testing.T) {
	t.Parallel()

	failure := models.PullResult{
		ModelName:          "voice",
		ManagedPullOutcome: "TIMED_OUT",
		ReadinessState:     "FAILED",
	}
	root := &rootFake{
		pull: func(context.Context, string) (models.PullResult, error) {
			return models.PullResult{}, &models.PullError{Result: failure, Cause: errors.New("deadline exceeded")}
		},
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.PullModel(recorder, httptest.NewRequest(http.MethodPost, "/models/voice/pull", nil), "voice")

	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 body = %s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ModelPullResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeTIMEDOUT {
		t.Fatalf("pull outcome = %q, want TIMED_OUT", response.ManagedRuntimePull.PullOutcome)
	}
}

func TestAdapter_ScopedPullDecodesIntoRootRequest(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:http-scope")
	if err != nil {
		t.Fatalf("parse Models scope: %v", err)
	}
	var pullRequest models.PullModelRequest
	root := &rootFake{
		pullForScope: func(_ context.Context, request models.PullModelRequest) (models.PullResult, error) {
			pullRequest = request
			return models.PullResult{ModelName: request.Name, Outcome: "PULLED"}, nil
		},
	}
	handler := NewHandlerFromRoot(RootBinding{Models: root, Scope: scope}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.PullModel(recorder, httptest.NewRequest(http.MethodPost, "/models/voice/pull", nil), "voice")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if pullRequest.Scope != scope || pullRequest.Name != "voice" {
		t.Fatalf("PullModelForScope request = %#v, want scoped voice", pullRequest)
	}
}
