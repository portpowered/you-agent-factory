package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
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

func TestHandlerFromRootRequiresSelectedLogger(t *testing.T) {
	t.Parallel()
	binding := testRootBinding(&rootFake{})
	if NewHandlerFromRoot(binding, nil) != nil || NewHandler(NewAdapter(binding.Models, noopModelInvoker{}, noopContentPreparation{}), nil) != nil {
		t.Fatal("nil logger must retain the direct constructor's invalid result")
	}
}

// A hook observes actual emission; attaching it to NewNop does not enable logs.
func selectedDiagnosticLogger(quiet bool, owner string) (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zap.ErrorLevel)
	logger := zap.New(core)
	if quiet {
		logger = zap.NewNop().WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
			_ = core.Write(entry, nil)
			return nil
		}))
	}
	return logger.With(zap.String("owner", owner)), logs
}

func assertSelectedDiagnostic(t *testing.T, logs *observer.ObservedLogs, quiet bool, owner, message string, err error) {
	t.Helper()
	if quiet {
		if logs.Len() != 0 {
			t.Fatalf("disabled logger emitted %v", logs.All())
		}
		return
	}
	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("diagnostics = %v, want one", entries)
	}
	wantFields := map[string]any{"owner": owner, "error": err.Error()}
	if entries[0].Level != zap.ErrorLevel || entries[0].Message != message || !reflect.DeepEqual(entries[0].ContextMap(), wantFields) {
		t.Fatalf("diagnostic = %#v %v, want %q %v", entries[0].Entry, entries[0].ContextMap(), message, wantFields)
	}
}

func assertSelectedHTTPResponse(t *testing.T, recorder *httptest.ResponseRecorder, status int, body string) {
	t.Helper()
	if recorder.Code != status || recorder.Body.String() != body || !reflect.DeepEqual(recorder.Header(), http.Header{"Content-Type": {"application/json"}}) {
		t.Fatalf("response = %d %v %q, want %d application/json %q", recorder.Code, recorder.Header(), recorder.Body.String(), status, body)
	}
}

type diagnosticFailingWriter struct {
	*httptest.ResponseRecorder
	err    error
	writes int
}

func (w *diagnosticFailingWriter) Write(body []byte) (int, error) {
	w.writes++
	// Retain attempted bytes as an observer; the client write still fails.
	_, _ = w.ResponseRecorder.Write(body)
	return 0, w.err
}

func selectedDiagnosticBinding(t *testing.T, operation string, failure error, calls *int) RootBinding {
	t.Helper()
	binding := testRootBinding(&rootFake{})
	root := binding.Models.(*rootFake)
	root.listCatalog = func(_ context.Context, request models.ListModelsRequest) (models.ListModelsResult, error) {
		*calls++
		if request.Scope != binding.Scope {
			t.Errorf("list scope = %v", request.Scope)
		}
		if operation == "writer" {
			return models.ListModelsResult{}, nil
		}
		return models.ListModelsResult{}, failure
	}
	root.getCatalog = func(_ context.Context, request models.GetModelRequest) (models.GetModelResult, error) {
		*calls++
		if request.Scope != binding.Scope || request.Name != "voice" {
			t.Errorf("get request = %+v", request)
		}
		return models.GetModelResult{}, failure
	}
	binding.Invoker = invokeInvokerFake{invoke: func(_ context.Context, name string, request models.Request) (models.Result, error) {
		*calls++
		if name != "voice" || request.Operation != "TTS" || len(request.Content) != 1 || request.Content[0].Text != "private-payload-marker" {
			t.Errorf("invocation = %s %+v", name, request)
		}
		return models.Result{}, failure
	}}
	return binding
}

