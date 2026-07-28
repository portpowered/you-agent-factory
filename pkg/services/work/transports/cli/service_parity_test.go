package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
	workcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestConstructedService_ListJSONOutputPreservesAcceptedEnvelope(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{{
				Name:         "Review PRD",
				WorkId:       stringPtr("work-1"),
				WorkTypeName: stringPtr("story"),
				State: &factoryapi.WorkState{
					Name: "review",
					Type: factoryapi.WorkStateTypePROCESSING,
				},
			}},
		})
	}))
	defer srv.Close()

	service := constructedWorkCLIService(t, testListRequestPreparation{}, nil)
	var out bytes.Buffer
	err := service.List(workcli.ListConfig{
		Context: context.Background(),
		Server:  strings.TrimSuffix(srv.URL, "/"),
		JSON:    true,
		Output:  &out,
		HTTP:    testHTTPProtocol(t),
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	var got factoryapi.ListWorkResponse
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json output is invalid: %v\n%s", err, out.String())
	}
	if len(got.Results) != 1 || stringValue(got.Results[0].WorkId) != "work-1" {
		t.Fatalf("results = %#v, want work-1", got.Results)
	}
	if bytes.Contains(out.Bytes(), []byte("WORK ID")) {
		t.Fatalf("json output included human-readable text: %q", out.String())
	}
}

func TestConstructedService_ListVerboseDiagnosticsOnHTTPFailure(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(factoryapi.ErrorResponse{
			Message: "service unavailable",
			Code:    "INTERNAL_ERROR",
		})
	}))
	defer srv.Close()

	service := constructedWorkCLIService(t, testListRequestPreparation{}, nil)
	var out bytes.Buffer
	var diagnostics bytes.Buffer
	err := service.List(workcli.ListConfig{
		Context:     context.Background(),
		Server:      strings.TrimSuffix(srv.URL, "/"),
		Verbose:     true,
		Output:      &out,
		Diagnostics: &diagnostics,
		HTTP:        testHTTPProtocol(t),
	})
	if err == nil {
		t.Fatal("expected list failure")
	}
	if !strings.Contains(err.Error(), "list work failed (500): service unavailable") {
		t.Fatalf("error = %q, want HTTP rejection message", err.Error())
	}
	diag := diagnostics.String()
	if !strings.Contains(diag, "work list response") || !strings.Contains(diag, "status=500") {
		t.Fatalf("diagnostics missing failure status:\n%s", diag)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should stay empty on failure, got %q", out.String())
	}
}

func TestConstructedService_ListValidationFailurePreservesStateTypeMessage(t *testing.T) {
	t.Parallel()

	service := constructedWorkCLIService(t, workdomain.NewListRequestPreparation(), nil)
	var out bytes.Buffer
	err := service.List(workcli.ListConfig{
		Context:   context.Background(),
		StateType: "INVALID",
		Output:    &out,
		HTTP:      testHTTPProtocol(t),
	})
	if err == nil || err.Error() != "--state-type must be one of INITIAL, PROCESSING, TERMINAL, or FAILED" {
		t.Fatalf("error = %v, want state-type validation message", err)
	}
}

func TestConstructedService_ShowHumanOutputPreservesAcceptedSummary(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.Work{
			Name:         "Review PRD",
			WorkId:       stringPtr("work-review-1"),
			WorkTypeName: stringPtr("story"),
			State: &factoryapi.WorkState{
				Name: "review",
				Type: factoryapi.WorkStateTypePROCESSING,
			},
			CurrentChainingTraceId: stringPtr("trace-chain-1"),
		})
	}))
	defer srv.Close()

	service := constructedWorkCLIService(t, workdomain.NewListRequestPreparation(), nil)
	var out bytes.Buffer
	err := service.Show(workcli.ShowConfig{
		Context: context.Background(),
		Server:  strings.TrimSuffix(srv.URL, "/"),
		WorkID:  "work-review-1",
		Output:  &out,
		HTTP:    testHTTPProtocol(t),
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

func TestConstructedService_ShowJSONOutputEmitsWorkObject(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.Work{
			Name:         "Plan feature",
			WorkId:       stringPtr("work-1"),
			WorkTypeName: stringPtr("story"),
			State: &factoryapi.WorkState{
				Name: "init",
				Type: factoryapi.WorkStateTypeINITIAL,
			},
			TraceId: stringPtr("trace-1"),
		})
	}))
	defer srv.Close()

	service := constructedWorkCLIService(t, workdomain.NewListRequestPreparation(), nil)
	var out bytes.Buffer
	err := service.Show(workcli.ShowConfig{
		Context: context.Background(),
		Server:  strings.TrimSuffix(srv.URL, "/"),
		WorkID:  "work-1",
		JSON:    true,
		Output:  &out,
		HTTP:    testHTTPProtocol(t),
	})
	if err != nil {
		t.Fatalf("Show: %v", err)
	}

	var work factoryapi.Work
	if err := json.Unmarshal(out.Bytes(), &work); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out.String())
	}
	if stringValue(work.WorkId) != "work-1" || work.Name != "Plan feature" {
		t.Fatalf("work = %#v, want work-1 Plan feature", work)
	}
}

