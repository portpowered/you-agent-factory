package http

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

const (
	responseEventSlowConsumerFloodCount      = 200
	responseEventSlowConsumerPublishBudget   = 3 * time.Second
	responseEventSlowConsumerScenarioTimeout = 10 * time.Second
)

type gatedSSEResponseReader struct {
	src          io.Reader
	mu           sync.Mutex
	blocked      bool
	blockedReads atomic.Int64
}

func newGatedSSEResponseReader(src io.Reader) *gatedSSEResponseReader {
	return &gatedSSEResponseReader{src: src}
}

func (r *gatedSSEResponseReader) block() {
	r.mu.Lock()
	r.blocked = true
	r.mu.Unlock()
}

func (r *gatedSSEResponseReader) release() {
	r.mu.Lock()
	r.blocked = false
	r.mu.Unlock()
}

func (r *gatedSSEResponseReader) blockedReadAttemptsCount() int64 {
	return r.blockedReads.Load()
}

func (r *gatedSSEResponseReader) Read(p []byte) (int, error) {
	for {
		r.mu.Lock()
		blocked := r.blocked
		r.mu.Unlock()
		if !blocked {
			return r.src.Read(p)
		}
		r.blockedReads.Add(1)
		time.Sleep(1 * time.Millisecond)
	}
}

// TestFactoryResponseEventsBySessionID_SlowConsumerFloodDoesNotBlockPublication
// proves the public SSE response-event surface keeps store publication
// non-blocking while a client deliberately stops draining.
func TestFactoryResponseEventsBySessionID_SlowConsumerFloodDoesNotBlockPublication(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping response-event SSE slow-consumer fixture in short mode")
	}

	store, err := responseeventstore.NewSessionResponseEventStoreWithLimits(
		"session-beta",
		responseeventstore.RetentionLimits{MaxEvents: 16, MaxBytes: 64 * 1024},
	)
	if err != nil {
		t.Fatalf("new response-event store: %v", err)
	}

	publishResponseProgress(t, store, "seed")

	srv := newTestServer(&testutil.MockFactory{SessionFactories: map[string]*testutil.MockFactory{
		"session-beta": {ResponseEventStore: store},
	}})
	httpServer := httptest.NewServer(srv.Handler())
	defer httpServer.Close()

	scenarioCtx, cancelScenario := context.WithTimeout(context.Background(), responseEventSlowConsumerScenarioTimeout)
	defer cancelScenario()

	request, err := http.NewRequestWithContext(
		scenarioCtx,
		http.MethodGet,
		httpServer.URL+"/factory-sessions/session-beta/response-events",
		nil,
	)
	if err != nil {
		t.Fatalf("new response-event request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("open response-event stream: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("response-event status = %d, want 200", response.StatusCode)
	}

	gated := newGatedSSEResponseReader(response.Body)
	gated.block()

	floodDone := make(chan error, 1)
	go func() {
		publishStarted := time.Now()
		if _, err := store.Publish(responseEventRunCompletedInput("run-completed")); err != nil {
			floodDone <- err
			return
		}
		for i := 0; i < responseEventSlowConsumerFloodCount; i++ {
			if _, err := store.Publish(responseEventDeltaPublishInput(i)); err != nil {
				floodDone <- err
				return
			}
		}
		if elapsed := time.Since(publishStarted); elapsed > responseEventSlowConsumerPublishBudget {
			floodDone <- fmt.Errorf("publish elapsed = %v, want <= %v", elapsed, responseEventSlowConsumerPublishBudget)
			return
		}
		store.Complete()
		floodDone <- nil
	}()

	select {
	case err := <-floodDone:
		if err != nil {
			t.Fatalf("publish flood while SSE consumer lagged: %v", err)
		}
	case <-time.After(responseEventSlowConsumerPublishBudget):
		t.Fatalf("timed out waiting for publish flood within %v while SSE consumer was blocked", responseEventSlowConsumerPublishBudget)
	}

	wantFinalRun, ok := retainedResponseRunCompleted(t, store)
	if !ok {
		t.Fatal("terminal RUN completed event evicted under flood pressure")
	}

	gated.release()
	reader := bufio.NewReader(gated)
	drainCtx, cancelDrain := context.WithTimeout(scenarioCtx, 3*time.Second)
	defer cancelDrain()
	if err := drainResponseEventSSEUntilClosed(drainCtx, reader, wantFinalRun.EventID); err != nil {
		t.Fatalf("drain SSE after releasing slow consumer: %v", err)
	}
}

