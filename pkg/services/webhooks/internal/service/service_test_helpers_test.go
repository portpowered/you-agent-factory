package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
)

// exactWebhookClock implements only the declared Webhooks clock port. Its
// scheduler is controlled explicitly so retry tests never depend on wall time
// or on an optional capability discovered at runtime.
type exactWebhookClock struct {
	mu        sync.Mutex
	now       time.Time
	waiters   []webhookClockWaiter
	scheduled chan time.Duration
}

type webhookClockWaiter struct {
	at    time.Time
	ready chan time.Time
}

var _ interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
} = (*exactWebhookClock)(nil)

func newExactWebhookClock(now time.Time) *exactWebhookClock {
	return &exactWebhookClock{
		now:       now,
		scheduled: make(chan time.Duration, 8),
	}
}

func (clock *exactWebhookClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *exactWebhookClock) After(delay time.Duration) <-chan time.Time {
	ready := make(chan time.Time, 1)
	clock.mu.Lock()
	clock.waiters = append(clock.waiters, webhookClockWaiter{
		at:    clock.now.Add(delay),
		ready: ready,
	})
	clock.mu.Unlock()
	clock.scheduled <- delay
	return ready
}

func (clock *exactWebhookClock) WaitForSchedule() time.Duration {
	return <-clock.scheduled
}

func (clock *exactWebhookClock) Advance(delay time.Duration) {
	clock.mu.Lock()
	clock.now = clock.now.Add(delay)
	now := clock.now
	var ready []chan time.Time
	remaining := clock.waiters[:0]
	for _, waiter := range clock.waiters {
		if !waiter.at.After(now) {
			ready = append(ready, waiter.ready)
			continue
		}
		remaining = append(remaining, waiter)
	}
	clock.waiters = remaining
	clock.mu.Unlock()
	for _, channel := range ready {
		channel <- now
	}
}

type receivedRequest struct {
	body    []byte
	headers http.Header
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

type trackingResponseBody struct {
	remaining int
	read      int
	closed    chan struct{}
}

func (body *trackingResponseBody) Read(buffer []byte) (int, error) {
	if body.remaining == 0 {
		return 0, io.EOF
	}
	count := len(buffer)
	if count > body.remaining {
		count = body.remaining
	}
	body.remaining -= count
	body.read += count
	return count, nil
}

func (body *trackingResponseBody) Close() error {
	select {
	case <-body.closed:
	default:
		close(body.closed)
	}
	return nil
}

func receiveRequest(t *testing.T, requests <-chan receivedRequest) receivedRequest {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for webhook request")
		return receivedRequest{}
	}
}

func waitForSubscriptions(t *testing.T, root *recordingRootStub, count int) {
	t.Helper()
	for index := 0; index < count; index++ {
		select {
		case <-root.started:
		case <-time.After(30 * time.Second):
			t.Fatalf("timed out waiting for subscription %d of %d", index+1, count)
		}
	}
}

type recordingRootStub struct {
	recordings.Service
	mu      sync.Mutex
	streams []chan recordings.SubscriptionOutcome
	started chan struct{}
}

func newRecordingRootStub() *recordingRootStub {
	return &recordingRootStub{started: make(chan struct{}, 8)}
}

func (root *recordingRootStub) SubscribeFrom(
	context.Context,
	recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	stream := make(chan recordings.SubscriptionOutcome, 4)
	root.mu.Lock()
	root.streams = append(root.streams, stream)
	root.mu.Unlock()
	root.started <- struct{}{}
	return recordings.SubscribeResult{
		Subscription: recordings.EventSubscription(func(ctx context.Context) recordings.SubscriptionOutcome {
			select {
			case outcome := <-stream:
				return outcome
			case <-ctx.Done():
				return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
			}
		}),
	}, nil
}

func (root *recordingRootStub) Publish(outcome recordings.SubscriptionOutcome) {
	root.mu.Lock()
	streams := append([]chan recordings.SubscriptionOutcome(nil), root.streams...)
	root.mu.Unlock()
	for _, stream := range streams {
		stream <- outcome
	}
}

func (root *recordingRootStub) SubscriptionCount() int {
	root.mu.Lock()
	defer root.mu.Unlock()
	return len(root.streams)
}

// overflowRecordingRootStub models a bounded live subscriber whose producer
// reports a gap after a slow webhook delivery lets its one-slot buffer fill.
// The reconnect subscription represents the retained recording history that
// can replay the event after the last delivered cursor.
type overflowRecordingRootStub struct {
	*recordingRootStub
	later recordings.CanonicalEvent

	mu               sync.Mutex
	first            *overflowSubscriptionStream
	subscribeCalls   int
	reconnectRequest recordings.SubscribeRequest
}

