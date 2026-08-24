package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestLifecycleControlStatusErrorIncludesAPIMessage(t *testing.T) {
	t.Parallel()
	resp := &http.Response{
		StatusCode: http.StatusConflict,
		Body:       io.NopCloser(strings.NewReader(`{"message":"session is terminal"}`)),
	}
	err := lifecycleControlStatusError("pause", resp)
	if !strings.Contains(err.Error(), "session is terminal") {
		t.Fatalf("lifecycleControlStatusError = %q", err)
	}
}

func TestPause_OmittedSessionIDRoutesToDefaultCompatibilitySession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/factory-sessions/~default/pause"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		encodeLifecycleControlHTTPResponse(t, w, factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "~default",
			Operation: factoryapi.FactorySessionLifecycleControlKindPause,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusPaused,
		})
	}))
	defer srv.Close()

	if err := NewPause(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server: srv.URL,
		Output: &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("Pause default compatibility session: %v", err)
	}
}

func TestPause_NamedLiveSessionIDRoutesWithoutDurableValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/factory-sessions/session-beta/pause"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		encodeLifecycleControlHTTPResponse(t, w, factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "session-beta",
			Operation: factoryapi.FactorySessionLifecycleControlKindPause,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusPaused,
		})
	}))
	defer srv.Close()

	if err := NewPause(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    srv.URL,
		SessionID: "session-beta",
		Output:    &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("Pause named live session: %v", err)
	}
}

func TestPause_HumanOutputReportsPausedOutcome(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encodeLifecycleControlHTTPResponse(t, w, samplePauseAcceptedResponse("dur-sess-pause-001"))
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewPause(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    srv.URL,
		SessionID: "dur-sess-pause-001",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	text := strings.TrimSpace(out.String())
	if text != "Paused Factory session dur-sess-pause-001 (lifecycle status: PAUSED)." {
		t.Fatalf("output = %q", text)
	}
}

func TestPause_HumanOutputReportsAlreadyPausedOutcome(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encodeLifecycleControlHTTPResponse(t, w, factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "dur-sess-paused-001",
			Operation: factoryapi.FactorySessionLifecycleControlKindPause,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeNoOp,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusPaused,
		})
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewPause(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    srv.URL,
		SessionID: "dur-sess-paused-001",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Pause no-op: %v", err)
	}
	text := strings.TrimSpace(out.String())
	if text != "Factory session dur-sess-paused-001 is already paused." {
		t.Fatalf("output = %q", text)
	}
}

func TestPause_HumanOutputReportsInvalidStateOutcome(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		if err := json.NewEncoder(w).Encode(factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "dur-sess-awaiting-001",
			Operation: factoryapi.FactorySessionLifecycleControlKindPause,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeInvalidState,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusAwaitingApproval,
			Detail:    lifecycleControlStringPtr("pause is not allowed while awaiting approval"),
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewPause(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    srv.URL,
		SessionID: "dur-sess-awaiting-001",
		Output:    &out,
	})
	if err == nil {
		t.Fatal("expected invalid-state error")
	}
	text := strings.TrimSpace(out.String())
	want := "Factory session dur-sess-awaiting-001 cannot be paused while lifecycle status is AWAITING_APPROVAL. pause is not allowed while awaiting approval"
	if text != want {
		t.Fatalf("output = %q, want %q", text, want)
	}
}

func TestPause_SuccessReturnsAcceptedOutcome(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encodeLifecycleControlHTTPResponse(t, w, samplePauseAcceptedResponse("dur-sess-pause-001"))
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewPause(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    srv.URL,
		SessionID: "dur-sess-pause-001",
		JSON:      true,
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}

	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out.String())
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindPause {
		t.Fatalf("operation = %q, want PAUSE", response.Operation)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", response.Status)
	}
}

func TestPause_NoOpReturnsNoOpOutcome(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encodeLifecycleControlHTTPResponse(t, w, factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "dur-sess-paused-001",
			Operation: factoryapi.FactorySessionLifecycleControlKindPause,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeNoOp,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusPaused,
		})
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewPause(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    srv.URL,
		SessionID: "dur-sess-paused-001",
		JSON:      true,
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Pause no-op: %v", err)
	}

	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("outcome = %q, want NO_OP", response.Outcome)
	}
}

func TestPause_InvalidStateReturnsTypedRejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		if err := json.NewEncoder(w).Encode(factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "dur-sess-awaiting-001",
			Operation: factoryapi.FactorySessionLifecycleControlKindPause,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeInvalidState,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusAwaitingApproval,
			Detail:    lifecycleControlStringPtr("pause is not allowed while awaiting approval"),
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewPause(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    srv.URL,
		SessionID: "dur-sess-awaiting-001",
		JSON:      true,
		Output:    &out,
	})
	var rejected *LifecycleControlRejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("error = %v, want LifecycleControlRejectedError", err)
	}
	if rejected.Response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeInvalidState {
		t.Fatalf("outcome = %q, want INVALID_STATE", rejected.Response.Outcome)
	}

	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeInvalidState {
		t.Fatalf("stdout outcome = %q, want INVALID_STATE", response.Outcome)
	}
}

func TestPause_NotFoundReturnsDistinctError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"NOT_FOUND","message":"factory session not found"}`))
	}))
	defer srv.Close()

	err := NewPause(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    srv.URL,
		SessionID: "dur-sess-missing-001",
		Output:    &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected not-found error")
	}
	if strings.Contains(err.Error(), "INVALID_STATE") || strings.Contains(err.Error(), "NO_OP") {
		t.Fatalf("not-found error should not look like typed lifecycle outcome: %v", err)
	}
	if !strings.Contains(err.Error(), `factory session "dur-sess-missing-001" not found`) {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestPause_UnreachableHostReturnsTransportError(t *testing.T) {
	err := NewPause(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    "http://127.0.0.1:1",
		SessionID: "dur-sess-pause-001",
		Output:    &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected unreachable error")
	}
	if !strings.Contains(err.Error(), "not reachable") {
		t.Fatalf("error = %q, want unreachable-host wording", err.Error())
	}
}

func TestResume_HumanOutputReportsResumedOutcome(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encodeLifecycleControlHTTPResponse(t, w, factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "dur-sess-paused-001",
			Operation: factoryapi.FactorySessionLifecycleControlKindResume,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusRunning,
		})
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewResume(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    srv.URL,
		SessionID: "dur-sess-paused-001",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	text := strings.TrimSpace(out.String())
	if text != "Resumed Factory session dur-sess-paused-001 (lifecycle status: RUNNING)." {
		t.Fatalf("output = %q", text)
	}
}

func TestResume_HumanOutputReportsAlreadyRunningOutcome(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encodeLifecycleControlHTTPResponse(t, w, factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "dur-sess-running-001",
			Operation: factoryapi.FactorySessionLifecycleControlKindResume,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeNoOp,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusRunning,
		})
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewResume(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    srv.URL,
		SessionID: "dur-sess-running-001",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Resume no-op: %v", err)
	}
	text := strings.TrimSpace(out.String())
	if text != "Factory session dur-sess-running-001 is already running." {
		t.Fatalf("output = %q", text)
	}
}

func TestResume_SuccessReturnsAcceptedOutcome(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/resume") {
			t.Fatalf("path = %q, want resume endpoint", r.URL.Path)
		}
		encodeLifecycleControlHTTPResponse(t, w, factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "dur-sess-paused-001",
			Operation: factoryapi.FactorySessionLifecycleControlKindResume,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusRunning,
		})
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewResume(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    srv.URL,
		SessionID: "dur-sess-paused-001",
		JSON:      true,
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}

	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", response.Status)
	}
}

func TestResume_OmittedSessionIDRoutesToDefaultCompatibilitySession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/factory-sessions/~default/resume"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		encodeLifecycleControlHTTPResponse(t, w, factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "~default",
			Operation: factoryapi.FactorySessionLifecycleControlKindResume,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusRunning,
		})
	}))
	defer srv.Close()

	if err := NewResume(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server: srv.URL,
		Output: &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("Resume default compatibility session: %v", err)
	}
}

func TestResume_NamedLiveSessionIDRoutesWithoutDurableValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/factory-sessions/session-beta/resume"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		encodeLifecycleControlHTTPResponse(t, w, factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "session-beta",
			Operation: factoryapi.FactorySessionLifecycleControlKindResume,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusRunning,
		})
	}))
	defer srv.Close()

	if err := NewResume(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    srv.URL,
		SessionID: "session-beta",
		Output:    &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("Resume named live session: %v", err)
	}
}

func TestResume_HumanOutputReportsInvalidStateOutcome(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		if err := json.NewEncoder(w).Encode(factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "dur-sess-awaiting-001",
			Operation: factoryapi.FactorySessionLifecycleControlKindResume,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeInvalidState,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusAwaitingApproval,
			Detail:    lifecycleControlStringPtr("resume is not allowed while awaiting approval"),
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewResume(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    srv.URL,
		SessionID: "dur-sess-awaiting-001",
		Output:    &out,
	})
	if err == nil {
		t.Fatal("expected invalid-state error")
	}
	text := strings.TrimSpace(out.String())
	want := "Factory session dur-sess-awaiting-001 cannot be resumed while lifecycle status is AWAITING_APPROVAL. resume is not allowed while awaiting approval"
	if text != want {
		t.Fatalf("output = %q, want %q", text, want)
	}
}

func TestResume_InvalidStateReturnsTypedRejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		if err := json.NewEncoder(w).Encode(factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "dur-sess-awaiting-001",
			Operation: factoryapi.FactorySessionLifecycleControlKindResume,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeInvalidState,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusAwaitingApproval,
			Detail:    lifecycleControlStringPtr("resume is not allowed while awaiting approval"),
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewResume(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    srv.URL,
		SessionID: "dur-sess-awaiting-001",
		JSON:      true,
		Output:    &out,
	})
	var rejected *LifecycleControlRejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("error = %v, want LifecycleControlRejectedError", err)
	}
	if rejected.Response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeInvalidState {
		t.Fatalf("outcome = %q, want INVALID_STATE", rejected.Response.Outcome)
	}

	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeInvalidState {
		t.Fatalf("stdout outcome = %q, want INVALID_STATE", response.Outcome)
	}
}

func TestResume_NotFoundReturnsDistinctError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"NOT_FOUND","message":"factory session not found"}`))
	}))
	defer srv.Close()

	err := NewResume(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    srv.URL,
		SessionID: "dur-sess-missing-001",
		Output:    &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected not-found error")
	}
	if strings.Contains(err.Error(), "INVALID_STATE") || strings.Contains(err.Error(), "NO_OP") {
		t.Fatalf("not-found error should not look like typed lifecycle outcome: %v", err)
	}
	if !strings.Contains(err.Error(), `factory session "dur-sess-missing-001" not found`) {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestResume_UnreachableHostReturnsTransportError(t *testing.T) {
	err := NewResume(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    "http://127.0.0.1:1",
		SessionID: "dur-sess-paused-001",
		Output:    &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected unreachable error")
	}
	if !strings.Contains(err.Error(), "not reachable") {
		t.Fatalf("error = %q, want unreachable-host wording", err.Error())
	}
}

func TestResume_NoOpAlreadyRunningReturnsNoOpOutcome(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encodeLifecycleControlHTTPResponse(t, w, factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: "dur-sess-running-001",
			Operation: factoryapi.FactorySessionLifecycleControlKindResume,
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeNoOp,
			Status:    factoryapi.FactorySessionDurableLifecycleStatusRunning,
		})
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewResume(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    srv.URL,
		SessionID: "dur-sess-running-001",
		JSON:      true,
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Resume no-op: %v", err)
	}

	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("outcome = %q, want NO_OP", response.Outcome)
	}
}

