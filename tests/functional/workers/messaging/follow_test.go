package messaging

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// Serve sequential customer host invocations on one root Process. Stopping a
// host releases Messaging observers without closing process-owned Workers.
type deliveryHostRebind struct{ bound chan string }

func (g *deliveryHostRebind) wrap(next platformhttpserver.Starter) platformhttpserver.Starter {
	first := true
	return func(ctx context.Context, request platformhttpserver.StartRequest) error {
		onBound := request.OnBound
		request.OnBound = func(binding platformhttpserver.Binding) {
			onBound(binding)
			g.bound <- fmt.Sprintf("http://%s:%d", binding.Host, binding.Port)
		}
		if first {
			first = false
			return next(ctx, request)
		}
		return support.NewProcessAPIServer().Start(ctx, request)
	}
}

func TestActiveRecoveryRebindPreservesPendingUntilProviderProgress(t *testing.T) {
	t.Parallel()
	progress := make(chan struct{})
	revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{}), progress: progress}
	host := &deliveryHostRebind{bound: make(chan string, 1)}
	core, _ := observer.New(zap.InfoLevel)
	signal := &subscriptionSignal{Core: core, ready: make(chan struct{}), once: &sync.Once{}, workerRegistrations: make(chan string, 32)}
	f := startHostRunner(t, t.TempDir(), t.TempDir(), "", serviceedges.Edges{ProcessLogger: zap.New(signal)}, host.wrap, revival, "lead", "lane")
	f.endpoint = <-host.bound
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	f.endWorker(t, "lead")
	before := messageFactoryStreams(t, f, "lead", "lane")
	delete(before, "~default") // Host lifecycle changes its own default Session.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	frames, finished := make(chan []byte, 16), make(chan error, 1)
	go streamFramesAt(ctx, f.endpoint, "/messages?follow=true", lane, frames, finished)
	select {
	case <-signal.ready:
	case <-ctx.Done():
		t.Fatal("active recovery follow did not register")
	}
	request := map[string]any{"requestId": "provider-confirmation", "to": map[string]string{"workerSessionId": "lead"}, "body": "normalized continuation input"}
	message := f.sendRaw(t, request, lane)
	assertPendingProviderResponse(t, message)
	assertPendingProviderFrames(t, ctx, message, frames, finished)
	successor := <-revival.started
	command := <-revival.requests
	if string(command.Stdin) != message.Body || !strings.Contains(strings.Join(command.Args, " "), "resume controlled-thread") {
		t.Fatal("active recovery changed the recorded resume input")
	}
	awaitActiveRecoveryObserver(t, ctx, signal, successor["YOU_WORKER_SESSION_ID"])
	cancel()
	<-finished
	f.endWorker(t, "lane")
	f.server.Stop(t)
	ctx, cancel = context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	repeatActiveRecoveryHosts(t, ctx, f, host, signal, revival, successor, lane, request, message)
	stop := restartActiveDeliveryHost(t, f, host)
	defer stop()
	ctx, cancel = context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	frames, finished = make(chan []byte, 16), make(chan error, 1)
	go streamFramesAt(ctx, f.endpoint, "/messages?follow=true&maxResults=64", nil, frames, finished)
	// Register the public follow before releasing provider progress.
	// Operator observation remains read-only after the original caller ends.
	awaitActiveRecoveryObserver(t, ctx, signal, successor["YOU_WORKER_SESSION_ID"])
	awaitActiveRecoveryObserver(t, ctx, signal, "")
	close(progress)
	final := f.awaitDeliveryOutcome(t, nil, message)
	assertProviderFinalOutcome(t, ctx, f, final, successor, frames, finished, "")
	assertActiveRecoveryRead(t, f, lane, request, final)
	after := messageFactoryStreams(t, f, "lead", "lane")
	delete(after, "~default")
	if !reflect.DeepEqual(before, after) {
		t.Fatal("active recovery changed canonical Factory Events")
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(15 * time.Second):
		t.Fatal("active recovery follow did not join")
	}
	close(revival.release)
	f.waitEndedWorker(t, successor["YOU_WORKER_SESSION_ID"])
	stop()
	f.server.Close(t)
	assertRestartedDelivery(t, f, revival, message, final, successor["YOU_WORKER_SESSION_ID"], false)
}

