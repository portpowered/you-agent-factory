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
	"sync"
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

// FactoryResponseEventStreamOutcome classifies the result of waiting for one
// Response Event frame.
type FactoryResponseEventStreamOutcome string

const (
	FactoryResponseEventStreamOutcomeFrame     FactoryResponseEventStreamOutcome = "frame"
	FactoryResponseEventStreamOutcomeTimeout   FactoryResponseEventStreamOutcome = "timeout"
	FactoryResponseEventStreamOutcomeEOF       FactoryResponseEventStreamOutcome = "eof"
	FactoryResponseEventStreamOutcomeCanceled  FactoryResponseEventStreamOutcome = "canceled"
	FactoryResponseEventStreamOutcomeReadError FactoryResponseEventStreamOutcome = "read_error"
)

// FactoryResponseEventStreamWaitResult preserves the observable outcome of a
// single frame wait, including terminal diagnostics that the legacy boolean
// TryNextFrame API intentionally does not expose.
type FactoryResponseEventStreamWaitResult struct {
	Frame      FactoryResponseEventFrame
	Outcome    FactoryResponseEventStreamOutcome
	StatusCode int
	Waited     time.Duration
	FrameCount int
	Err        error
}

// Diagnostic returns an actionable description for a non-frame wait outcome.
// NextFrame uses this exact diagnostic before failing the test.
func (r FactoryResponseEventStreamWaitResult) Diagnostic() string {
	switch r.Outcome {
	case FactoryResponseEventStreamOutcomeTimeout:
		return fmt.Sprintf(
			"timed out waiting for factory response event stream frame: HTTP status=%d, waited=%s, frames=%d",
			r.StatusCode,
			r.Waited,
			r.FrameCount,
		)
	case FactoryResponseEventStreamOutcomeEOF:
		return fmt.Sprintf(
			"factory response event stream closed: HTTP status=%d, reason=EOF, waited=%s, frames=%d",
			r.StatusCode,
			r.Waited,
			r.FrameCount,
		)
	case FactoryResponseEventStreamOutcomeCanceled:
		return fmt.Sprintf(
			"factory response event stream closed: HTTP status=%d, reason=deliberate cancellation, waited=%s, frames=%d",
			r.StatusCode,
			r.Waited,
			r.FrameCount,
		)
	case FactoryResponseEventStreamOutcomeReadError:
		return fmt.Sprintf(
			"factory response event stream read error: HTTP status=%d, waited=%s, frames=%d, error=%v",
			r.StatusCode,
			r.Waited,
			r.FrameCount,
			r.Err,
		)
	default:
		return fmt.Sprintf(
			"factory response event stream outcome=%q: HTTP status=%d, waited=%s, frames=%d",
			r.Outcome,
			r.StatusCode,
			r.Waited,
			r.FrameCount,
		)
	}
}

// FactoryResponseEventStream owns one live Response Event SSE subscription
// through the public session response-events endpoint.
type FactoryResponseEventStream struct {
	t      testing.TB
	cancel context.CancelFunc
	done   chan struct{}
	events chan FactoryResponseEventFrame

	stateMu               sync.RWMutex
	statusCode            int
	terminalOutcome       FactoryResponseEventStreamOutcome
	terminalErr           error
	frameCount            int
	cancellationRequested bool
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
		t:          t,
		cancel:     cancel,
		done:       make(chan struct{}),
		events:     make(chan FactoryResponseEventFrame, 4096),
		statusCode: response.StatusCode,
	}
	go stream.read(response)
	t.Cleanup(stream.Close)
	return stream
}

