package watch_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

const (
	workWatchAttachmentTimeout = 5 * time.Second
	workWatchEventPath         = "/factory-sessions/~default/events"
)

// workWatchStreamGate observes the successful response boundary of the exact
// public Factory Event SSE request. ReverseProxy keeps the production server,
// handler, subscription, and response body on the executable spine while the
// package-local server exposes a deterministic test signal.
type workWatchStreamGate struct {
	server   *httptest.Server
	attached chan struct{}
	once     sync.Once

	mu            sync.Mutex
	lastProbe     string
	lastStatus    int
	lastMediaType string
}

func newWorkWatchStreamGate(t *testing.T, targetURL string) *workWatchStreamGate {
	t.Helper()
	target, err := url.Parse(targetURL)
	if err != nil {
		t.Fatalf("parse functional API server URL for Work watch gate: %v", err)
	}

	gate := &workWatchStreamGate{attached: make(chan struct{})}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ModifyResponse = func(response *http.Response) error {
		gate.mu.Lock()
		gate.lastStatus = response.StatusCode
		gate.lastMediaType = response.Header.Get("Content-Type")
		gate.mu.Unlock()
		if response.Request == nil || response.Request.Method != http.MethodGet ||
			response.Request.URL.Path != workWatchEventPath ||
			response.StatusCode != http.StatusOK ||
			!strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") ||
			strings.TrimSpace(response.Header.Get(factorysessions.SessionEventStreamRetainedCountHeader)) == "" {
			return nil
		}
		gate.once.Do(func() { close(gate.attached) })
		return nil
	}
	gate.server = httptest.NewServer(proxy)
	t.Cleanup(func() { gate.server.Close() })
	return gate
}

func (gate *workWatchStreamGate) URL() string {
	if gate == nil || gate.server == nil {
		return ""
	}
	return gate.server.URL
}

func (gate *workWatchStreamGate) wait(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), workWatchAttachmentTimeout)
	defer cancel()
	select {
	case <-gate.attached:
	case <-ctx.Done():
		gate.mu.Lock()
		status, mediaType := gate.lastStatus, gate.lastMediaType
		gate.mu.Unlock()
		t.Fatalf(
			"timed out waiting for exact public Work watch SSE attachment: %v (last response status=%d content-type=%q)",
			ctx.Err(), status, mediaType,
		)
	}
}