func TestHandlerFromRootSelectedDiagnostics(t *testing.T) {
	t.Parallel()
	for _, quiet := range []bool{false, true} {
		for _, operation := range []string{"list", "get", "invoke", "writer"} {
			t.Run(operation+map[bool]string{false: "/capture", true: "/quiet"}[quiet], func(t *testing.T) {
				t.Parallel()
				logger, logs := selectedDiagnosticLogger(quiet, operation)
				failure := errors.New("controlled catalog failure")
				if operation == "invoke" {
					failure = errors.New("open /internal/model.bin: controlled failure")
				}
				calls := 0
				binding := selectedDiagnosticBinding(t, operation, failure, &calls)
				handler := NewHandlerFromRoot(binding, logger)
				recorder := httptest.NewRecorder()
				message := "failed to list models"
				status := http.StatusInternalServerError
				request := httptest.NewRequest(http.MethodGet, "/models", nil)
				switch operation {
				case "list":
					handler.ListModels(recorder, request)
				case "get":
					message = "failed to load model"
					handler.GetModel(recorder, request, "voice")
				case "invoke":
					message = "model invocation failed"
					handler.InvokeModel(recorder, httptest.NewRequest(http.MethodPost, "/models/voice/invocations", strings.NewReader(`{"operation":"TTS","content":[{"type":"TEXT","text":"private-payload-marker"}]}`)), "voice")
				case "writer":
					message = "encode response failed"
					failure = errors.New("controlled writer failure")
					writer := &diagnosticFailingWriter{ResponseRecorder: recorder, err: failure}
					handler.ListModels(writer, request)
					if writer.writes != 1 {
						t.Fatalf("writes = %d", writer.writes)
					}
					status = http.StatusOK
				}
				body := `{"code":"INTERNAL_ERROR","family":"INTERNAL_SERVER_ERROR","message":"` + message + `"}` + "\n"
				if operation == "writer" {
					body = "{\"results\":[]}\n"
				}
				assertSelectedHTTPResponse(t, recorder, status, body)
				assertSelectedDiagnostic(t, logs, quiet, operation, message, failure)
				if calls != 1 {
					t.Fatalf("delegate calls = %d, want one", calls)
				}
			})
		}
	}
}

func TestHandlerFromRootOverlappingSelectedLoggers(t *testing.T) {
	t.Parallel()
	for _, quietPeer := range []bool{false, true} {
		t.Run(map[bool]string{false: "capture-peer", true: "quiet-peer"}[quietPeer], func(t *testing.T) {
			t.Parallel()
			enteredA := make(chan models.ListModelsRequest, 1)
			enteredB := make(chan models.ListModelsRequest, 1)
			release := make(chan struct{})
			done := make(chan struct{}, 2)
			failure := errors.New("adapter A catalog failure")
			writeFailure := errors.New("adapter B writer failure")
			loggerA, logsA := selectedDiagnosticLogger(false, "A")
			loggerB, logsB := selectedDiagnosticLogger(quietPeer, "B")
			scopeA, err := (models.RuntimeScopeRef{}).Parse("factory-session:adapter-a")
			if err != nil {
				t.Fatal(err)
			}
			scopeB, err := (models.RuntimeScopeRef{}).Parse("factory-session:adapter-b")
			if err != nil {
				t.Fatal(err)
			}
			rootA := &rootFake{listCatalog: func(_ context.Context, request models.ListModelsRequest) (models.ListModelsResult, error) {
				enteredA <- request
				<-release
				return models.ListModelsResult{}, failure
			}}
			rootB := &rootFake{listCatalog: func(_ context.Context, request models.ListModelsRequest) (models.ListModelsResult, error) {
				enteredB <- request
				<-release
				return models.ListModelsResult{Models: []models.Summary{{Name: "peer-result-marker"}}}, nil
			}}
			handlerA := NewHandlerFromRoot(RootBinding{Models: rootA, Scope: scopeA}, loggerA)
			handlerB := NewHandlerFromRoot(RootBinding{Models: rootB, Scope: scopeB}, loggerB)
			recorderA, recorderB := httptest.NewRecorder(), httptest.NewRecorder()
			writerB := &diagnosticFailingWriter{ResponseRecorder: recorderB, err: writeFailure}
			go func() {
				handlerA.ListModels(recorderA, httptest.NewRequest(http.MethodGet, "/models", nil))
				done <- struct{}{}
			}()
			go func() {
				handlerB.ListModels(writerB, httptest.NewRequest(http.MethodGet, "/models", nil))
				done <- struct{}{}
			}()
			first, second := <-enteredA, <-enteredB
			close(release)
			<-done
			<-done
			if first != (models.ListModelsRequest{Scope: scopeA}) || second != (models.ListModelsRequest{Scope: scopeB}) {
				t.Fatalf("delegate scopes = %v %v", first.Scope, second.Scope)
			}
			assertSelectedHTTPResponse(t, recorderA, 500, "{\"code\":\"INTERNAL_ERROR\",\"family\":\"INTERNAL_SERVER_ERROR\",\"message\":\"failed to list models\"}\n")
			assertSelectedHTTPResponse(t, recorderB, 200, `{"results":[{"loadState":"","managedRuntime":{"diagnostics":{},"identity":"","lifecycleState":"","locality":"","readinessState":"","supportedOperations":[]},"modalities":[],"name":"peer-result-marker","operations":[],"providerLocality":"","resources":[],"status":""}]}`+"\n")
			if writerB.writes != 1 {
				t.Fatalf("peer writes = %d", writerB.writes)
			}
			assertSelectedDiagnostic(t, logsA, false, "A", "failed to list models", failure)
			assertSelectedDiagnostic(t, logsB, quietPeer, "B", "encode response failed", writeFailure)
		})
	}
}
