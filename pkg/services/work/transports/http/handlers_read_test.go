package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestListWorkBySessionId_EncodesFakeRootListResults(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		listWork: func(
			_ context.Context,
			sessionID string,
			options work.ListOptions,
		) (work.ListResult, error) {
			invoked = true
			if sessionID != "session-1" {
				t.Fatalf("sessionID = %q, want session-1", sessionID)
			}
			if options.WorkTypeName != "prd" {
				t.Fatalf("options = %#v, want workTypeName prd", options)
			}
			return work.ListResult{
				Results: []work.ReadModel{{
					CursorID:     "tok-1",
					WorkID:       "work-1",
					Name:         "Review PRD",
					WorkTypeName: "prd",
					State:        &work.State{Name: "init", Type: work.StateTypeInitial},
				}},
				MaxResults: 50,
			}, nil
		},
	})
	recorder := httptest.NewRecorder()
	workTypeName := factoryapi.WorkListWorkTypeName("prd")

	adapter.ListWorkBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/work", nil),
		"session-1",
		factoryapi.ListWorkBySessionIdParams{WorkTypeName: &workTypeName},
	)

	if !invoked {
		t.Fatal("ListWorkBySessionId must invoke the injected Work root")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d %s, want 200", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ListWorkResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(response.Results) != 1 || response.Results[0].Name != "Review PRD" {
		t.Fatalf("response = %#v, want one encoded work item", response)
	}
}

