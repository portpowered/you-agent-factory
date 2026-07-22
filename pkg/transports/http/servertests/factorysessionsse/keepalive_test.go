package factorysessionsse

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFactorySessionSSEKeepalive_UsesConnectionKeepAliveHeader(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.WorkAPI()).Handler())
	defer server.Close()

	harness := newFactorySessionSSEHarness(t, 2*time.Second)
	stream := harness.Open(server.URL, fixture.SessionID, "")
	defer stream.Close()

	stream.AssertConnectionKeepAlive()
}

func TestFactorySessionSSEKeepalive_IdleStreamEmitsKeepaliveWithinBoundedTimeout(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.WorkAPI()).Handler())
	defer server.Close()

	idleTimeout := 300 * time.Millisecond
	harness := newFactorySessionSSEHarness(t, 2*time.Second)
	stream := harness.Open(server.URL, fixture.SessionID, "")
	defer stream.Close()

	_ = stream.ReadEvents(len(fixture.Retained))
	signal, err := stream.TryWaitForKeepalive(idleTimeout)
	if err != nil {
		t.Fatalf("wait for idle keepalive: %v", err)
	}
	assertFactorySessionSSEKeepaliveSignal(t, signal)
}

func TestFactorySessionSSEKeepalive_IdleKeepaliveDoesNotSerializeFactoryEventKinds(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.WorkAPI()).Handler())
	defer server.Close()

	harness := newFactorySessionSSEHarness(t, 2*time.Second)
	stream := harness.Open(server.URL, fixture.SessionID, "")
	defer stream.Close()

	_ = stream.ReadEvents(len(fixture.Retained))
	signal, err := stream.TryWaitForKeepalive(300 * time.Millisecond)
	if err != nil {
		t.Fatalf("wait for idle keepalive: %v", err)
	}
	if signal.SSEComment != "" && strings.Contains(strings.ToLower(signal.SSEComment), "factory-event") {
		t.Fatalf("keepalive comment = %q, want non-FactoryEvent SSE traffic", signal.SSEComment)
	}
}

func TestFactorySessionSSEKeepalive_DeliversLiveEventAfterKeepaliveObserved(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.WorkAPI()).Handler())
	defer server.Close()

	harness := newFactorySessionSSEHarness(t, 2*time.Second)
	stream := harness.Open(server.URL, fixture.SessionID, "")
	defer stream.Close()

	_ = stream.ReadEvents(len(fixture.Retained))
	signal, err := stream.TryWaitForKeepalive(300 * time.Millisecond)
	if err != nil {
		t.Fatalf("wait for idle keepalive: %v", err)
	}
	assertFactorySessionSSEKeepaliveSignal(t, signal)

	live := fixture.LiveDispatchEvent(t)
	fixture.PublishLive(live)
	gotLive := stream.ReadNextEvent()
	if gotLive.Id != live.Id {
		t.Fatalf("live event id = %q, want %q after keepalive", gotLive.Id, live.Id)
	}
}

func TestFactorySessionSSEKeepalive_FailsClosedWhenKeepaliveNeverArrives(t *testing.T) {
	stream := &FactorySessionSSEStream{
		t:       t,
		timeout: 50 * time.Millisecond,
		Response: &http.Response{
			Header: http.Header{},
		},
	}

	_, err := stream.TryWaitForKeepalive(50 * time.Millisecond)
	if err == nil {
		t.Fatal("expected keepalive wait failure when Connection keep-alive is missing, got nil")
	}
	if !errors.Is(err, errFactorySessionSSEKeepaliveMissing) {
		t.Fatalf("error = %v, want %v", err, errFactorySessionSSEKeepaliveMissing)
	}
}

func assertFactorySessionSSEKeepaliveSignal(t *testing.T, signal FactorySessionSSEKeepaliveSignal) {
	t.Helper()
	switch {
	case signal.SSEComment != "":
		if strings.TrimSpace(signal.SSEComment) == "" {
			t.Fatal("keepalive SSE comment is empty")
		}
	case signal.OpenConnectionIdle && signal.ConnectionKeepAlive:
	default:
		t.Fatalf(
			"keepalive signal = %#v, want SSE comment traffic or open Connection keep-alive idle",
			signal,
		)
	}
}
