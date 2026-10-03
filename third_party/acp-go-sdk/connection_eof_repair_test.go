package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// These tests cover the receive-EOF repair in connection.go: a response the peer
// already delivered must not be lost to a disconnect that follows it, the
// notification watermark behind a delivered response must be allowed to finish
// inside the existing bounded inbound drain instead of failing on Done, and a
// response that arrives after the caller stopped waiting must fail instead of
// reporting a result the caller can no longer attribute to itself.
//
// Every scenario is forced with in-memory pipes plus explicit channels. There is
// no process, file, or sleep-based scheduling: the reader only reports receive
// EOF when a test asks for it, and the outbound writer only returns when the
// scenario's readiness condition is observed.

// eofWaitGuard is only a deadlock guard around the channel synchronization below.
// It is never the mechanism a scenario waits on.
const eofWaitGuard = 2 * time.Second

// scriptedPeerReader is an in-memory peer stream. Payloads are handed over only
// when a test queues them, and receive EOF is reported only after closeWithEOF
// has been called, so a test decides exactly when EOF becomes observable.
type scriptedPeerReader struct {
	payloads chan []byte
	remain   []byte
	eofOnce  sync.Once
}

func newScriptedPeerReader() *scriptedPeerReader {
	return &scriptedPeerReader{payloads: make(chan []byte)}
}

// deliver queues one inbound payload for the receive loop. It returns once the
// receive loop has taken the payload.
func (r *scriptedPeerReader) deliver(payload string) {
	r.payloads <- []byte(payload)
}

// closeWithEOF reports receive EOF after every queued payload has been read. It
// is safe to call more than once so tests and their cleanups can both use it.
func (r *scriptedPeerReader) closeWithEOF() {
	r.eofOnce.Do(func() { close(r.payloads) })
}

func (r *scriptedPeerReader) Read(p []byte) (int, error) {
	for len(r.remain) == 0 {
		payload, ok := <-r.payloads
		if !ok {
			return 0, io.EOF
		}
		r.remain = payload
	}
	n := copy(p, r.remain)
	r.remain = r.remain[n:]
	return n, nil
}

// scriptedPeerWriter answers the first outbound request with one canned inbound
// payload and then optionally blocks before returning. Holding the outbound
// write open keeps the caller inside sendMessage, which is how a test makes an
// already-buffered response and receive EOF simultaneously ready before the
// caller starts waiting for them.
type scriptedPeerWriter struct {
	reader *scriptedPeerReader
	reply  func(id json.RawMessage) string
	// afterReply runs on the caller's goroutine while the write is held, after
	// the reply has been handed to the receive loop.
	afterReply func()

	mu       sync.Mutex
	answered bool
}

func (w *scriptedPeerWriter) Write(p []byte) (int, error) {
	var outbound struct {
		ID     *json.RawMessage `json:"id"`
		Method string           `json:"method"`
	}
	if err := json.Unmarshal(p, &outbound); err != nil {
		return 0, err
	}
	if outbound.ID == nil || outbound.Method == "" {
		// Notification traffic such as $/cancel_request is not part of these
		// scripted exchanges.
		return len(p), nil
	}

	w.mu.Lock()
	first := !w.answered
	w.answered = true
	w.mu.Unlock()

	if first {
		if w.reply != nil {
			w.reader.deliver(w.reply(*outbound.ID))
		}
		if w.afterReply != nil {
			w.afterReply()
		}
	}
	return len(p), nil
}

// eofOnlyWriter never answers a request; it reports receive EOF and returns.
type eofOnlyWriter struct {
	reader *scriptedPeerReader
}

func (w *eofOnlyWriter) Write(p []byte) (int, error) {
	w.reader.closeWithEOF()
	return len(p), nil
}

const (
	// scriptedInitializeReply reports protocol version 999, which this SDK does
	// not support, so a delivered result cannot be confused with a default.
	scriptedInitializeReply = `{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":999,"agentCapabilities":{},"authMethods":[]}}` + "\n"
	// scriptedPromptNotification is one pre-response session/update notification.
	scriptedPromptNotification = `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"eof-fixture","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"primary result"}}}}` + "\n"
	// scriptedPromptReply is the final session/prompt response.
	scriptedPromptReply = `{"jsonrpc":"2.0","id":%s,"result":{"stopReason":"end_turn"}}` + "\n"

	scriptedInitializeReplyVersion = 999
)