func repeatActiveRecoveryHosts(t *testing.T, ctx context.Context, f *fixture, host *deliveryHostRebind, signal *subscriptionSignal,
	revival *workerGate, successor, lane map[string]string, request map[string]any, message api.AgentMessage) {
	t.Helper()
	for range 2 {
		stop := restartActiveDeliveryHost(t, f, host)
		awaitActiveRecoveryObserver(t, ctx, signal, successor["YOU_WORKER_SESSION_ID"])
		assertActiveRecoveryRead(t, f, lane, request, message)
		select {
		case <-revival.requests:
			t.Fatal("active recovery branched another provider command")
		default:
		}
		stop()
	}
}

func restartActiveDeliveryHost(t *testing.T, f *fixture, host *deliveryHostRebind) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	inputs := support.FakeInputs(ctx, []string{"you", "run", "--continuously", "--with-server", "--quiet", "--dir", f.dir, "--no-record", "--listen", "127.0.0.1:1"})
	inputs.Input.Env, inputs.Input.WorkingDirectory = f.env, f.dir
	done := make(chan error, 1)
	go func() { done <- f.server.Execute(t, inputs.Input) }()
	select {
	case f.endpoint = <-host.bound:
	case err := <-done:
		t.Fatalf("replacement host failed: %v %s", err, inputs.Stderr())
	case <-time.After(15 * time.Second):
		t.Fatal("replacement host did not bind")
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case <-done:
			case <-time.After(15 * time.Second):
				t.Fatal("replacement host did not join")
			}
		})
	}
	t.Cleanup(stop)
	return stop
}

func awaitActiveRecoveryObserver(t *testing.T, ctx context.Context, signal *subscriptionSignal, successor string) {
	t.Helper()
	for {
		select {
		case topic := <-signal.workerRegistrations:
			if topic == "worker-session/"+successor+"/events" || (successor == "" && topic == "agent-messages/events") {
				return
			}
		case <-ctx.Done():
			t.Fatal("active successor observer did not register")
		}
	}
}

func assertActiveRecoveryRead(t *testing.T, f *fixture, lane map[string]string, request map[string]any, message api.AgentMessage) {
	t.Helper()
	results := make(chan outcomeSendResponse, 2)
	for range 2 {
		go func() {
			status, body := f.request(t, http.MethodPost, "/messages", request, lane)
			results <- outcomeSendResponse{status, body}
		}()
	}
	for range 2 {
		var result outcomeSendResponse
		select {
		case result = <-results:
		case <-time.After(15 * time.Second):
			t.Fatal("active recovery retry did not complete")
		}
		var response api.ErrorResponse
		if result.status != http.StatusForbidden || json.Unmarshal(result.body, &response) != nil || response.Code != "MESSAGE_NOT_PERMITTED" {
			t.Fatalf("released host caller retained authority: %d %s", result.status, result.body)
		}
	}
	page := readPage(t, f, "/messages?threadId="+message.ThreadId, nil)
	var cli api.AgentMessageListResponse
	if json.Unmarshal(f.cli(t, nil, "list", "--thread", message.ThreadId), &cli) != nil || len(page.Messages) != 1 ||
		!reflect.DeepEqual(page.Messages[0], message) || !reflect.DeepEqual(cli.Messages, page.Messages) {
		t.Fatal("rebound host lost original pending delivery or CLI/HTTP read parity")
	}
}

