package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestStageSubmitWorkFileBySessionId_EncodesFakeRootStageResult(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		stageContent: func(_ context.Context, request work.StageContentRequest) (work.StageContentResult, error) {
			invoked = true
			if request.ItemType != "image" || request.FileName != "ui.png" || request.MediaType != "image/png" {
				t.Fatalf("request = %#v, want image/ui.png", request)
			}
			if string(request.Content) != "png-bytes" {
				t.Fatalf("content = %q, want png-bytes", string(request.Content))
			}
			return work.StageContentResult{
				StagedFileRef: "staged://ui.png",
				FileName:      "ui.png",
				MediaType:     "image/png",
				URL:           "file://staged/ui.png",
			}, nil
		},
	})
	recorder := httptest.NewRecorder()
	body := fmt.Sprintf(`{
		"itemType":"image",
		"fileName":"ui.png",
		"mediaType":"image/png",
		"contentBase64":"%s"
	}`, base64.StdEncoding.EncodeToString([]byte("png-bytes")))

	adapter.StageSubmitWorkFileBySessionId(
		recorder,
		httptest.NewRequest(http.MethodPost, "/factory-sessions/session-1/work/staged-files", strings.NewReader(body)),
		"session-1",
	)

	if !invoked {
		t.Fatal("StageSubmitWorkFileBySessionId must invoke the injected Work root")
	}
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d %s, want 201", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.StageSubmitWorkFileResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if response.StagedFileRef != "staged://ui.png" || response.FileName != "ui.png" {
		t.Fatalf("response = %#v, want encoded stage result", response)
	}
}

func TestStageSubmitWorkFileBySessionId_RejectsInvalidDecodeBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		stageContent: func(context.Context, work.StageContentRequest) (work.StageContentResult, error) {
			invoked = true
			return work.StageContentResult{}, nil
		},
	})
	recorder := httptest.NewRecorder()

	adapter.StageSubmitWorkFileBySessionId(
		recorder,
		httptest.NewRequest(http.MethodPost, "/factory-sessions/session-1/work/staged-files", strings.NewReader(`{
			"itemType":"text",
			"fileName":"notes.txt",
			"mediaType":"text/plain",
			"contentBase64":"dGV4dA=="
		}`)),
		"session-1",
	)

	if invoked {
		t.Fatal("invalid stage request must be rejected before Work root invocation")
	}
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"BAD_REQUEST"`) {
		t.Fatalf("response = %d %s, want typed bad request", recorder.Code, recorder.Body.String())
	}
}

func TestSubmitWorkBySessionId_EncodesFakeRootAdmissionResult(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		submitWorkRequestForSession: func(_ context.Context, sessionID string, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
			invoked = true
			if sessionID != "session-1" {
				t.Fatalf("sessionID = %q, want session-1", sessionID)
			}
			if len(request.Works) != 1 || request.Works[0].WorkTypeID != "prd" {
				t.Fatalf("request = %#v, want one prd work item", request)
			}
			return work.WorkRequestSubmitResult{
				RequestID:    "request-1",
				TraceID:      "trace-1",
				WorkID:       "work-1",
				Name:         "draft-prd",
				WorkTypeName: "prd",
				Accepted:     true,
			}, nil
		},
	})
	recorder := httptest.NewRecorder()

	adapter.SubmitWorkBySessionId(
		recorder,
		httptest.NewRequest(http.MethodPost, "/factory-sessions/session-1/work", strings.NewReader(`{
			"name":"draft-prd",
			"workTypeName":"prd",
			"traceId":"trace-1",
			"payload":{"title":"Draft PRD"}
		}`)),
		"session-1",
	)

	if !invoked {
		t.Fatal("SubmitWorkBySessionId must invoke the injected Work root")
	}
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d %s, want 201", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.SubmitWorkResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if response.RequestId != "request-1" || response.TraceId != "trace-1" || !response.Accepted {
		t.Fatalf("response = %#v, want encoded submit result", response)
	}
	if response.WorkId == nil || *response.WorkId != "work-1" {
		t.Fatalf("response = %#v, want work id", response)
	}
}

func TestSubmitWorkBySessionId_PassesSessionDefaultWorkTypeToRoot(t *testing.T) {
	t.Parallel()

	var gotDefault string
	adapter := NewAdapter(&rootFake{
		prepareWorkRequest: func(_ context.Context, input work.WorkRequestPreparation) (work.WorkRequest, error) {
			gotDefault = input.DefaultWorkTypeID
			return input.Request, nil
		},
		submitWorkRequestForSession: func(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
			return work.WorkRequestSubmitResult{Accepted: true, RequestID: "request-1"}, nil
		},
	}).WithDefaultWorkTypeResolver(func(context.Context, string) (string, error) {
		return "default-task", nil
	})
	recorder := httptest.NewRecorder()

	adapter.SubmitWorkBySessionId(
		recorder,
		httptest.NewRequest(http.MethodPost, "/factory-sessions/session-1/work", strings.NewReader(`{"name":"draft"}`)),
		"session-1",
	)

	if recorder.Code != http.StatusCreated || gotDefault != "default-task" {
		t.Fatalf("status = %d, default work type = %q, want 201/default-task", recorder.Code, gotDefault)
	}
}

func TestSubmitWorkBySessionId_RejectsInvalidContentBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		submitWorkRequestForSession: func(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
			invoked = true
			return work.WorkRequestSubmitResult{}, nil
		},
	})
	recorder := httptest.NewRecorder()

	adapter.SubmitWorkBySessionId(
		recorder,
		httptest.NewRequest(http.MethodPost, "/factory-sessions/session-1/work", strings.NewReader(`{
			"workTypeName":"prd",
			"content":[{"type":"image","text":"wrong-field"}]
		}`)),
		"session-1",
	)

	if invoked {
		t.Fatal("invalid submit request must be rejected before Work root invocation")
	}
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"BAD_REQUEST"`) {
		t.Fatalf("response = %d %s, want typed bad request", recorder.Code, recorder.Body.String())
	}
}