func initializeReply(id json.RawMessage) string {
	return fmt.Sprintf(scriptedInitializeReply, string(id))
}

// promptReplyWithNotification delivers a pre-response notification and the final
// response in one payload, which is how the ordering barrier is exercised.
func promptReplyWithNotification(id json.RawMessage) string {
	return scriptedPromptNotification + fmt.Sprintf(scriptedPromptReply, string(id))
}

// requestOutcome carries one observed client-side request result.
type requestOutcome struct {
	stopReason StopReason
	err        error
}

// runPrompt starts a Prompt request and returns a channel that receives its
// single outcome.
func runPrompt(clientConn *ClientSideConnection, ctx context.Context) <-chan requestOutcome {
	outcomes := make(chan requestOutcome, 1)
	go func() {
		resp, err := clientConn.Prompt(ctx, PromptRequest{
			SessionId: "eof-fixture",
			Prompt:    []ContentBlock{TextBlock("fixture")},
		})
		outcomes <- requestOutcome{stopReason: resp.StopReason, err: err}
	}()
	return outcomes
}

func awaitSignal(t *testing.T, signal <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(eofWaitGuard):
		t.Fatalf("timeout waiting for %s", what)
	}
}

func requireRequestErrorCode(t *testing.T, err error, wantCode int) *RequestError {
	t.Helper()
	if err == nil {
		t.Fatalf("expected a typed request error, got success")
	}
	var reqErr *RequestError
	if !errors.As(err, &reqErr) {
		t.Fatalf("error %v (%T) is not a *RequestError", err, err)
	}
	if reqErr.Code != wantCode {
		t.Fatalf("error code = %d, want %d (error %v)", reqErr.Code, wantCode, err)
	}
	return reqErr
}

func requireRequestErrorData(t *testing.T, reqErr *RequestError, want string) {
	t.Helper()
	data, ok := reqErr.Data.(map[string]any)
	if !ok {
		t.Fatalf("error data = %#v, want a map carrying the error text", reqErr.Data)
	}
	if data["error"] != want {
		t.Fatalf("error text = %v, want %q", data["error"], want)
	}
}

// TestSendRequest_BufferedResponseWinsOverReceiveEOF forces the delivered race:
// the peer sends a complete response and then EOF while the outbound write is
// still held, so the response envelope is buffered and Done is already closed
// before waitForResponse runs. Both select cases are then ready and Go picks one
// at random, so only the buffer recheck keeps the delivered result alive.
func TestSendRequest_BufferedResponseWinsOverReceiveEOF(t *testing.T) {
	reader := newScriptedPeerReader()
	writer := &scriptedPeerWriter{reader: reader, reply: initializeReply}
	t.Cleanup(reader.closeWithEOF)

	clientConn := NewClientSideConnection(&clientFuncs{}, writer, reader)
	writer.afterReply = func() {
		reader.closeWithEOF()
		// Returning from the write only once Done is closed guarantees the
		// buffered response and receive EOF are both ready before the caller
		// starts waiting for them.
		<-clientConn.Done()
	}

	resp, err := clientConn.Initialize(context.Background(), InitializeRequest{ProtocolVersion: ProtocolVersionNumber})
	if err != nil {
		t.Fatalf("delivered initialize response was lost to receive EOF: %v", err)
	}
	if resp.ProtocolVersion != scriptedInitializeReplyVersion {
		t.Fatalf("protocol version = %d, want the delivered %d", resp.ProtocolVersion, scriptedInitializeReplyVersion)
	}
}

