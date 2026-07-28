package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	modelcontract "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

type modelAPIFake struct {
	modelcontract.Service
	list        func(context.Context) (modelcontract.List, error)
	get         func(context.Context, string) (modelcontract.Detail, error)
	pull        func(context.Context, string) (modelcontract.PullResult, error)
	listCatalog func(
		context.Context,
		modelcontract.ListModelsRequest,
	) (modelcontract.ListModelsResult, error)
	getCatalog func(
		context.Context,
		modelcontract.GetModelRequest,
	) (modelcontract.GetModelResult, error)
	pullForScope func(
		context.Context,
		modelcontract.PullModelRequest,
	) (modelcontract.PullResult, error)
}

func (fake modelAPIFake) ListModels(ctx context.Context) (modelcontract.List, error) {
	return fake.list(ctx)
}

func (fake modelAPIFake) GetModel(ctx context.Context, name string) (modelcontract.Detail, error) {
	return fake.get(ctx, name)
}

func (fake modelAPIFake) PullModel(ctx context.Context, name string) (modelcontract.PullResult, error) {
	return fake.pull(ctx, name)
}

func (fake modelAPIFake) ListCatalog(
	ctx context.Context,
	request modelcontract.ListModelsRequest,
) (modelcontract.ListModelsResult, error) {
	return fake.listCatalog(ctx, request)
}

func (fake modelAPIFake) GetCatalogModel(
	ctx context.Context,
	request modelcontract.GetModelRequest,
) (modelcontract.GetModelResult, error) {
	return fake.getCatalog(ctx, request)
}

func (fake modelAPIFake) PullModelForScope(
	ctx context.Context,
	request modelcontract.PullModelRequest,
) (modelcontract.PullResult, error) {
	return fake.pullForScope(ctx, request)
}

type modelInvokerFake struct {
	invoke func(context.Context, string, modelcontract.Request) (modelcontract.Result, error)
}

func (fake modelInvokerFake) InvokeModel(ctx context.Context, name string, request modelcontract.Request) (modelcontract.Result, error) {
	return fake.invoke(ctx, name, request)
}

func newTestHandler(service modelcontract.Service, invoker workers.ModelInvoker) *Handler {
	return NewHandler(NewAdapter(service, invoker, passthroughContentPreparation{}), zap.NewNop())
}

type passthroughContentPreparation struct{}

func (passthroughContentPreparation) PrepareWorkContent(_ context.Context, content []work.WorkContentPart) ([]work.WorkContentPart, error) {
	return content, nil
}

func TestNewHandlerRequiresInjectedAdapter(t *testing.T) {
	if handler := NewHandler(nil, zap.NewNop()); handler != nil {
		t.Fatalf("NewHandler(nil) = %T, want nil", handler)
	}
	if handler := NewHandler(NewAdapter(modelAPIFake{}, modelInvokerFake{}, passthroughContentPreparation{}), nil); handler != nil {
		t.Fatalf("NewHandler(adapter, nil) = %T, want nil", handler)
	}
	if handler := newTestHandler(modelAPIFake{}, modelInvokerFake{}); handler == nil || handler.adapter == nil || handler.logger == nil {
		t.Fatalf("NewHandler(adapter) = %#v, want injected adapter", handler)
	}
}

