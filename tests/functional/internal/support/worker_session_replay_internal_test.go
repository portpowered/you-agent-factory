package support

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestWaitForCompleteWorkerSessionReplayWaitsForTerminalRetention(t *testing.T) {
	var requests atomic.Int32
	secondRequest := make(chan struct{})
	releaseTerminal := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("replayOnly") != "true" {
			http.Error(w, "replayOnly is required", http.StatusBadRequest)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		writeEvent := func(event map[string]any) {
			payload, err := json.Marshal(event)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}

		switch requests.Add(1) {
		case 1:
			writeEvent(map[string]any{
				"delivery":        "RECORD",
				"workerSessionId": "worker-1",
				"event":           map[string]any{"position": 1},
			})
			writeEvent(map[string]any{
				"delivery":        "REPLAY_SUMMARY",
				"workerSessionId": "worker-1",
				"replaySummary": map[string]any{
					"kind":          "replay-summary",
					"complete":      false,
					"reason":        "session-terminal-record-missing",
					"eventsEmitted": 1,
				},
			})
		case 2:
			close(secondRequest)
			<-releaseTerminal
			writeEvent(map[string]any{
				"delivery":        "RECORD",
				"workerSessionId": "worker-1",
				"event":           map[string]any{"position": 1},
			})
			writeEvent(map[string]any{
				"delivery":        "TERMINAL_REPLAY",
				"workerSessionId": "worker-1",
				"event":           map[string]any{"position": 2},
			})
			writeEvent(map[string]any{
				"delivery":        "REPLAY_SUMMARY",
				"workerSessionId": "worker-1",
				"replaySummary": map[string]any{
					"kind":          "replay-summary",
					"complete":      true,
					"eventsEmitted": 2,
				},
			})
		default:
			http.Error(w, "unexpected replay request", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	replayDone := make(chan struct {
		events []factoryapi.WorkerSessionEvent
		err    error
	}, 1)
	go func() {
		events, err := waitForCompleteWorkerSessionReplay(ctx, server.URL+"/worker-sessions/worker-1/events?replayOnly=true")
		replayDone <- struct {
			events []factoryapi.WorkerSessionEvent
			err    error
		}{events: events, err: err}
	}()

	select {
	case <-secondRequest:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the second replay observation")
	}
	select {
	case result := <-replayDone:
		t.Fatalf("replay returned before terminal retention: events=%#v err=%v", result.events, result.err)
	default:
	}

	close(releaseTerminal)
	select {
	case result := <-replayDone:
		if result.err != nil {
			t.Fatalf("waitForCompleteWorkerSessionReplay() error = %v", result.err)
		}
		if len(result.events) != 3 {
			t.Fatalf("complete replay events = %d, want opening, terminal, and summary: %#v", len(result.events), result.events)
		}
		summary := result.events[2].ReplaySummary
		if summary == nil || !summary.Complete || summary.EventsEmitted != 2 {
			t.Fatalf("complete replay summary = %#v, want complete two-record summary", summary)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for complete replay")
	}

	if got := requests.Load(); got != 2 {
		t.Fatalf("replay requests = %d, want one incomplete and one complete observation", got)
	}
}
