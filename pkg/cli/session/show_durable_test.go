package session

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestShow_DurableSessionJSONUsesDurableReadModel(t *testing.T) {
	interruptedAt := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/factory-sessions/dur-sess-js-interrupted-001"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.FactorySessionDurableReadModel{
			SessionId:        "dur-sess-js-interrupted-001",
			Status:           factoryapi.FactorySessionDurableLifecycleStatusInterrupted,
			OrchestratorKind: factoryapi.JAVASCRIPT,
			Lifecycle: &factoryapi.FactorySessionDurableLifecycleTimestamps{
				InterruptedAt: &interruptedAt,
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	if err := Show(ShowConfig{
		Server:    srv.URL,
		SessionID: "dur-sess-js-interrupted-001",
		JSON:      true,
		Output:    &out,
	}); err != nil {
		t.Fatalf("Show durable JSON: %v", err)
	}

	var session factoryapi.FactorySessionDurableReadModel
	if err := json.Unmarshal(out.Bytes(), &session); err != nil {
		t.Fatalf("decode durable JSON: %v", err)
	}
	if session.SessionId != "dur-sess-js-interrupted-001" {
		t.Fatalf("sessionId = %q", session.SessionId)
	}
	if session.Status != factoryapi.FactorySessionDurableLifecycleStatusInterrupted {
		t.Fatalf("status = %q, want INTERRUPTED", session.Status)
	}
}

func TestShow_DurableSessionHumanOutputRendersLifecycleContinuity(t *testing.T) {
	interruptedAt := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	resumedAt := time.Date(2026, 6, 30, 12, 5, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.FactorySessionDurableReadModel{
			SessionId:        "dur-sess-js-interrupted-001",
			Status:           factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
			OrchestratorKind: factoryapi.JAVASCRIPT,
			Lifecycle: &factoryapi.FactorySessionDurableLifecycleTimestamps{
				InterruptedAt: &interruptedAt,
				ResumedAt:     &resumedAt,
			},
			ResultSummary: &factoryapi.FactorySessionDurableResultSummary{
				ResultStatus: factoryapi.FactorySessionResultStatusFinal,
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	if err := Show(ShowConfig{
		Server:    srv.URL,
		SessionID: "dur-sess-js-interrupted-001",
		Output:    &out,
	}); err != nil {
		t.Fatalf("Show durable human: %v", err)
	}

	output := out.String()
	for _, want := range []string{
		"Factory session:\tdur-sess-js-interrupted-001",
		"Lifecycle status:\tSUCCEEDED",
		"Interrupted at:\t2026-06-30T12:00:00Z",
		"Resumed at:\t2026-06-30T12:05:00Z",
		"Result status:\tFINAL",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}
