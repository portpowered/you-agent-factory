package support_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestWaitForSessionTerminalStatusUsesExactSessionAndReturnsWithoutStabilityDelay(t *testing.T) {
	const sessionID = "session with identity"
	canceled := factoryapi.FactorySessionDurableLifecycleStatusCanceled
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory-sessions/session with identity/status" {
			t.Errorf("status path = %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.StatusResponse{
			Categories:             factoryapi.StatusCategories{Failed: 1},
			LifecycleControlStatus: &canceled,
			RuntimeStatus:          "IDLE",
			TotalTokens:            1,
		})
	}))
	defer server.Close()

	started := time.Now()
	status := support.WaitForSessionTerminalStatus(t, server.URL, sessionID, time.Second)
	if elapsed := time.Since(started); elapsed >= 200*time.Millisecond {
		t.Fatalf("terminal observation took %s, want no 300ms stability delay", elapsed)
	}
	if status.LifecycleControlStatus == nil ||
		*status.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusCanceled {
		t.Fatalf("terminal lifecycle status = %#v, want CANCELED", status.LifecycleControlStatus)
	}
}

func TestWaitForSessionTerminalStatusRejectsTransientActiveGap(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		request := requests.Add(1)
		status := factoryapi.StatusResponse{
			Categories:    factoryapi.StatusCategories{Terminal: 1},
			RuntimeStatus: "ACTIVE",
			TotalTokens:   1,
		}
		if request == 2 {
			status.Categories = factoryapi.StatusCategories{Processing: 1}
		}
		if request >= 3 {
			status.RuntimeStatus = "IDLE"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}))
	defer server.Close()

	status := support.WaitForSessionTerminalStatus(t, server.URL, "session-transient", time.Second)
	if requests.Load() < 3 {
		t.Fatalf("status requests = %d, accepted transient active gap", requests.Load())
	}
	if status.RuntimeStatus != "IDLE" || status.Categories.Terminal != 1 {
		t.Fatalf("terminal status = %#v", status)
	}
}

func TestUpsertDefaultSessionWorkRequest_PostsGeneratedWorkRequest(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotBody   []byte
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.UpsertWorkRequestResponse{
			RequestId: "request-public",
			TraceId:   "trace-public",
			Works: []factoryapi.UpsertWorkRequestSubmittedWork{{
				Name:         "work-public",
				WorkId:       "work-public",
				WorkTypeName: "task",
			}},
		})
	}))
	defer server.Close()

	workID := "work-public"
	workType := "task"
	works := []factoryapi.Work{{
		Name:         "work-public",
		WorkId:       &workID,
		WorkTypeName: &workType,
		Payload:      map[string]any{"title": "public contract"},
	}}
	support.UpsertDefaultSessionWorkRequest(t, server.URL, factoryapi.WorkRequest{
		RequestId: "request-public",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works:     &works,
	})

	if gotMethod != http.MethodPut {
		t.Fatalf("method = %q, want PUT", gotMethod)
	}
	if !strings.Contains(gotPath, "/work-requests/request-public") {
		t.Fatalf("path = %q, want session work-requests path", gotPath)
	}
	var decoded factoryapi.WorkRequest
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatalf("decode posted body: %v body=%s", err, gotBody)
	}
	if decoded.RequestId != "request-public" {
		t.Fatalf("posted requestId = %q, want request-public", decoded.RequestId)
	}
	if decoded.Type != factoryapi.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("posted type = %q, want FACTORY_REQUEST_BATCH", decoded.Type)
	}
	if decoded.Works == nil || len(*decoded.Works) != 1 {
		t.Fatalf("posted works = %#v, want one work item", decoded.Works)
	}
	if support.StringPointerValue((*decoded.Works)[0].WorkId) != "work-public" {
		t.Fatalf("posted workId = %#v, want work-public", (*decoded.Works)[0].WorkId)
	}
}
