package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
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
	readiness func(
		context.Context,
		modelcontract.GetModelReadinessRequest,
	) (modelcontract.GetModelReadinessResult, error)
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

func (fake modelAPIFake) GetModelReadiness(
	ctx context.Context,
	request modelcontract.GetModelReadinessRequest,
) (modelcontract.GetModelReadinessResult, error) {
	if fake.readiness != nil {
		return fake.readiness(ctx, request)
	}
	if fake.getCatalog != nil {
		model, err := fake.getCatalog(ctx, modelcontract.GetModelRequest{
			Scope: request.Scope, Name: request.Name, Operation: request.Operation,
		})
		if err != nil {
			return modelcontract.GetModelReadinessResult{}, err
		}
		return modelcontract.GetModelReadinessResult{
			ModelName: model.Model.Name, Readiness: model.Model.ManagedRuntime,
		}, nil
	}
	if fake.get != nil {
		model, err := fake.get(ctx, request.Name)
		if err != nil {
			return modelcontract.GetModelReadinessResult{}, err
		}
		return modelcontract.GetModelReadinessResult{
			ModelName: model.Name, Readiness: model.ManagedRuntime,
		}, nil
	}
	return modelcontract.GetModelReadinessResult{}, modelcontract.ErrUnsupportedOperation
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

// TestModelsHTTPCharacterizationListResponse pins the complete JSON body
// emitted by GET /models at the public Models handler boundary.
func TestModelsHTTPCharacterizationListResponse(t *testing.T) {
	t.Parallel()
	root := &rootFake{
		listCatalog: func(context.Context, modelcontract.ListModelsRequest) (modelcontract.ListModelsResult, error) {
			return modelcontract.ListModelsResult{Models: []modelcontract.Summary{characterizationHTTPModelSummary()}}, nil
		},
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.ListModels(recorder, httptest.NewRequest(http.MethodGet, "/models", nil))
	assertModelsHTTPCharacterizationJSON(t, recorder, http.StatusOK, characterizationHTTPListBody)
}

func TestModelsHTTPCharacterizationDetailResponse(t *testing.T) {
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
}

func TestModelsHTTPCharacterizationInvocationResponse(t *testing.T) {
	t.Parallel()
	invoker := modelInvokerFake{
		invoke: func(_ context.Context, name string, request modelcontract.Request) (modelcontract.Result, error) {
			if name != "OMNIVOICE_Q4_K_M" || request.Operation != "TTS" {
				t.Fatalf("invoke request = (%q, %#v), want OMNIVOICE_Q4_K_M/TTS", name, request)
			}
			return modelcontract.Result{
				ModelName: name, Worker: "tts-executor", Operation: request.Operation,
				ProviderLocality: string(modelcontract.LocalityLocal),
				Content: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeAudio, File: "artifacts/output.wav", ContentType: "audio/wav",
				}},
				Bindings: []modelcontract.ResolvedModelOperationBinding{{
					Slot: "text", Source: "INPUT", Content: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText, Text: "hello world",
					}},
				}},
			}, nil
		},
	}
	handler := NewHandlerFromRoot(RootBinding{Models: &rootFake{}, Invoker: invoker}, zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost, "/models/OMNIVOICE_Q4_K_M/invocations",
		strings.NewReader(`{"operation":"TTS","content":[{"type":"TEXT","text":"hello world"}]}`),
	)
	handler.InvokeModel(recorder, request, "OMNIVOICE_Q4_K_M")
	assertModelsHTTPCharacterizationJSON(t, recorder, http.StatusOK, characterizationHTTPInvocationBody)
}

