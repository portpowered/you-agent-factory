package session

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestPause_PerformsPOSTFactorySessionPause(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "~default",
			Operation: factoryapi.FactorySessionLifecycleControlKindPause,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusPaused,
		})
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Pause(LifecycleControlConfig{
		Server:  srv.URL,
		Output:  &out,
	})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/factory-sessions/~default/pause" {
		t.Fatalf("path = %q, want /factory-sessions/~default/pause", gotPath)
	}
	if !strings.Contains(out.String(), "Paused factory session ~default") {
		t.Fatalf("output = %q, want paused confirmation", out.String())
	}
}

func TestResume_PerformsPOSTFactorySessionResume(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory-sessions/session-beta/resume" {
			t.Fatalf("path = %q, want /factory-sessions/session-beta/resume", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "session-beta",
			Operation: factoryapi.FactorySessionLifecycleControlKindResume,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusRunning,
		})
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Resume(LifecycleControlConfig{
		Server:    srv.URL,
		SessionID: "session-beta",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if !strings.Contains(out.String(), "Resumed factory session session-beta") {
		t.Fatalf("output = %q, want resumed confirmation", out.String())
	}
}

func TestPause_NoOpRendersAlreadyPausedCopy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "~default",
			Operation: factoryapi.FactorySessionLifecycleControlKindPause,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeNoOp,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusPaused,
		})
	}))
	defer srv.Close()

	var out bytes.Buffer
	if err := Pause(LifecycleControlConfig{Server: srv.URL, Output: &out}); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if !strings.Contains(out.String(), "already paused") {
		t.Fatalf("output = %q, want no-op paused copy", out.String())
	}
}

func TestPause_ReturnsNotFoundForMissingSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(factoryapi.ErrorResponse{
			Code:    factoryapi.NOTFOUND,
			Message: "factory session not found",
		})
	}))
	defer srv.Close()

	err := Pause(LifecycleControlConfig{
		Server:    srv.URL,
		SessionID: "missing-session",
	})
	if err == nil || !strings.Contains(err.Error(), `factory session "missing-session" not found`) {
		t.Fatalf("error = %v, want not found", err)
	}
}

func TestResume_ReturnsConflictForInvalidState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "~default",
			Operation: factoryapi.FactorySessionLifecycleControlKindResume,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeInvalidState,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusRunning,
			Detail:    stringPtr("resume rejected for running session"),
		})
	}))
	defer srv.Close()

	err := Resume(LifecycleControlConfig{Server: srv.URL})
	if err == nil || !strings.Contains(err.Error(), "resume rejected") {
		t.Fatalf("error = %v, want invalid-state conflict", err)
	}
}

func TestPause_JSONEmitsLifecycleControlResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "~default",
			Operation: factoryapi.FactorySessionLifecycleControlKindPause,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusPaused,
		})
	}))
	defer srv.Close()

	var out bytes.Buffer
	if err := Pause(LifecycleControlConfig{Server: srv.URL, JSON: true, Output: &out}); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	var payload factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if payload.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", payload.Outcome)
	}
}