func TestSubmitWorkBySessionId_MapsSessionNotFound(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{
		submitWorkRequestForSession: func(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
			return work.WorkRequestSubmitResult{}, fmt.Errorf("%w: session-1", apisurface.ErrFactorySessionNotFound)
		},
	})
	recorder := httptest.NewRecorder()

	adapter.SubmitWorkBySessionId(
		recorder,
		httptest.NewRequest(http.MethodPost, "/factory-sessions/session-1/work", strings.NewReader(`{
			"name":"draft-prd",
			"workTypeName":"prd"
		}`)),
		"session-1",
	)

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "factory session not found") {
		t.Fatalf("response = %d %s, want session not found", recorder.Code, recorder.Body.String())
	}
}

func TestUpsertWorkRequestBySessionId_EncodesFakeRootAdmissionResult(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		submitWorkRequestForSession: func(_ context.Context, sessionID string, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
			invoked = true
			if sessionID != "session-1" || request.RequestID != "request-1" {
				t.Fatalf("submit(%q, %#v), want session-1/request-1", sessionID, request)
			}
			return work.WorkRequestSubmitResult{
				RequestID: "request-1",
				TraceID:   "trace-1",
				Works: []work.WorkRequestSubmittedWork{{
					Name:         "draft",
					WorkTypeName: "prd",
					WorkID:       "work-1",
				}},
			}, nil
		},
	})
	recorder := httptest.NewRecorder()

	adapter.UpsertWorkRequestBySessionId(
		recorder,
		httptest.NewRequest(http.MethodPut, "/factory-sessions/session-1/work-requests/request-1", strings.NewReader(`{
			"requestId":"request-1",
			"type":"FACTORY_REQUEST_BATCH",
			"works":[{"name":"draft","workTypeName":"prd"}]
		}`)),
		"session-1",
		"request-1",
	)

	if !invoked {
		t.Fatal("UpsertWorkRequestBySessionId must invoke the injected Work root")
	}
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d %s, want 201", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.UpsertWorkRequestResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if response.RequestId != "request-1" || len(response.Works) != 1 || response.Works[0].WorkId != "work-1" {
		t.Fatalf("response = %#v, want encoded upsert result", response)
	}
}

func TestUpsertWorkRequestBySessionId_RejectsMismatchedRequestIDBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		submitWorkRequestForSession: func(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
			invoked = true
			return work.WorkRequestSubmitResult{}, nil
		},
	})
	recorder := httptest.NewRecorder()

	adapter.UpsertWorkRequestBySessionId(
		recorder,
		httptest.NewRequest(http.MethodPut, "/factory-sessions/session-1/work-requests/request-1", strings.NewReader(`{
			"requestId":"request-2",
			"type":"FACTORY_REQUEST_BATCH",
			"works":[{"name":"draft","workTypeName":"prd"}]
		}`)),
		"session-1",
		"request-1",
	)

	if invoked {
		t.Fatal("mismatched request id must be rejected before Work root invocation")
	}
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "request_id path and requestId body must match") {
		t.Fatalf("response = %d %s, want bad request", recorder.Code, recorder.Body.String())
	}
}