func newOverflowRecordingRootStub(later recordings.CanonicalEvent) *overflowRecordingRootStub {
	return &overflowRecordingRootStub{
		recordingRootStub: newRecordingRootStub(),
		later:             later,
	}
}

func (root *overflowRecordingRootStub) SubscribeFrom(
	ctx context.Context,
	request recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	_ = ctx
	root.mu.Lock()
	root.subscribeCalls++
	switch root.subscribeCalls {
	case 1:
		stream := newOverflowSubscriptionStream()
		root.first = stream
		root.mu.Unlock()
		root.started <- struct{}{}
		return recordings.SubscribeResult{Subscription: stream.Next}, nil
	case 2:
		// The request value is copied so the test can assert the exact cursor
		// passed back after the bounded subscription reports its gap.
		root.reconnectRequest = request
		root.mu.Unlock()
		return root.reconnectSubscription(), nil
	default:
		root.mu.Unlock()
		return recordings.SubscribeResult{Subscription: func(ctx context.Context) recordings.SubscriptionOutcome {
			select {
			case <-ctx.Done():
				return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
			default:
				return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
			}
		}}, nil
	}
}

func (root *overflowRecordingRootStub) reconnectSubscription() recordings.SubscribeResult {
	outcomes := make(chan recordings.SubscriptionOutcome, 1)
	outcomes <- recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: root.later}
	close(outcomes)
	return recordings.SubscribeResult{Subscription: func(ctx context.Context) recordings.SubscriptionOutcome {
		select {
		case outcome, ok := <-outcomes:
			if !ok {
				return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
			}
			return outcome
		case <-ctx.Done():
			return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
		}
	}}
}

func (root *overflowRecordingRootStub) Publish(outcome recordings.SubscriptionOutcome) {
	root.mu.Lock()
	stream := root.first
	root.mu.Unlock()
	if stream != nil {
		stream.Publish(outcome)
	}
}

func (root *overflowRecordingRootStub) SubscriptionCount() int {
	root.mu.Lock()
	defer root.mu.Unlock()
	return root.subscribeCalls
}

func (root *overflowRecordingRootStub) ReconnectRequest() recordings.SubscribeRequest {
	root.mu.Lock()
	defer root.mu.Unlock()
	return root.reconnectRequest
}

type overflowSubscriptionStream struct {
	mu            sync.Mutex
	outcomes      chan recordings.SubscriptionOutcome
	overflowed    bool
	reconnectFrom recordings.CanonicalEventCursor
}

func newOverflowSubscriptionStream() *overflowSubscriptionStream {
	return &overflowSubscriptionStream{
		outcomes: make(chan recordings.SubscriptionOutcome, 1),
	}
}

func (stream *overflowSubscriptionStream) Next(ctx context.Context) recordings.SubscriptionOutcome {
	stream.mu.Lock()
	if stream.overflowed {
		cursor := stream.reconnectFrom
		stream.mu.Unlock()
		return recordings.SubscriptionOutcome{
			Kind: recordings.SubscriptionGap,
			Gap: &recordings.SubscriptionGapFacts{
				Cause:         recordings.SubscriptionBackpressure,
				ReconnectFrom: cursor,
			},
		}
	}
	stream.mu.Unlock()
	select {
	case outcome := <-stream.outcomes:
		return outcome
	case <-ctx.Done():
		return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
	}
}

func (stream *overflowSubscriptionStream) Publish(outcome recordings.SubscriptionOutcome) {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	if outcome.Kind == recordings.SubscriptionEvent && stream.reconnectFrom == (recordings.CanonicalEventCursor{}) {
		stream.reconnectFrom = outcome.Event.Cursor
	}
	select {
	case stream.outcomes <- outcome:
	default:
		stream.overflowed = true
	}
}

type testLoadedFactorySource struct {
	factorydefinitions.RuntimeDefinitionLookup
}

func (testLoadedFactorySource) FactoryDir() string { return "/factories/test" }
func (testLoadedFactorySource) FactoryConfig() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{}
}
func (testLoadedFactorySource) RuntimeBaseDir() string { return "/runtime/test" }

var _ factorydefinitions.LoadedFactorySource = testLoadedFactorySource{}
var _ recordings.Service = (*recordingRootStub)(nil)
var _ recordings.Service = (*overflowRecordingRootStub)(nil)

type webhookLogCapture struct {
	mu      sync.Mutex
	entries []string
	signal  chan struct{}
}