// A blocked alias flush lets the real recipient subscription fill its live
// buffer. Gates act only on injected filesystem/provider edges, not Events.
func TestRecipientEndDeliverySurvivesObservationBackpressure(t *testing.T) {
	t.Parallel()
	core, _ := observer.New(zap.InfoLevel)
	signal := &subscriptionSignal{Core: core, ready: make(chan struct{}), once: &sync.Once{}, backpressureTopic: "worker-session/lead/events",
		registered: make(chan string, 2)}
	flush := &messageOutcomeFileSystem{entered: make(chan struct{}), release: make(chan struct{}), kind: "REQUEST_ALIAS"}
	revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
	f := startHostRunner(t, t.TempDir(), t.TempDir(), "", serviceedges.Edges{ProcessLogger: zap.New(signal), AgentMessageStoreFileSystem: flush}, nil, revival, "lead", "lane")
	t.Cleanup(func() { flush.unblock.Do(func() { close(flush.release) }) })
	gate := f.runner.workers["lead"]
	// Source-native stderr progress uses the same Worker topic, without
	// claiming provider-confirmed resumed work.
	var output strings.Builder
	for i := range 128 {
		fmt.Fprintf(&output, "controlled progress %d\n", i)
	}
	gate.startupDiagnostics = []byte(output.String())
	progress := make(chan struct{})
	gate.progress = progress
	lead := f.invoke(t, "lead", nil)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	lane := f.invoke(t, "lane", lead)
	first := f.cliSend(t, lane, "lead", "--request-id", "first", "--body", "first after backpressure")
	select {
	case cursor := <-signal.registered:
		if cursor != "0" {
			t.Fatal("initial observer omitted retained catch-up")
		}
	case <-ctx.Done():
		t.Fatal("recipient observer did not register")
	}
	aliases := make(chan api.AgentMessage, 1)
	go func() { aliases <- f.cliSend(t, lane, "lead", "--request-id", "alias", "--body", first.Body) }()
	select {
	case <-flush.entered:
	case <-ctx.Done():
		t.Fatal("alias did not reach its durable flush")
	}
	close(progress)
	select {
	case <-signal.ready:
	case <-ctx.Done():
		t.Fatal("recipient observer did not encounter actual backpressure")
	}
	flush.unblock.Do(func() { close(flush.release) })
	select {
	case alias := <-aliases:
		if !reflect.DeepEqual(alias, first) {
			t.Fatal("blocked alias changed the original message")
		}
	case <-ctx.Done():
		t.Fatal("alias did not complete after flush release")
	}
	select {
	case cursor := <-signal.registered:
		if cursor == "0" {
			t.Fatal("backpressure replayed the entire retained stream")
		}
	case <-ctx.Done():
		t.Fatal("recipient observer did not recover after backpressure")
	}
	assertBackpressureRevival(t, f, revival, first, lead, lane)
}

func assertBackpressureRevival(t *testing.T, f *fixture, revival *workerGate, first api.AgentMessage, lead, lane map[string]string) {
	t.Helper()
	second := f.cliSend(t, lane, "lead", "--request-id", "second", "--body", "second after backpressure")
	held := f.cliSend(t, lane, "lead", "--request-id", "hold", "--body", "keep held", "--if-ended", "hold")
	if first.Status != "QUEUED" || second.Status != "QUEUED" || f.workerEnded(t, "lead") {
		t.Fatal("backpressure guessed recipient completion")
	}
	before := messageFactoryStreams(t, f, "lead", "lane")
	f.endWorker(t, "lead")
	pending := "REVIVE_PENDING"
	first.Reason = &pending
	final := f.awaitDeliveryOutcome(t, lane, first)
	batch := final
	batch.Body = first.Body + "\n\n" + second.Body
	head := assertReviveAdmission(t, f, revival, batch, lead)
	page := readPage(t, f, "/messages?toWorkerSessionId=lead", nil)
	if len(page.Messages) != 3 || page.Messages[0].MessageId != first.MessageId || page.Messages[1].MessageId != second.MessageId ||
		page.Messages[1].Status != "DELIVERED_REVIVE" || page.Messages[1].RevivedWorkerSessionId == nil ||
		*page.Messages[1].RevivedWorkerSessionId != head["YOU_WORKER_SESSION_ID"] || !reflect.DeepEqual(page.Messages[2], held) {
		t.Fatal("backpressure lost ordered delivery or changed HOLD")
	}
	close(revival.release)
	f.waitEndedWorker(t, head["YOU_WORKER_SESSION_ID"])
	if retry := f.cliSend(t, lane, "lead", "--request-id", "first", "--body", first.Body); !reflect.DeepEqual(retry, final) {
		t.Fatal("backpressure delivery retry changed durable successor facts")
	}
	select {
	case <-revival.requests:
		t.Fatal("retained/live replay duplicated continuation")
	default:
	}
	if after := messageFactoryStreams(t, f, "lead", "lane"); !reflect.DeepEqual(before, after) {
		t.Fatal("backpressure messaging changed Factory Events")
	}
}

