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
	if fake.listCatalog == nil && fake.list != nil {
		listed, err := fake.list(ctx)
		return modelcontract.ListModelsResult{Models: listed.Results}, err
	}
	return fake.listCatalog(ctx, request)
}

func (fake modelAPIFake) GetCatalogModel(
	ctx context.Context,
	request modelcontract.GetModelRequest,
) (modelcontract.GetModelResult, error) {
	if fake.getCatalog == nil && fake.get != nil {
		detail, err := fake.get(ctx, request.Name)
		return modelcontract.GetModelResult{Model: detail}, err
	}
	return fake.getCatalog(ctx, request)
}

func (fake modelAPIFake) PullModelForScope(
	ctx context.Context,
	request modelcontract.PullModelRequest,
) (modelcontract.PullResult, error) {
	if fake.pullForScope == nil && fake.pull != nil {
		return fake.pull(ctx, request.Name)
	}
	return fake.pullForScope(ctx, request)
}

type modelInvokerFake struct {
	invoke func(context.Context, string, modelcontract.Request) (modelcontract.Result, error)
}

func (fake modelInvokerFake) InvokeModel(ctx context.Context, name string, request modelcontract.Request) (modelcontract.Result, error) {
	return fake.invoke(ctx, name, request)
}

func newTestHandler(service modelcontract.Service, invoker workers.ModelInvoker) *Handler {
	scope, err := (modelcontract.RuntimeScopeRef{}).Parse("factory-session:http-test")
	if err != nil {
		panic(err)
	}
	return NewHandler(NewAdapter(service, invoker, passthroughContentPreparation{}, scope), zap.NewNop())
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

	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "cache unavailable") {
		t.Fatalf("response = %d %s, want mapped internal error", recorder.Code, recorder.Body.String())
	}
}

