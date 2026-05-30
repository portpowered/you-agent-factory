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

func TestDelete_PerformsDELETEWithEscapedSessionPath(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Delete(DeleteConfig{
		Port:      serverPort(t, srv),
		SessionID: "session/beta",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("method = %s, want DELETE", gotMethod)
	}
	if gotPath != "/factory-sessions/session%2Fbeta" {
		t.Fatalf("path = %q, want /factory-sessions/session%%2Fbeta", gotPath)
	}
}

func TestDelete_Success204PrintsHumanConfirmation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Delete(DeleteConfig{
		Port:      serverPort(t, srv),
		SessionID: "session-beta",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := out.String(); got != "Closed factory session session-beta\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestDelete_Success204EmitsJSONConfirmation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Delete(DeleteConfig{
		Port:      serverPort(t, srv),
		SessionID: "session-beta",
		JSON:      true,
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var got DeleteResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out.String())
	}
	if got.SessionID != "session-beta" {
		t.Fatalf("sessionId = %q, want session-beta", got.SessionID)
	}
}

func TestDelete_NotFoundReturnsClearMessage(t *testing.T) {
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

	err := Delete(DeleteConfig{
		Port:      serverPort(t, srv),
		SessionID: "missing-session",
		Output:    ioDiscardWriter{t},
	})
	if err == nil {
		t.Fatal("expected not-found error")
	}
	if !strings.Contains(err.Error(), "factory session not found") {
		t.Fatalf("error = %q", err.Error())
	}
	if !strings.Contains(err.Error(), "missing-session") {
		t.Fatalf("error = %q, want session id in message", err.Error())
	}
}

func TestDelete_UnreachableServiceNamesEndpoint(t *testing.T) {
	var out bytes.Buffer
	err := Delete(DeleteConfig{
		Port:      1,
		SessionID: "session-beta",
		JSON:      true,
		Output:    &out,
	})
	if err == nil {
		t.Fatal("expected unreachable error")
	}
	want := "factory sessions endpoint not reachable at http://localhost:1/factory-sessions/session-beta"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q", err.Error())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty output when --json is set", out.String())
	}
}

func TestDelete_APIErrorSurfacesMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(factoryapi.ErrorResponse{
			Message: "failed to close factory session",
			Code:    "INTERNAL_ERROR",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	err := Delete(DeleteConfig{
		Port:      serverPort(t, srv),
		SessionID: "session-beta",
		Output:    ioDiscardWriter{t},
	})
	if err == nil {
		t.Fatal("expected close error")
	}
	if !strings.Contains(err.Error(), "close factory session failed (500): failed to close factory session") {
		t.Fatalf("error = %q", err.Error())
	}
}

type ioDiscardWriter struct {
	t *testing.T
}

func (w ioDiscardWriter) Write(p []byte) (int, error) {
	w.t.Helper()
	if len(p) > 0 {
		w.t.Fatalf("unexpected output: %s", string(p))
	}
	return len(p), nil
}

func TestDelete_RejectsMissingSessionID(t *testing.T) {
	err := Delete(DeleteConfig{Port: 8080, SessionID: "   "})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "session id is required") {
		t.Fatalf("error = %q", err.Error())
	}
}