func TestHandlerListModelsInvokesInjectedAPI(t *testing.T) {
	models := modelAPIFake{
		list: func(context.Context) (modelcontract.List, error) {
			return modelcontract.List{Results: []modelcontract.Summary{{Name: "voice"}}}, nil
		},
	}
	handler := newTestHandler(models, modelInvokerFake{})
	recorder := httptest.NewRecorder()

	handler.ListModels(recorder, httptest.NewRequest(http.MethodGet, "/models", nil))

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"name":"voice"`) {
		t.Fatalf("response = %d %s, want model list", recorder.Code, recorder.Body.String())
	}
}

func TestAdapterUsesOpenedModelsScopeForCatalogReads(t *testing.T) {
	scope, err := (modelcontract.RuntimeScopeRef{}).Parse("factory-session:http-scope")
	if err != nil {
		t.Fatalf("parse Models scope: %v", err)
	}
	service := modelAPIFake{
		listCatalog: func(
			_ context.Context,
			request modelcontract.ListModelsRequest,
		) (modelcontract.ListModelsResult, error) {
			if request.Scope != scope {
				t.Fatalf("ListCatalog scope = %q, want %q", request.Scope, scope)
			}
			return modelcontract.ListModelsResult{
				Models: []modelcontract.Summary{{Name: "voice"}},
			}, nil
		},
		getCatalog: func(
			_ context.Context,
			request modelcontract.GetModelRequest,
		) (modelcontract.GetModelResult, error) {
			if request.Scope != scope || request.Name != "voice" {
				t.Fatalf("GetCatalogModel request = %#v, want opened scope and voice", request)
			}
			return modelcontract.GetModelResult{
				Model: modelcontract.Detail{
					Summary: modelcontract.Summary{Name: "voice"},
				},
			}, nil
		},
		pullForScope: func(
			_ context.Context,
			request modelcontract.PullModelRequest,
		) (modelcontract.PullResult, error) {
			if request.Scope != scope || request.Name != "voice" {
				t.Fatalf("PullModelForScope request = %#v, want opened scope and voice", request)
			}
			return modelcontract.PullResult{ModelName: "voice", Outcome: "PULLED"}, nil
		},
	}
	adapter := NewAdapter(service, modelInvokerFake{}, passthroughContentPreparation{}, scope)

	listed, err := adapter.ListModels(t.Context())
	if err != nil || len(listed.Results) != 1 || listed.Results[0].Name != "voice" {
		t.Fatalf("ListModels() = (%#v, %v), want scoped voice model", listed, err)
	}
	detail, err := adapter.GetModel(t.Context(), "voice")
	if err != nil || detail.Name != "voice" {
		t.Fatalf("GetModel() = (%#v, %v), want scoped voice detail", detail, err)
	}
	pulled, err := adapter.PullModel(t.Context(), "voice")
	if err != nil || pulled.ModelName != "voice" || pulled.Outcome != "PULLED" {
		t.Fatalf("PullModel() = (%#v, %v), want scoped voice pull", pulled, err)
	}
}

func TestHandlerInvokeModelOwnsRequestValidation(t *testing.T) {
	invoker := modelInvokerFake{
		invoke: func(context.Context, string, modelcontract.Request) (modelcontract.Result, error) {
			t.Fatal("InvokeModel called for invalid request")
			return modelcontract.Result{}, nil
		},
	}
	handler := newTestHandler(modelAPIFake{}, invoker)
	recorder := httptest.NewRecorder()

	handler.InvokeModel(recorder, httptest.NewRequest(http.MethodPost, "/models/voice/invocations", strings.NewReader(`{"operation":"TTS","content":{}}`)), "voice")

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "content must be an array") {
		t.Fatalf("response = %d %s, want content validation error", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerInvokeModelPreservesContentPartValidation(t *testing.T) {
	invoker := modelInvokerFake{
		invoke: func(context.Context, string, modelcontract.Request) (modelcontract.Result, error) {
			t.Fatal("InvokeModel called for invalid content part")
			return modelcontract.Result{}, nil
		},
	}
	handler := newTestHandler(modelAPIFake{}, invoker)
	recorder := httptest.NewRecorder()

	handler.InvokeModel(recorder, httptest.NewRequest(http.MethodPost, "/models/voice/invocations", strings.NewReader(`{"operation":"TTS","content":[{"type":"TEXT","url":"unexpected"}]}`)), "voice")

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "content[0].url is not supported") {
		t.Fatalf("response = %d %s, want discriminated content validation error", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerPullModelOwnsErrorMapping(t *testing.T) {
	models := modelAPIFake{
		pull: func(context.Context, string) (modelcontract.PullResult, error) {
			return modelcontract.PullResult{}, errors.New("cache unavailable")
		},
	}
	handler := newTestHandler(models, modelInvokerFake{})
	recorder := httptest.NewRecorder()

	handler.PullModel(recorder, httptest.NewRequest(http.MethodPost, "/models/voice/pull", nil), "voice")

	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "failed to pull model") {
		t.Fatalf("response = %d %s, want mapped internal error", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "cache unavailable") {
		t.Fatalf("response leaks raw internal error: %s", recorder.Body.String())
	}
}