// TestModelsHTTPCharacterizationSuccessResponses pins the complete JSON bodies
// emitted by the public Models handler. The collaborators are deterministic so
// this baseline observes the HTTP representation without starting a local
// model host, downloading assets, or invoking an external executable.
func TestModelsHTTPCharacterizationSuccessResponses(t *testing.T) {
	t.Parallel()

	t.Run("GET /models", func(t *testing.T) {
		t.Parallel()
		root := &rootFake{
			listCatalog: func(context.Context, modelcontract.ListModelsRequest) (modelcontract.ListModelsResult, error) {
				return modelcontract.ListModelsResult{
					Models: []modelcontract.Summary{characterizationHTTPModelSummary()},
				}, nil
			},
		}
		handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
		recorder := httptest.NewRecorder()

		handler.ListModels(recorder, httptest.NewRequest(http.MethodGet, "/models", nil))

		assertModelsHTTPCharacterizationJSON(t, recorder, http.StatusOK, characterizationHTTPListBody)
	})

	t.Run("GET /models/OMNIVOICE_Q4_K_M", func(t *testing.T) {
		t.Parallel()
		root := &rootFake{
			getCatalog: func(context.Context, modelcontract.GetModelRequest) (modelcontract.GetModelResult, error) {
				return modelcontract.GetModelResult{Model: characterizationHTTPModelDetail()}, nil
			},
		}
		handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
		recorder := httptest.NewRecorder()

		handler.GetModel(
			recorder,
			httptest.NewRequest(http.MethodGet, "/models/OMNIVOICE_Q4_K_M", nil),
			"OMNIVOICE_Q4_K_M",
		)

		assertModelsHTTPCharacterizationJSON(t, recorder, http.StatusOK, characterizationHTTPDetailBody)
	})

	t.Run("POST /models/OMNIVOICE_Q4_K_M/invocations", func(t *testing.T) {
		t.Parallel()
		invoker := modelInvokerFake{
			invoke: func(_ context.Context, name string, request modelcontract.Request) (modelcontract.Result, error) {
				if name != "OMNIVOICE_Q4_K_M" || request.Operation != "TTS" {
					t.Fatalf("invoke request = (%q, %#v), want OMNIVOICE_Q4_K_M/TTS", name, request)
				}
				return modelcontract.Result{
					ModelName:        name,
					Worker:           "tts-executor",
					Operation:        request.Operation,
					ProviderLocality: string(modelcontract.LocalityLocal),
					Content: []work.WorkContentPart{{
						Type:        work.WorkContentPartTypeAudio,
						File:        "artifacts/output.wav",
						ContentType: "audio/wav",
					}},
					Bindings: []modelcontract.ResolvedModelOperationBinding{{
						Slot:   "text",
						Source: "INPUT",
						Content: []work.WorkContentPart{{
							Type: work.WorkContentPartTypeText,
							Text: "hello world",
						}},
					}},
				}, nil
			},
		}
		handler := NewHandlerFromRoot(RootBinding{Models: &rootFake{}, Invoker: invoker}, zap.NewNop())
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(
			http.MethodPost,
			"/models/OMNIVOICE_Q4_K_M/invocations",
			strings.NewReader(`{"operation":"TTS","content":[{"type":"TEXT","text":"hello world"}]}`),
		)

		handler.InvokeModel(recorder, request, "OMNIVOICE_Q4_K_M")

		assertModelsHTTPCharacterizationJSON(t, recorder, http.StatusOK, characterizationHTTPInvocationBody)
	})

	t.Run("POST /models/OMNIVOICE_Q4_K_M/pull", func(t *testing.T) {
		t.Parallel()
		root := &rootFake{
			pullForScope: func(_ context.Context, request modelcontract.PullModelRequest) (modelcontract.PullResult, error) {
				return modelcontract.PullResult{
					ModelName: request.Name, ProviderLocality: string(modelcontract.LocalityLocal), Outcome: "PULLED",
					CachePath: "/models/OMNIVOICE_Q4_K_M/rev-2026", Revision: "rev-2026",
					ManagedPullOutcome: "INSTALLED_SUCCESSFULLY", ReadinessState: "READY",
					DownloadedFiles: []modelcontract.DownloadedFile{
						{Path: "weights.gguf", Bytes: 42, SHA256: "abc123"},
						{Path: "config.json", Bytes: 7},
					},
				}, nil
			},
		}
		handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
		recorder := httptest.NewRecorder()

		handler.PullModel(
			recorder,
			httptest.NewRequest(http.MethodPost, "/models/OMNIVOICE_Q4_K_M/pull", nil),
			"OMNIVOICE_Q4_K_M",
		)

		assertModelsHTTPCharacterizationJSON(t, recorder, http.StatusOK, characterizationHTTPPullBody)
	})
}

func TestModelsHTTPCharacterizationUnknownModelErrors(t *testing.T) {
	t.Parallel()

	t.Run("catalog detail", func(t *testing.T) {
		t.Parallel()
		root := &rootFake{
			getCatalog: func(context.Context, modelcontract.GetModelRequest) (modelcontract.GetModelResult, error) {
				return modelcontract.GetModelResult{}, modelcontract.ErrNotFound
			},
		}
		handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
		recorder := httptest.NewRecorder()

		handler.GetModel(recorder, httptest.NewRequest(http.MethodGet, "/models/MISSING", nil), "MISSING")

		assertModelsHTTPCharacterizationJSON(t, recorder, http.StatusNotFound, characterizationHTTPNotFoundBody)
	})

	t.Run("pull", func(t *testing.T) {
		t.Parallel()
		root := &rootFake{
			pullForScope: func(context.Context, modelcontract.PullModelRequest) (modelcontract.PullResult, error) {
				return modelcontract.PullResult{}, modelcontract.ErrNotFound
			},
		}
		handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
		recorder := httptest.NewRecorder()

		handler.PullModel(recorder, httptest.NewRequest(http.MethodPost, "/models/MISSING/pull", nil), "MISSING")

		assertModelsHTTPCharacterizationJSON(t, recorder, http.StatusNotFound, characterizationHTTPNotFoundBody)
	})

	t.Run("invocation", func(t *testing.T) {
		t.Parallel()
		invoker := modelInvokerFake{
			invoke: func(context.Context, string, modelcontract.Request) (modelcontract.Result, error) {
				// Characterized, not endorsed: an unknown direct-invocation model
				// currently reaches the runtime classifier as a generic failure,
				// unlike catalog and pull lookups that preserve NOT_FOUND.
				return modelcontract.Result{}, &modelcontract.InferenceFailure{
					Class:     modelcontract.InferenceFailureClassRuntimeFailure,
					Message:   `inference failed for model "MISSING" operation "TTS": model not found: MISSING`,
					ModelName: "MISSING",
					Operation: "TTS",
				}
			},
		}
		handler := NewHandlerFromRoot(RootBinding{Models: &rootFake{}, Invoker: invoker}, zap.NewNop())
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(
			http.MethodPost,
			"/models/MISSING/invocations",
			strings.NewReader(`{"operation":"TTS","content":[{"type":"TEXT","text":"hello world"}]}`),
		)

		handler.InvokeModel(recorder, request, "MISSING")

		assertModelsHTTPCharacterizationJSON(t, recorder, http.StatusInternalServerError, characterizationHTTPInvocationNotFoundBody)
	})
}