func retainedResponseRunCompleted(
	t *testing.T,
	store *responseeventstore.SessionResponseEventStore,
) (responseevents.FactoryResponseEvent, bool) {
	t.Helper()
	for _, event := range store.Events() {
		if event.Kind == responseevents.KindRun && event.Phase == responseevents.PhaseCompleted {
			return event, true
		}
	}
	return responseevents.FactoryResponseEvent{}, false
}

func drainResponseEventSSEUntilClosed(ctx context.Context, reader *bufio.Reader, wantFinalRunEventID string) error {
	sawFinalRun := false
	for {
		if err := ctx.Err(); err != nil {
			if sawFinalRun {
				return nil
			}
			return fmt.Errorf("timed out before terminal RUN event %q was observed", wantFinalRunEventID)
		}
		event, err := tryReadSSEFactoryResponseEvent(reader)
		if err == io.EOF {
			if !sawFinalRun {
				return fmt.Errorf("SSE stream closed before terminal RUN event %q", wantFinalRunEventID)
			}
			return nil
		}
		if err != nil {
			return err
		}
		if event.EventID == wantFinalRunEventID {
			sawFinalRun = true
		}
	}
}

func tryReadSSEFactoryResponseEvent(reader *bufio.Reader) (responseevents.FactoryResponseEvent, error) {
	var idLine, dataLine string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return responseevents.FactoryResponseEvent{}, err
		}
		line = trimSSELine(line)
		if line == "" {
			break
		}
		switch {
		case hasSSEPrefix(line, "id: "):
			idLine = trimSSEPrefix(line, "id: ")
		case hasSSEPrefix(line, "data: "):
			dataLine = trimSSEPrefix(line, "data: ")
		default:
			return responseevents.FactoryResponseEvent{}, fmt.Errorf("unexpected response-event SSE line %q", line)
		}
	}
	if idLine == "" || dataLine == "" {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf("SSE message id=%q data=%q, want exactly one of each", idLine, dataLine)
	}
	var event responseevents.FactoryResponseEvent
	if err := json.Unmarshal([]byte(dataLine), &event); err != nil {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf("decode response-event SSE data: %w", err)
	}
	if idLine != fmt.Sprint(event.Sequence) {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf("SSE id = %q, want event sequence %d", idLine, event.Sequence)
	}
	if err := responseevents.ValidateEvent(event); err != nil {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf("SSE response event is schema-invalid: %w", err)
	}
	return event, nil
}

func trimSSELine(line string) string {
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return line
}

func hasSSEPrefix(line, prefix string) bool {
	return len(line) >= len(prefix) && line[:len(prefix)] == prefix
}

func trimSSEPrefix(line, prefix string) string {
	return line[len(prefix):]
}

func responseEventRunCompletedInput(status string) responseevents.FactoryResponseEvent {
	payload, _ := json.Marshal(responseevents.RunPayload{Status: status})
	return responseevents.FactoryResponseEvent{
		FactorySessionID: "session-beta",
		Kind:             responseevents.KindRun,
		Phase:            responseevents.PhaseCompleted,
		Provenance: responseevents.Provenance{
			Provider:        "test-provider",
			NativeEventType: "run",
			Delivery:        responseevents.DeliveryNativeStream,
			Representation:  responseevents.RepresentationNotification,
			Fidelity:        responseevents.FidelityLossless,
		},
		RunID:   "run-test",
		Payload: payload,
	}
}

func responseEventDeltaPublishInput(index int) responseevents.FactoryResponseEvent {
	payload, _ := json.Marshal(responseevents.MessageDeltaPayload{
		ContentBlockIndex: 0,
		ContentBlockKind:  responseevents.ContentBlockText,
		TextDelta:         "delta-" + strconv.Itoa(index),
	})
	return responseevents.FactoryResponseEvent{
		FactorySessionID: "session-beta",
		Kind:             responseevents.KindMessage,
		Phase:            responseevents.PhaseDelta,
		Provenance: responseevents.Provenance{
			Provider:        "test-provider",
			NativeEventType: "message.delta",
			Delivery:        responseevents.DeliveryNativeStream,
			Representation:  responseevents.RepresentationNotification,
			Fidelity:        responseevents.FidelityLossless,
		},
		RunID:   "run-test",
		Payload: payload,
	}
}