func TestLifecycleControl_OutcomesDistinguishableWithoutHTTPStatusText(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		response factoryapi.FactorySessionLifecycleControlResponse
		wantErr  bool
	}{
		{
			name:     "accepted",
			status:   http.StatusOK,
			response: samplePauseAcceptedResponse("dur-sess-001"),
			wantErr:  false,
		},
		{
			name:   "no-op",
			status: http.StatusOK,
			response: factoryapi.FactorySessionLifecycleControlResponse{
				SessionId: "dur-sess-001",
				Operation: factoryapi.FactorySessionLifecycleControlKindPause,
				Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeNoOp,
				Status:    factoryapi.FactorySessionDurableLifecycleStatusPaused,
			},
			wantErr: false,
		},
		{
			name:   "invalid-state",
			status: http.StatusConflict,
			response: factoryapi.FactorySessionLifecycleControlResponse{
				SessionId: "dur-sess-001",
				Operation: factoryapi.FactorySessionLifecycleControlKindPause,
				Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeInvalidState,
				Status:    factoryapi.FactorySessionDurableLifecycleStatusAwaitingApproval,
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				if err := json.NewEncoder(w).Encode(tc.response); err != nil {
					t.Fatalf("encode response: %v", err)
				}
			}))
			defer srv.Close()

			var out bytes.Buffer
			err := NewPause(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
				Server:    srv.URL,
				SessionID: "dur-sess-001",
				JSON:      true,
				Output:    &out,
			})
			if tc.wantErr {
				var rejected *LifecycleControlRejectedError
				if !errors.As(err, &rejected) {
					t.Fatalf("error = %v, want LifecycleControlRejectedError", err)
				}
				if rejected.Response.Outcome != tc.response.Outcome {
					t.Fatalf("rejected outcome = %q, want %q", rejected.Response.Outcome, tc.response.Outcome)
				}
			} else if err != nil {
				t.Fatalf("Pause: %v", err)
			}

			var decoded factoryapi.FactorySessionLifecycleControlResponse
			if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
				t.Fatalf("decode JSON: %v", err)
			}
			if decoded.Outcome != tc.response.Outcome {
				t.Fatalf("stdout outcome = %q, want %q", decoded.Outcome, tc.response.Outcome)
			}
		})
	}
}