func TestConstructedService_ShowNotFoundPreservesExitRelevantError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(factoryapi.ErrorResponse{
			Code:    factoryapi.ErrorResponseCodeNOTFOUND,
			Message: "work not found",
		})
	}))
	defer srv.Close()

	service := constructedWorkCLIService(t, workdomain.NewListRequestPreparation(), nil)
	var out bytes.Buffer
	err := service.Show(workcli.ShowConfig{
		Context: context.Background(),
		Server:  strings.TrimSuffix(srv.URL, "/"),
		WorkID:  "missing-work",
		Output:  &out,
		HTTP:    testHTTPProtocol(t),
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

func TestConstructedService_MoveHumanOutputPreservesMoveSummary(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			writeJSONResponse(t, w, factoryapi.Work{
				WorkId: stringPtr("work-move-1"),
				State:  &factoryapi.WorkState{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
			})
		case r.Method == http.MethodPost:
			writeJSONResponse(t, w, factoryapi.Work{
				WorkId: stringPtr("work-move-1"),
				State:  &factoryapi.WorkState{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
			})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	service := constructedWorkCLIService(t, workdomain.NewListRequestPreparation(), nil)
	var out bytes.Buffer
	err := service.Move(workcli.MoveConfig{
		Context:   context.Background(),
		Server:    strings.TrimSuffix(srv.URL, "/"),
		WorkID:    "work-move-1",
		StateName: "complete",
		Output:    &out,
		HTTP:      testHTTPProtocol(t),
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

func TestConstructedService_MoveJSONOutputEmitsStableEnvelope(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			writeJSONResponse(t, w, factoryapi.Work{
				WorkId: stringPtr("work-move-1"),
				State:  &factoryapi.WorkState{Name: "failed", Type: factoryapi.WorkStateTypeFAILED},
			})
		case r.Method == http.MethodPost:
			writeJSONResponse(t, w, factoryapi.Work{
				WorkId: stringPtr("work-move-1"),
				State:  &factoryapi.WorkState{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
			})
		}
	}))
	defer srv.Close()

	service := constructedWorkCLIService(t, workdomain.NewListRequestPreparation(), nil)
	var out bytes.Buffer
	err := service.Move(workcli.MoveConfig{
		Context:   context.Background(),
		Server:    strings.TrimSuffix(srv.URL, "/"),
		WorkID:    "work-move-1",
		StateName: "init",
		JSON:      true,
		Output:    &out,
		HTTP:      testHTTPProtocol(t),
	})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}

	var result workcli.MoveSuccessResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out.String())
	}
	if result.WorkID != "work-move-1" || result.PreviousState != "failed" || result.NewState != "init" {
		t.Fatalf("result = %#v, want work-move-1 failed -> init", result)
	}
}

func TestConstructedService_MoveHTTPRejectionPreservesDiagnosticsAndError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSONResponse(t, w, factoryapi.Work{
				WorkId: stringPtr("work-busy"),
				State:  &factoryapi.WorkState{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		writeJSONResponse(t, w, factoryapi.ErrorResponse{
			Code:    factoryapi.ErrorResponseCodeBADREQUEST,
			Message: "work is in an active dispatch",
		})
	}))
	defer srv.Close()

	service := constructedWorkCLIService(t, workdomain.NewListRequestPreparation(), nil)
	var out bytes.Buffer
	var diagnostics bytes.Buffer
	err := service.Move(workcli.MoveConfig{
		Context:     context.Background(),
		Server:      strings.TrimSuffix(srv.URL, "/"),
		WorkID:        "work-busy",
		StateName:     "complete",
		Verbose:       true,
		Output:        &out,
		Diagnostics:   &diagnostics,
		HTTP:          testHTTPProtocol(t),
	})
	if err == nil {
		t.Fatal("expected in-flight dispatch error")
	}
	if !strings.Contains(err.Error(), "move work failed (400): work is in an active dispatch") {
		t.Fatalf("error = %q, want in-flight dispatch message", err.Error())
	}
	diag := diagnostics.String()
	if !strings.Contains(diag, "work move response") || !strings.Contains(diag, "status=400") {
		t.Fatalf("diagnostics missing failure status:\n%s", diag)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on failure", out.String())
	}
}

func TestConstructedService_VisualizeFailureLeavesOutputEmpty(t *testing.T) {
	t.Parallel()

	want := errors.New("invalid JSON")
	operation := func(workdomain.VisualizationRequest) (string, error) {
		return "", want
	}
	service := constructedWorkCLIService(t, workdomain.NewListRequestPreparation(), operation)
	var out bytes.Buffer
	err := service.Visualize(workcli.VisualizeConfig{
		BatchFile: "batch.json",
		Output:    &out,
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q, want empty", out.String())
	}
}

func TestConstructedService_ShowHonorsCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	service := constructedWorkCLIService(t, workdomain.NewListRequestPreparation(), nil)
	var out bytes.Buffer
	err := service.Show(workcli.ShowConfig{
		Context: ctx,
		Server:  "https://factory.example",
		WorkID:  "work-1",
		Output:  &out,
		HTTP:    testHTTPProtocol(t),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func writeJSONResponse(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