// TestSendRequest_MissingResponseAtReceiveEOFFails pins the failure that has to
// stay a failure: receive EOF with nothing buffered is a genuine missing
// response, so it remains a typed peer-disconnect error rather than a
// zero-valued success.
func TestSendRequest_MissingResponseAtReceiveEOFFails(t *testing.T) {
	reader := newScriptedPeerReader()
	t.Cleanup(reader.closeWithEOF)

	clientConn := NewClientSideConnection(&clientFuncs{}, &eofOnlyWriter{reader: reader}, reader)

	resp, err := clientConn.Initialize(context.Background(), InitializeRequest{ProtocolVersion: ProtocolVersionNumber})
	reqErr := requireRequestErrorCode(t, err, -32603)
	requireRequestErrorData(t, reqErr, "peer disconnected before response")
	if resp.ProtocolVersion != 0 {
		t.Fatalf("protocol version = %d, want the zero value on failure", resp.ProtocolVersion)
	}
}

// TestSendRequest_HeldNotificationHandlerCompletesAfterReceiveEOF covers the
// notification watermark carried by a delivered response. The pre-response
// notification handler is held while the final response and EOF both arrive, so
// Done closes first; releasing the handler must then deliver the final result
// instead of failing the request on disconnect.
func TestSendRequest_HeldNotificationHandlerCompletesAfterReceiveEOF(t *testing.T) {
	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	release := sync.OnceFunc(func() { close(releaseHandler) })
	t.Cleanup(release)

	client := &clientFuncs{SessionUpdateFunc: func(context.Context, SessionNotification) error {
		close(handlerStarted)
		<-releaseHandler
		return nil
	}}

	reader := newScriptedPeerReader()
	writer := &scriptedPeerWriter{reader: reader, reply: promptReplyWithNotification}
	t.Cleanup(reader.closeWithEOF)

	clientConn := NewClientSideConnection(client, writer, reader)
	eofObserved := make(chan struct{})
	writer.afterReply = func() {
		reader.closeWithEOF()
		<-clientConn.Done()
		close(eofObserved)
	}

	outcomes := runPrompt(clientConn, context.Background())
	awaitSignal(t, handlerStarted, "pre-response notification handler to start")
	awaitSignal(t, eofObserved, "receive EOF while the notification handler is held")

	select {
	case outcome := <-outcomes:
		t.Fatalf("Prompt returned before the held notification handler finished: %+v", outcome)
	default:
	}

	release()

	select {
	case outcome := <-outcomes:
		if outcome.err != nil {
			t.Fatalf("Prompt lost the delivered result after receive EOF: %v", outcome.err)
		}
		if outcome.stopReason != StopReasonEndTurn {
			t.Fatalf("stop reason = %q, want %q", outcome.stopReason, StopReasonEndTurn)
		}
	case <-time.After(eofWaitGuard):
		t.Fatalf("timeout waiting for Prompt to return after the notification handler finished")
	}
}

// TestSendRequest_CallerCancellationWhileHandlerHeldFails proves that waiting
// through the drain does not weaken caller cancellation: while the notification
// handler is still held, a canceled caller context must fail the request with
// the caller's own cancellation and must not report a result. The connection is
// deliberately left open so the observed cause is unambiguously the caller.
func TestSendRequest_CallerCancellationWhileHandlerHeldFails(t *testing.T) {
	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	release := sync.OnceFunc(func() { close(releaseHandler) })
	t.Cleanup(release)

	client := &clientFuncs{SessionUpdateFunc: func(context.Context, SessionNotification) error {
		close(handlerStarted)
		<-releaseHandler
		return nil
	}}

	reader := newScriptedPeerReader()
	writer := &scriptedPeerWriter{reader: reader, reply: promptReplyWithNotification}
	t.Cleanup(reader.closeWithEOF)

	clientConn := NewClientSideConnection(client, writer, reader)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	outcomes := runPrompt(clientConn, ctx)
	awaitSignal(t, handlerStarted, "pre-response notification handler to start")

	cancel()

	select {
	case outcome := <-outcomes:
		requireRequestErrorCode(t, outcome.err, -32800)
		data, ok := outcome.err.(*RequestError).Data.(map[string]any)
		if !ok || !strings.Contains(fmt.Sprint(data["error"]), context.Canceled.Error()) {
			t.Fatalf("cancellation error data = %#v, want it to report context canceled", outcome.err)
		}
		if outcome.stopReason != "" {
			t.Fatalf("stop reason = %q, want no result on a canceled request", outcome.stopReason)
		}
	case <-time.After(eofWaitGuard):
		t.Fatalf("timeout waiting for the canceled request to fail")
	}
}