// Hold one customer response write, without changing the handler, Events or
// durable store. Three messages suffice to exhaust a one-record subscription.
type heldStreamWriter struct {
	http.ResponseWriter
	ctx              context.Context
	entered, release chan struct{}
	once             sync.Once
}

func (w *heldStreamWriter) Write(body []byte) (int, error) {
	if strings.Contains(string(body), "event: message") {
		w.once.Do(func() {
			close(w.entered)
			select {
			case <-w.release:
			case <-w.ctx.Done():
			}
		})
	}
	return w.ResponseWriter.Write(body)
}

func (w *heldStreamWriter) Flush() { w.ResponseWriter.(http.Flusher).Flush() }

func heldFollowTransport(entered, release chan struct{}) func(platformhttpserver.Starter) platformhttpserver.Starter {
	return func(start platformhttpserver.Starter) platformhttpserver.Starter {
		return func(ctx context.Context, request platformhttpserver.StartRequest) error {
			handler := request.Handler
			request.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/messages" && r.URL.Query().Get("follow") == "true" {
					w = &heldStreamWriter{ResponseWriter: w, ctx: r.Context(), entered: entered, release: release}
				}
				handler.ServeHTTP(w, r)
			})
			return start(ctx, request)
		}
	}
}

func TestInboxFollowBackpressurePreservesDurablePull(t *testing.T) {
	t.Parallel()
	core, _ := observer.New(zap.InfoLevel)
	signal := &subscriptionSignal{Core: core, ready: make(chan struct{}), once: &sync.Once{}}
	entered, release := make(chan struct{}), make(chan struct{})
	f := startHostTransport(t, t.TempDir(), t.TempDir(), "", serviceedges.Edges{ProcessLogger: zap.New(signal)}, heldFollowTransport(entered, release), "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	frames, finished := make(chan []byte, 8), make(chan error, 1)
	go streamFramesAt(ctx, f.server.URL(), "/messages?follow=true&toMe=true&maxResults=1", lead, frames, finished)
	select {
	case <-signal.ready:
	case <-ctx.Done():
		t.Fatal("follow did not register")
	}
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	first := f.send(t, map[string]any{"requestId": "slow-0", "to": map[string]string{"workerSessionId": "lead"}, "body": "slow consumer first"}, lane)
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("follow did not reach controlled transport write")
	}
	for i := 1; i < 3; i++ {
		f.send(t, map[string]any{"requestId": fmt.Sprintf("slow-%d", i), "to": map[string]string{"workerSessionId": "lead"}, "body": fmt.Sprintf("slow consumer %d", i)}, lane)
	}
	close(release)
	released = true
	assertBackpressureFrames(t, ctx, frames, finished, first.MessageId)
	page := readPage(t, f, "/messages?toMe=true", lead)
	if len(page.Messages) != 3 {
		t.Fatal("backpressure lost committed inbox records")
	}
	for _, message := range page.Messages {
		if message.Status != "QUEUED" {
			t.Fatal("follow changed durable read status")
		}
	}
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("follow did not terminate")
	}
}

