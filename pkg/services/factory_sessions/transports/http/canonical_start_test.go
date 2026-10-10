package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	"go.uber.org/zap"
)

func TestHandlerFromRoot_CanonicalStartMapsArgsSourceWait(t *testing.T) {
	t.Parallel()

	var gotAsync, gotSync factorysessions.SessionStartRequest
	root := &httpSessionsRootFake{
		onStart: func(_ context.Context, req factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
			if req.Synchronous {
				gotSync = req
				return factorysessions.SessionStartResult{
					Mode:   factorysessions.SessionOperationModeDurable,
					Status: "SUCCEEDED",
					Sync: &factorysessions.SyncStartResult{
						AsyncStartResult: factorysessions.AsyncStartResult{SessionID: "dur-sess-sync-1", Status: "SUCCEEDED"},
						SyncOutcome:      "COMPLETED",
					},
				}, nil
			}
			gotAsync = req
			return factorysessions.SessionStartResult{
				Mode:   factorysessions.SessionOperationModeDurable,
				Status: "RUNNING",
				Async:  &factorysessions.AsyncStartResult{SessionID: "dur-sess-async-1", Status: "RUNNING"},
			}, nil
		},
	}
	handler := factorysessionshttp.NewLifecycleHandler(root, root, root, testRequestPreparation{}, zap.NewNop())
	body := `{"requestId":"req-map-1","source":{"kind":"FACTORY_ID","factoryId":"factory-alpha"},"args":{"branch":"main"},"requestedPolicy":{"priority":"high"},"wait":{"timeoutMillis":5000,"cancelOnTimeout":true}}`

	asyncRec := httptest.NewRecorder()
	handler.StartDurableFactorySessionAsync(asyncRec, httptest.NewRequest(http.MethodPost, "/factory-sessions/execution/async", bytes.NewBufferString(body)))
	if asyncRec.Code != http.StatusOK {
		t.Fatalf("async status = %d, want 200: %s", asyncRec.Code, asyncRec.Body.String())
	}
	var asyncResp map[string]any
	if err := json.Unmarshal(asyncRec.Body.Bytes(), &asyncResp); err != nil {
		t.Fatalf("decode async response: %v", err)
	}

	syncRec := httptest.NewRecorder()
	handler.StartDurableFactorySessionSync(syncRec, httptest.NewRequest(http.MethodPost, "/factory-sessions/execution/sync", bytes.NewBufferString(body)))
	if syncRec.Code != http.StatusOK {
		t.Fatalf("sync status = %d, want 200: %s", syncRec.Code, syncRec.Body.String())
	}

	for name, got := range map[string]factorysessions.SessionStartRequest{"async": gotAsync, "sync": gotSync} {
		assertCanonicalDurableStartRequest(t, name, got)
	}
	if gotAsync.Synchronous {
		t.Fatal("async Synchronous = true, want false")
	}
	if !gotSync.Synchronous {
		t.Fatal("sync Synchronous = false, want true")
	}
}

func assertCanonicalDurableStartRequest(t *testing.T, name string, got factorysessions.SessionStartRequest) {
	t.Helper()
	if got.Mode != factorysessions.SessionOperationModeDurable {
		t.Fatalf("%s mode = %q, want durable", name, got.Mode)
	}
	if got.Correlation.RequestID != "req-map-1" {
		t.Fatalf("%s requestID = %q, want req-map-1", name, got.Correlation.RequestID)
	}
	if got.Source.FactoryID != "factory-alpha" {
		t.Fatalf("%s source factory = %q", name, got.Source.FactoryID)
	}
	if got.FolderPath != "/test-factory" {
		t.Fatalf("%s project root = %q, want current Factory directory", name, got.FolderPath)
	}
	if got.Args["branch"] != "main" {
		t.Fatalf("%s args = %#v, want branch=main", name, got.Args)
	}
	if got.Policy["priority"] != "high" {
		t.Fatalf("%s policy = %#v", name, got.Policy)
	}
	if got.Wait.TimeoutMillis != 5000 || !got.Wait.CancelOnTimeout {
		t.Fatalf("%s wait = %+v, want 5000/cancel", name, got.Wait)
	}
}

type explicitHostedSessionRoot struct {
	*httpSessionsRootFake
	live []factorysessions.SessionView
}

func (root *explicitHostedSessionRoot) Get(context.Context, factorysessions.SessionGetRequest) (factorysessions.SessionGetResult, error) {
	return factorysessions.SessionGetResult{}, factorysessions.ErrSessionNotFound
}

func (root *explicitHostedSessionRoot) List(_ context.Context, request factorysessions.SessionListRequest) (factorysessions.SessionListResult, error) {
	if request.Mode != factorysessions.SessionOperationModeLive {
		return factorysessions.SessionListResult{}, factorysessions.ErrSessionNotFound
	}
	return factorysessions.SessionListResult{Mode: request.Mode, Sessions: root.live}, nil
}

func TestDurableStartUsesSoleExplicitHostedSessionProjectRoot(t *testing.T) {
	t.Parallel()
	root := &explicitHostedSessionRoot{
		httpSessionsRootFake: &httpSessionsRootFake{onStart: func(_ context.Context, request factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
			if request.FolderPath != "/selected-factory" {
				t.Fatalf("durable project root = %q, want selected live Factory", request.FolderPath)
			}
			return factorysessions.SessionStartResult{Async: &factorysessions.AsyncStartResult{SessionID: "durable-1", Status: "RUNNING"}}, nil
		}},
		live: []factorysessions.SessionView{{SessionID: "explicit-1", FactoryDir: "/selected-factory"}},
	}
	handler := factorysessionshttp.NewLifecycleHandler(root, root, root, testRequestPreparation{}, zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.StartDurableFactorySessionAsync(recorder, httptest.NewRequest(http.MethodPost, "/factory-sessions/async", bytes.NewBufferString(`{"requestId":"durable-1","source":{"kind":"FACTORY_ID","factoryId":"factory-alpha"}}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
}

func TestDurableStartRejectsAmbiguousExplicitHostedSessions(t *testing.T) {
	t.Parallel()
	root := &explicitHostedSessionRoot{
		httpSessionsRootFake: &httpSessionsRootFake{onStart: func(context.Context, factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
			t.Fatal("ambiguous project root must not start durable execution")
			return factorysessions.SessionStartResult{}, nil
		}},
		live: []factorysessions.SessionView{{SessionID: "explicit-1", FactoryDir: "/first"}, {SessionID: "explicit-2", FactoryDir: "/second"}},
	}
	handler := factorysessionshttp.NewLifecycleHandler(root, root, root, testRequestPreparation{}, zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.StartDurableFactorySessionAsync(recorder, httptest.NewRequest(http.MethodPost, "/factory-sessions/async", bytes.NewBufferString(`{"requestId":"durable-1","source":{"kind":"FACTORY_ID","factoryId":"factory-alpha"}}`)))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "current Factory Session is ambiguous") {
		t.Fatalf("status = %d, body = %s, want actionable ambiguous selection", recorder.Code, recorder.Body.String())
	}
}