// TestSendRequest_IncompleteInboundDrainAfterReceiveEOFFails covers the drain
// that cannot finish. The pre-response notification handler is never released
// before the connection closes, so the bounded inbound drain runs out, inboundCtx
// ends, and the request must fail with the typed pre-response-notification error
// instead of blocking forever or reporting success.
//
// The upstream drain bound is the behavior under test here, so waiting for
// inboundCtx is the observation; no shorter deterministic trigger exists.
func TestSendRequest_IncompleteInboundDrainAfterReceiveEOFFails(t *testing.T) {
	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	release := sync.OnceFunc(func() { close(releaseHandler) })
	t.Cleanup(release)

	client := &clientFuncs{SessionUpdateFunc: func(context.Context, SessionNotification) error {
		close(handlerStarted)
		<-releaseHandler
		return nil
	}}

	reader := newScriptedPeerReader()
	writer := &scriptedPeerWriter{reader: reader, reply: promptReplyWithNotification}
	t.Cleanup(reader.closeWithEOF)

	clientConn := NewClientSideConnection(client, writer, reader)
	writer.afterReply = func() {
		reader.closeWithEOF()
		<-clientConn.Done()
	}

	outcomes := runPrompt(clientConn, context.Background())
	awaitSignal(t, handlerStarted, "pre-response notification handler to start")

	select {
	case <-clientConn.conn.inboundCtx.Done():
	case <-time.After(2 * notificationQueueDrainTimeout):
		t.Fatalf("timeout waiting for the bounded inbound drain to end")
	}

	select {
	case outcome := <-outcomes:
		reqErr := requireRequestErrorCode(t, outcome.err, -32603)
		requireRequestErrorData(t, reqErr, "peer disconnected while waiting for pre-response notifications")
		if outcome.stopReason != "" {
			t.Fatalf("stop reason = %q, want no result when the drain cannot finish", outcome.stopReason)
		}
	case <-time.After(eofWaitGuard):
		t.Fatalf("timeout waiting for the incomplete drain to fail the request")
	}
}

// The scenarios below cover the second half of the repair: a delivered response
// is not a result while the caller has stopped waiting for it. The peer can
// answer and close the stream, and the caller can end its context, before the
// outbound write returns, so the caller only reaches the response wait after it
// already gave up. Readiness is still forced with explicit channels: nothing
// here sleeps, polls, or depends on process scheduling.

