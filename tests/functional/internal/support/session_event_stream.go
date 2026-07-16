package support

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// FactorySessionEventStream reads the canonical Factory Session event SSE
// endpoint. It exposes only decoded public events and bounded cancellation.
type FactorySessionEventStream struct {
	t      *testing.T
	cancel context.CancelFunc
	done   chan struct{}
	events chan factoryapi.FactoryEvent
	errs   chan error
}

// OpenDefaultSessionFactoryEventStream opens the default session's canonical
// event stream for a composed functional host.
func OpenDefaultSessionFactoryEventStream(
	t *testing.T,
	client *http.Client,
	baseURL string,
) *FactorySessionEventStream {
	t.Helper()
	return OpenFactorySessionEventStream(t, client, DefaultSessionEventsURL(baseURL))
}

// OpenFactorySessionEventStream opens one documented Factory Session event
// stream and verifies its public HTTP/SSE contract before returning.
func OpenFactorySessionEventStream(
	t *testing.T,
	client *http.Client,
	endpoint string,
) *FactorySessionEventStream {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		cancel()
		t.Fatalf("build Factory Session event stream request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("GET Factory Session event stream: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		cancel()
		t.Fatalf("GET Factory Session event stream: status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		defer resp.Body.Close()
		cancel()
		t.Fatalf("Factory Session event stream content type = %q, want text/event-stream", resp.Header.Get("Content-Type"))
	}

	stream := &FactorySessionEventStream{
		t:      t,
		cancel: cancel,
		done:   make(chan struct{}),
		events: make(chan factoryapi.FactoryEvent, 128),
		errs:   make(chan error, 1),
	}
	go stream.read(resp)
	t.Cleanup(stream.Close)
	return stream
}

func (stream *FactorySessionEventStream) read(resp *http.Response) {
	defer close(stream.done)
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var dataLines []string
	flush := func() {
		if len(dataLines) == 0 {
			return
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &event); err != nil {
			stream.reportError(fmt.Errorf("decode Factory Session event: %w", err))
			return
		}
		select {
		case stream.events <- event:
		default:
			stream.reportError(fmt.Errorf("Factory Session event stream buffer overflow"))
		}
		dataLines = nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	flush()
	if err := scanner.Err(); err != nil && err != context.Canceled {
		stream.reportError(err)
	}
}

func (stream *FactorySessionEventStream) reportError(err error) {
	select {
	case stream.errs <- err:
	default:
	}
}

// Next returns the next public event or reports a bounded stream diagnostic.
func (stream *FactorySessionEventStream) Next(timeout time.Duration) factoryapi.FactoryEvent {
	stream.t.Helper()
	select {
	case event := <-stream.events:
		return event
	case err := <-stream.errs:
		stream.t.Fatalf("Factory Session event stream error: %v", err)
	case <-time.After(timeout):
		stream.t.Fatalf("timed out waiting for Factory Session event within %s", timeout)
	}
	return factoryapi.FactoryEvent{}
}

// Close cancels the stream and waits briefly for its owned reader to exit.
func (stream *FactorySessionEventStream) Close() {
	stream.cancel()
	select {
	case <-stream.done:
	case <-time.After(2 * time.Second):
	}
}
