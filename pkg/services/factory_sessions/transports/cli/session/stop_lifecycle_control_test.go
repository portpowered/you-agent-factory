package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestRemoteCancelAndTerminateUseLiveEndpoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		path   string
		kind   factoryapi.FactorySessionLifecycleControlKind
		invoke func(LifecycleControlConfig) error
	}{
		{name: "cancel", path: "cancel", kind: factoryapi.FactorySessionLifecycleControlKindCancel, invoke: func(cfg LifecycleControlConfig) error { return Cancel(cfg) }},
		{name: "terminate", path: "terminate", kind: factoryapi.FactorySessionLifecycleControlKindTerminate, invoke: func(cfg LifecycleControlConfig) error { return Terminate(cfg) }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			const sessionID = "session-live-stop"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/factory-sessions/"+sessionID+"/"+test.path {
					t.Fatalf("request = %s %s, want POST live %s endpoint", r.Method, r.URL.Path, test.path)
				}
				encodeLifecycleControlHTTPResponse(t, w, factoryapi.FactorySessionLifecycleControlResponse{
					SessionId: sessionID,
					Operation: test.kind,
					Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
					Status:    factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
				})
			}))
			defer server.Close()

			var output bytes.Buffer
			if err := test.invoke(LifecycleControlConfig{
				Context: context.Background(), Server: server.URL, SessionID: sessionID, HTTP: testHTTPProtocol(t),
				JSON: true, Output: &output,
			}); err != nil {
				t.Fatalf("%s: %v", test.name, err)
			}
			var response factoryapi.FactorySessionLifecycleControlResponse
			if err := json.Unmarshal(output.Bytes(), &response); err != nil {
				t.Fatalf("decode %s response: %v", test.name, err)
			}
			if response.Operation != test.kind || response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
				t.Fatalf("%s response = %#v, want accepted %s", test.name, response, test.kind)
			}
		})
	}
}

func TestRemoteCancelAndTerminatePreserveTerminalConflictAndNotFoundOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		kind     factoryapi.FactorySessionLifecycleControlKind
		status   int
		outcome  factoryapi.FactorySessionLifecycleControlOutcome
		invoke   func(LifecycleControlConfig) error
		notFound bool
	}{
		{name: "cancel-terminal", path: "cancel", kind: factoryapi.FactorySessionLifecycleControlKindCancel, status: http.StatusConflict, outcome: factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession, invoke: func(cfg LifecycleControlConfig) error { return Cancel(cfg) }},
		{name: "terminate-conflict", path: "terminate", kind: factoryapi.FactorySessionLifecycleControlKindTerminate, status: http.StatusConflict, outcome: factoryapi.FactorySessionLifecycleControlOutcomeConflict, invoke: func(cfg LifecycleControlConfig) error { return Terminate(cfg) }},
		{name: "cancel-not-found", path: "cancel", kind: factoryapi.FactorySessionLifecycleControlKindCancel, status: http.StatusNotFound, notFound: true, invoke: func(cfg LifecycleControlConfig) error { return Cancel(cfg) }},
		{name: "terminate-not-found", path: "terminate", kind: factoryapi.FactorySessionLifecycleControlKindTerminate, status: http.StatusNotFound, notFound: true, invoke: func(cfg LifecycleControlConfig) error { return Terminate(cfg) }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			const sessionID = "session-live-stop"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/factory-sessions/"+sessionID+"/"+test.path {
					t.Fatalf("path = %q, want %s", r.URL.Path, test.path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.status)
				if test.notFound {
					_ = json.NewEncoder(w).Encode(factoryapi.ErrorResponse{Code: factoryapi.ErrorResponseCodeNOTFOUND, Message: "factory session not found"})
					return
				}
				_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionLifecycleControlResponse{
					SessionId: sessionID,
					Operation: test.kind,
					Outcome:   test.outcome,
					Status:    factoryapi.FactorySessionDurableLifecycleStatusRunning,
				})
			}))
			defer server.Close()

			var output bytes.Buffer
			err := test.invoke(LifecycleControlConfig{
				Context: context.Background(), Server: server.URL, SessionID: sessionID, HTTP: testHTTPProtocol(t),
				JSON: true, Output: &output,
			})
			if test.notFound {
				if err == nil || !strings.Contains(err.Error(), `factory session "`+sessionID+`" not found`) {
					t.Fatalf("%s error = %v, want stable not-found diagnostic", test.name, err)
				}
				return
			}
			var rejected *LifecycleControlRejectedError
			if !errors.As(err, &rejected) {
				t.Fatalf("%s error = %v, want typed lifecycle rejection", test.name, err)
			}
			if rejected.Response.Operation != test.kind || rejected.Response.Outcome != test.outcome {
				t.Fatalf("%s rejection = %#v, want %s/%s", test.name, rejected.Response, test.kind, test.outcome)
			}
		})
	}
}
