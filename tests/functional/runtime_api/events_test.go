package runtime_api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type factoryEventHTTPStream struct {
	t      *testing.T
	cancel context.CancelFunc
	done   chan struct{}
	events chan factoryapi.FactoryEvent
	errs   chan error

	closeOnce sync.Once
	closeErr  error
	hookMu    sync.Mutex
	closeHook func()
	closed    bool
}

func openDefaultSessionFactoryEventHTTPStream(t *testing.T, baseURL string) *factoryEventHTTPStream {
	t.Helper()
	return openFactoryEventHTTPStream(t, support.DefaultSessionEventsURL(baseURL))
}

// openFactoryEventHTTPStream opens one explicit factory event SSE endpoint. Runtime
// API smokes that represent dashboard, Factory Session, or replay behavior should
// call openDefaultSessionFactoryEventHTTPStream instead of process-global /events.
func openFactoryEventHTTPStream(t *testing.T, endpoint string) *factoryEventHTTPStream {
	t.Helper()
	return openFactoryEventHTTPStreamWithHook(t, endpoint, nil)
}

func openFactoryEventHTTPStreamWithHook(t *testing.T, endpoint string, closeHook func()) *factoryEventHTTPStream {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		cancel()
		t.Fatalf("build event stream request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("GET event stream: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		cancel()
		t.Fatalf("GET event stream status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		defer resp.Body.Close()
		cancel()
		t.Fatalf("GET event stream content type = %q, want text/event-stream", resp.Header.Get("Content-Type"))
	}

	stream := &factoryEventHTTPStream{
		t:         t,
		cancel:    cancel,
		done:      make(chan struct{}),
		events:    make(chan factoryapi.FactoryEvent, 4096),
		errs:      make(chan error, 1),
		closeHook: closeHook,
	}
	go stream.read(resp)
	t.Cleanup(func() {
		if err := stream.close(); err != nil {
			t.Errorf("close /events stream: %v", err)
		}
	})
	return stream
}

func (s *factoryEventHTTPStream) read(resp *http.Response) {
	defer close(s.done)
	defer resp.Body.Close()
	defer s.notifyClosed()

	scanner := bufio.NewScanner(resp.Body)
	var dataLines []string
	flush := func() {
		if len(dataLines) == 0 {
			return
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &event); err != nil {
			select {
			case s.errs <- fmt.Errorf("decode /events payload: %w", err):
			default:
			}
			return
		}
		select {
		case s.events <- event:
		default:
			select {
			case s.errs <- fmt.Errorf("/events test buffer overflow"):
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
			case s.errs <- fmt.Errorf("/events emitted named SSE event line %q", line):
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

func (s *factoryEventHTTPStream) next(timeout time.Duration) factoryapi.FactoryEvent {
	s.t.Helper()
	if timeout <= 0 {
		timeout = time.Nanosecond
	}
	select {
	case event := <-s.events:
		return event
	case err := <-s.errs:
		s.t.Fatalf("/events stream error: %v", err)
	case <-time.After(timeout):
		s.t.Fatalf("timed out waiting for /events payload within %s", timeout)
	}
	return factoryapi.FactoryEvent{}
}

func (s *factoryEventHTTPStream) setCloseHook(closeHook func()) {
	if s == nil || closeHook == nil {
		return
	}
	s.hookMu.Lock()
	if s.closed {
		s.hookMu.Unlock()
		closeHook()
		return
	}
	s.closeHook = closeHook
	s.hookMu.Unlock()
}

func (s *factoryEventHTTPStream) notifyClosed() {
	if s == nil {
		return
	}
	s.hookMu.Lock()
	if s.closed {
		s.hookMu.Unlock()
		return
	}
	s.closed = true
	closeHook := s.closeHook
	s.closeHook = nil
	s.hookMu.Unlock()
	if closeHook != nil {
		closeHook()
	}
}

func (s *factoryEventHTTPStream) close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.cancel()
		select {
		case <-s.done:
		case <-time.After(2 * time.Second):
			s.closeErr = errors.New("timed out waiting for /events stream shutdown")
		}
	})
	return s.closeErr
}

func requireFunctionalEventStreamPrelude(
	t *testing.T,
	stream *factoryEventHTTPStream,
) (factoryapi.FactoryEvent, factoryapi.FactoryEvent) {
	t.Helper()

	runStarted := stream.next(5 * time.Second)
	if runStarted.Type != factoryapi.FactoryEventTypeRunRequest || runStarted.Context.Tick != 0 {
		t.Fatalf("first /events payload = %#v, want run-request at tick 0", runStarted)
	}

	initialStructure := stream.next(5 * time.Second)
	if initialStructure.Type != factoryapi.FactoryEventTypeInitialStructureRequest || initialStructure.Context.Tick != 0 {
		t.Fatalf("second /events payload = %#v, want initial structure at tick 0", initialStructure)
	}
	if initialStructure.Context.Sequence <= runStarted.Context.Sequence {
		t.Fatalf(
			"/events prelude sequences = run_request:%d initial_structure_request:%d, want increasing order",
			runStarted.Context.Sequence,
			initialStructure.Context.Sequence,
		)
	}

	return runStarted, initialStructure
}
