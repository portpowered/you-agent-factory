package factorysessionsse

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type factorySessionSSEDeadlineBody struct {
	reader   io.Reader
	deadline time.Time
}

func (b *factorySessionSSEDeadlineBody) Read(p []byte) (int, error) {
	if b.reader != nil {
		return b.reader.Read(p)
	}
	if b.deadline.IsZero() {
		return 0, io.EOF
	}
	for time.Now().Before(b.deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	return 0, &net.OpError{Op: "read", Err: context.DeadlineExceeded}
}

func (b *factorySessionSSEDeadlineBody) Close() error { return nil }

func (b *factorySessionSSEDeadlineBody) SetReadDeadline(deadline time.Time) error {
	b.deadline = deadline
	return nil
}

func TestTryReadNextSSEFrame_ParsesCommentFrame(t *testing.T) {
	t.Parallel()

	commentReader := bufio.NewReader(strings.NewReader(": heartbeat\n\n"))
	commentFrame, ok, err := tryReadNextSSEFrame(commentReader)
	if err != nil || !ok {
		t.Fatalf("comment frame: ok=%v err=%v", ok, err)
	}
	if commentFrame.kind != factorySessionSSEFrameComment || commentFrame.comment != "heartbeat" {
		t.Fatalf("comment frame = %#v, want heartbeat comment", commentFrame)
	}
}

func TestTryReadNextSSEFrame_ParsesEventAndDataFrames(t *testing.T) {
	t.Parallel()

	eventReader := bufio.NewReader(strings.NewReader("event: ping\n\n"))
	eventFrame, ok, err := tryReadNextSSEFrame(eventReader)
	if err != nil || !ok {
		t.Fatalf("event frame: ok=%v err=%v", ok, err)
	}
	if eventFrame.kind != factorySessionSSEFrameOther || eventFrame.comment != "ping" {
		t.Fatalf("event frame = %#v, want ping other frame", eventFrame)
	}

	dataReader := bufio.NewReader(strings.NewReader(
		"data: {\"schemaVersion\":\"agent-factory.event.v1\",\"id\":\"evt-1\",\"type\":\"RUN_REQUEST\",\"context\":{\"sequence\":0},\"payload\":{}}\n\n",
	))
	dataFrame, ok, err := tryReadNextSSEFrame(dataReader)
	if err != nil || !ok {
		t.Fatalf("data frame: ok=%v err=%v", ok, err)
	}
	if dataFrame.kind != factorySessionSSEFrameData || dataFrame.event.Id != "evt-1" {
		t.Fatalf("data frame = %#v, want RUN_REQUEST event", dataFrame)
	}

	invalidReader := bufio.NewReader(strings.NewReader("data: not-json\n\n"))
	invalidFrame, ok, err := tryReadNextSSEFrame(invalidReader)
	if err != nil || !ok {
		t.Fatalf("invalid data frame: ok=%v err=%v", ok, err)
	}
	if invalidFrame.kind != factorySessionSSEFrameOther || invalidFrame.comment != "not-json" {
		t.Fatalf("invalid data frame = %#v, want other kind with raw data", invalidFrame)
	}
}

func TestFirstNonEmptyFactorySessionSSEString_ReturnsFirstTrimmedValue(t *testing.T) {
	t.Parallel()

	if got := firstNonEmptyFactorySessionSSEString("", "  ping  ", "later"); got != "  ping  " {
		t.Fatalf("first non-empty = %q, want trimmed first value", got)
	}
	if got := firstNonEmptyFactorySessionSSEString("", " "); got != "" {
		t.Fatalf("all blank = %q, want empty", got)
	}
}

func TestFactorySessionSSEKeepaliveSignalFromFrame_ClassifiesFrameKinds(t *testing.T) {
	t.Parallel()

	base := FactorySessionSSEKeepaliveSignal{ConnectionKeepAlive: true}

	commentSignal, err := factorySessionSSEKeepaliveSignalFromFrame(base, factorySessionSSEFrame{
		kind:    factorySessionSSEFrameComment,
		comment: "heartbeat",
	})
	if err != nil || commentSignal.SSEComment != "heartbeat" {
		t.Fatalf("comment signal = %#v err=%v", commentSignal, err)
	}

	otherSignal, err := factorySessionSSEKeepaliveSignalFromFrame(base, factorySessionSSEFrame{
		kind:    factorySessionSSEFrameOther,
		comment: "ping",
	})
	if err != nil || otherSignal.SSEComment != "ping" {
		t.Fatalf("other signal = %#v err=%v", otherSignal, err)
	}

	_, err = factorySessionSSEKeepaliveSignalFromFrame(base, factorySessionSSEFrame{
		kind:  factorySessionSSEFrameData,
		event: factoryapi.FactoryEvent{Id: "evt-live"},
	})
	if err == nil || !strings.Contains(err.Error(), "evt-live") {
		t.Fatalf("data frame error = %v, want factory event id in message", err)
	}

	_, err = factorySessionSSEKeepaliveSignalFromFrame(base, factorySessionSSEFrame{kind: factorySessionSSEFrameKind(99)})
	if err == nil || !strings.Contains(err.Error(), "unexpected idle keepalive frame kind") {
		t.Fatalf("unknown frame error = %v, want unexpected kind message", err)
	}
}

func TestIsFactorySessionSSEHarnessReadTimeout_DetectsTimeoutErrors(t *testing.T) {
	t.Parallel()

	if isFactorySessionSSEHarnessReadTimeout(nil) {
		t.Fatal("nil error should not be a timeout")
	}
	if !isFactorySessionSSEHarnessReadTimeout(context.DeadlineExceeded) {
		t.Fatal("context.DeadlineExceeded should be a timeout")
	}
	if !isFactorySessionSSEHarnessReadTimeout(&net.OpError{Op: "read", Err: context.DeadlineExceeded}) {
		t.Fatal("net timeout error should be a timeout")
	}
	if isFactorySessionSSEHarnessReadTimeout(io.EOF) {
		t.Fatal("EOF should not be a timeout")
	}
}

func TestFactorySessionSSEStream_TryWaitForKeepalive_FallsBackWhenBodyLacksDeadlines(t *testing.T) {
	t.Parallel()

	stream := &FactorySessionSSEStream{
		t:       t,
		timeout: 30 * time.Millisecond,
		Response: &http.Response{
			Header: http.Header{"Connection": []string{"keep-alive"}},
			Body:   io.NopCloser(strings.NewReader("")),
		},
		reader: bufio.NewReader(strings.NewReader("")),
	}

	signal, err := stream.TryWaitForKeepalive(30 * time.Millisecond)
	if err != nil {
		t.Fatalf("wait for keepalive via timer fallback: %v", err)
	}
	if !signal.OpenConnectionIdle || !signal.ConnectionKeepAlive {
		t.Fatalf("signal = %#v, want open idle keepalive", signal)
	}
}

func TestNewFactorySessionSSEHarness_UsesDefaultTimeoutWhenUnset(t *testing.T) {
	t.Parallel()

	harness := NewFactorySessionSSEHarness(t, 0)
	if harness.timeout != defaultFactorySessionSSEHarnessTimeout {
		t.Fatalf("timeout = %s, want default %s", harness.timeout, defaultFactorySessionSSEHarnessTimeout)
	}
}

func TestFactorySessionSSEHarness_OpenAcceptsReconnectQuery(t *testing.T) {
	fixture := NewFactorySessionSSEFixture(t)
	server := httptest.NewServer(newAPITestServer(fixture.RootMockFactory()).Handler())
	defer server.Close()

	harness := NewFactorySessionSSEHarness(t, 2*time.Second)
	stream := harness.Open(
		server.URL,
		fixture.SessionID,
		"after_event_id="+factorySessionSSEFixtureRetainedEventOneID,
	)
	defer stream.Close()

	if stream.Response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", stream.Response.StatusCode)
	}
	stream.AssertConnectionKeepAlive()
}

