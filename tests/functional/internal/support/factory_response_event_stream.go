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

// FactoryResponseEventFrame pairs one SSE id line with the decoded public
// FactoryResponseEvent payload from the same message.
type FactoryResponseEventFrame struct {
	SSEID string
	Event factoryapi.FactoryResponseEvent
}

// FactoryResponseEventStream owns one live Response Event SSE subscription
// through the public session response-events endpoint.
type FactoryResponseEventStream struct {
	t      testing.TB
	cancel context.CancelFunc
	done   chan struct{}
	events chan FactoryResponseEventFrame
	errs   chan error
}

// OpenFactoryResponseEventStreamAt opens the canonical Response Event SSE
// stream at endpoint without reading until the stream becomes quiet.
func OpenFactoryResponseEventStreamAt(t testing.TB, endpoint string) *FactoryResponseEventStream {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		cancel()
		t.Fatalf("build factory response event stream request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		cancel()
		t.Fatalf("GET factory response event stream: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		defer response.Body.Close()
		cancel()
		body, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"GET factory response event stream status = %d url = %q body = %s, want 200",
			response.StatusCode,
			endpoint,
			strings.TrimSpace(string(body)),
		)
	}
	if !strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		defer response.Body.Close()
		cancel()
		t.Fatalf(
			"GET factory response event stream content type = %q, want text/event-stream",
			response.Header.Get("Content-Type"),
		)
	}

	stream := &FactoryResponseEventStream{
		t:      t,
		cancel: cancel,
		done:   make(chan struct{}),
		events: make(chan FactoryResponseEventFrame, 4096),
		errs:   make(chan error, 1),
	}
	go stream.read(response)
	t.Cleanup(stream.Close)
	return stream
}

func (s *FactoryResponseEventStream) read(response *http.Response) {
	defer close(s.done)
	defer response.Body.Close()

	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		frame, err := readFactoryResponseEventSSEMessage(scanner, line)
		if err != nil {
			select {
			case s.errs <- err:
			default:
			}
			return
		}
		if frame == nil {
			continue
		}
		select {
		case s.events <- *frame:
		default:
			select {
			case s.errs <- fmt.Errorf("factory response event stream buffer overflow"):
			default:
			}
			return
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		select {
		case s.errs <- err:
		default:
		}
	}
}

func readFactoryResponseEventSSEMessage(
	scanner *bufio.Scanner,
	firstLine string,
) (*FactoryResponseEventFrame, error) {
	var idLine, dataLine string
	line := firstLine
	for {
		switch {
		case strings.HasPrefix(line, "id:"):
			idLine = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		case strings.HasPrefix(line, "data:"):
			dataLine = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		default:
			return nil, fmt.Errorf("unexpected response-event SSE line %q", line)
		}
		if !scanner.Scan() {
			break
		}
		line = strings.TrimSpace(scanner.Text())
		if line == "" {
			break
		}
	}
	if idLine == "" && dataLine == "" {
		return nil, nil
	}
	if idLine == "" || dataLine == "" {
		return nil, fmt.Errorf("SSE message id=%q data=%q, want exactly one of each", idLine, dataLine)
	}
	var event factoryapi.FactoryResponseEvent
	if err := json.Unmarshal([]byte(dataLine), &event); err != nil {
		return nil, fmt.Errorf("decode response-event SSE data: %w", err)
	}
	return &FactoryResponseEventFrame{SSEID: idLine, Event: event}, nil
}

// NextFrame waits for the next Response Event frame or fails the test on timeout.
func (s *FactoryResponseEventStream) NextFrame(timeout time.Duration) FactoryResponseEventFrame {
	s.t.Helper()
	frame, ok := s.TryNextFrame(timeout)
	if !ok {
		s.t.Fatalf("timed out waiting for factory response event stream frame within %s", timeout)
	}
	return frame
}

// TryNextFrame waits for the next Response Event frame until timeout or stream close.
func (s *FactoryResponseEventStream) TryNextFrame(timeout time.Duration) (FactoryResponseEventFrame, bool) {
	s.t.Helper()
	if timeout <= 0 {
		timeout = time.Nanosecond
	}
	select {
	case frame := <-s.events:
		return frame, true
	case err := <-s.errs:
		s.t.Fatalf("factory response event stream error: %v", err)
	case <-s.done:
		return FactoryResponseEventFrame{}, false
	case <-time.After(timeout):
		return FactoryResponseEventFrame{}, false
	}
	return FactoryResponseEventFrame{}, false
}

// WaitClosed waits until the SSE connection ends.
func (s *FactoryResponseEventStream) WaitClosed(timeout time.Duration) {
	s.t.Helper()
	if timeout <= 0 {
		timeout = functionalServerReadyTimeout
	}
	select {
	case <-s.done:
	case <-time.After(timeout):
		s.t.Fatalf("timed out waiting for factory response event stream close within %s", timeout)
	}
}

// Close cancels the stream request and waits briefly for the reader to exit.
func (s *FactoryResponseEventStream) Close() {
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