func assertBackpressureFrames(t *testing.T, ctx context.Context, frames <-chan []byte, finished <-chan error, first string) {
	t.Helper()
	var observation api.AgentMessageObservation
	if json.Unmarshal(nextFrame(t, ctx, frames, finished), &observation) != nil || observation.Message.MessageId != first {
		t.Fatal("slow stream lost its initial admitted record")
	}
	// Events drains the already queued record before reporting its terminal fault.
	if json.Unmarshal(nextFrame(t, ctx, frames, finished), &observation) != nil || observation.Kind != "SENT" {
		t.Fatal("slow stream did not drain the queued observation")
	}
	var failure api.ErrorResponse
	if json.Unmarshal(nextFrame(t, ctx, frames, finished), &failure) != nil || failure.Code != "MESSAGE_STREAM_BACKPRESSURE" {
		t.Fatal("slow stream did not report typed backpressure")
	}
}

// An injected diagnostic sink supplies a registration gate, never the
// behavioral assertion. Assertions consume only the customer HTTP stream.
type subscriptionSignal struct {
	zapcore.Core
	ready               chan struct{}
	once                *sync.Once
	backpressureTopic   string
	registered          chan string
	workerRegistrations chan string
}

func (c *subscriptionSignal) With(fields []zapcore.Field) zapcore.Core {
	return &subscriptionSignal{Core: c.Core.With(fields), ready: c.ready, once: c.once, backpressureTopic: c.backpressureTopic,
		registered: c.registered, workerRegistrations: c.workerRegistrations}
}
func (c *subscriptionSignal) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return checked.AddCore(entry, c)
	}
	return checked
}
func (c *subscriptionSignal) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	values := zapcore.NewMapObjectEncoder()
	for _, field := range fields {
		field.AddTo(values)
	}
	if entry.Message == "events subscribe outcome" && values.Fields["topic"] == "agent-messages/events" && values.Fields["outcome"] == "accepted" {
		if c.backpressureTopic == "" {
			c.once.Do(func() { close(c.ready) })
		}
	}
	if c.backpressureTopic != "" && entry.Message == "events subscribe backpressure" && values.Fields["topic"] == c.backpressureTopic {
		c.once.Do(func() { close(c.ready) })
	}
	if c.registered != nil && entry.Message == "events subscribe outcome" && values.Fields["topic"] == c.backpressureTopic && fmt.Sprint(values.Fields["limit"]) == "64" {
		select {
		case c.registered <- fmt.Sprint(values.Fields["from"]):
		default:
		}
	}
	if c.workerRegistrations != nil && entry.Message == "events subscribe outcome" && values.Fields["outcome"] == "accepted" && fmt.Sprint(values.Fields["limit"]) == "64" {
		select {
		case c.workerRegistrations <- fmt.Sprint(values.Fields["topic"]):
		default:
		}
	}
	return c.Core.Write(entry, fields)
}