// silenceConnectionLogs keeps the forced disconnects out of the test log. The
// disconnect itself is the fixture, not a diagnostic worth reporting.
func silenceConnectionLogs(clientConn *ClientSideConnection) {
	clientConn.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// forcedResponseBeforeCallerEndDraw runs one Initialize draw in which the peer
// answers, the stream then reaches receive EOF, and endCaller ends the caller
// context, all while the outbound write is still held open. Returning from the
// write is the last event before the caller starts waiting for the response, so
// the buffered response, the closed Done channel, and the ended context are
// simultaneously ready. A nil endCaller leaves the context to the caller.
func forcedResponseBeforeCallerEndDraw(t *testing.T, ctx context.Context, cancel context.CancelFunc, endCaller func()) (InitializeResponse, error) {
	t.Helper()

	reader := newScriptedPeerReader()
	writer := &scriptedPeerWriter{reader: reader, reply: initializeReply}
	t.Cleanup(reader.closeWithEOF)

	clientConn := NewClientSideConnection(&clientFuncs{}, writer, reader)
	silenceConnectionLogs(clientConn)
	writer.afterReply = func() {
		reader.closeWithEOF()
		// Waiting for Done keeps the outbound write open until the disconnect that
		// follows the delivered response has already been observed.
		<-clientConn.Done()
		if endCaller != nil {
			endCaller()
		}
	}

	return clientConn.Initialize(ctx, InitializeRequest{ProtocolVersion: ProtocolVersionNumber})
}

// requireNoFalseSuccess requires the caller's exact cause to win over EOF.
func requireNoFalseSuccess(t *testing.T, resp InitializeResponse, err error, cause error) {
	t.Helper()
	if resp.ProtocolVersion != 0 {
		t.Fatalf("protocol version = %d, want no result after caller cancellation", resp.ProtocolVersion)
	}
	wantCode := -32603
	if errors.Is(cause, context.Canceled) {
		wantCode = -32800
	}
	reqErr := requireRequestErrorCode(t, err, wantCode)
	requireRequestErrorData(t, reqErr, cause.Error())
}

// TestSendRequest_CanceledCallerBeforeWriteReturnsNeverSucceeds forces the
// reported false success. The peer answers and closes the stream, and the caller
// is canceled, all before the outbound write returns, so the caller used to be
// handed the buffered version-999 response with no error. Every draw must fail
// with a zero result instead.
func TestSendRequest_CanceledCallerBeforeWriteReturnsNeverSucceeds(t *testing.T) {
	// The witness drew 16 times and reported 14 false successes; the draws here
	// repeat that count so the same magnitude of false success cannot pass.
	const draws = 16

	for draw := 1; draw <= draws; draw++ {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		resp, err := forcedResponseBeforeCallerEndDraw(t, ctx, cancel, cancel)
		requireNoFalseSuccess(t, resp, err, ctx.Err())
	}
}

// TestSendRequest_ExpiredDeadlineBeforeWriteReturnsNeverSucceeds is the same
// forced ordering for a deadline instead of an explicit cancel. The deadline is
// already in the past when the request starts, so no clock wait is involved.
func TestSendRequest_ExpiredDeadlineBeforeWriteReturnsNeverSucceeds(t *testing.T) {
	const draws = 16

	for draw := 1; draw <= draws; draw++ {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-eofWaitGuard))
		t.Cleanup(cancel)

		resp, err := forcedResponseBeforeCallerEndDraw(t, ctx, cancel, nil)
		requireNoFalseSuccess(t, resp, err, ctx.Err())
	}
}

// TestSendRequest_CanceledCallerWithBufferedResponseReportsCallerCause pins the
// caller cause itself. The connection stays open here, so the buffered response
// and the ended context are the only ready cases and both report the caller's
// own cancellation rather than a result.
func TestSendRequest_CanceledCallerWithBufferedResponseReportsCallerCause(t *testing.T) {
	reader := newScriptedPeerReader()
	writer := &scriptedPeerWriter{reader: reader, reply: initializeReply}
	t.Cleanup(reader.closeWithEOF)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	clientConn := NewClientSideConnection(&clientFuncs{}, writer, reader)
	silenceConnectionLogs(clientConn)
	writer.afterReply = cancel

	resp, err := clientConn.Initialize(ctx, InitializeRequest{ProtocolVersion: ProtocolVersionNumber})
	reqErr := requireRequestErrorCode(t, err, -32800)
	requireRequestErrorData(t, reqErr, context.Canceled.Error())
	if resp.ProtocolVersion != 0 {
		t.Fatalf("protocol version = %d, want the zero value on a canceled request", resp.ProtocolVersion)
	}
}

// TestSendRequestNoResult_CanceledCallerBeforeWriteReturnsFails covers the same
// hole on the request variant that returns no payload. It has no decode step to
// lose a partial result in, so its final success return is the only place the
// caller's own cancellation can still be honored.
func TestSendRequestNoResult_CanceledCallerBeforeWriteReturnsFails(t *testing.T) {
	reader := newScriptedPeerReader()
	writer := &scriptedPeerWriter{reader: reader, reply: initializeReply}
	t.Cleanup(reader.closeWithEOF)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	clientConn := NewClientSideConnection(&clientFuncs{}, writer, reader)
	silenceConnectionLogs(clientConn)
	writer.afterReply = cancel

	reqErr := requireRequestErrorCode(t, clientConn.conn.SendRequestNoResult(ctx, "test/no-result", nil), -32800)
	requireRequestErrorData(t, reqErr, context.Canceled.Error())
}

// waitForCompletedNotification blocks until the connection has finished the
// given notification sequence. It observes the connection's own condition
// variable, so it cannot miss a completion that already happened.
func waitForCompletedNotification(t *testing.T, conn *Connection, target uint64) {
	t.Helper()

	conn.notifyMu.Lock()
	defer conn.notifyMu.Unlock()
	for conn.completedNotificationSeq < target {
		conn.notifyCond.Wait()
	}
}