func (*webhookLogCapture) Debug(string, ...any)   {}
func (*webhookLogCapture) Info(string, ...any)    {}
func (*webhookLogCapture) Warn(string, ...any)    {}
func (*webhookLogCapture) Verbose(string, ...any) {}
func (logger *webhookLogCapture) Error(message string, fields ...any) {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	logger.entries = append(logger.entries, fmt.Sprint(message, fields))
	if logger.signal != nil {
		select {
		case logger.signal <- struct{}{}:
		default:
		}
	}
}
func (logger *webhookLogCapture) wait(t *testing.T) {
	t.Helper()
	logger.mu.Lock()
	if len(logger.entries) != 0 {
		logger.mu.Unlock()
		return
	}
	logger.signal = make(chan struct{}, 1)
	signal := logger.signal
	logger.mu.Unlock()
	select {
	case <-signal:
	case <-time.After(30 * time.Second):
		t.Fatal("missing diagnostic")
	}
}
func (logger *webhookLogCapture) assertSafeSingle(t *testing.T, message string, canaries ...string) {
	t.Helper()
	logger.mu.Lock()
	defer logger.mu.Unlock()
	if len(logger.entries) != 1 || !strings.Contains(logger.entries[0], message) {
		t.Fatalf("diagnostics = %v", logger.entries)
	}
	for _, canary := range canaries {
		if strings.Contains(logger.entries[0], canary) {
			t.Fatalf("diagnostic leaked %q", canary)
		}
	}
}
func TestServiceSkipsEventsAtOrBeforeActivationCursor(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		cursor *recordings.CanonicalEventCursor
		want   []string
	}{
		{"activation", &recordings.CanonicalEventCursor{StreamGenerationID: "G", Sequence: 7}, []string{"event-8", "event-H1"}},
		{"absent", nil, []string{"event-6", "event-7", "event-8", "event-H1"}},
		{"empty generation", &recordings.CanonicalEventCursor{Sequence: 7}, []string{"event-6", "event-7", "event-8", "event-H1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := newRecordingRootStub()
			requests := make(chan receivedRequest, 8)
			service := New(root, roundTripFunc(func(request *http.Request) (*http.Response, error) {
				body, _ := io.ReadAll(request.Body)
				requests <- receivedRequest{body: body, headers: request.Header.Clone()}
				return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
			}), testSecretResolver, platformclock.Real{}, func(string, []byte) error { return nil }, logging.NoopLogger{})
			closeSubscription, err := service.Start(t.Context(), webhooks.StartRequest{
				Definitions:   []factorydefinitions.FactoryWebhookConfig{retryWebhookDefinition("cursor", "https://receiver.test", nil)},
				RuntimeSource: testLoadedFactorySource{}, ActivationCursor: test.cursor,
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = closeSubscription(context.Background()) })
			waitForSubscriptions(t, root, 1)
			for _, sequence := range []int{6, 7, 8, 1} {
				event := testWorkEvent(t)
				event.ID = recordings.CanonicalEventID(fmt.Sprintf("event-%d", sequence))
				event.Cursor = recordings.CanonicalEventCursor{StreamGenerationID: "G", Sequence: recordings.CanonicalEventSequence(sequence)}
				if sequence == 1 {
					event.ID = "event-H1"
					event.Cursor.StreamGenerationID = "H"
				}
				root.Publish(recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: event})
			}
			for _, id := range test.want {
				if got := receiveRequest(t, requests).headers.Get(webhooks.EventIDHeader); got != id {
					t.Fatalf("delivered %q, want %q", got, id)
				}
			}
			if err := closeSubscription(context.Background()); err != nil {
				t.Fatal(err)
			}
			select {
			case extra := <-requests:
				t.Fatalf("unexpected delivery: %v", extra.headers)
			default:
			}
		})
	}
}