func TestInboxFollowRevalidatesAuthorityBeforePublication(t *testing.T) {
	t.Parallel()
	core, _ := observer.New(zap.InfoLevel)
	signal := &subscriptionSignal{Core: core, ready: make(chan struct{}), once: &sync.Once{}}
	f := startHostEffects(t, t.TempDir(), t.TempDir(), "", serviceedges.Edges{ProcessLogger: zap.New(signal)}, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	frames := make(chan []byte, 2)
	finished := make(chan error, 1)
	go streamFrames(ctx, f.server.URL(), lead, frames, finished)
	select {
	case <-signal.ready:
	case err := <-finished:
		t.Fatalf("follow before readiness: %v", err)
	case <-ctx.Done():
		t.Fatal("follow did not register")
	}
	message := f.send(t, map[string]any{"requestId": "live", "to": map[string]string{"workerSessionId": "lead"}, "body": "live inbox"}, lane)
	var observation api.AgentMessageObservation
	frame := nextFrame(t, ctx, frames, finished)
	if json.Unmarshal(frame, &observation) != nil || observation.Message.MessageId != message.MessageId || observation.Kind != "SENT" {
		t.Fatal("follow did not publish admitted message")
	}
	status, _ := f.request(t, http.MethodPost, "/worker-sessions/lead/cancel", map[string]any{}, nil)
	if status != http.StatusOK {
		t.Fatal("controlled recipient did not cancel")
	}
	f.send(t, map[string]any{"requestId": "after-end", "to": map[string]string{"workerSessionId": "lead"}, "body": "must not publish after owner loss"}, lane)
	frame = nextFrame(t, ctx, frames, finished)
	var failure api.ErrorResponse
	if json.Unmarshal(frame, &failure) != nil || failure.Code != "MESSAGE_NOT_PERMITTED" || strings.Contains(string(frame), "must not publish") {
		t.Fatal("follow published after recipient authority loss")
	}
	cancel()
	select {
	case <-finished:
	case <-t.Context().Done():
		t.Fatal("follow did not join")
	}
}

func nextFrame(t *testing.T, ctx context.Context, frames <-chan []byte, finished <-chan error) []byte {
	t.Helper()
	select {
	case frame := <-frames:
		return frame
	case err := <-finished:
		// The producer queues the last frame before reporting completion.
		// A select can choose completion while that frame is already buffered.
		select {
		case frame := <-frames:
			return frame
		default:
		}
		t.Fatalf("follow ended before frame: %v", err)
	case <-ctx.Done():
		t.Fatal("follow frame unavailable")
	}
	return nil
}

func streamFrames(ctx context.Context, server string, caller map[string]string, frames chan<- []byte, finished chan<- error) {
	streamFramesAt(ctx, server, "/messages?follow=true&toMe=true&fromWorkerSessionId=lane", caller, frames, finished)
}

func streamFramesAt(ctx context.Context, server, path string, caller map[string]string, frames chan<- []byte, finished chan<- error) {
	defer close(finished)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server+path, nil)
	if err != nil {
		finished <- err
		return
	}
	if caller != nil {
		req.Header.Set("X-You-Worker-Session-Id", caller["YOU_WORKER_SESSION_ID"])
		req.Header.Set("Authorization", "Bearer "+caller["YOU_WORKER_SESSION_TOKEN"])
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		finished <- err
		return
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		if data, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
			select {
			case frames <- []byte(data):
			case <-ctx.Done():
				finished <- ctx.Err()
				return
			}
		}
	}
	finished <- scanner.Err()
}