func samplePauseAcceptedResponse(sessionID string) factoryapi.FactorySessionLifecycleControlResponse {
	return factoryapi.FactorySessionLifecycleControlResponse{
		SessionId: sessionID,
		Operation: factoryapi.FactorySessionLifecycleControlKindPause,
		Outcome:   factoryapi.FactorySessionLifecycleControlOutcomeAccepted,
		Status:    factoryapi.FactorySessionDurableLifecycleStatusPaused,
	}
}

func encodeLifecycleControlHTTPResponse(
	t *testing.T,
	w http.ResponseWriter,
	response factoryapi.FactorySessionLifecycleControlResponse,
) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func lifecycleControlStringPtr(value string) *string {
	return &value
}

func TestNewLocalLifecycleControlsUsesLiveStopCapability(t *testing.T) {
	t.Parallel()

	const sessionID = "session-live-stop"
	root := &localLifecycleServiceStub{}
	controls := NewLocalLifecycleControls(root)
	if controls == nil {
		t.Fatal("NewLocalLifecycleControls = nil, want local controls")
	}

	assertLocalLifecycleControl(t, sessionID,
		factoryapi.FactorySessionLifecycleControlKindCancel, "cancel-1", "operator cancel",
		controls.Cancel, &root.liveCancelCalls, &root.durableCancelCalls, &root.lastCancel)
	assertLocalLifecycleControl(t, sessionID,
		factoryapi.FactorySessionLifecycleControlKindTerminate, "terminate-1", "operator terminate",
		controls.Terminate, &root.liveTerminateCalls, &root.durableTerminateCalls, &root.lastTerminate)
}

func assertLocalLifecycleControl(
	t *testing.T,
	sessionID string,
	kind factoryapi.FactorySessionLifecycleControlKind,
	requestID string,
	reason string,
	invoke func(LifecycleControlConfig) error,
	liveCalls *int,
	durableCalls *int,
	lastRequest *factorysessions.ControlRequest,
) {
	t.Helper()
	var output bytes.Buffer
	if err := invoke(LifecycleControlConfig{
		Context: context.Background(), SessionID: sessionID, RequestID: requestID,
		Reason: reason, JSON: true, Output: &output,
	}); err != nil {
		t.Fatalf("local %s: %v", kind, err)
	}
	if *liveCalls != 1 || *durableCalls != 0 || lastRequest.RequestID != requestID || lastRequest.Reason != reason {
		t.Fatalf("%s routing = live:%d durable:%d request:%#v, want live capability", kind, *liveCalls, *durableCalls, *lastRequest)
	}
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode local %s: %v", kind, err)
	}
	if response.Operation != kind || response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("local %s response = %#v, want accepted", kind, response)
	}
}

type localLifecycleServiceStub struct {
	factorysessions.Service
	liveCancelCalls       int
	liveTerminateCalls    int
	durableCancelCalls    int
	durableTerminateCalls int
	lastCancel            factorysessions.ControlRequest
	lastTerminate         factorysessions.ControlRequest
}

func (s *localLifecycleServiceStub) Cancel(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	s.durableCancelCalls++
	return s.liveResult(sessionID, factorysessions.LifecycleControlCancel), nil
}

func (s *localLifecycleServiceStub) Terminate(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	s.durableTerminateCalls++
	return s.liveResult(sessionID, factorysessions.LifecycleControlTerminate), nil
}

func (s *localLifecycleServiceStub) CancelLiveFactorySession(_ context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	s.liveCancelCalls++
	s.lastCancel = request
	return s.liveResult(sessionID, factorysessions.LifecycleControlCancel), nil
}

func (s *localLifecycleServiceStub) TerminateLiveFactorySession(_ context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	s.liveTerminateCalls++
	s.lastTerminate = request
	return s.liveResult(sessionID, factorysessions.LifecycleControlTerminate), nil
}

func (s *localLifecycleServiceStub) liveResult(sessionID string, operation factorysessions.LifecycleControlKind) factorysessions.LifecycleControlResult {
	return factorysessions.LifecycleControlResult{
		SessionID: sessionID,
		Operation: operation,
		Outcome:   factorysessions.LifecycleControlOutcomeAccepted,
		Status:    factorysessions.LifecycleStatusSucceeded,
	}
}

var _ factorysessions.Service = (*localLifecycleServiceStub)(nil)
var _ factorysessions.LiveLifecycleControlService = (*localLifecycleServiceStub)(nil)

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
