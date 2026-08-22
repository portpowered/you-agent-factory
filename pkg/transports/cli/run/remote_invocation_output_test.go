package run

import (
	"encoding/json"
	"net/http"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func remoteSuccessHandler(
	t *testing.T,
	gotRequest *factoryapi.FactorySessionExecutionRequest,
	resultCalls *int,
) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/selected/factory-sessions/async":
			if err := json.NewDecoder(r.Body).Decode(gotRequest); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionExecutionResponse{
				SessionId:        "dur-sess-remote",
				Status:           factoryapi.FactorySessionDurableLifecycleStatusQueued,
				OrchestratorKind: factoryapi.JAVASCRIPT,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/selected/factory-sessions/dur-sess-remote/results":
			*resultCalls = *resultCalls + 1
			if *resultCalls == 1 {
				status := factoryapi.FactorySessionDurableLifecycleStatusRunning
				retryable := true
				_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionResult{
					SessionId:     "dur-sess-remote",
					ResultStatus:  factoryapi.FactorySessionResultStatusNotReady,
					SessionStatus: &status,
					Availability:  &factoryapi.FactorySessionResultAvailabilityDetail{Retryable: &retryable},
				})
				return
			}
			status := factoryapi.FactorySessionDurableLifecycleStatusSucceeded
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionResult{
				SessionId:     "dur-sess-remote",
				ResultStatus:  factoryapi.FactorySessionResultStatusFinal,
				SessionStatus: &status,
				PrimaryResult: remoteTextContent(t, "remote result"),
			})
		default:
			t.Errorf("request = %s %s, want selected durable start or result endpoint", r.Method, r.URL.Path)
		}
	})
}
