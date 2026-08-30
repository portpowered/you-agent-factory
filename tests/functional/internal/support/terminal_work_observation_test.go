package support

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestWaitForSessionWorkCountDoesNotReturnBeforeDelayedAdmission(t *testing.T) {
	const sessionID = "delayed-admission"
	var listRequests atomic.Int32
	var releaseAdmission sync.Once
	admissionReleased := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/factory-sessions/" + sessionID + "/events":
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set(factorysessions.SessionEventStreamRetainedCountHeader, "0")
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Error("delayed-admission event server does not support flushing")
				return
			}
			writeEvent := func(event factoryapi.FactoryEvent) {
				payload, err := json.Marshal(event)
				if err != nil {
					t.Fatalf("marshal delayed-admission event: %v", err)
				}
				_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
				flusher.Flush()
			}
			writeEvent(factoryapi.FactoryEvent{
				Id:   "work-1-terminal",
				Type: factoryapi.FactoryEventTypeWorkStateChange,
			})
			select {
			case <-admissionReleased:
			case <-r.Context().Done():
				return
			}
			writeEvent(factoryapi.FactoryEvent{
				Id:   "work-2-admitted",
				Type: factoryapi.FactoryEventTypeWorkRequest,
			})
			<-r.Context().Done()
		case "/factory-sessions/" + sessionID + "/work":
			request := listRequests.Add(1)
			results := []factoryapi.Work{terminalTestWork("work-1")}
			if request > 1 {
				results = append(results, terminalTestWork("work-2"))
			}
			if request == 1 {
				releaseAdmission.Do(func() { close(admissionReleased) })
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{Results: results})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	WaitForSessionWorkCountTerminalFromFactoryEvents(t, server.URL, sessionID, 2, time.Second)
	if got := listRequests.Load(); got < 2 {
		t.Fatalf("Work projection reads = %d, want delayed admission read after the first terminal item", got)
	}
}

func terminalTestWork(workID string) factoryapi.Work {
	return factoryapi.Work{
		WorkId: &workID,
		State:  &factoryapi.WorkState{Type: factoryapi.WorkStateTypeTERMINAL},
	}
}
