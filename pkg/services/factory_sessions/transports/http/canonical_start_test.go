package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		if got.Mode != factorysessions.SessionOperationModeDurable {
			t.Fatalf("%s mode = %q, want durable", name, got.Mode)
		}
		if got.Correlation.RequestID != "req-map-1" {
			t.Fatalf("%s requestID = %q, want req-map-1", name, got.Correlation.RequestID)
		}
		if got.Source.FactoryID != "factory-alpha" {
			t.Fatalf("%s source factory = %q", name, got.Source.FactoryID)
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
	if gotAsync.Synchronous {
		t.Fatal("async Synchronous = true, want false")
	}
	if !gotSync.Synchronous {
		t.Fatal("sync Synchronous = false, want true")
	}
}
