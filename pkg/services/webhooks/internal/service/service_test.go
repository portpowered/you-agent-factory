package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
)

func TestServiceDeliversCanonicalWorkEventWithSignedBody(t *testing.T) {
	root := newRecordingRootStub()
	secret := "test-signing-secret"
	when := time.Date(2026, time.August, 10, 12, 30, 0, 0, time.UTC)
	received := make(chan receivedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		received <- receivedRequest{body: body, headers: request.Header.Clone()}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	service := New(
		http.DefaultClient,
		func(context.Context, factorydefinitions.LoadedFactorySource, string) (string, error) {
			return secret, nil
		},
		platformclock.NewDeterministic(when, time.Second),
		logging.NoopLogger{},
	)
	subscription, err := service.Start(context.Background(), webhooks.StartRequest{
		Definitions: []factorydefinitions.FactoryWebhookConfig{{
			Name:             "monitor",
			Enabled:          true,
			URL:              server.URL,
			SigningSecretRef: "secrets/monitor",
			Filter: factorydefinitions.FactoryWebhookFilterConfig{
				EventTypes: []string{factorydefinitions.FactoryWebhookEventTypeWorkStateChange},
			},
		}},
		Events:        root,
		RuntimeSource: testLoadedFactorySource{},
		Scope:         recordings.CanonicalEventScope{FactorySessionID: "~default"},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer subscription(context.Background())
	waitForSubscriptions(t, root, 1)

	event := testWorkEvent(t)
	root.Publish(recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: event})
	got := receiveRequest(t, received)

	wantContext := factorydefinitions.FactoryEventContext{}
	if err := json.Unmarshal([]byte(event.SourceContext), &wantContext); err != nil {
		t.Fatalf("decode source context: %v", err)
	}
	wantBody, err := json.Marshal(factorydefinitions.FactoryEvent{
		Context:       wantContext,
		Id:            string(event.ID),
		Payload:       json.RawMessage(event.Payload),
		SchemaVersion: factorydefinitions.FactoryEventSchemaVersionV1,
		Type:          factorydefinitions.FactoryEventType(event.Kind),
	})
	if err != nil {
		t.Fatalf("marshal expected Factory Event: %v", err)
	}
	if !bytes.Equal(got.body, wantBody) {
		t.Fatalf("request body = %s, want unchanged canonical envelope %s", got.body, wantBody)
	}
	if got.headers.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got.headers.Get("Content-Type"))
	}
	if got.headers.Get(webhooks.EventIDHeader) != string(event.ID) {
		t.Fatalf("event ID header = %q, want %q", got.headers.Get(webhooks.EventIDHeader), event.ID)
	}
	if got.headers.Get(webhooks.TimestampHeader) != "1786365000" {
		t.Fatalf("timestamp header = %q, want deterministic Unix timestamp", got.headers.Get(webhooks.TimestampHeader))
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(got.headers.Get(webhooks.TimestampHeader) + "."))
	_, _ = mac.Write(got.body)
	wantSignature := webhooks.SignatureVersionV1 + "=" + hex.EncodeToString(mac.Sum(nil))
	if got.headers.Get(webhooks.SignatureHeader) != wantSignature {
		t.Fatalf("signature header = %q, want %q", got.headers.Get(webhooks.SignatureHeader), wantSignature)
	}
}

