package admission_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// CLI and canonical HTTP parity here protect the selected Work REST error
// contract. One root process owns the server and all explicit session reads.
func runSelectedWorkReadJourneys(t *testing.T, server *support.FunctionalAPIServer) {
	_, sessionID := openSubmissionSession(t, server)
	_, emptyID := openSubmissionSession(t, server)
	submitted := postWorkViaRESTAPI(t, server.URL(), sessionID)
	workID := support.StringPointerValue(submitted.WorkId)
	submissionWaitForWorkIDsComplete(t, server.URL(), []string{workID}, 30*time.Second, sessionID)
	if !t.Run("Cases", func(t *testing.T) {
		t.Run("READ1Known", func(t *testing.T) {
			t.Parallel()
			assertSelectedReadSuccess(t, server, sessionID, "", workID)
			assertSelectedReadSuccess(t, server, sessionID, workID, workID)
		})
		t.Run("READ2AbsentSession", func(t *testing.T) {
			t.Parallel()
			for _, id := range []string{"", workID} {
				assertSelectedReadAbsence(t, server, "absent-selected-session", id, "factory session not found")
			}
			assertSelectedReadSuccess(t, server, sessionID, workID, workID)
		})
		t.Run("READ3EmptyAndAbsentWork", func(t *testing.T) {
			t.Parallel()
			assertSelectedReadSuccess(t, server, emptyID, "", "")
			assertSelectedReadAbsence(t, server, emptyID, "unknown-work", "work not found")
		})
		t.Run("READ4UnavailableTransport", func(t *testing.T) {
			t.Parallel()
			// A closed scenario-owned endpoint is a real unavailable network
			// edge, rather than an invented owner error. No peer listener closes.
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			unavailable := "http://" + listener.Addr().String()
			if err := listener.Close(); err != nil {
				t.Fatal(err)
			}
			inputs := selectedReadInputs(t, t.Context(), unavailable, sessionID, "")
			if err := server.Execute(t, inputs.Input); err == nil || inputs.Stdout() != "" || strings.Contains(inputs.Stderr(), "NOT_FOUND") {
				t.Fatalf("unavailable read = %v, stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
			}
			assertSelectedReadSuccess(t, server, sessionID, workID, workID)
		})
		t.Run("READ5Canceled", func(t *testing.T) {
			t.Parallel()
			// The owned server is ready and the selected Work is retained;
			// cancel only this read invocation before dispatching its request.
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			inputs := selectedReadInputs(t, ctx, server.URL(), sessionID, "")
			if err := server.Execute(t, inputs.Input); !errors.Is(err, context.Canceled) || inputs.Stdout() != "" {
				t.Fatalf("canceled read = %v, stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
			}
			assertSelectedReadSuccess(t, server, sessionID, workID, workID)
		})
	}) {
		return
	}
}

func selectedReadInputs(t *testing.T, ctx context.Context, baseURL, sessionID, workID string) *support.CapturedInputs {
	t.Helper()
	args := []string{"you", "--server", baseURL, "--json", "work", "list", "--session", sessionID}
	if workID != "" {
		args = []string{"you", "--server", baseURL, "--json", "work", "show", workID, "--session", sessionID}
	}
	inputs := support.FakeInputs(ctx, args)
	home := t.TempDir()
	inputs.Input.Env = submissionActivationHomeEnvironment(home)
	inputs.Input.WorkingDirectory = home
	return inputs
}

func assertSelectedReadAbsence(t *testing.T, server *support.FunctionalAPIServer, sessionID, workID, message string) {
	t.Helper()
	status, body := selectedReadHTTP(t, server.URL(), sessionID, workID)
	if status != http.StatusNotFound || body["code"] != "NOT_FOUND" || body["message"] != message {
		t.Fatalf("selected HTTP absence = %d %v, want 404 NOT_FOUND %q", status, body, message)
	}
	inputs := selectedReadInputs(t, t.Context(), server.URL(), sessionID, workID)
	if err := server.Execute(t, inputs.Input); err == nil || inputs.Stdout() != "" || !strings.Contains(err.Error(), message) {
		t.Fatalf("selected CLI absence = %v stdout=%q stderr=%q, want %q", err, inputs.Stdout(), inputs.Stderr(), message)
	}
}

func assertSelectedReadSuccess(t *testing.T, server *support.FunctionalAPIServer, sessionID, id, wantID string) {
	t.Helper()
	status, body := selectedReadHTTP(t, server.URL(), sessionID, id)
	if status != http.StatusOK {
		t.Fatalf("selected HTTP read = %d %v", status, body)
	}
	inputs := selectedReadInputs(t, t.Context(), server.URL(), sessionID, id)
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("selected CLI read = %v: %s", err, inputs.Stderr())
	}
	var cliBody map[string]any
	if err := json.Unmarshal([]byte(inputs.Stdout()), &cliBody); err != nil {
		t.Fatalf("decode CLI read: %v: %s", err, inputs.Stdout())
	}
	if id != "" {
		if body["workId"] != wantID || cliBody["workId"] != wantID {
			t.Fatalf("selected identity: HTTP=%v CLI=%v", body, cliBody)
		}
		if stringJSON(body["state"]) != stringJSON(cliBody["state"]) || stringJSON(body["structuredResult"]) != stringJSON(cliBody["structuredResult"]) {
			t.Fatalf("selected state/result: HTTP=%v CLI=%v", body, cliBody)
		}
		return
	}
	assertSelectedListIdentity(t, body, wantID)
	assertSelectedListIdentity(t, cliBody, wantID)
}

func assertSelectedListIdentity(t *testing.T, result map[string]any, wantID string) {
	t.Helper()
	items, _ := result["results"].([]any)
	if wantID == "" {
		if len(items) != 0 {
			t.Fatalf("empty Session read = %v", result)
		}
		return
	}
	if len(items) != 1 || items[0].(map[string]any)["workId"] != wantID {
		t.Fatalf("selected Session list = %v, want only %s", result, wantID)
	}
}

func stringJSON(value any) string { data, _ := json.Marshal(value); return string(data) }

func selectedReadHTTP(t *testing.T, baseURL, sessionID, workID string) (int, map[string]any) {
	t.Helper()
	suffix := "/work"
	if workID != "" {
		suffix += "/" + workID
	}
	response, err := http.Get(support.SessionWorkURL(baseURL, sessionID, suffix))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, body
}