func TestListWorkBySessionId_RejectsInvalidQueryBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		listWork: func(context.Context, string, work.ListOptions) (work.ListResult, error) {
			invoked = true
			return work.ListResult{}, nil
		},
	})
	recorder := httptest.NewRecorder()
	invalid := factoryapi.StateType("RUNNING")

	adapter.ListWorkBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/work", nil),
		"session-1",
		factoryapi.ListWorkBySessionIdParams{StateType: &invalid},
	)

	if invoked {
		t.Fatal("invalid list query must be rejected before Work root invocation")
	}
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"BAD_REQUEST"`) {
		t.Fatalf("response = %d %s, want typed bad request", recorder.Code, recorder.Body.String())
	}
}

func TestListWorkBySessionId_MapsSessionNotFound(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{
		listWork: func(context.Context, string, work.ListOptions) (work.ListResult, error) {
			return work.ListResult{}, fmt.Errorf("%w: session-1", apisurface.ErrFactorySessionNotFound)
		},
	})
	recorder := httptest.NewRecorder()

	adapter.ListWorkBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/work", nil),
		"session-1",
		factoryapi.ListWorkBySessionIdParams{},
	)

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), `"code":"NOT_FOUND"`) {
		t.Fatalf("response = %d %s, want session not found", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "factory session not found") {
		return
	}
	t.Fatalf("response = %s, want factory session not found message", recorder.Body.String())
}

func TestGetWorkBySessionId_EncodesFakeRootReadModel(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		getWork: func(_ context.Context, sessionID, workID string) (work.ReadModel, error) {
			invoked = true
			if sessionID != "session-1" || workID != "work-1" {
				t.Fatalf("GetWork(%q, %q), want session-1/work-1", sessionID, workID)
			}
			return work.ReadModel{
				CursorID:     "tok-1",
				WorkID:       "work-1",
				Name:         "Review PRD",
				WorkTypeName: "prd",
				State:        &work.State{Name: "init", Type: work.StateTypeInitial},
			}, nil
		},
	})
	recorder := httptest.NewRecorder()

	adapter.GetWorkBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/work/work-1", nil),
		"session-1",
		"work-1",
	)

	if !invoked {
		t.Fatal("GetWorkBySessionId must invoke the injected Work root")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d %s, want 200", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.Work
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if response.Name != "Review PRD" || response.WorkId == nil || *response.WorkId != "work-1" {
		t.Fatalf("response = %#v, want encoded work read model", response)
	}
}

func TestGetWorkBySessionId_MapsWorkNotFound(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{
		getWork: func(context.Context, string, string) (work.ReadModel, error) {
			return work.ReadModel{}, work.ErrWorkNotFound
		},
	})
	recorder := httptest.NewRecorder()

	adapter.GetWorkBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/work/missing", nil),
		"session-1",
		"missing",
	)

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), `"code":"NOT_FOUND"`) {
		t.Fatalf("response = %d %s, want work not found", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "work not found") {
		t.Fatalf("response = %s, want work not found message", recorder.Body.String())
	}
}

func TestListWorkBySessionId_RejectsEmptySessionBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		listWork: func(context.Context, string, work.ListOptions) (work.ListResult, error) {
			invoked = true
			return work.ListResult{}, nil
		},
	})
	recorder := httptest.NewRecorder()

	adapter.ListWorkBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/%20/work", nil),
		"   ",
		factoryapi.ListWorkBySessionIdParams{},
	)

	if invoked {
		t.Fatal("empty session id must be rejected before Work root invocation")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s, want bad request", recorder.Code, recorder.Body.String())
	}
}

func TestListWorkBySessionId_MapsRootValidationError(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{
		listWork: func(context.Context, string, work.ListOptions) (work.ListResult, error) {
			return work.ListResult{}, &work.ValidationError{Field: "sortBy", Message: "sortBy must be state.type"}
		},
	})
	recorder := httptest.NewRecorder()

	adapter.ListWorkBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/work", nil),
		"session-1",
		factoryapi.ListWorkBySessionIdParams{},
	)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "sortBy must be state.type") {
		t.Fatalf("response = %d %s, want validation bad request", recorder.Code, recorder.Body.String())
	}
}

func TestListWorkBySessionId_MapsUnmappedRootFailureToInternalError(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{
		listWork: func(context.Context, string, work.ListOptions) (work.ListResult, error) {
			return work.ListResult{}, fmt.Errorf("unexpected list failure")
		},
	})
	recorder := httptest.NewRecorder()

	adapter.ListWorkBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/work", nil),
		"session-1",
		factoryapi.ListWorkBySessionIdParams{},
	)

	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("response = %d %s, want internal error", recorder.Code, recorder.Body.String())
	}
}

func TestGetWorkBySessionId_MapsUnmappedRootFailureToInternalError(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{
		getWork: func(context.Context, string, string) (work.ReadModel, error) {
			return work.ReadModel{}, fmt.Errorf("unexpected get failure")
		},
	})
	recorder := httptest.NewRecorder()

	adapter.GetWorkBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/work/work-1", nil),
		"session-1",
		"work-1",
	)

	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("response = %d %s, want internal error", recorder.Code, recorder.Body.String())
	}
}

func TestGetWorkBySessionId_MapsSessionNotFoundDistinctFromWorkNotFound(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{
		getWork: func(context.Context, string, string) (work.ReadModel, error) {
			return work.ReadModel{}, fmt.Errorf("%w: session-1", factorysessions.ErrSessionNotFound)
		},
	})
	recorder := httptest.NewRecorder()

	adapter.GetWorkBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/work/work-1", nil),
		"session-1",
		"work-1",
	)

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "factory session not found") {
		t.Fatalf("response = %d %s, want factory session not found", recorder.Code, recorder.Body.String())
	}
}

func TestGetWorkBySessionId_RejectsEmptyWorkIDBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	adapter := NewAdapter(&rootFake{
		getWork: func(context.Context, string, string) (work.ReadModel, error) {
			invoked = true
			return work.ReadModel{}, nil
		},
	})
	recorder := httptest.NewRecorder()

	adapter.GetWorkBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/work/%20", nil),
		"session-1",
		"   ",
	)

	if invoked {
		t.Fatal("empty work id must be rejected before Work root invocation")
	}
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"BAD_REQUEST"`) {
		t.Fatalf("response = %d %s, want typed bad request", recorder.Code, recorder.Body.String())
	}
}

func TestGetWorkBySessionId_DefaultFakeRootReturnsWorkNotFound(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	adapter := NewAdapter(&rootFake{})

	adapter.GetWorkBySessionId(
		recorder,
		httptest.NewRequest(http.MethodGet, "/factory-sessions/session-1/work/work-1", nil),
		"session-1",
		"work-1",
	)

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "work not found") {
		t.Fatalf("response = %d %s, want default fake-root work not found", recorder.Code, recorder.Body.String())
	}
}
