package support

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFactoryEventStreamDrainsReceivedEventsBeforeEOF(t *testing.T) {
	for _, method := range []string{"context", "timeout"} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				for index := 0; index < 32; index++ {
					_, _ = fmt.Fprintf(w, "data: {\"id\":\"event-%d\",\"type\":\"RUN_RESPONSE\"}\n\n", index)
				}
			}))
			t.Cleanup(server.Close)
			stream := OpenFactoryEventStreamAt(t, server.URL)
			// Join the reader before consuming, reproducing a subscriber that
			// observes cancellation or replay closeout after SSE has reached EOF.
			stream.WaitClosed(time.Second)
			for index := 0; index < 32; index++ {
				var gotID string
				if method == "context" {
					gotID = stream.NextEventContext(context.Background()).Id
				} else {
					event, ok := stream.TryNextEvent(time.Second)
					if !ok {
						t.Fatalf("EOF before received event %d", index)
					}
					gotID = event.Id
				}
				if want := fmt.Sprintf("event-%d", index); gotID != want {
					t.Fatalf("event ID = %q, want %q", gotID, want)
				}
			}
			if _, ok := stream.TryNextEvent(time.Second); ok {
				t.Fatal("event after drained EOF")
			}
		})
	}
}
