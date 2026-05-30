package work

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestShow_HumanOutputIncludesWorkSummary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory-sessions/~default/work/tok-review-1" {
			t.Fatalf("path = %q, want /factory-sessions/~default/work/tok-review-1", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.TokenResponse{
			Id:                     "tok-review-1",
			PlaceId:                "story:review",
			WorkId:                 "work-review-1",
			WorkType:               "story",
			Name:                   stringPtr("Review PRD"),
			TraceId:                "trace-legacy",
			CurrentChainingTraceId: stringPtr("trace-chain-1"),
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Show(ShowConfig{
		Server: serverBase(t, srv),
		WorkID: "tok-review-1",
		Output: &out,
	})
	if err != nil {
		t.Fatalf("Show: %v", err)
	}

	want := "" +
		"WORK ID:\twork-review-1\n" +
		"NAME:\tReview PRD\n" +
		"WORK TYPE:\tstory\n" +
		"STATE NAME:\treview\n" +
		"STATE TYPE:\tPROCESSING\n" +
		"TRACE:\ttrace-chain-1\n" +
		"RELATIONS:\tnone\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestShow_JSONOutputEmitsWorkObject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.TokenResponse{
			Id:       "tok-1",
			PlaceId:  "story:init",
			WorkId:   "work-1",
			WorkType: "story",
			Name:     stringPtr("Plan feature"),
			TraceId:  "trace-1",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Show(ShowConfig{
		Server: serverBase(t, srv),
		WorkID: "tok-1",
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
			Message: "token not found",
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
	if !errors.Is(err, ErrWorkNotFound) {
		t.Fatalf("error = %v, want ErrWorkNotFound", err)
	}
}

func TestShow_SessionScopedRouteUsesFactorySessionPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.TokenResponse{
			Id:       "tok-1",
			PlaceId:  "story:init",
			WorkId:   "work-1",
			WorkType: "story",
			TraceId:  "trace-1",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Show(ShowConfig{
		Server:    serverBase(t, srv),
		SessionID: "session-beta",
		WorkID:    "tok-1",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if gotPath != "/factory-sessions/session-beta/work/tok-1" {
		t.Fatalf("path = %q, want /factory-sessions/session-beta/work/tok-1", gotPath)
	}
}

func TestShow_PrimaryTraceFallsBackToTraceId(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.TokenResponse{
			Id:       "tok-1",
			PlaceId:  "story:init",
			WorkId:   "work-1",
			WorkType: "story",
			TraceId:  "trace-only",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Show(ShowConfig{
		Server: serverBase(t, srv),
		WorkID: "tok-1",
		Output: &out,
	})
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if !strings.Contains(out.String(), "TRACE:\ttrace-only\n") {
		t.Fatalf("output = %q, want trace-only primary trace", out.String())
	}
}