func TestInboxFollowPublishesOnlyAuthorizedRedactedContent(t *testing.T) {
	t.Parallel()
	core, logs := observer.New(zap.DebugLevel)
	signal := &subscriptionSignal{Core: core, ready: make(chan struct{}), once: &sync.Once{}}
	f := startHostEffects(t, t.TempDir(), t.TempDir(), "", serviceedges.Edges{ProcessLogger: zap.New(signal)}, "lead", "lane", "lane-token", "other", "other-lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	tokenLane := f.invoke(t, "lane-token", lead)
	other := f.invoke(t, "other", nil)
	otherLane := f.invoke(t, "other-lane", other)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	frames, finished := make(chan []byte, 8), make(chan error, 1)
	go streamFramesAt(ctx, f.server.URL(), "/messages?follow=true&toMe=true", lead, frames, finished)
	select {
	case <-signal.ready:
	case <-ctx.Done():
		t.Fatal("follow did not register")
	case <-finished:
		t.Fatal("follow ended before registration")
	}
	foreign := f.send(t, map[string]any{"requestId": "foreign", "to": map[string]string{"workerSessionId": "other"}, "body": "unrelated private inbox"}, otherLane)
	secret := "declared-stream-secret"
	message := f.send(t, map[string]any{"requestId": "private", "to": map[string]string{"workerSessionId": "lead"}, "body": secret, "bodySecret": true}, lane)
	assertRedactedFrame(t, nextFrame(t, ctx, frames, finished), message.MessageId, "SENT", foreign.MessageId, secret, lane["YOU_WORKER_SESSION_TOKEN"], lead["YOU_WORKER_SESSION_TOKEN"])
	status, _ := f.request(t, http.MethodGet, "/messages/"+message.MessageId, nil, lead)
	if status != http.StatusOK {
		t.Fatal("recipient could not read streamed message")
	}
	assertRedactedFrame(t, nextFrame(t, ctx, frames, finished), message.MessageId, "READ", foreign.MessageId, secret, lane["YOU_WORKER_SESSION_TOKEN"], lead["YOU_WORKER_SESSION_TOKEN"])
	replySecret := "private-stream-reply"
	reply := f.send(t, map[string]any{"requestId": "private-reply", "inReplyTo": message.MessageId, "body": replySecret, "bodySecret": true}, lead)
	assertRedactedFrame(t, nextFrame(t, ctx, frames, finished), message.MessageId, "REPLIED", foreign.MessageId, secret, replySecret, lane["YOU_WORKER_SESSION_TOKEN"], lead["YOU_WORKER_SESSION_TOKEN"])
	var inbox api.AgentMessageListResponse
	if json.Unmarshal(f.cli(t, lane, "list", "--to-me"), &inbox) != nil || len(inbox.Messages) != 1 || inbox.Messages[0].MessageId != reply.MessageId || inbox.Messages[0].BodyRedactionCount == 0 || strings.Contains(inbox.Messages[0].Body, replySecret) {
		t.Fatal("reply inbox exposed declared content or lost reply identity")
	}
	// Credential-bearing text is redacted even without bodySecret.
	credential := "caller credential " + tokenLane["YOU_WORKER_SESSION_TOKEN"]
	second := f.send(t, map[string]any{"requestId": "credential", "to": map[string]string{"workerSessionId": "lead"}, "body": credential}, tokenLane)
	assertRedactedFrame(t, nextFrame(t, ctx, frames, finished), second.MessageId, "SENT", foreign.MessageId, secret, credential, tokenLane["YOU_WORKER_SESSION_TOKEN"], lead["YOU_WORKER_SESSION_TOKEN"])
	cancel()
	select {
	case <-finished:
	case <-t.Context().Done():
		t.Fatal("follow did not join")
	}
	f.server.Close(t)
	assertPrivateStreamDiagnostics(t, logs, secret, replySecret, lane["YOU_WORKER_SESSION_TOKEN"], tokenLane["YOU_WORKER_SESSION_TOKEN"], lead["YOU_WORKER_SESSION_TOKEN"])
	assertPrivateArtifacts(t, f.home, []string{secret, replySecret, lane["YOU_WORKER_SESSION_TOKEN"], tokenLane["YOU_WORKER_SESSION_TOKEN"], lead["YOU_WORKER_SESSION_TOKEN"]})
}

func assertPrivateStreamDiagnostics(t *testing.T, logs *observer.ObservedLogs, private ...string) {
	t.Helper()
	for _, entry := range logs.All() {
		encoded, err := json.Marshal(entry.ContextMap())
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range private {
			if strings.Contains(entry.Message+string(encoded), value) {
				t.Fatal("stream diagnostics exposed private content")
			}
		}
	}
}

func assertRedactedFrame(t *testing.T, frame []byte, id, kind string, private ...string) {
	t.Helper()
	var observation api.AgentMessageObservation
	if json.Unmarshal(frame, &observation) != nil || observation.Message.MessageId != id || string(observation.Kind) != kind {
		t.Fatal("follow published an unrelated record or wrong transition")
	}
	hash := sha256.Sum256([]byte(observation.Message.Body))
	if observation.Message.BodyRedactionCount == 0 || observation.Message.BodySha256 != hex.EncodeToString(hash[:]) {
		t.Fatal("follow published content before privacy normalization")
	}
	for _, value := range private {
		if strings.Contains(string(frame), value) {
			t.Fatal("follow exposed private content or unrelated identity")
		}
	}
}
