package session

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestPause_PerformsPOSTWithEscapedSessionPath(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		writeLifecycleControlResponse(t, w, factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "session/beta",
			Operation: factoryapi.FactorySessionLifecycleControlKindPause,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusPaused,
		})
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Pause(LifecycleControlConfig{
		Server:    srv.URL,
		SessionID: "session/beta",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/factory-sessions/session%2Fbeta/pause" {
		t.Fatalf("path = %q, want /factory-sessions/session%%2Fbeta/pause", gotPath)
	}
}

func TestPause_DefaultSessionUsesCompatibilitySessionPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		writeLifecycleControlResponse(t, w, factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "~default",
			Operation: factoryapi.FactorySessionLifecycleControlKindPause,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusPaused,
		})
	}))
	defer srv.Close()

	err := Pause(LifecycleControlConfig{
		Server: srv.URL,
		Output: io.Discard,
	})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if gotPath != "/factory-sessions/~default/pause" {
		t.Fatalf("path = %q, want /factory-sessions/~default/pause", gotPath)
	}
}

func TestPause_AcceptedPrintsHumanConfirmation(t *testing.T) {
	srv := lifecycleControlTestServer(t, factoryapi.FactorySessionLifecycleControlResponse{
		SessionId: "session-beta",
		Operation: factoryapi.FactorySessionLifecycleControlKindPause,
		Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
		Status:    factoryapi.FactorySessionDurableLifecycleStatusPaused,
	})
	defer srv.Close()

	var out bytes.Buffer
	err := Pause(LifecycleControlConfig{
		Server:    srv.URL,
		SessionID: "session-beta",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if got := out.String(); got != "Paused factory session session-beta\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestPause_NoOpPrintsAlreadyPausedMessage(t *testing.T) {
	srv := lifecycleControlTestServer(t, factoryapi.FactorySessionLifecycleControlResponse{
		SessionId: "session-beta",
		Operation: factoryapi.FactorySessionLifecycleControlKindPause,
		Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeNoOp,
		Status:    factoryapi.FactorySessionDurableLifecycleStatusPaused,
	})
	defer srv.Close()

	var out bytes.Buffer
	err := Pause(LifecycleControlConfig{
		Server:    srv.URL,
		SessionID: "session-beta",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if got := out.String(); got != "Factory session session-beta is already paused\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestPause_JSONEmitsAPIShapedLifecycleControlResponse(t *testing.T) {
	response := factoryapi.FactorySessionLifecycleControlResponse{
		SessionId: "session-beta",
		Operation: factoryapi.FactorySessionLifecycleControlKindPause,
		Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
		Status:    factoryapi.FactorySessionDurableLifecycleStatusPaused,
	}
	srv := lifecycleControlTestServer(t, response)
	defer srv.Close()

	var out bytes.Buffer
	err := Pause(LifecycleControlConfig{
		Server:    srv.URL,
		SessionID: "session-beta",
		JSON:      true,
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}

	var got factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out.String())
	}
	if got.SessionId != response.SessionId || got.Outcome != response.Outcome {
		t.Fatalf("response = %#v, want %#v", got, response)
	}
}

func TestResume_AcceptedPrintsHumanConfirmation(t *testing.T) {
	srv := lifecycleControlTestServer(t, factoryapi.FactorySessionLifecycleControlResponse{
		SessionId: "session-beta",
		Operation: factoryapi.FactorySessionLifecycleControlKindResume,
		Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
		Status:    factoryapi.FactorySessionDurableLifecycleStatusRunning,
	})
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
	if got := out.String(); got != "Resumed factory session session-beta\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestResume_NoOpPrintsAlreadyRunningMessage(t *testing.T) {
	srv := lifecycleControlTestServer(t, factoryapi.FactorySessionLifecycleControlResponse{
		SessionId: "session-beta",
		Operation: factoryapi.FactorySessionLifecycleControlKindResume,
		Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeNoOp,
		Status:    factoryapi.FactorySessionDurableLifecycleStatusRunning,
	})
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
	if got := out.String(); got != "Factory session session-beta is already running\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestResume_InvalidStateReturnsRejectedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		if err := json.NewEncoder(w).Encode(factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "session-beta",
			Operation: factoryapi.FactorySessionLifecycleControlKindResume,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeInvalidState,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusRunning,
			Detail:    stringPtr("RESUME rejected for session session-beta in factory state RUNNING"),
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	err := Resume(LifecycleControlConfig{
		Server:    srv.URL,
		SessionID: "session-beta",
		Output:    ioDiscardWriter{t},
	})
	if err == nil {
		t.Fatal("expected rejected error")
	}
	if !strings.Contains(err.Error(), "resume rejected for session-beta") {
		t.Fatalf("error = %q", err.Error())
	}
	if !strings.Contains(err.Error(), "RESUME rejected") {
		t.Fatalf("error = %q, want API message", err.Error())
	}
}

func TestPause_NotFoundReturnsClearMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		if err := json.NewEncoder(w).Encode(factoryapi.ErrorResponse{
			Message: "factory session not found",
			Code:    "NOT_FOUND",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	err := Pause(LifecycleControlConfig{
		Server:    srv.URL,
		SessionID: "missing-session",
		Output:    ioDiscardWriter{t},
	})
	if err == nil {
		t.Fatal("expected not-found error")
	}
	if !strings.Contains(err.Error(), "factory session \"missing-session\" not found") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestResume_UnreachableServiceNamesEndpoint(t *testing.T) {
	var out bytes.Buffer
	err := Resume(LifecycleControlConfig{
		Server:    "http://127.0.0.1:1",
		SessionID: "session-beta",
		JSON:      true,
		Output:    &out,
	})
	if err == nil {
		t.Fatal("expected unreachable error")
	}
	want := "factory sessions endpoint not reachable at http://127.0.0.1:1/factory-sessions/session-beta/resume"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q", err.Error())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty output when --json is set", out.String())
	}
}

func TestResume_DrainsBufferedWorkWithoutPostResumeExternalSignal(t *testing.T) {
	state := &lifecycleControlDrainState{}
	srv := httptest.NewServer(http.HandlerFunc(state.handler))
	defer srv.Close()

	if err := Pause(LifecycleControlConfig{
		Server: srv.URL,
		Output: io.Discard,
	}); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	submitResp, err := http.Post(
		srv.URL+"/factory-sessions/~default/work",
		"application/json",
		strings.NewReader(`{"name":"cli-paused-submit","workTypeName":"task"}`),
	)
	if err != nil {
		t.Fatalf("submit while paused: %v", err)
	}
	submitResp.Body.Close()
	if submitResp.StatusCode != http.StatusOK && submitResp.StatusCode != http.StatusCreated {
		t.Fatalf("submit status = %d, want 200 or 201", submitResp.StatusCode)
	}

	if err := Resume(LifecycleControlConfig{
		Server: srv.URL,
		Output: io.Discard,
	}); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	if !slices.Equal(state.calls(), []string{"pause", "submit", "resume"}) {
		t.Fatalf("calls = %#v, want pause then submit then resume without post-resume external signal", state.calls())
	}
	if !state.resumeAfterBufferedSubmit {
		t.Fatal("resume did not follow buffered submit while paused")
	}
}

type lifecycleControlDrainState struct {
	mu                        sync.Mutex
	callLog                   []string
	paused                    bool
	resumeAfterBufferedSubmit bool
	submittedWhilePaused      bool
}

func (s *lifecycleControlDrainState) calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.callLog))
	copy(out, s.callLog)
	return out
}

