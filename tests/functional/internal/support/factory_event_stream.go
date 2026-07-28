package support

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// FactoryEventStream owns one live Factory Event SSE subscription through the
// public session events endpoint.
type FactoryEventStream struct {
	t      testing.TB
	cancel context.CancelFunc
	done   chan struct{}
	events chan factoryapi.FactoryEvent
	errs   chan error
}

// OpenFactoryEventStreamAt opens the canonical Factory Event SSE stream at
// endpoint without reading until the stream becomes quiet.
func OpenFactoryEventStreamAt(t testing.TB, endpoint string) *FactoryEventStream {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		cancel()
		t.Fatalf("build factory event stream request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		cancel()
		t.Fatalf("GET factory event stream: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		defer response.Body.Close()
		cancel()
		body, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"GET factory event stream status = %d url = %q body = %s, want 200",
			response.StatusCode,
			endpoint,
			strings.TrimSpace(string(body)),
		)
	}
	if !strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		defer response.Body.Close()
		cancel()
		t.Fatalf(
			"GET factory event stream content type = %q, want text/event-stream",
			response.Header.Get("Content-Type"),
		)
	}

	stream := &FactoryEventStream{
		t:      t,
		cancel: cancel,
		done:   make(chan struct{}),
		events: make(chan factoryapi.FactoryEvent, 4096),
		errs:   make(chan error, 1),
	}
	go stream.read(response)
	t.Cleanup(stream.Close)
	return stream
}

func (s *FactoryEventStream) read(response *http.Response) {
	defer close(s.done)
	defer response.Body.Close()

	scanner := bufio.NewScanner(response.Body)
	var dataLines []string
	flush := func() {
		if len(dataLines) == 0 {
			return
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &event); err != nil {
			select {
			case s.errs <- fmt.Errorf("decode factory event stream payload: %w", err):
			default:
			}
			return
		}
		select {
		case s.events <- event:
		default:
			select {
			case s.errs <- fmt.Errorf("factory event stream buffer overflow"):
			default:
			}
		}
		dataLines = nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "event:") {
			select {
			case s.errs <- fmt.Errorf("factory event stream emitted named SSE event line %q", line):
			default:
			}
			return
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	flush()
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		select {
		case s.errs <- err:
		default:
		}
	}
}

// NextEvent waits for the next live Factory Event or fails the test on timeout.
func (s *FactoryEventStream) NextEvent(timeout time.Duration) factoryapi.FactoryEvent {
	s.t.Helper()
	event, ok := s.TryNextEvent(timeout)
	if !ok {
		s.t.Fatalf("timed out waiting for factory event stream payload within %s", timeout)
	}
	return event
}

// TryNextEvent waits for the next live Factory Event until timeout or stream close.
func (s *FactoryEventStream) TryNextEvent(timeout time.Duration) (factoryapi.FactoryEvent, bool) {
	s.t.Helper()
	if timeout <= 0 {
		timeout = time.Nanosecond
	}
	select {
	case event := <-s.events:
		return event, true
	case err := <-s.errs:
		s.t.Fatalf("factory event stream error: %v", err)
	case <-s.done:
		return factoryapi.FactoryEvent{}, false
	case <-time.After(timeout):
		return factoryapi.FactoryEvent{}, false
	}
	return factoryapi.FactoryEvent{}, false
}

// WaitClosed waits until the SSE connection ends.
func (s *FactoryEventStream) WaitClosed(timeout time.Duration) {
	s.t.Helper()
	if timeout <= 0 {
		timeout = functionalServerReadyTimeout
	}
	select {
	case <-s.done:
	case <-time.After(timeout):
		s.t.Fatalf("timed out waiting for factory event stream close within %s", timeout)
	}
}

// Close cancels the stream request and waits briefly for the reader to exit.
func (s *FactoryEventStream) Close() {
	if s == nil {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
	}
}