func assertModelsHTTPCharacterizationJSON(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	wantStatus int,
	wantBody string,
) {
	t.Helper()
	if recorder.Code != wantStatus {
		t.Fatalf("status = %d, want %d body = %q", recorder.Code, wantStatus, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	wantBody = strings.ReplaceAll(wantBody, `\n`, "\n")
	if got := recorder.Body.String(); got != wantBody {
		t.Fatalf("body = %q, want exact %q", got, wantBody)
	}
}

func characterizationHTTPModelSummary() modelcontract.Summary {
	return modelcontract.Summary{
		Name: "OMNIVOICE_Q4_K_M", ProviderLocality: modelcontract.LocalityLocal,
		Status: modelcontract.StatusReady, LoadState: modelcontract.LoadStateUnloaded,
		Operations: []modelcontract.Operation{characterizationHTTPTTSOperation()},
		Modalities: []string{"TEXT", "AUDIO"},
		Resources: []modelcontract.ResourceSummary{{
			Name: "omnivoice-cache", Type: "MODEL", Capacity: 1,
			Model: characterizationHTTPString("OMNIVOICE_Q4_K_M"), Backend: characterizationHTTPString("LLAMACPP"),
			LoadPolicy: characterizationHTTPString("ON_DEMAND"),
		}},
		ManagedRuntime: modelcontract.Runtime{
			Identity: "OMNIVOICE_Q4_K_M", ReadinessState: modelcontract.ReadinessStateReady,
			LifecycleState: modelcontract.LifecycleStateInstalled, Locality: modelcontract.LocalityLocal,
			SupportedOperations: []modelcontract.Operation{characterizationHTTPTTSOperation()},
			Diagnostics:         map[string]string{"cache": "omnivoice-cache"},
		},
	}
}

func characterizationHTTPModelDetail() modelcontract.Detail {
	summary := characterizationHTTPModelSummary()
	return modelcontract.Detail{
		Summary: summary,
		Capabilities: []modelcontract.Capability{{
			Worker: "tts-executor", ProviderLocality: modelcontract.LocalityLocal,
			ModelProvider: characterizationHTTPString("CODEX"), Operations: []modelcontract.Operation{characterizationHTTPTTSOperation()},
			ResourceNames: []string{"omnivoice-cache"},
		}},
		Diagnostics: map[string]string{"statusReason": "managed runtime is discoverable"},
	}
}

func characterizationHTTPTTSOperation() modelcontract.Operation {
	required := true
	return modelcontract.Operation{
		Name: "TTS",
		Inputs: []modelcontract.OperationSlot{{
			Name: "text", ContentTypes: []string{"TEXT"}, Required: &required,
		}},
		Outputs: []modelcontract.OperationSlot{{
			Name: "audio", ContentTypes: []string{"AUDIO"},
		}},
	}
}

func characterizationHTTPString(value string) *string {
	return &value
}

const (
	characterizationHTTPListBody               = `{"results":[{"loadState":"UNLOADED","managedRuntime":{"diagnostics":{"cache":"omnivoice-cache"},"identity":"OMNIVOICE_Q4_K_M","lifecycleState":"INSTALLED","locality":"LOCAL","readinessState":"READY","supportedOperations":[{"inputs":[{"contentTypes":["TEXT"],"name":"text","required":true}],"name":"TTS","outputs":[{"contentTypes":["AUDIO"],"name":"audio"}]}]},"modalities":["TEXT","AUDIO"],"name":"OMNIVOICE_Q4_K_M","operations":[{"inputs":[{"contentTypes":["TEXT"],"name":"text","required":true}],"name":"TTS","outputs":[{"contentTypes":["AUDIO"],"name":"audio"}]}],"providerLocality":"LOCAL","resources":[{"backend":"LLAMACPP","capacity":1,"loadPolicy":"ON_DEMAND","model":"OMNIVOICE_Q4_K_M","name":"omnivoice-cache","type":"MODEL"}],"status":"READY"}]}\n`
	characterizationHTTPDetailBody             = `{"capabilities":[{"modelProvider":"CODEX","operations":[{"inputs":[{"contentTypes":["TEXT"],"name":"text","required":true}],"name":"TTS","outputs":[{"contentTypes":["AUDIO"],"name":"audio"}]}],"providerLocality":"LOCAL","resourceNames":["omnivoice-cache"],"worker":"tts-executor"}],"diagnostics":{"statusReason":"managed runtime is discoverable"},"loadState":"UNLOADED","managedRuntime":{"diagnostics":{"cache":"omnivoice-cache"},"identity":"OMNIVOICE_Q4_K_M","lifecycleState":"INSTALLED","locality":"LOCAL","readinessState":"READY","supportedOperations":[{"inputs":[{"contentTypes":["TEXT"],"name":"text","required":true}],"name":"TTS","outputs":[{"contentTypes":["AUDIO"],"name":"audio"}]}]},"modalities":["TEXT","AUDIO"],"name":"OMNIVOICE_Q4_K_M","operations":[{"inputs":[{"contentTypes":["TEXT"],"name":"text","required":true}],"name":"TTS","outputs":[{"contentTypes":["AUDIO"],"name":"audio"}]}],"providerLocality":"LOCAL","resources":[{"backend":"LLAMACPP","capacity":1,"loadPolicy":"ON_DEMAND","model":"OMNIVOICE_Q4_K_M","name":"omnivoice-cache","type":"MODEL"}],"status":"READY"}\n`
	characterizationHTTPInvocationBody         = `{"bindings":[{"content":[{"text":"hello world","type":"text"}],"slot":"text","source":"INPUT"}],"content":[{"contentType":"audio/wav","file":"artifacts/output.wav","type":"AUDIO","url":""}],"modelName":"OMNIVOICE_Q4_K_M","operation":"TTS","providerLocality":"LOCAL","worker":"tts-executor"}\n`
	characterizationHTTPPullBody               = `{"cachePath":"/models/OMNIVOICE_Q4_K_M/rev-2026","downloadedFiles":[{"bytes":42,"path":"weights.gguf","sha256":"abc123"},{"bytes":7,"path":"config.json"}],"managedRuntimePull":{"cachePath":"/models/OMNIVOICE_Q4_K_M/rev-2026","downloadedFiles":[{"bytes":42,"path":"weights.gguf","sha256":"abc123"},{"bytes":7,"path":"config.json"}],"identity":"OMNIVOICE_Q4_K_M","pullOutcome":"INSTALLED_SUCCESSFULLY","readinessState":"READY","revision":"rev-2026"},"modelName":"OMNIVOICE_Q4_K_M","outcome":"PULLED","providerLocality":"LOCAL","revision":"rev-2026"}\n`
	characterizationHTTPNotFoundBody           = `{"code":"NOT_FOUND","family":"NOT_FOUND","message":"model not found"}\n`
	characterizationHTTPInvocationNotFoundBody = `{"code":"MODEL_INFERENCE_RUNTIME_FAILURE","family":"INTERNAL_SERVER_ERROR","message":"inference failed for model \"MISSING\" operation \"TTS\": model not found: MISSING"}\n`
)
