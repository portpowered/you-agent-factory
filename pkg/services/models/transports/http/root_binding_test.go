package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"go.uber.org/zap"
)

func TestHandlerFromRoot_ListModelsInvokesModelsRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	root := &rootFake{
		list: func(context.Context) (models.List, error) {
			invoked = true
			return models.List{Results: []models.Summary{{Name: "voice"}}}, nil
		},
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.ListModels(recorder, httptest.NewRequest(http.MethodGet, "/models", nil))

	if !invoked {
		t.Fatal("ListModels did not invoke the injected Models root")
	}
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"name":"voice"`) {
		t.Fatalf("response = %d %s, want encoded model list from Models root", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerFromRoot_ListModelsRequiresInjectedRoot(t *testing.T) {
	t.Parallel()

	handler := NewHandlerFromRoot(RootBinding{}, zap.NewNop())
	if handler != nil {
		t.Fatalf("NewHandlerFromRoot without Models root = %#v, want nil", handler)
	}
}

func TestNewHandlerFromRoot_ExposesInjectedModelsRoot(t *testing.T) {
	t.Parallel()

	root := &rootFake{}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	if handler == nil || handler.adapter == nil {
		t.Fatal("NewHandlerFromRoot returned nil handler or adapter")
	}
	if handler.adapter.Root() != root {
		t.Fatal("adapter must expose the injected Models root")
	}
}

func TestNewAdapterFromRoot_RejectsNilModelsRoot(t *testing.T) {
	t.Parallel()

	if adapter := NewAdapter(nil, noopModelInvoker{}, noopContentPreparation{}); adapter != nil {
		t.Fatalf("NewAdapter(nil) = %#v, want nil", adapter)
	}
	if handler := NewHandlerFromRoot(RootBinding{}, zap.NewNop()); handler != nil {
		t.Fatalf("NewHandlerFromRoot without Models root = %#v, want nil", handler)
	}
}
