package work

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestMove_HumanOutputIncludesMoveSummary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/factory-sessions/~default/work/work-move-1":
			writeJSON(t, w, factoryapi.Work{
				WorkId: stringPtr("work-move-1"),
				State:  &factoryapi.WorkState{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/factory-sessions/~default/work/work-move-1/move":
			var req factoryapi.MoveWorkRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode move body: %v", err)
			}
			if req.StateName != "complete" || req.RequestId != nil {
				t.Fatalf("move request = %#v, want stateName complete without requestId", req)
			}
			writeJSON(t, w, factoryapi.Work{
				WorkId: stringPtr("work-move-1"),
				State:  &factoryapi.WorkState{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
			})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Move(MoveConfig{
		Server:    serverBase(t, srv),
		WorkID:    "work-move-1",
		StateName: "complete",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}

	want := "" +
		"Work ID:\twork-move-1\n" +
		"Previous state:\tinit\n" +
		"New state:\tcomplete\n" +
		"Session ID:\t~default\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestMove_JSONOutputEmitsStableEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			writeJSON(t, w, factoryapi.Work{
				WorkId: stringPtr("work-move-1"),
				State:  &factoryapi.WorkState{Name: "failed", Type: factoryapi.WorkStateTypeFAILED},
			})
		case r.Method == http.MethodPost:
			writeJSON(t, w, factoryapi.Work{
				WorkId: stringPtr("work-move-1"),
				State:  &factoryapi.WorkState{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
			})
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Move(MoveConfig{
		Server:    serverBase(t, srv),
		WorkID:    "work-move-1",
		StateName: "init",
		JSON:      true,
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}

	var result MoveSuccessResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out.String())
	}
	if result.WorkID != "work-move-1" || result.PreviousState != "failed" || result.NewState != "init" {
		t.Fatalf("result = %#v, want work-move-1 failed -> init", result)
	}
	if result.SessionID != "~default" || result.EndpointPath != "/factory-sessions/~default/work/work-move-1/move" {
		t.Fatalf("result = %#v, want default session and move endpoint path", result)
	}
}

func TestMove_SessionScopedRouteUsesFactorySessionPath(t *testing.T) {
	var gotMovePath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			gotMovePath = r.URL.Path
		}
		writeJSON(t, w, factoryapi.Work{
			WorkId: stringPtr("work-1"),
			State:  &factoryapi.WorkState{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
		})
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Move(MoveConfig{
		Server:    serverBase(t, srv),
		SessionID: "session-beta",
		WorkID:    "work-1",
		StateName: "complete",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if gotMovePath != "/factory-sessions/session-beta/work/work-1/move" {
		t.Fatalf("move path = %q, want /factory-sessions/session-beta/work/work-1/move", gotMovePath)
	}
}

func TestMove_InFlightDispatchReturnsClearError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSON(t, w, factoryapi.Work{
				WorkId: stringPtr("work-busy"),
				State:  &factoryapi.WorkState{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(t, w, factoryapi.ErrorResponse{
			Code:    factoryapi.BADREQUEST,
			Message: "work is in an active dispatch",
		})
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Move(MoveConfig{
		Server:    serverBase(t, srv),
		WorkID:    "work-busy",
		StateName: "complete",
		Output:    &out,
	})
	if err == nil {
		t.Fatal("expected in-flight dispatch error")
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on failure", out.String())
	}
	if !strings.Contains(err.Error(), "move work failed (400): work is in an active dispatch") {
		t.Fatalf("error = %q, want in-flight dispatch message", err.Error())
	}
}

func TestMove_Returns409ForDuplicateRequestId(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSON(t, w, factoryapi.Work{
				WorkId: stringPtr("work-dup"),
				State:  &factoryapi.WorkState{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		writeJSON(t, w, factoryapi.ErrorResponse{
			Code:    factoryapi.MOVEWORKREQUESTALREADYAPPLIED,
			Message: "Operator move request was already applied.",
		})
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Move(MoveConfig{
		Server:    serverBase(t, srv),
		WorkID:    "work-dup",
		StateName: "complete",
		RequestID: "move-req-1",
		Output:    &out,
	})
	if err == nil {
		t.Fatal("expected duplicate request id error")
	}
	if !strings.Contains(err.Error(), "move work failed (409)") {
		t.Fatalf("error = %q, want 409 conflict", err.Error())
	}
	if !strings.Contains(err.Error(), "already applied") {
		t.Fatalf("error = %q, want already applied message", err.Error())
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
