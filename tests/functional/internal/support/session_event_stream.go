package support

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// FactorySessionEventStream reads the canonical Factory Session event SSE
// endpoint. It exposes only decoded public events and bounded cancellation.
type FactorySessionEventStream struct {
	t      *testing.T
	ctx    context.Context
	cancel context.CancelFunc
	body   io.ReadCloser
	done   chan struct{}
	events chan factoryapi.FactoryEvent
	errs   chan error

	observationMu sync.Mutex
	observation   string
	closeOnce     sync.Once
	closeErr      error
}

const (
	factorySessionEventStreamOpenTimeout  = 5 * time.Second
	factorySessionEventStreamCloseTimeout = 2 * time.Second
)

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
	resp, err := openFactorySessionEventStreamResponse(ctx, cancel, client, endpoint, factorySessionEventStreamOpenTimeout)
	if err != nil {
		t.Fatalf("GET Factory Session event stream: %v; last public observation: no response headers observed", err)
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
		t:           t,
		ctx:         ctx,
		cancel:      cancel,
		body:        resp.Body,
		done:        make(chan struct{}),
		events:      make(chan factoryapi.FactoryEvent, 128),
		errs:        make(chan error, 1),
		observation: fmt.Sprintf("HTTP %s with Content-Type %q", resp.Status, resp.Header.Get("Content-Type")),
	}
	go stream.read(resp)
	t.Cleanup(func() {
		if err := stream.Close(); err != nil {
			t.Errorf("close Factory Session event stream: %v", err)
		}
	})
	return stream
}

type eventStreamResponse struct {
	response *http.Response
	err      error
}

func openFactorySessionEventStreamResponse(
	ctx context.Context,
	cancel context.CancelFunc,
	client *http.Client,
	endpoint string,
	timeout time.Duration,
) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("build request: %w", err)
	}
	result := make(chan eventStreamResponse)
	go func() {
		response, requestErr := client.Do(req)
		select {
		case result <- eventStreamResponse{response: response, err: requestErr}:
		case <-ctx.Done():
			if response != nil {
				_ = response.Body.Close()
			}
		}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case completed := <-result:
		if completed.err != nil {
			cancel()
		}
		return completed.response, completed.err
	case <-ctx.Done():
		cancel()
		return nil, ctx.Err()
	case <-timer.C:
		cancel()
		return nil, fmt.Errorf("timed out after %s waiting for response headers", timeout)
	}
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
		stream.setObservation(fmt.Sprintf("Factory Event %q", event.Type))
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
	if err := scanner.Err(); err != nil && stream.ctx.Err() == nil {
		stream.reportError(err)
	}
}

func (stream *FactorySessionEventStream) setObservation(observation string) {
	stream.observationMu.Lock()
	defer stream.observationMu.Unlock()
	stream.observation = observation
}

func (stream *FactorySessionEventStream) lastObservation() string {
	stream.observationMu.Lock()
	defer stream.observationMu.Unlock()
	return stream.observation
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

// Close cancels the stream, closes its response body, and reports failure if
// the owned reader cannot exit within the bounded shutdown interval.
func (stream *FactorySessionEventStream) Close() error {
	stream.closeOnce.Do(func() {
		stream.closeErr = stream.closeWithin(factorySessionEventStreamCloseTimeout)
	})
	return stream.closeErr
}

func (stream *FactorySessionEventStream) closeWithin(timeout time.Duration) error {
	stream.cancel()
	_ = stream.body.Close()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-stream.done:
		return nil
	case <-timer.C:
		return fmt.Errorf("timed out after %s waiting for owned reader shutdown; last public observation: %s", timeout, stream.lastObservation())
	}
}