func TestServiceEvaluatesEnabledEndpointFiltersIndependently(t *testing.T) {
	root := newRecordingRootStub()
	matching := make(chan struct{}, 1)
	nonMatching := make(chan struct{}, 1)
	matchingServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		matching <- struct{}{}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer matchingServer.Close()
	nonMatchingServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		nonMatching <- struct{}{}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer nonMatchingServer.Close()

	service := New(http.DefaultClient, testSecretResolver, platformclock.Real{}, logging.NoopLogger{})
	subscription, err := service.Start(context.Background(), webhooks.StartRequest{
		Definitions: []factorydefinitions.FactoryWebhookConfig{
			{
				Name:    "matching",
				Enabled: true,
				URL:     matchingServer.URL,
				Filter: factorydefinitions.FactoryWebhookFilterConfig{
					EventTypes: []string{factorydefinitions.FactoryWebhookEventTypeWorkStateChange},
				},
			},
			{
				Name:    "dispatch-only",
				Enabled: true,
				URL:     nonMatchingServer.URL,
				Filter: factorydefinitions.FactoryWebhookFilterConfig{
					EventTypes: []string{factorydefinitions.FactoryWebhookEventTypeDispatchResponse},
				},
			},
		},
		Events:        root,
		RuntimeSource: testLoadedFactorySource{},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer subscription(context.Background())
	waitForSubscriptions(t, root, 2)
	root.Publish(recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: testWorkEvent(t)})
	select {
	case <-matching:
	case <-time.After(time.Second):
		t.Fatal("matching endpoint did not receive the Work transition")
	}
	select {
	case <-nonMatching:
		t.Fatal("nonmatching endpoint received the Work transition")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestServiceDeliversOnlyMatchingCanonicalDispatchFailures(t *testing.T) {
	root := newRecordingRootStub()
	received := make(chan receivedRequest, 8)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		received <- receivedRequest{body: body, headers: request.Header.Clone()}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	service := New(http.DefaultClient, testSecretResolver, platformclock.Real{}, logging.NoopLogger{})
	subscription, err := service.Start(context.Background(), webhooks.StartRequest{
		Definitions: []factorydefinitions.FactoryWebhookConfig{{
			Name:             "dispatch-failures",
			Enabled:          true,
			URL:              server.URL,
			SigningSecretRef: "secrets/dispatch-failures",
			Filter: factorydefinitions.FactoryWebhookFilterConfig{
				EventTypes: []string{
					factorydefinitions.FactoryWebhookEventTypeDispatchResponse,
					factorydefinitions.FactoryWebhookEventTypeDispatchReconciled,
					factorydefinitions.FactoryWebhookEventTypeDispatchInterrupted,
				},
				DispatchStatuses: []string{
					factorydefinitions.FactoryWebhookDispatchStatusFailed,
					factorydefinitions.FactoryWebhookDispatchStatusInterrupted,
				},
			},
		}},
		Events:        root,
		RuntimeSource: testLoadedFactorySource{},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer subscription(context.Background())
	waitForSubscriptions(t, root, 1)

	matchingResponse := testDispatchEvent(t, "dispatch-response-failed", factorydefinitions.FactoryWebhookEventTypeDispatchResponse, `{"outcome":"FAILED","transitionId":"execute"}`)
	nonMatchingResponse := testDispatchEvent(t, "dispatch-response-success", factorydefinitions.FactoryWebhookEventTypeDispatchResponse, `{"outcome":"ACCEPTED","transitionId":"execute"}`)
	matchingReconciled := testDispatchEvent(t, "dispatch-reconciled-failed", factorydefinitions.FactoryWebhookEventTypeDispatchReconciled, `{"reconciledStatus":"FAILED","reconciliationSource":"RUNTIME_RECONCILER"}`)
	nonMatchingReconciled := testDispatchEvent(t, "dispatch-reconciled-completed", factorydefinitions.FactoryWebhookEventTypeDispatchReconciled, `{"reconciledStatus":"COMPLETED","reconciliationSource":"RUNTIME_RECONCILER"}`)
	matchingInterrupted := testDispatchEvent(t, "dispatch-interrupted", factorydefinitions.FactoryWebhookEventTypeDispatchInterrupted, `{"observedStatus":"INTERRUPTED","reason":"operator stop"}`)
	nonMatchingInterrupted := testDispatchEvent(t, "dispatch-interrupted-failed", factorydefinitions.FactoryWebhookEventTypeDispatchInterrupted, `{"observedStatus":"FAILED","reason":"provider failure"}`)
	for _, event := range []recordings.CanonicalEvent{
		matchingResponse,
		nonMatchingResponse,
		matchingReconciled,
		nonMatchingReconciled,
		matchingInterrupted,
		nonMatchingInterrupted,
		testWorkEvent(t),
	} {
		root.Publish(recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: event})
	}

	seen := make(map[string]bool, 3)
	for index := 0; index < 3; index++ {
		request := receiveRequest(t, received)
		var envelope factorydefinitions.FactoryEvent
		if err := json.Unmarshal(request.body, &envelope); err != nil {
			t.Fatalf("decode delivered Factory Event: %v", err)
		}
		seen[envelope.Id] = true
		if envelope.Id == string(nonMatchingResponse.ID) || envelope.Id == string(nonMatchingReconciled.ID) || envelope.Id == string(nonMatchingInterrupted.ID) {
			t.Fatalf("delivered nonmatching dispatch event %q: %s", envelope.Id, request.body)
		}
		if envelope.Type == factorydefinitions.FactoryEventTypeDispatchResponse && !bytes.Equal(envelope.Payload, []byte(matchingResponse.Payload)) {
			t.Fatalf("response payload = %s, want canonical payload %s", envelope.Payload, matchingResponse.Payload)
		}
	}
	for _, id := range []string{string(matchingResponse.ID), string(matchingReconciled.ID), string(matchingInterrupted.ID)} {
		if !seen[id] {
			t.Fatalf("matching dispatch event %q was not delivered; seen=%v", id, seen)
		}
	}
	select {
	case request := <-received:
		t.Fatalf("received unexpected extra dispatch event: %s", request.body)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestServiceDefaultsDispatchSelectionToFailureStatus(t *testing.T) {
	root := newRecordingRootStub()
	received := make(chan receivedRequest, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		received <- receivedRequest{body: body, headers: request.Header.Clone()}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	service := New(http.DefaultClient, testSecretResolver, platformclock.Real{}, logging.NoopLogger{})
	subscription, err := service.Start(context.Background(), webhooks.StartRequest{
		Definitions: []factorydefinitions.FactoryWebhookConfig{{
			Name:    "response-failures",
			Enabled: true,
			URL:     server.URL,
			Filter: factorydefinitions.FactoryWebhookFilterConfig{
				EventTypes: []string{factorydefinitions.FactoryWebhookEventTypeDispatchResponse},
			},
		}},
		Events:        root,
		RuntimeSource: testLoadedFactorySource{},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer subscription(context.Background())
	waitForSubscriptions(t, root, 1)
	failed := testDispatchEvent(t, "default-filter-failed", factorydefinitions.FactoryWebhookEventTypeDispatchResponse, `{"outcome":"FAILED"}`)
	root.Publish(recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: failed})
	root.Publish(recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: testDispatchEvent(t, "default-filter-success", factorydefinitions.FactoryWebhookEventTypeDispatchResponse, `{"outcome":"ACCEPTED"}`)})
	request := receiveRequest(t, received)
	var envelope factorydefinitions.FactoryEvent
	if err := json.Unmarshal(request.body, &envelope); err != nil {
		t.Fatalf("decode delivered Factory Event: %v", err)
	}
	if envelope.Id != string(failed.ID) || envelope.Type != factorydefinitions.FactoryEventTypeDispatchResponse {
		t.Fatalf("delivered envelope = %#v, want failed response %q", envelope, failed.ID)
	}
	select {
	case request := <-received:
		t.Fatalf("successful dispatch response was delivered: %s", request.body)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestServiceDoesNotResolveOrSubscribeDisabledEndpoint(t *testing.T) {
	root := newRecordingRootStub()
	var resolved int
	service := New(
		http.DefaultClient,
		func(context.Context, factorydefinitions.LoadedFactorySource, string) (string, error) {
			resolved++
			return "secret", nil
		},
		platformclock.Real{},
		logging.NoopLogger{},
	)
	subscription, err := service.Start(context.Background(), webhooks.StartRequest{
		Definitions: []factorydefinitions.FactoryWebhookConfig{{
			Name:    "disabled",
			Enabled: false,
		}},
		Events: root,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := subscription(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := root.SubscriptionCount(); got != 0 {
		t.Fatalf("SubscribeFrom calls = %d, want zero for disabled endpoint", got)
	}
	if resolved != 0 {
		t.Fatalf("secret resolutions = %d, want zero for disabled endpoint", resolved)
	}
}

func TestServiceMissingSecretStopsEndpointWithoutReturningStartError(t *testing.T) {
	root := newRecordingRootStub()
	resolved := make(chan struct{}, 1)
	received := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		received <- struct{}{}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	service := New(
		http.DefaultClient,
		func(context.Context, factorydefinitions.LoadedFactorySource, string) (string, error) {
			resolved <- struct{}{}
			return "", context.Canceled
		},
		platformclock.Real{},
		logging.NoopLogger{},
	)
	subscription, err := service.Start(context.Background(), webhooks.StartRequest{
		Definitions: []factorydefinitions.FactoryWebhookConfig{{
			Name:    "missing-secret",
			Enabled: true,
			URL:     server.URL,
			Filter: factorydefinitions.FactoryWebhookFilterConfig{
				EventTypes: []string{factorydefinitions.FactoryWebhookEventTypeWorkStateChange},
			},
		}},
		Events:        root,
		RuntimeSource: testLoadedFactorySource{},
	})
	if err != nil {
		t.Fatalf("Start() error = %v, want missing secret to remain a delivery failure", err)
	}
	defer subscription(context.Background())
	waitForSubscriptions(t, root, 1)
	root.Publish(recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: testWorkEvent(t)})
	select {
	case <-resolved:
	case <-time.After(time.Second):
		t.Fatal("secret resolver was not called")
	}
	select {
	case <-received:
		t.Fatal("endpoint received an event despite missing secret")
	default:
	}
}

func TestServiceBoundsReceiverResponseBodyReads(t *testing.T) {
	root := newRecordingRootStub()
	body := &trackingResponseBody{
		remaining: webhooks.MaxResponseBodySize * 2,
		closed:    make(chan struct{}),
	}
	service := New(
		roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusNoContent, Body: body}, nil
		}),
		testSecretResolver,
		platformclock.Real{},
		logging.NoopLogger{},
	)
	subscription, err := service.Start(context.Background(), webhooks.StartRequest{
		Definitions: []factorydefinitions.FactoryWebhookConfig{{
			Name:    "bounded-response",
			Enabled: true,
			URL:     "http://webhook.test/events",
			Filter: factorydefinitions.FactoryWebhookFilterConfig{
				EventTypes: []string{factorydefinitions.FactoryWebhookEventTypeWorkStateChange},
			},
		}},
		Events:        root,
		RuntimeSource: testLoadedFactorySource{},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer subscription(context.Background())
	waitForSubscriptions(t, root, 1)
	root.Publish(recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: testWorkEvent(t)})
	select {
	case <-body.closed:
	case <-time.After(time.Second):
		t.Fatal("response body was not closed")
	}
	if body.read > webhooks.MaxResponseBodySize {
		t.Fatalf("receiver response bytes read = %d, want at most %d", body.read, webhooks.MaxResponseBodySize)
	}
}

func TestServiceRetriesWithBoundedRetryAfterAndFreshSignedTimestamp(t *testing.T) {
	root := newRecordingRootStub()
	secret := "retry-signing-secret"
	start := time.Date(2026, time.August, 10, 13, 0, 0, 0, time.UTC)
	clock := clockwork.NewFakeClockAt(start)
	attempts := make(chan receivedRequest, 2)
	var calls int
	client := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		attempts <- receivedRequest{body: body, headers: request.Header.Clone()}
		calls++
		if calls == 1 {
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Header:     http.Header{"Retry-After": []string{"5"}},
				Body:       io.NopCloser(strings.NewReader("receiver response is bounded")),
			}, nil
		}
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})
	service := New(client, func(context.Context, factorydefinitions.LoadedFactorySource, string) (string, error) {
		return secret, nil
	}, clock, logging.NoopLogger{})
	subscription, err := service.Start(context.Background(), webhooks.StartRequest{
		Definitions: []factorydefinitions.FactoryWebhookConfig{retryWebhookDefinition(
			"retry-success",
			"https://monitor.example/events",
			webhookDeliveryPolicy(2, time.Second, 2*time.Second, 2),
		)},
		Events:        root,
		RuntimeSource: testLoadedFactorySource{},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer subscription(context.Background())
	waitForSubscriptions(t, root, 1)
	event := testWorkEvent(t)
	root.Publish(recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: event})
	first := receiveRequest(t, attempts)
	clock.BlockUntil(1)
	clock.Advance(2 * time.Second)
	second := receiveRequest(t, attempts)

	if !bytes.Equal(first.body, second.body) {
		t.Fatalf("retry body = %s, want unchanged initial body %s", second.body, first.body)
	}
	if first.headers.Get(webhooks.EventIDHeader) != second.headers.Get(webhooks.EventIDHeader) {
		t.Fatalf("retry event ID changed from %q to %q", first.headers.Get(webhooks.EventIDHeader), second.headers.Get(webhooks.EventIDHeader))
	}
	if first.headers.Get(webhooks.TimestampHeader) != "1786366800" || second.headers.Get(webhooks.TimestampHeader) != "1786366802" {
		t.Fatalf("retry timestamps = %q, %q; want current timestamps across the bounded delay", first.headers.Get(webhooks.TimestampHeader), second.headers.Get(webhooks.TimestampHeader))
	}
	for _, request := range []receivedRequest{first, second} {
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(request.headers.Get(webhooks.TimestampHeader) + "."))
		_, _ = mac.Write(request.body)
		want := webhooks.SignatureVersionV1 + "=" + hex.EncodeToString(mac.Sum(nil))
		if request.headers.Get(webhooks.SignatureHeader) != want {
			t.Fatalf("signature = %q, want independently verified %q", request.headers.Get(webhooks.SignatureHeader), want)
		}
	}
}

func TestServiceExhaustionAppendsOneRedactedDeadLetter(t *testing.T) {
	root := newRecordingRootStub()
	secret := "must-not-be-written"
	start := time.Date(2026, time.August, 10, 14, 0, 0, 0, time.UTC)
	clock := clockwork.NewFakeClockAt(start)
	attempts := make(chan receivedRequest, 3)
	client := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		attempts <- receivedRequest{body: body, headers: request.Header.Clone()}
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(strings.NewReader("receiver body must not be retained")),
		}, nil
	})
	deadLetters := make(chan []byte, 1)
	service := NewWithDeadLetterAppender(
		client,
		func(context.Context, factorydefinitions.LoadedFactorySource, string) (string, error) {
			return secret, nil
		},
		clock,
		func(path string, line []byte) error {
			if path != "runtime/.you-agent-factory/webhooks/dead-letter.jsonl" {
				t.Errorf("dead-letter path = %q, want session-owned runtime path", path)
			}
			deadLetters <- append([]byte(nil), line...)
			return nil
		},
		logging.NoopLogger{},
	)
	subscription, err := service.Start(context.Background(), webhooks.StartRequest{
		Definitions: []factorydefinitions.FactoryWebhookConfig{retryWebhookDefinition(
			"retry-exhausted",
			"https://monitor.example/events?token=secret",
			webhookDeliveryPolicy(3, time.Second, 2*time.Second, 2),
		)},
		Events:         root,
		RuntimeSource:  testLoadedFactorySource{},
		DeadLetterPath: "runtime/.you-agent-factory/webhooks/dead-letter.jsonl",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer subscription(context.Background())
	waitForSubscriptions(t, root, 1)
	event := testWorkEvent(t)
	root.Publish(recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: event})
	for attempt := 1; attempt <= 3; attempt++ {
		receiveRequest(t, attempts)
		if attempt < 3 {
			clock.BlockUntil(1)
			clock.Advance(time.Duration(1<<uint(attempt-1)) * time.Second)
		}
	}
	line := receiveDeadLetter(t, deadLetters)
	assertExhaustedDeadLetter(t, line, event, secret)
	select {
	case extra := <-deadLetters:
		t.Fatalf("dead-letter append count exceeded one: %s", extra)
	default:
	}
}

func assertExhaustedDeadLetter(
	t *testing.T,
	line []byte,
	event recordings.CanonicalEvent,
	secret string,
) {
	t.Helper()
	var record deadLetterRecord
	if err := json.Unmarshal(line, &record); err != nil {
		t.Fatalf("decode dead-letter record: %v; line=%s", err, line)
	}
	if record.EndpointName != "retry-exhausted" || record.EventID != string(event.ID) || record.EventType != string(event.Kind) {
		t.Fatalf("dead-letter identity = %#v, want endpoint/event %q/%q", record, event.ID, event.Kind)
	}
	if record.DestinationOrigin != "https://monitor.example" || record.DestinationPath != "/events" {
		t.Fatalf("dead-letter destination = %q %q, want origin/path without query", record.DestinationOrigin, record.DestinationPath)
	}
	if record.AttemptCount != 3 || record.StatusCode != http.StatusServiceUnavailable || record.TerminalReason != "retry_exhausted" {
		t.Fatalf("dead-letter terminal facts = %#v, want 3 attempts/503/retry_exhausted", record)
	}
	wantBody, err := marshalCanonicalEvent(event)
	if err != nil {
		t.Fatalf("marshal canonical event: %v", err)
	}
	if !bytes.Equal(record.CanonicalBody, wantBody) {
		t.Fatalf("dead-letter body = %s, want canonical body %s", record.CanonicalBody, wantBody)
	}
	if strings.Contains(string(line), secret) || strings.Contains(string(line), "receiver body must not be retained") || strings.Contains(string(line), "token=secret") {
		t.Fatalf("dead-letter record leaked secret, response body, or query: %s", line)
	}
}

func TestServiceNonRetryableResponseDeadLettersWithoutRetry(t *testing.T) {
	root := newRecordingRootStub()
	attempts := make(chan struct{}, 1)
	deadLetters := make(chan []byte, 1)
	service := NewWithDeadLetterAppender(
		roundTripFunc(func(*http.Request) (*http.Response, error) {
			attempts <- struct{}{}
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader("bad request details")),
			}, nil
		}),
		testSecretResolver,
		platformclock.Real{},
		func(_ string, line []byte) error {
			deadLetters <- append([]byte(nil), line...)
			return nil
		},
		logging.NoopLogger{},
	)
	subscription, err := service.Start(context.Background(), webhooks.StartRequest{
		Definitions: []factorydefinitions.FactoryWebhookConfig{retryWebhookDefinition(
			"terminal-4xx",
			"https://monitor.example/events",
			webhookDeliveryPolicy(5, time.Second, 30*time.Second, 2),
		)},
		Events:         root,
		RuntimeSource:  testLoadedFactorySource{},
		DeadLetterPath: "runtime/dead-letter.jsonl",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer subscription(context.Background())
	waitForSubscriptions(t, root, 1)
	root.Publish(recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: testWorkEvent(t)})
	select {
	case <-attempts:
	case <-time.After(time.Second):
		t.Fatal("terminal response was not attempted")
	}
	line := receiveDeadLetter(t, deadLetters)
	var record deadLetterRecord
	if err := json.Unmarshal(line, &record); err != nil {
		t.Fatalf("decode dead-letter record: %v", err)
	}
	if record.AttemptCount != 1 || record.StatusCode != http.StatusBadRequest || record.TerminalReason != "non_retryable_http_status" {
		t.Fatalf("terminal 4xx record = %#v, want one terminal attempt", record)
	}
	select {
	case <-attempts:
		t.Fatal("non-retryable 4xx response was retried")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestServiceCancellationStopsRetryWithoutDeadLetter(t *testing.T) {
	root := newRecordingRootStub()
	clock := clockwork.NewFakeClock()
	attempts := make(chan struct{}, 2)
	deadLetters := make(chan []byte, 1)
	service := NewWithDeadLetterAppender(
		roundTripFunc(func(*http.Request) (*http.Response, error) {
			attempts <- struct{}{}
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("retry"))}, nil
		}),
		testSecretResolver,
		clock,
		func(_ string, line []byte) error {
			deadLetters <- append([]byte(nil), line...)
			return nil
		},
		logging.NoopLogger{},
	)
	ctx, cancel := context.WithCancel(context.Background())
	subscription, err := service.Start(ctx, webhooks.StartRequest{
		Definitions: []factorydefinitions.FactoryWebhookConfig{retryWebhookDefinition(
			"canceled",
			"https://monitor.example/events",
			webhookDeliveryPolicy(5, time.Hour, 2*time.Hour, 2),
		)},
		Events:         root,
		RuntimeSource:  testLoadedFactorySource{},
		DeadLetterPath: "runtime/dead-letter.jsonl",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitForSubscriptions(t, root, 1)
	root.Publish(recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: testWorkEvent(t)})
	select {
	case <-attempts:
	case <-time.After(time.Second):
		t.Fatal("initial retryable attempt was not made")
	}
	clock.BlockUntil(1)
	cancel()
	if err := subscription(context.Background()); err != nil {
		t.Fatalf("Close() after cancellation: %v", err)
	}
	select {
	case line := <-deadLetters:
		t.Fatalf("cancellation appended dead-letter: %s", line)
	default:
	}
	select {
	case <-attempts:
		t.Fatal("canceled retry was attempted")
	default:
	}
}

func retryWebhookDefinition(name, endpoint string, policy *factorydefinitions.FactoryWebhookDeliveryPolicyConfig) factorydefinitions.FactoryWebhookConfig {
	return factorydefinitions.FactoryWebhookConfig{
		Name:             name,
		Enabled:          true,
		URL:              endpoint,
		SigningSecretRef: "secrets/webhook",
		Filter: factorydefinitions.FactoryWebhookFilterConfig{
			EventTypes: []string{factorydefinitions.FactoryWebhookEventTypeWorkStateChange},
		},
		DeliveryPolicy: policy,
	}
}

func webhookDeliveryPolicy(maxAttempts int, initial, maximum time.Duration, multiplier float64) *factorydefinitions.FactoryWebhookDeliveryPolicyConfig {
	requestTimeout := "1s"
	initialValue := initial.String()
	maximumValue := maximum.String()
	return &factorydefinitions.FactoryWebhookDeliveryPolicyConfig{
		RequestTimeout:    &requestTimeout,
		MaxAttempts:       &maxAttempts,
		InitialBackoff:    &initialValue,
		BackoffMultiplier: &multiplier,
		MaxBackoff:        &maximumValue,
	}
}

func receiveDeadLetter(t *testing.T, records <-chan []byte) []byte {
	t.Helper()
	select {
	case record := <-records:
		return record
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for dead-letter record")
		return nil
	}
}

func testSecretResolver(context.Context, factorydefinitions.LoadedFactorySource, string) (string, error) {
	return "secret", nil
}

func testWorkEvent(t *testing.T) recordings.CanonicalEvent {
	t.Helper()
	sessionID := "~default"
	contextValue := factorydefinitions.FactoryEventContext{
		EventTime: time.Date(2026, time.August, 10, 12, 29, 59, 0, time.UTC),
		Sequence:  7,
		SessionID: &sessionID,
		SessionSequence: func() *int {
			value := 3
			return &value
		}(),
		Tick: 11,
		RequestID: func() *string {
			value := "request-1"
			return &value
		}(),
	}
	encoded, err := json.Marshal(contextValue)
	if err != nil {
		t.Fatalf("marshal event context: %v", err)
	}
	return recordings.CanonicalEvent{
		ID:            "event-1",
		Sequence:      7,
		FactoryTick:   11,
		Scope:         recordings.CanonicalEventScope{FactorySessionID: sessionID},
		Cursor:        recordings.CanonicalEventCursor{StreamGenerationID: "generation-1", Sequence: 7},
		RecordedAt:    contextValue.EventTime,
		Kind:          recordings.CanonicalEventKind(factorydefinitions.FactoryWebhookEventTypeWorkStateChange),
		Payload:       `{"workId":"work-1","fromState":"queued","toState":"done"}`,
		SourceContext: string(encoded),
	}
}

func testDispatchEvent(t *testing.T, id string, kind string, payload string) recordings.CanonicalEvent {
	t.Helper()
	event := testWorkEvent(t)
	event.ID = recordings.CanonicalEventID(id)
	event.Kind = recordings.CanonicalEventKind(kind)
	event.Payload = payload
	return event
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