func TestUpsertWorkRequestBySessionId_RejectsInvalidDecodeBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		submitWorkRequestForSession: func(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
			invoked = true
			return work.WorkRequestSubmitResult{}, nil
		},
	})
	recorder := httptest.NewRecorder()

	adapter.UpsertWorkRequestBySessionId(
		recorder,
		httptest.NewRequest(http.MethodPut, "/factory-sessions/session-1/work-requests/request-1", strings.NewReader(`{
			"requestId":"request-1",
			"type":"FACTORY_REQUEST_BATCH",
			"works":[{"name":"draft","workTypeName":"prd","content":[{"type":"image","text":"wrong-field"}]}]
		}`)),
		"session-1",
		"request-1",
	)

	if invoked {
		t.Fatal("invalid upsert request must be rejected before Work root invocation")
	}
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"BAD_REQUEST"`) {
		t.Fatalf("response = %d %s, want typed bad request", recorder.Code, recorder.Body.String())
	}
}

func TestSubmitWorkBySessionId_UsesPrepareContentBeforeAdmission(t *testing.T) {
	t.Parallel()

	var prepared bool
	adapter := NewAdapter(&rootFake{
		prepareContent: func(_ context.Context, items []work.StagedSubmissionItem) ([]work.WorkContentPart, error) {
			prepared = true
			if len(items) != 1 || items[0].StagedFileRef != "staged://ui.png" {
				t.Fatalf("items = %#v, want staged image item", items)
			}
			return []work.WorkContentPart{{Type: work.WorkContentPartTypeImage, URL: "file://staged/ui.png"}}, nil
		},
		submitWorkRequestForSession: func(_ context.Context, _ string, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
			if len(request.Works) != 1 || len(request.Works[0].Content) != 1 {
				t.Fatalf("request = %#v, want prepared content on work item", request)
			}
			return work.WorkRequestSubmitResult{RequestID: "request-1", Accepted: true}, nil
		},
	})
	recorder := httptest.NewRecorder()

	adapter.SubmitWorkBySessionId(
		recorder,
		httptest.NewRequest(http.MethodPost, "/factory-sessions/session-1/work", strings.NewReader(`{
			"name":"draft-prd",
			"workTypeName":"prd",
			"items":[{"type":"image","url":"file://staged/ui.png","stagedFileRef":"staged://ui.png","fileName":"ui.png","mediaType":"image/png"}]
		}`)),
		"session-1",
	)

	if !prepared {
		t.Fatal("structured submit items must be prepared through the Work root before admission")
	}
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d %s, want 201", recorder.Code, recorder.Body.String())
	}
}

func TestSubmitWorkBySessionId_MapsAdmissionTypedFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "invalid",
			err:        work.ErrInvalidWorkRequest,
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name:       "conflict",
			err:        work.ErrWorkRequestConflict,
			wantStatus: http.StatusConflict,
			wantCode:   "CONFLICT",
		},
		{
			name:       "rejected",
			err:        work.ErrWorkRequestRejected,
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			adapter := NewAdapter(&rootFake{
				submitWorkRequestForSession: func(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
					return work.WorkRequestSubmitResult{}, testCase.err
				},
			})
			recorder := httptest.NewRecorder()

			adapter.SubmitWorkBySessionId(
				recorder,
				httptest.NewRequest(http.MethodPost, "/factory-sessions/session-1/work", strings.NewReader(`{
					"name":"draft-prd",
					"workTypeName":"prd"
				}`)),
				"session-1",
			)

			body := recorder.Body.String()
			if recorder.Code != testCase.wantStatus || !strings.Contains(body, `"code":"`+testCase.wantCode+`"`) {
				t.Fatalf("response = %d %s, want %d with code %s", recorder.Code, body, testCase.wantStatus, testCase.wantCode)
			}
			if strings.Contains(body, `"code":"INTERNAL_ERROR"`) {
				t.Fatalf("response = %s, must not collapse admission failure into internal error", body)
			}
		})
	}
}

func TestStageSubmitWorkFileBySessionId_MapsContentStagingFailure(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{
		stageContent: func(context.Context, work.StageContentRequest) (work.StageContentResult, error) {
			return work.StageContentResult{}, &work.ContentStagingError{Message: "staging unavailable"}
		},
	})
	recorder := httptest.NewRecorder()

	adapter.StageSubmitWorkFileBySessionId(
		recorder,
		httptest.NewRequest(http.MethodPost, "/factory-sessions/session-1/work/stage-submit-file", strings.NewReader(`{
			"itemType":"document",
			"contentBase64":"dGVzdA==",
			"fileName":"draft.txt",
			"mediaType":"text/plain"
		}`)),
		"session-1",
	)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "staging unavailable") {
		t.Fatalf("response = %d %s, want staging bad request", recorder.Code, recorder.Body.String())
	}
}