// TestWaitNotificationsUpTo_CanceledCallerNeverCompletes exercises the watermark
// helper on the two paths that used to return success without looking at the
// caller: the zero watermark that returns immediately, and a watermark that is
// already reached when the loop is skipped. A live caller must still complete on
// both, so the fix cannot be paid for with a lost success.
func TestWaitNotificationsUpTo_CanceledCallerNeverCompletes(t *testing.T) {
	handled := make(chan struct{})
	handledOnce := sync.OnceFunc(func() { close(handled) })

	reader := newScriptedPeerReader()
	conn := NewConnection(func(context.Context, string, json.RawMessage) (any, *RequestError) {
		handledOnce()
		return nil, nil
	}, io.Discard, reader)
	t.Cleanup(reader.closeWithEOF)
	conn.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))

	reader.deliver(scriptedPromptNotification)
	awaitSignal(t, handled, "the notification handler to run")
	waitForCompletedNotification(t, conn, 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for _, target := range []uint64{0, 1} {
		reqErr := requireRequestErrorCode(t, conn.waitNotificationsUpTo(ctx, target), -32800)
		requireRequestErrorData(t, reqErr, context.Canceled.Error())
	}

	for _, target := range []uint64{0, 1} {
		if err := conn.waitNotificationsUpTo(context.Background(), target); err != nil {
			t.Fatalf("watermark %d for a live caller = %v, want success", target, err)
		}
	}
}

// cancelingDecodeTarget stands in for a result type whose decode can take
// arbitrarily long. It finishes decoding only after the caller context has ended
// and still reports success, which is the state a slow decoder leaves behind
// when the caller gives up mid-decode.
var decodeHook struct {
	sync.Mutex
	run func()
}

type cancelingDecodeTarget string

func (target *cancelingDecodeTarget) UnmarshalJSON([]byte) error {
	decodeHook.Lock()
	run := decodeHook.run
	decodeHook.Unlock()
	run()
	*target = "decoded after cancellation"
	return nil
}

// TestSendRequest_CancelDuringCustomDecodeReturnsZeroResult proves that a
// cancellation that happens during the decode cannot publish the decoded result.
// The decode is released by the real context cancellation, so the request is
// guaranteed to have produced a complete payload that still has to be dropped.
func TestSendRequest_CancelDuringCustomDecodeReturnsZeroResult(t *testing.T) {
	reader := newScriptedPeerReader()
	writer := &scriptedPeerWriter{reader: reader, reply: initializeReply}
	t.Cleanup(reader.closeWithEOF)

	clientConn := NewClientSideConnection(&clientFuncs{}, writer, reader)
	silenceConnectionLogs(clientConn)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	decoded := new(string)
	started := make(chan struct{})
	decodeHook.Lock()
	decodeHook.run = func() { close(started); <-ctx.Done(); *decoded = "decoded after cancellation" }
	decodeHook.Unlock()
	t.Cleanup(func() { decodeHook.Lock(); decodeHook.run = nil; decodeHook.Unlock() })

	type decodeOutcome struct {
		result cancelingDecodeTarget
		err    error
	}
	outcomes := make(chan decodeOutcome, 1)
	go func() {
		result, err := SendRequest[cancelingDecodeTarget](clientConn.conn, ctx, "test/decode", nil)
		outcomes <- decodeOutcome{result: result, err: err}
	}()

	awaitSignal(t, started, "the custom decoder to start")
	cancel()

	select {
	case outcome := <-outcomes:
		if outcome.result != "" {
			t.Fatalf("a request canceled during its decode returned %#v, want no result", outcome.result)
		}
		if *decoded != "decoded after cancellation" {
			t.Fatalf("decoded payload = %q, want the decode to have run to completion", *decoded)
		}
		reqErr := requireRequestErrorCode(t, outcome.err, -32800)
		requireRequestErrorData(t, reqErr, context.Canceled.Error())
	case <-time.After(eofWaitGuard):
		t.Fatalf("timeout waiting for the request canceled during its decode to fail")
	}
}
