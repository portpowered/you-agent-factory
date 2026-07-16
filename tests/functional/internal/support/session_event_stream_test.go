package support

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestOpenFactorySessionEventStreamResponse_CancelsStalledHeaders(t *testing.T) {
	requestCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
		close(requestCanceled)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	_, err := openFactorySessionEventStreamResponse(ctx, cancel, server.Client(), server.URL, 50*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "waiting for response headers") {
		t.Fatalf("open stream error = %v, want bounded response-header timeout", err)
	}
	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("stalled event-stream request remained live after timeout")
	}
}

func TestFactorySessionEventStream_CloseReportsReaderThatDoesNotExit(t *testing.T) {
	body := newStalledReadCloser()
	ctx, cancel := context.WithCancel(context.Background())
	stream := &FactorySessionEventStream{
		t:           t,
		ctx:         ctx,
		cancel:      cancel,
		body:        body,
		done:        make(chan struct{}),
		events:      make(chan factoryapi.FactoryEvent),
		errs:        make(chan error, 1),
		observation: "HTTP 200 OK with Content-Type text/event-stream",
	}

	go func() {
		defer close(stream.done)
		_, _ = body.Read(nil)
	}()
	err := stream.closeWithin(50 * time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "last public observation: HTTP 200 OK") {
		t.Fatalf("close stream error = %v, want bounded last-observation diagnostic", err)
	}
	select {
	case <-body.closed:
	case <-time.After(time.Second):
		t.Fatal("stream close did not close its owned response body")
	}
	close(body.release)
	select {
	case <-stream.done:
	case <-time.After(time.Second):
		t.Fatal("test stream reader did not exit after release")
	}
}

type stalledReadCloser struct {
	closed  chan struct{}
	release chan struct{}
	once    sync.Once
}

func newStalledReadCloser() *stalledReadCloser {
	return &stalledReadCloser{closed: make(chan struct{}), release: make(chan struct{})}
}

func (body *stalledReadCloser) Read([]byte) (int, error) {
	<-body.release
	return 0, io.EOF
}

func (body *stalledReadCloser) Close() error {
	body.once.Do(func() { close(body.closed) })
	return nil
}