func TestModelsHTTPCharacterizationPullResponse(t *testing.T) {
	t.Parallel()
	root := &rootFake{
		pullForScope: func(_ context.Context, request modelcontract.PullModelRequest) (modelcontract.PullResult, error) {
			return modelcontract.PullResult{
				ModelName: request.Name, ProviderLocality: string(modelcontract.LocalityLocal), Outcome: "PULLED",
				CachePath: "/models/OMNIVOICE_Q4_K_M/rev-2026", Revision: "rev-2026",
				ManagedPullOutcome: "INSTALLED_SUCCESSFULLY", ReadinessState: "READY",
				DownloadedFiles: []modelcontract.DownloadedFile{
					{Path: "weights.gguf", Bytes: 42, SHA256: "abc123"}, {Path: "config.json", Bytes: 7},
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

type multipartTestPart struct {
	name, contentType string
	data              []byte
}

func genericMultipartRequest(t *testing.T, parts []multipartTestPart) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, value := range parts {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q`, value.name))
		if value.contentType != "" {
			header.Set("Content-Type", value.contentType)
		}
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(value.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/models/invocations", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func TestGenericMultipartPreservesAudioVideoAndRepeatedSlotOrder(t *testing.T) {
	t.Parallel()
	audio := []byte{0, 255, 1, 128}
	video1 := []byte{0, 0, 0, 1, 255}
	video2 := []byte{255, 0, 33, 128}
	requestJSON := `{"scope":"factory-session:http-test","holder":"operator","model":{"nameOrUri":"omni"},"operation":"OMNI","inputs":[{"name":"prompt","modality":"TEXT","content":"compare"},{"name":"audio","modality":"AUDIO","mediaType":"audio/wav","contentType":"audio/wav"},{"name":"video","modality":"VIDEO"},{"name":"video","modality":"VIDEO","mediaType":"video/mp4"}]}`
	parts := []multipartTestPart{
		{name: "files", contentType: "audio/wav", data: audio},
		{name: "request", contentType: "application/json", data: []byte(requestJSON)},
		{name: "files", contentType: "video/mp4", data: video1},
		{name: "files", contentType: "video/mp4", data: video2},
	}
	var captured modelcontract.InvokeModelRequest
	root := &rootFake{invokeGeneric: func(_ context.Context, request modelcontract.InvokeModelRequest) (modelcontract.InvokeModelResult, error) {
		captured = request
		return modelcontract.InvokeModelResult{}, nil
	}}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.InvokeGenericModel(recorder, genericMultipartRequest(t, parts))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if len(captured.Inputs) != 4 || captured.Inputs[0].Content != "compare" ||
		!bytes.Equal([]byte(captured.Inputs[1].Content), audio) ||
		!bytes.Equal([]byte(captured.Inputs[2].Content), video1) ||
		!bytes.Equal([]byte(captured.Inputs[3].Content), video2) ||
		captured.Inputs[1].MediaType != "audio/wav" || captured.Inputs[2].MediaType != "video/mp4" {
		t.Fatalf("mapped multipart inputs = %#v", captured.Inputs)
	}
}

// Uploads are not constrained by the old per-file, request-part, or total-body
// limits. Stream the fixture body so the test does not retain a second upload.
func TestGenericMultipartAcceptsLargeMediaAndPrompt(t *testing.T) {
	t.Parallel()
	const size = 65 << 20
	prompt := strings.Repeat("p", 1<<20+1)
	metadata := `{"scope":"factory-session:http-test","holder":"operator","model":{"nameOrUri":"asr"},"operation":"ASR","inputs":[{"name":"prompt","modality":"TEXT","content":"` + prompt + `"},{"name":"audio","modality":"AUDIO","mediaType":"video/mp4"}]}`
	const boundary = "models-large-input"
	prefix := "--" + boundary + "\r\nContent-Disposition: form-data; name=\"request\"\r\nContent-Type: application/json\r\n\r\n" + metadata + "\r\n--" + boundary + "\r\nContent-Disposition: form-data; name=\"files\"; filename=\"clip.mp4\"\r\nContent-Type: video/mp4\r\n\r\n"
	body := io.MultiReader(strings.NewReader(prefix), io.LimitReader(zeroUploadReader{}, size), strings.NewReader("\r\n--"+boundary+"--\r\n"))
	request := httptest.NewRequest(http.MethodPost, "/models/invocations", body)
	request.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	calls := 0
	root := &rootFake{invokeGeneric: func(_ context.Context, request modelcontract.InvokeModelRequest) (modelcontract.InvokeModelResult, error) {
		calls++
		if len(request.Inputs) != 2 || request.Inputs[0].Content != prompt || len(request.Inputs[1].Content) != size || request.Inputs[1].MediaType != "video/mp4" {
			t.Fatal("large multipart input was truncated or changed")
		}
		return modelcontract.InvokeModelResult{}, nil
	}}
	recorder := httptest.NewRecorder()
	NewHandlerFromRoot(testRootBinding(root), zap.NewNop()).InvokeGenericModel(recorder, request)
	if recorder.Code != http.StatusOK || calls != 1 {
		t.Fatalf("large upload response = %d, calls = %d, want 200/1", recorder.Code, calls)
	}
}

type zeroUploadReader struct{}

func (zeroUploadReader) Read(data []byte) (int, error) {
	clear(data)
	return len(data), nil
}

func TestGenericMultipartRejectsInvalidPartsBeforeRoot(t *testing.T) {
	t.Parallel()
	base := `{"scope":"factory-session:http-test","holder":"operator","model":{"nameOrUri":"omni"},"operation":"OMNI","inputs":[{"name":"audio","modality":"AUDIO","mediaType":"audio/wav"}]}`
	requestPart := multipartTestPart{name: "request", contentType: "application/json", data: []byte(base)}
	filePart := multipartTestPart{name: "files", contentType: "audio/wav", data: []byte{0, 255}}
	cases := []struct {
		name, message string
		parts         []multipartTestPart
	}{
		{name: "missing file", message: "required", parts: []multipartTestPart{requestPart}},
		{name: "extra file", message: "no matching", parts: []multipartTestPart{requestPart, filePart, filePart}},
		{name: "missing request", message: "request part is required", parts: []multipartTestPart{filePart}},
		{name: "duplicate request", message: "must occur once", parts: []multipartTestPart{requestPart, requestPart, filePart}},
		{name: "unexpected field", message: "unexpected multipart field", parts: []multipartTestPart{requestPart, {name: "other", data: []byte("x")}}},
		{name: "malformed JSON", message: "invalid request payload", parts: []multipartTestPart{{name: "request", data: []byte("{")}, filePart}},
		{name: "unknown JSON field", message: "invalid request payload", parts: []multipartTestPart{{name: "request", data: []byte(strings.Replace(base, `"holder":`, `"unexpected":true,"holder":`, 1))}, filePart}},
		{name: "media mismatch", message: "does not match", parts: []multipartTestPart{requestPart, {name: "files", contentType: "video/mp4", data: []byte("x")}}},
		{name: "content type mismatch", message: "does not match input contentType", parts: []multipartTestPart{{name: "request", data: []byte(strings.Replace(base, `"mediaType":"audio/wav"`, `"mediaType":"audio/wav","contentType":"audio/mpeg"`, 1))}, filePart}},
		{name: "empty file", message: "must not be empty", parts: []multipartTestPart{requestPart, {name: "files", contentType: "audio/wav"}}},
		{name: "ambiguous carrier", message: "only one content carrier", parts: []multipartTestPart{{name: "request", data: []byte(strings.Replace(base, `"mediaType":"audio/wav"`, `"mediaType":"audio/wav","content":"a","artifactRef":"models:asset"`, 1))}}},
		{name: "file with existing carrier", message: "no matching", parts: []multipartTestPart{{name: "request", data: []byte(strings.Replace(base, `"mediaType":"audio/wav"`, `"mediaType":"audio/wav","content":"a"`, 1))}, filePart}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := &rootFake{invokeGeneric: func(context.Context, modelcontract.InvokeModelRequest) (modelcontract.InvokeModelResult, error) {
				t.Fatal("root invoked for invalid multipart request")
				return modelcontract.InvokeModelResult{}, nil
			}}
			handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
			recorder := httptest.NewRecorder()
			handler.InvokeGenericModel(recorder, genericMultipartRequest(t, test.parts))
			if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), test.message) {
				t.Fatalf("response = %d %s, want 400 containing %q", recorder.Code, recorder.Body.String(), test.message)
			}
		})
	}
}
