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

func TestShow_HumanOutputIncludesWorkSummary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory-sessions/~default/work/work-review-1" {
			t.Fatalf("path = %q, want /factory-sessions/~default/work/work-review-1", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.Work{
			Name:         "Review PRD",
			WorkId:       stringPtr("work-review-1"),
			WorkTypeName: stringPtr("story"),
			State: &factoryapi.WorkState{
				Name: "review",
				Type: factoryapi.WorkStateTypePROCESSING,
			},
			TraceId:                stringPtr("trace-legacy"),
			CurrentChainingTraceId: stringPtr("trace-chain-1"),
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Show(ShowConfig{
		Server: serverBase(t, srv),
		WorkID: "work-review-1",
		Output: &out,
	})
	if err != nil {
		t.Fatalf("Show: %v", err)
	}

	want := "" +
		"Work ID:\twork-review-1\n" +
		"Name:\tReview PRD\n" +
		"Work type:\tstory\n" +
		"State name:\treview\n" +
		"State type:\tPROCESSING\n" +
		"Trace:\ttrace-chain-1\n" +
		"Relations:\tnone\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestShow_JSONOutputEmitsWorkObject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.Work{
			Name:         "Plan feature",
			WorkId:       stringPtr("work-1"),
			WorkTypeName: stringPtr("story"),
			State: &factoryapi.WorkState{
				Name: "init",
				Type: factoryapi.WorkStateTypeINITIAL,
			},
			TraceId: stringPtr("trace-1"),
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Show(ShowConfig{
		Server: serverBase(t, srv),
		WorkID: "work-1",
		JSON:   true,
		Output: &out,
	})
	if err != nil {
		t.Fatalf("Show: %v", err)
	}

	var work factoryapi.Work
	if err := json.Unmarshal(out.Bytes(), &work); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out.String())
	}
	if stringValue(work.WorkId) != "work-1" || work.Name != "Plan feature" || stringValue(work.WorkTypeName) != "story" {
		t.Fatalf("work = %#v, want work-1 Plan feature story", work)
	}
	if work.State == nil || work.State.Name != "init" {
		t.Fatalf("state = %#v, want init", work.State)
	}
	if stringValue(work.TraceId) != "trace-1" {
		t.Fatalf("traceId = %q, want trace-1", stringValue(work.TraceId))
	}
}

func TestShow_NotFoundExitsWithClearError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		if err := json.NewEncoder(w).Encode(factoryapi.ErrorResponse{
			Code:    factoryapi.NOTFOUND,
			Message: "work not found",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Show(ShowConfig{
		Server: serverBase(t, srv),
		WorkID: "missing-work",
		Output: &out,
	})
	if err == nil {
		t.Fatal("expected error for missing work")
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on failure", out.String())
	}
	if !strings.Contains(err.Error(), `work "missing-work" not found`) {
		t.Fatalf("error = %q, want not-found message", err.Error())
	}
}

func TestShow_SessionScopedRouteUsesFactorySessionPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.Work{
			Name:         "Plan feature",
			WorkId:       stringPtr("work-1"),
			WorkTypeName: stringPtr("story"),
			TraceId:      stringPtr("trace-1"),
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Show(ShowConfig{
		Server:    serverBase(t, srv),
		SessionID: "session-beta",
		WorkID:    "work-1",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if gotPath != "/factory-sessions/session-beta/work/work-1" {
		t.Fatalf("path = %q, want /factory-sessions/session-beta/work/work-1", gotPath)
	}
}

func TestShow_PrimaryTraceFallsBackToTraceId(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.Work{
			Name:         "Plan feature",
			WorkId:       stringPtr("work-1"),
			WorkTypeName: stringPtr("story"),
			TraceId:      stringPtr("trace-only"),
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Show(ShowConfig{
		Server: serverBase(t, srv),
		WorkID: "work-1",
		Output: &out,
	})
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if !strings.Contains(out.String(), "Trace:\ttrace-only\n") {
		t.Fatalf("output = %q, want trace-only primary trace", out.String())
	}
}
