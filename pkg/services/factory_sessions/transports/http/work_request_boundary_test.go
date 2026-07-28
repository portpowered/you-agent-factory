package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// TestWorkRequestBoundary_ConstructsAndSubmitsThroughWorkService proves Factory
// Sessions HTTP admission constructs and submits Work Requests only through the
// published work.Service PrepareWorkRequest and SubmitWorkRequestForSession
// contracts.
func TestWorkRequestBoundary_ConstructsAndSubmitsThroughWorkService(t *testing.T) {
	t.Parallel()

	recording := &recordingAdmissionWorkService{
		submitResult: work.WorkRequestSubmitResult{
			RequestID:    "request-1",
			TraceID:      "trace-1",
			Accepted:     true,
			WorkID:       "work-1",
			Name:         "draft-prd",
			WorkTypeName: "prd",
		},
	}
	server := NewHandler(Dependencies{WorkService: recording}, nil)

	name := "draft-prd"
	traceID := "trace-1"
	req := factoryapi.SubmitWorkBySessionIdJSONRequestBody{
		Name:         &name,
		WorkTypeName: "prd",
		TraceId:      &traceID,
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/factory-sessions/session-1/work", nil)
	server.submitWorkCore(
		response,
		request,
		req,
		nil,
		"session-1",
		func(ctx context.Context, workRequest work.WorkRequest) (work.WorkRequestSubmitResult, error) {
			return recording.SubmitWorkRequestForSession(ctx, "session-1", workRequest)
		},
	)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if recording.prepCalls != 1 {
		t.Fatalf("PrepareWorkRequest calls = %d, want 1", recording.prepCalls)
	}
	if recording.submitCalls != 1 {
		t.Fatalf("SubmitWorkRequestForSession calls = %d, want 1", recording.submitCalls)
	}
	if recording.lastSubmitSession != "session-1" {
		t.Fatalf("submit session = %q, want session-1", recording.lastSubmitSession)
	}
	if len(recording.lastSubmitRequest.Works) != 1 || recording.lastSubmitRequest.Works[0].WorkTypeID != "prd" {
		t.Fatalf("submitted request = %#v, want one prd work", recording.lastSubmitRequest)
	}

	var body factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.RequestId != "request-1" || body.TraceId != "trace-1" || !body.Accepted {
		t.Fatalf("response = %#v, want accepted request-1/trace-1", body)
	}
	if stringValue(body.WorkId) != "work-1" {
		t.Fatalf("workId = %q, want work-1", stringValue(body.WorkId))
	}
}

// TestWorkRequestBoundary_RejectsPreparedRequestThroughWorkService proves typed
// Work Request preparation failures from work.Service surface as observable HTTP
// rejections instead of bypassing the Work root.
func TestWorkRequestBoundary_RejectsPreparedRequestThroughWorkService(t *testing.T) {
	t.Parallel()

	recording := &recordingAdmissionWorkService{
		prepareRequestErr: &work.RequestPreparationError{Message: "works[0].name is required"},
	}
	server := NewHandler(Dependencies{WorkService: recording}, nil)

	name := "draft-prd"
	req := factoryapi.SubmitWorkBySessionIdJSONRequestBody{
		Name:         &name,
		WorkTypeName: "prd",
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/factory-sessions/session-1/work", nil)
	server.submitWorkCore(
		response,
		request,
		req,
		nil,
		"session-1",
		func(ctx context.Context, workRequest work.WorkRequest) (work.WorkRequestSubmitResult, error) {
			return recording.SubmitWorkRequestForSession(ctx, "session-1", workRequest)
		},
	)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	if recording.prepCalls != 1 {
		t.Fatalf("PrepareWorkRequest calls = %d, want 1", recording.prepCalls)
	}
	if recording.submitCalls != 0 {
		t.Fatalf("SubmitWorkRequestForSession calls = %d, want 0 after preparation failure", recording.submitCalls)
	}

	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != "BAD_REQUEST" {
		t.Fatalf("error code = %q, want BAD_REQUEST", body.Code)
	}
	if body.Message != "works[0].name is required" {
		t.Fatalf("error message = %q, want works[0].name is required", body.Message)
	}
}

// TestWorkRequestBoundary_RejectsAdmissionThroughWorkService proves typed Work
// admission failures from work.Service surface as observable HTTP rejections.
func TestWorkRequestBoundary_RejectsAdmissionThroughWorkService(t *testing.T) {
	t.Parallel()

	recording := &recordingAdmissionWorkService{
		submitErr: fmt.Errorf("work_request: invalid Work Request: %w", work.ErrInvalidWorkRequest),
	}
	server := NewHandler(Dependencies{WorkService: recording}, nil)

	name := "draft-prd"
	req := factoryapi.SubmitWorkBySessionIdJSONRequestBody{
		Name:         &name,
		WorkTypeName: "prd",
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/factory-sessions/session-1/work", nil)
	server.submitWorkCore(
		response,
		request,
		req,
		nil,
		"session-1",
		func(ctx context.Context, workRequest work.WorkRequest) (work.WorkRequestSubmitResult, error) {
			return recording.SubmitWorkRequestForSession(ctx, "session-1", workRequest)
		},
	)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	if recording.prepCalls != 1 {
		t.Fatalf("PrepareWorkRequest calls = %d, want 1", recording.prepCalls)
	}
	if recording.submitCalls != 1 {
		t.Fatalf("SubmitWorkRequestForSession calls = %d, want 1", recording.submitCalls)
	}
	if !errors.Is(recording.submitErr, work.ErrInvalidWorkRequest) {
		t.Fatalf("submit error = %v, want ErrInvalidWorkRequest", recording.submitErr)
	}

	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != "BAD_REQUEST" {
		t.Fatalf("error code = %q, want BAD_REQUEST", body.Code)
	}
	if body.Message == "" {
		t.Fatalf("error message = %q, want non-empty admission failure", body.Message)
	}
}
