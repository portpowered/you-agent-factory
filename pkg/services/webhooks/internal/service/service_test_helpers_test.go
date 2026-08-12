package service

import (
	"context"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
)

// exactWebhookClock implements only the public Webhooks clock port. Its
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

var _ webhooks.Clock = (*exactWebhookClock)(nil)

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
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for webhook request")
		return receivedRequest{}
	}
}

func waitForSubscriptions(t *testing.T, root *recordingRootStub, count int) {
	t.Helper()
	for index := 0; index < count; index++ {
		select {
		case <-root.started:
		case <-time.After(time.Second):
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
