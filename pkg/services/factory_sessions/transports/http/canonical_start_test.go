package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
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
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
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
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.StartDurableFactorySessionAsync(recorder, httptest.NewRequest(http.MethodPost, "/factory-sessions/async", bytes.NewBufferString(`{"requestId":"durable-1","source":{"kind":"FACTORY_ID","factoryId":"factory-alpha"}}`)))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "current Factory Session is ambiguous") {
		t.Fatalf("status = %d, body = %s, want actionable ambiguous selection", recorder.Code, recorder.Body.String())
	}
}

// This adapter proves header translation and refusal, with owner admission
// controlled separately from the live-owner tests.
func TestFactoryHTTPStartCaller(t *testing.T) {
	t.Parallel()
	for _, route := range []string{"open", "async", "sync"} {
		for _, variant := range []string{"valid", "absent", "partial", "malformed", "owner-refused"} {
			t.Run(route+"/"+variant, func(t *testing.T) {
				t.Parallel()
				runFactoryHTTPStartCaller(t, route, variant)
			})
		}
	}
}

func runFactoryHTTPStartCaller(t *testing.T, route, variant string) {
	t.Helper()

	calls := 0
	token := strings.Repeat("A", 43)
	root := factoryHTTPCallerStartRoot(t, variant, token, &calls)
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	body := `{"requestId":"selected-request","source":{"kind":"FACTORY_ID","factoryId":"factory-alpha"}}`
	if route == "open" {
		body = `{"folderPath":"/workspace"}`
	}
	req := httptest.NewRequest(http.MethodPost, "/factory-sessions", strings.NewReader(body))
	if variant != "absent" {
		req.Header.Set("X-You-Worker-Session-Id", "exact/caller")
	}
	if variant != "absent" && variant != "partial" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if variant == "malformed" {
		req.Header.Add("X-You-Worker-Session-Id", "peer")
	}
	recorder := httptest.NewRecorder()
	switch route {
	case "open":
		handler.OpenFactorySession(recorder, req)
	case "async":
		handler.StartDurableFactorySessionAsync(recorder, req)
	case "sync":
		handler.StartDurableFactorySessionSync(recorder, req)
	}
	assertFactoryHTTPCallerStartResponse(t, recorder, variant, token, calls)
}

func factoryHTTPCallerStartRoot(t *testing.T, variant, token string, calls *int) *httpSessionsRootFake {
	t.Helper()

	root := &httpSessionsRootFake{onStart: func(_ context.Context, req factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
		(*calls)++
		if variant == "absent" {
			if req.Caller != nil {
				t.Fatal("absent credentials acquired caller")
			}
		} else if req.Caller == nil || req.Caller.WorkerSessionID != "exact/caller" || req.Caller.Token != token {
			t.Fatal("caller pair changed at owner boundary")
		}
		encoded, err := json.Marshal(req)
		if err != nil || strings.Contains(string(encoded), token) || strings.Contains(string(encoded), "exact/caller") {
			t.Fatal("start serialized credentials")
		}
		if variant == "owner-refused" {
			return factorysessions.SessionStartResult{}, fmt.Errorf("owner refusal: %w", workersessions.ErrCallerInvalid)
		}
		return factorysessions.SessionStartResult{
			Live:  &factorysessions.SessionOpenResult{SessionID: "selected"},
			Async: &factorysessions.AsyncStartResult{SessionID: "selected", Status: "RUNNING"},
			Sync:  &factorysessions.SyncStartResult{AsyncStartResult: factorysessions.AsyncStartResult{SessionID: "selected", Status: "SUCCEEDED"}, SyncOutcome: "COMPLETED"},
		}, nil
	}}
	return root
}

func assertFactoryHTTPCallerStartResponse(t *testing.T, recorder *httptest.ResponseRecorder, variant, token string, calls int) {
	t.Helper()

	wantStatus, wantCalls := http.StatusOK, 1
	if variant == "partial" || variant == "malformed" {
		wantStatus, wantCalls = http.StatusForbidden, 0
	}
	if variant == "owner-refused" {
		wantStatus = http.StatusForbidden
	}
	if recorder.Code != wantStatus || calls != wantCalls {
		t.Fatalf("status=%d calls=%d; want %d/%d", recorder.Code, calls, wantStatus, wantCalls)
	}
	if strings.Contains(recorder.Body.String(), token) || strings.Contains(recorder.Body.String(), "exact/caller") {
		t.Fatal("response disclosed credentials")
	}
	if wantStatus == http.StatusForbidden {
		var response factoryapi.ErrorResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Code != factoryapi.ErrorResponseCodeWORKERSESSIONCALLERINVALID {
			t.Fatal("caller refusal lost typed code")
		}
	}
}
