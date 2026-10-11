package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
)

func TestFollowStreamsSourceNativeEnvelopeAndTypedTerminalError(t *testing.T) {
	t.Parallel()
	service := messageService{follow: func(_ context.Context, request agentmessages.ListRequest, observe func(agentmessages.Observation) error) error {
		if request.Caller.WorkerSessionID != "worker" || !request.ToMe || request.MarkRead {
			t.Fatal("follow lost read identity or acquired write intent")
		}
		if err := observe(agentmessages.Observation{RecordID: "transaction", Sequence: 42, Kind: "READ", Message: agentmessages.Message{MessageID: "m", Status: agentmessages.Read, Body: "[REDACTED]"}}); err != nil {
			return err
		}
		return fmt.Errorf("planted-secret: %w", agentmessages.ErrStreamGap)
	}}
	h := NewHandler(service, logging.NoopLogger{})
	w := httptest.NewRecorder()
	h.List(w, workerRequest("GET", "/messages?toMe=true&follow=true", ""))
	body := w.Body.String()
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/event-stream" || w.Header().Get("Cache-Control") != "no-cache" || !w.Flushed {
		t.Fatal("follow did not establish a flushed stream")
	}
	if !strings.Contains(body, "event: message\ndata: {\"kind\":\"READ\"") || !strings.Contains(body, `"recordId":"transaction","sequence":42`) || !strings.Contains(body, "event: error\ndata: ") || !strings.Contains(body, "MESSAGE_STREAM_GAP") || strings.Contains(body, "planted-secret") || strings.Contains(body, "factoryEvent") {
		t.Fatalf("unsafe or incomplete source-native stream: %s", body)
	}
}

func TestFollowInitialFailuresRemainTypedHTTPResponses(t *testing.T) {
	t.Parallel()
	for _, failure := range []error{agentmessages.ErrNotPermitted, agentmessages.ErrDisabled, agentmessages.ErrBadRequest, agentmessages.ErrStreamUnavailable, agentmessages.ErrStreamBackpressure} {
		h := NewHandler(messageService{follow: func(context.Context, agentmessages.ListRequest, func(agentmessages.Observation) error) error {
			return failure
		}}, logging.NoopLogger{})
		w := httptest.NewRecorder()
		h.List(w, workerRequest("GET", "/messages?follow=true", ""))
		if w.Code < 400 || w.Header().Get("Content-Type") != "application/json" || strings.Contains(w.Body.String(), "event:") {
			t.Fatal("initial authorization or attachment failure became a successful stream")
		}
	}
}

func TestFollowCompletionAndCancellationDoNotInventObservations(t *testing.T) {
	t.Parallel()
	for _, failure := range []error{nil, context.Canceled, context.DeadlineExceeded} {
		h := NewHandler(messageService{follow: func(_ context.Context, _ agentmessages.ListRequest, observe func(agentmessages.Observation) error) error {
			if failure != nil {
				if err := observe(agentmessages.Observation{RecordID: "r", Sequence: 1, Kind: "SENT"}); err != nil {
					return err
				}
			}
			return failure
		}}, logging.NoopLogger{})
		w := httptest.NewRecorder()
		h.List(w, workerRequest("GET", "/messages?follow=true", ""))
		if w.Code != 200 || strings.Contains(w.Body.String(), "event: error") || strings.Contains(w.Body.String(), "context") {
			t.Fatal("normal completion or cancellation fabricated an error observation")
		}
		if failure == nil && (w.Body.Len() != 0 || !w.Flushed || w.Header().Get("Content-Type") != "text/event-stream") {
			t.Fatal("empty normal completion invented a message")
		}
	}
}

type failingStream struct{ failedResponse }

func (*failingStream) Flush() {}

func TestFollowRefusesUnavailableWriterAndStopsOnWriteFailure(t *testing.T) {
	t.Parallel()
	logger := &diagnosticLogger{}
	called := false
	h := NewHandler(messageService{follow: func(_ context.Context, _ agentmessages.ListRequest, observe func(agentmessages.Observation) error) error {
		called = true
		return observe(agentmessages.Observation{RecordID: "r", Sequence: 1, Kind: "SENT", Message: agentmessages.Message{Body: "planted-secret"}})
	}}, logger)
	h.List(&failedResponse{header: make(http.Header)}, workerRequest("GET", "/messages?follow=true", ""))
	if called {
		t.Fatal("follow consumed source observations without a flushable writer")
	}
	logger.lines = nil
	h.List(&failingStream{failedResponse{header: make(http.Header)}}, workerRequest("GET", "/messages?follow=true", ""))
	if !called || len(logger.lines) != 1 || strings.Contains(strings.Join(logger.lines, " "), "planted-secret") {
		t.Fatal("stream write failure was not bounded or exposed content")
	}
}

func TestCanceledRequestCannotPublishAnotherStreamObservation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	h := NewHandler(messageService{follow: func(_ context.Context, _ agentmessages.ListRequest, observe func(agentmessages.Observation) error) error {
		cancel()
		if err := observe(agentmessages.Observation{RecordID: "r", Sequence: 1, Kind: "SENT"}); !errors.Is(err, context.Canceled) {
			t.Fatal("callback ignored request cancellation")
		}
		return context.Canceled
	}}, logging.NoopLogger{})
	w := httptest.NewRecorder()
	h.List(w, workerRequest(http.MethodGet, "/messages?follow=true", "").WithContext(ctx))
	if w.Body.Len() != 0 || w.Flushed {
		t.Fatal("canceled request published another observation")
	}
}