func (s *FactoryResponseEventStream) read(response *http.Response) {
	var terminalErr error
	defer func() {
		_ = response.Body.Close()
		outcome := FactoryResponseEventStreamOutcomeEOF
		if terminalErr != nil {
			outcome = FactoryResponseEventStreamOutcomeReadError
		} else if s.isCancellationRequested() {
			outcome = FactoryResponseEventStreamOutcomeCanceled
		}
		s.finish(outcome, terminalErr)
	}()

	scanner := bufio.NewScanner(response.Body)
readLoop:
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		frame, err := readFactoryResponseEventSSEMessage(scanner, line)
		if err != nil {
			terminalErr = err
			break
		}
		if frame == nil {
			continue
		}
		if !s.enqueueFrame(*frame) {
			terminalErr = fmt.Errorf("factory response event stream buffer overflow")
			break readLoop
		}
	}
	if terminalErr == nil {
		if err := scanner.Err(); err != nil {
			if !errors.Is(err, context.Canceled) || !s.isCancellationRequested() {
				terminalErr = err
			}
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

// NextFrame waits for the next Response Event frame or fails the test with a
// diagnostic that identifies timeout, closure, cancellation, or read error.
func (s *FactoryResponseEventStream) NextFrame(timeout time.Duration) FactoryResponseEventFrame {
	s.t.Helper()
	result := s.TryNextFrameResult(timeout)
	if result.Outcome != FactoryResponseEventStreamOutcomeFrame {
		s.t.Fatalf("%s", result.Diagnostic())
	}
	return result.Frame
}

// TryNextFrameResult waits for the next Response Event frame and preserves the
// exact outcome and terminal metadata for diagnostic-aware callers.
func (s *FactoryResponseEventStream) TryNextFrameResult(
	timeout time.Duration,
) FactoryResponseEventStreamWaitResult {
	s.t.Helper()
	if timeout <= 0 {
		timeout = time.Nanosecond
	}
	started := time.Now()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case frame := <-s.events:
		return s.waitResult(
			FactoryResponseEventStreamOutcomeFrame,
			frame,
			time.Since(started),
			nil,
		)
	case <-s.done:
		return s.terminalResult(time.Since(started))
	case <-timer.C:
		select {
		case <-s.done:
			return s.terminalResult(time.Since(started))
		default:
			return s.waitResult(
				FactoryResponseEventStreamOutcomeTimeout,
				FactoryResponseEventFrame{},
				time.Since(started),
				nil,
			)
		}
	}
}

// TryNextFrame retains the original quiet-period and closed-stream contract:
// a frame returns true, while timeout or normal stream termination returns
// false. Read failures still fail the test immediately.
func (s *FactoryResponseEventStream) TryNextFrame(timeout time.Duration) (FactoryResponseEventFrame, bool) {
	s.t.Helper()
	result := s.TryNextFrameResult(timeout)
	if result.Outcome == FactoryResponseEventStreamOutcomeReadError {
		s.t.Fatalf("%s", result.Diagnostic())
	}
	if result.Outcome != FactoryResponseEventStreamOutcomeFrame {
		return FactoryResponseEventFrame{}, false
	}
	return result.Frame, true
}

func (s *FactoryResponseEventStream) waitResult(
	outcome FactoryResponseEventStreamOutcome,
	frame FactoryResponseEventFrame,
	waited time.Duration,
	err error,
) FactoryResponseEventStreamWaitResult {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return FactoryResponseEventStreamWaitResult{
		Frame:      frame,
		Outcome:    outcome,
		StatusCode: s.statusCode,
		Waited:     waited,
		FrameCount: s.frameCount,
		Err:        err,
	}
}

func (s *FactoryResponseEventStream) terminalResult(waited time.Duration) FactoryResponseEventStreamWaitResult {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return FactoryResponseEventStreamWaitResult{
		Outcome:    s.terminalOutcome,
		StatusCode: s.statusCode,
		Waited:     waited,
		FrameCount: s.frameCount,
		Err:        s.terminalErr,
	}
}

func (s *FactoryResponseEventStream) enqueueFrame(frame FactoryResponseEventFrame) bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	select {
	case s.events <- frame:
		s.frameCount++
		return true
	default:
		return false
	}
}

func (s *FactoryResponseEventStream) isCancellationRequested() bool {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.cancellationRequested
}

func (s *FactoryResponseEventStream) finish(
	outcome FactoryResponseEventStreamOutcome,
	err error,
) {
	s.stateMu.Lock()
	s.terminalOutcome = outcome
	s.terminalErr = err
	s.stateMu.Unlock()
	close(s.done)
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
	s.stateMu.Lock()
	s.cancellationRequested = true
	s.stateMu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
	}
}