func TestServiceDeadLetterAppendFailureLogsWithoutRetryStorm(t *testing.T) {
	t.Parallel()
	root := newRecordingRootStub()
	logger := &webhookLogCapture{}
	attempts, appends := 0, 0
	clock := newExactWebhookClock(time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC))
	service := New(root, roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("receiver-canary"))}, nil
	}), testSecretResolver, clock, func(string, []byte) error {
		appends++
		return errors.New("storage-secret-canary")
	}, logger)
	closeSubscription, err := service.Start(t.Context(), webhooks.StartRequest{
		Definitions:   []factorydefinitions.FactoryWebhookConfig{retryWebhookDefinition("failure", "https://receiver.test", webhookDeliveryPolicy(3, time.Second, 2*time.Second, 2))},
		RuntimeSource: testLoadedFactorySource{}, DeadLetterPath: "owned/dead-letter.jsonl",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeSubscription(context.Background()) })
	waitForSubscriptions(t, root, 1)
	root.Publish(recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: testWorkEvent(t)})
	for _, delay := range []time.Duration{time.Second, 2 * time.Second} {
		if got := clock.WaitForSchedule(); got != delay {
			t.Fatalf("delay = %s, want %s", got, delay)
		}
		clock.Advance(delay)
	}
	logger.wait(t)
	if err := closeSubscription(context.Background()); err != nil {
		t.Fatal(err)
	}
	if attempts != 3 || appends != 1 {
		t.Fatalf("attempts/appends = %d/%d, want 3/1", attempts, appends)
	}
	logger.assertSafeSingle(t, "factory webhook dead-letter append failed", "storage-secret-canary", "receiver-canary", "test-secret")
}

func TestServiceSecretFailureLogsOneSafeDiagnostic(t *testing.T) {
	t.Parallel()
	root := newRecordingRootStub()
	logger := &webhookLogCapture{}
	deliveries := 0
	service := New(root, roundTripFunc(func(*http.Request) (*http.Response, error) {
		deliveries++
		return nil, errors.New("unexpected delivery")
	}),
		func(context.Context, factorydefinitions.LoadedFactorySource, string) (string, error) {
			return "secret-value-canary", errors.New("resolver-cause-canary")
		},
		platformclock.Real{}, func(string, []byte) error { t.Error("unexpected append"); return nil }, logger)
	closeSubscription, err := service.Start(t.Context(), webhooks.StartRequest{
		Definitions:   []factorydefinitions.FactoryWebhookConfig{retryWebhookDefinition("secret", "https://receiver.test", nil)},
		RuntimeSource: testLoadedFactorySource{},
	})
	if err != nil {
		t.Fatalf("asynchronous secret failure became Start failure: %v", err)
	}
	logger.wait(t)
	if err := closeSubscription(context.Background()); err != nil {
		t.Fatal(err)
	}
	if deliveries != 0 {
		t.Fatalf("deliveries = %d, want zero", deliveries)
	}
	logger.assertSafeSingle(t, "factory webhook secret resolution failed", "secret-value-canary", "resolver-cause-canary")
}

func TestServiceClosingOneSubscriptionLeavesPeerDelivering(t *testing.T) {
	t.Parallel()
	root := newRecordingRootStub()
	entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	peer := make(chan receivedRequest, 8)
	service := New(root, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/blocked" {
			close(entered)
			<-request.Context().Done()
			close(canceled)
			<-release // A's Close must join the in-flight owned effect.
			return nil, request.Context().Err()
		}
		peer <- receivedRequest{headers: request.Header.Clone()}
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
	}), testSecretResolver, platformclock.Real{}, func(string, []byte) error { t.Error("cancellation must not append"); return nil }, logging.NoopLogger{})
	start := func(endpoint string) webhooks.Subscription {
		subscription, err := service.Start(t.Context(), webhooks.StartRequest{
			Definitions: []factorydefinitions.FactoryWebhookConfig{retryWebhookDefinition(endpoint, "https://receiver.test/"+endpoint, nil)}, RuntimeSource: testLoadedFactorySource{},
		})
		if err != nil {
			t.Fatal(err)
		}
		return subscription
	}
	a, b := start("blocked"), start("peer")
	t.Cleanup(func() { _ = b(context.Background()) })
	waitForSubscriptions(t, root, 2)
	root.Publish(recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: testWorkEvent(t)})
	select {
	case <-entered:
	case <-time.After(30 * time.Second):
		t.Fatal("A never entered delivery")
	}
	receiveRequest(t, peer)
	joined := make(chan error, 1)
	go func() { joined <- a(context.Background()) }()
	select {
	case <-canceled:
	case <-time.After(30 * time.Second):
		t.Fatal("A was not canceled")
	}
	select {
	case err := <-joined:
		t.Fatalf("Close returned before effect joined: %v", err)
	default:
	}
	next := testWorkEvent(t)
	next.ID = "peer-after-close"
	root.Publish(recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: next})
	if got := receiveRequest(t, peer).headers.Get(webhooks.EventIDHeader); got != string(next.ID) {
		t.Fatalf("peer delivered %q", got)
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case err := <-joined:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("A did not join")
	}
	if err := a(nil); err != nil {
		t.Fatalf("idempotent Close: %v", err)
	}
}