func (s *lifecycleControlDrainState) handler(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pause"):
		s.callLog = append(s.callLog, "pause")
		s.paused = true
		writeLifecycleControlJSON(w, factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "~default",
			Operation: factoryapi.FactorySessionLifecycleControlKindPause,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusPaused,
		})
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/resume"):
		s.callLog = append(s.callLog, "resume")
		if s.paused && s.submittedWhilePaused {
			s.resumeAfterBufferedSubmit = true
		}
		s.paused = false
		writeLifecycleControlJSON(w, factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "~default",
			Operation: factoryapi.FactorySessionLifecycleControlKindResume,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusRunning,
		})
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/work"):
		s.callLog = append(s.callLog, "submit")
		if s.paused {
			s.submittedWhilePaused = true
		}
		w.WriteHeader(http.StatusCreated)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func lifecycleControlTestServer(t *testing.T, response factoryapi.FactorySessionLifecycleControlResponse) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeLifecycleControlResponse(t, w, response)
	}))
}

func writeLifecycleControlResponse(t *testing.T, w http.ResponseWriter, response factoryapi.FactorySessionLifecycleControlResponse) {
	t.Helper()
	writeLifecycleControlJSON(w, response)
}

func writeLifecycleControlJSON(w http.ResponseWriter, response factoryapi.FactorySessionLifecycleControlResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}
