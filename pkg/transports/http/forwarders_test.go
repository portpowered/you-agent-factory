package http

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestNewForwarderRejectsIncompleteRouteSurface(t *testing.T) {
	t.Parallel()

	forwarder, err := NewForwarder(ForwarderHandlers{})
	if forwarder != nil || err == nil {
		t.Fatalf("NewForwarder(empty) = (%#v, %v), want required-handler error", forwarder, err)
	}
	if got := err.Error(); got != "construct HTTP forwarder: PreviewFactory handler is required" {
		t.Fatalf("NewForwarder(empty) error = %q, want stable first missing route", got)
	}
}

func TestForwarderPassesGeneratedProtocolValuesUnchanged(t *testing.T) {
	t.Parallel()

	wantWriter := httptest.NewRecorder()
	wantRequest := httptest.NewRequest(http.MethodGet, "/models/example", nil)
	wantName := "example"
	wantParams := factoryapi.ListFactorySessionsParams{Scope: ptr(factoryapi.FactorySessionListScopeAll)}
	var gotWriter http.ResponseWriter
	var gotRequest *http.Request
	var gotName string
	var gotParams factoryapi.ListFactorySessionsParams

	handlers := completeForwarderHandlers(t)
	handlers.GetModel = func(writer http.ResponseWriter, request *http.Request, name string) {
		gotWriter, gotRequest, gotName = writer, request, name
	}
	handlers.ListFactorySessions = func(writer http.ResponseWriter, request *http.Request, params factoryapi.ListFactorySessionsParams) {
		gotWriter, gotRequest, gotParams = writer, request, params
	}
	forwarder, err := NewForwarder(handlers)
	if err != nil {
		t.Fatalf("NewForwarder() error = %v", err)
	}

	forwarder.GetModel(wantWriter, wantRequest, wantName)
	if gotWriter != wantWriter || gotRequest != wantRequest || gotName != wantName {
		t.Fatalf("GetModel forwarded (%p, %p, %q), want exact (%p, %p, %q)", gotWriter, gotRequest, gotName, wantWriter, wantRequest, wantName)
	}
	forwarder.ListFactorySessions(wantWriter, wantRequest, wantParams)
	if gotWriter != wantWriter || gotRequest != wantRequest || !reflect.DeepEqual(gotParams, wantParams) {
		t.Fatalf("ListFactorySessions forwarded (%p, %p, %#v), want exact (%p, %p, %#v)", gotWriter, gotRequest, gotParams, wantWriter, wantRequest, wantParams)
	}
}

func TestNewComposedHandlerRegistersRoutesAroundForwarder(t *testing.T) {
	t.Parallel()

	handlers := completeForwarderHandlers(t)
	called := false
	handlers.GetModel = func(writer http.ResponseWriter, request *http.Request, name string) {
		called = request.Method == http.MethodGet && request.URL.Path == "/models/one" && name == "one"
		writer.WriteHeader(http.StatusNoContent)
	}
	forwarder, err := NewForwarder(handlers)
	if err != nil {
		t.Fatalf("NewForwarder() error = %v", err)
	}
	router, err := NewComposedHandler(forwarder)
	if err != nil {
		t.Fatalf("NewComposedHandler() error = %v", err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/models/one", nil))
	if !called || response.Code != http.StatusNoContent {
		t.Fatalf("composed route called = %t, status = %d, want true/%d", called, response.Code, http.StatusNoContent)
	}
}

func TestNewComposedHandlerRejectsMissingForwarder(t *testing.T) {
	t.Parallel()

	if handler, err := NewComposedHandler(nil); handler != nil || err == nil {
		t.Fatalf("NewComposedHandler(nil) = (%#v, %v), want required-forwarder error", handler, err)
	}
}

// completeForwarderHandlers supplies protocol-only no-op callbacks for the
// route-completeness fixture. Production Wire composition supplies real owner
// callbacks for every field.
func completeForwarderHandlers(t *testing.T) ForwarderHandlers {
	t.Helper()
	var handlers ForwarderHandlers
	value := reflect.ValueOf(&handlers).Elem()
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		field.Set(reflect.MakeFunc(field.Type(), func([]reflect.Value) []reflect.Value { return nil }))
	}
	return handlers
}

func ptr[T any](value T) *T {
	return &value
}