func TestFactorySessionSSEStream_TryWaitForKeepalive_UsesReadDeadlineWhenSupported(t *testing.T) {
	t.Parallel()

	body := &factorySessionSSEDeadlineBody{reader: strings.NewReader(": heartbeat\n\n")}
	stream := &FactorySessionSSEStream{
		t:       t,
		timeout: time.Second,
		Response: &http.Response{
			Header: http.Header{"Connection": []string{"keep-alive"}},
			Body:   body,
		},
		reader: bufio.NewReader(body),
	}

	signal, err := stream.TryWaitForKeepalive(time.Second)
	if err != nil {
		t.Fatalf("wait for comment keepalive: %v", err)
	}
	if signal.SSEComment != "heartbeat" {
		t.Fatalf("SSE comment = %q, want heartbeat", signal.SSEComment)
	}
}

func TestFactorySessionSSEStream_SetReadDeadline_RequiresSupportedBody(t *testing.T) {
	t.Parallel()

	stream := &FactorySessionSSEStream{
		t: t,
		Response: &http.Response{
			Body: io.NopCloser(strings.NewReader("")),
		},
	}
	if err := stream.setReadDeadline(time.Now().Add(time.Second)); err == nil {
		t.Fatal("expected error when body lacks SetReadDeadline")
	}
}

func TestTryReadSSEFactoryEvent_RejectsInvalidJSONPayload(t *testing.T) {
	t.Parallel()

	reader := bufio.NewReader(strings.NewReader("data: not-json\n\n"))
	_, ok, err := tryReadSSEFactoryEvent(reader)
	if err == nil || ok {
		t.Fatalf("invalid json: ok=%v err=%v", ok, err)
	}
}

func TestTryReadSSEFactoryEvent_ReturnsFalseWhenFrameHasNoDataLine(t *testing.T) {
	t.Parallel()

	reader := bufio.NewReader(strings.NewReader(": heartbeat\n\n"))
	_, ok, err := tryReadSSEFactoryEvent(reader)
	if err != nil || ok {
		t.Fatalf("comment-only frame: ok=%v err=%v", ok, err)
	}
}

func TestFactorySessionSSEStream_Close_IsNilSafe(t *testing.T) {
	t.Parallel()

	var stream *FactorySessionSSEStream
	stream.Close()
}

func TestFactorySessionSSEStream_ClearReadDeadline_ClearsDeadline(t *testing.T) {
	t.Parallel()

	body := &factorySessionSSEDeadlineBody{}
	stream := &FactorySessionSSEStream{
		t: t,
		Response: &http.Response{
			Body: body,
		},
	}
	if err := stream.setReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	stream.clearReadDeadline()
	if !body.deadline.IsZero() {
		t.Fatalf("deadline = %v, want zero after clear", body.deadline)
	}
}
