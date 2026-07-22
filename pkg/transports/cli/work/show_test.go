package work

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
			StopSummary: &factoryapi.FactoryStopSummary{
				SessionId:                "session-beta",
				StopKind:                 factoryapi.FactoryStopKind("BLOCKED"),
				WorkState:                stringPtr("story:blocked"),
				LatestResultSummary:      stringPtr("provider timeout"),
				SuggestedRecoverySurface: stringPtr("existing work repair, work move, or follow-up submission controls"),
				SuggestedRecoveryAction:  stringPtr("Inspect the blocked work \"Review PRD\" [work-review-1], then use the existing work repair, work move, or follow-up submission controls to unblock it."),
				LatestDispatch: &factoryapi.FactoryStopDispatchSummary{
					DispatchId:      "dispatch-review-1",
					Status:          factoryapi.FactoryDispatchStatusFAILED,
					DispatchKind:    factoryapi.FactoryDispatchKindPETRITRANSITION,
					WorkstationName: stringPtr("Review"),
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewShow(testHTTPProtocol(t))(ShowConfig{Context: context.Background(),
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
		"Relations:\tnone\n" +
		"Stop summary:\tkind=BLOCKED session=session-beta state=story:blocked\n" +
		"Stop dispatch:\tdispatch-review-1 status=FAILED kind=PETRI_TRANSITION workstation=Review\n" +
		"Stop result:\tprovider timeout\n" +
		"Recovery surface:\texisting work repair, work move, or follow-up submission controls\n" +
		"Recovery action:\tInspect the blocked work \"Review PRD\" [work-review-1], then use the existing work repair, work move, or follow-up submission controls to unblock it.\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestShow_HumanOutputIncludesInterruptedStopSummary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory-sessions/session-beta/work/work-review-1" {
			t.Fatalf("path = %q, want /factory-sessions/session-beta/work/work-review-1", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.Work{
			Name:         "Review child",
			WorkId:       stringPtr("work-review-1"),
			WorkTypeName: stringPtr("goal"),
			State: &factoryapi.WorkState{
				Name: "review",
				Type: factoryapi.WorkStateTypePROCESSING,
			},
			TraceId: stringPtr("trace-review-1"),
			StopSummary: &factoryapi.FactoryStopSummary{
				SessionId:                "session-beta",
				StopKind:                 factoryapi.FactoryStopKind("INTERRUPTED"),
				WorkId:                   stringPtr("work-review-1"),
				WorkState:                stringPtr("goal:review"),
				LatestResultSummary:      stringPtr("Dispatch interrupted while waiting for review output"),
				SuggestedRecoverySurface: stringPtr("existing dispatch retry, work repair, or session workflow controls"),
				SuggestedRecoveryAction:  stringPtr("Inspect the interrupted dispatch in Factory Session \"session-beta\", then use the existing retry, repair, or session workflow controls to continue recovery."),
				LatestDispatch: &factoryapi.FactoryStopDispatchSummary{
					DispatchId:      "dispatch-1",
					Status:          factoryapi.FactoryDispatchStatusINTERRUPTED,
					DispatchKind:    factoryapi.FactoryDispatchKindJAVASCRIPTAGENT,
					WorkstationName: stringPtr("review child"),
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewShow(testHTTPProtocol(t))(ShowConfig{Context: context.Background(),
		Server:    serverBase(t, srv),
		SessionID: "session-beta",
		WorkID:    "work-review-1",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Show: %v", err)
	}

	want := "" +
		"Work ID:\twork-review-1\n" +
		"Name:\tReview child\n" +
		"Work type:\tgoal\n" +
		"State name:\treview\n" +
		"State type:\tPROCESSING\n" +
		"Trace:\ttrace-review-1\n" +
		"Relations:\tnone\n" +
		"Stop summary:\tkind=INTERRUPTED session=session-beta state=goal:review\n" +
		"Stop dispatch:\tdispatch-1 status=INTERRUPTED kind=JAVASCRIPT_AGENT workstation=review child\n" +
		"Stop result:\tDispatch interrupted while waiting for review output\n" +
		"Recovery surface:\texisting dispatch retry, work repair, or session workflow controls\n" +
		"Recovery action:\tInspect the interrupted dispatch in Factory Session \"session-beta\", then use the existing retry, repair, or session workflow controls to continue recovery.\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestShow_HumanOutputIncludesPausedLifecycleStopSummary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory-sessions/session-paused/work/work-review-1" {
			t.Fatalf("path = %q, want /factory-sessions/session-paused/work/work-review-1", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		lifecycle := factoryapi.FactorySessionDurableLifecycleStatusPaused
		if err := json.NewEncoder(w).Encode(factoryapi.Work{
			Name:         "Review child",
			WorkId:       stringPtr("work-review-1"),
			WorkTypeName: stringPtr("goal"),
			State: &factoryapi.WorkState{
				Name: "review",
				Type: factoryapi.WorkStateTypePROCESSING,
			},
			TraceId: stringPtr("trace-review-1"),
			StopSummary: &factoryapi.FactoryStopSummary{
				SessionId:              "session-paused",
				StopKind:               factoryapi.FactoryStopKind("PAUSED"),
				WorkState:              stringPtr("goal:review"),
				SessionLifecycleStatus: &lifecycle,
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewShow(testHTTPProtocol(t))(ShowConfig{Context: context.Background(),
		Server:    serverBase(t, srv),
		SessionID: "session-paused",
		WorkID:    "work-review-1",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Show: %v", err)
	}

	want := "" +
		"Work ID:\twork-review-1\n" +
		"Name:\tReview child\n" +
		"Work type:\tgoal\n" +
		"State name:\treview\n" +
		"State type:\tPROCESSING\n" +
		"Trace:\ttrace-review-1\n" +
		"Relations:\tnone\n" +
		"Stop summary:\tkind=PAUSED session=session-paused state=goal:review lifecycle=PAUSED\n"
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
	err := NewShow(testHTTPProtocol(t))(ShowConfig{Context: context.Background(),
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
			Code:    factoryapi.ErrorResponseCodeNOTFOUND,
			Message: "work not found",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewShow(testHTTPProtocol(t))(ShowConfig{Context: context.Background(),
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
		gotPath = r.URL.EscapedPath()
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
	err := NewShow(testHTTPProtocol(t))(ShowConfig{Context: context.Background(),
		Server:    serverBase(t, srv),
		SessionID: "session/beta",
		WorkID:    "work/review",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if gotPath != "/factory-sessions/session%2Fbeta/work/work%2Freview" {
		t.Fatalf("path = %q, want slash-sensitive identifiers escaped as segments", gotPath)
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
	err := NewShow(testHTTPProtocol(t))(ShowConfig{Context: context.Background(),
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
