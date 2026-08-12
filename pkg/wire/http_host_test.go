package wire

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	platformbrowser "github.com/portpowered/infinite-you/pkg/platform/browser"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
)

func TestProvideAPIServerStarterHonorsRootEdgeOverride(t *testing.T) {
	t.Parallel()

	called := false
	override := platformhttpserver.Starter(func(_ context.Context, request platformhttpserver.StartRequest) error {
		called = true
		if request.Port != 8123 || !request.AutoPort || request.Handler == nil {
			t.Fatalf("override request = %+v", request)
		}
		return nil
	})
	starter, err := provideAPIServerStarter(serviceedges.Edges{APIServerStarter: override})
	if err != nil {
		t.Fatalf("provideAPIServerStarter: %v", err)
	}
	if err := starter(t.Context(), platformhttpserver.StartRequest{
		Handler: http.NotFoundHandler(), Port: 8123, AutoPort: true,
	}); err != nil {
		t.Fatalf("starter override: %v", err)
	}
	if !called {
		t.Fatal("root APIServerStarter override was not selected")
	}
}

func TestProvideBrowserOpenerHonorsRootEdgeOverride(t *testing.T) {
	t.Parallel()

	called := false
	override := platformbrowser.Opener(func(context.Context, string) error {
		called = true
		return nil
	})
	selected := provideBrowserOpener(serviceedges.Edges{BrowserOpener: override})
	if err := selected(t.Context(), "https://factory.example"); err != nil || !called {
		t.Fatalf("browser override = (called %t, error %v)", called, err)
	}
	if provideBrowserOpener(serviceedges.Edges{}) == nil {
		t.Fatal("provideBrowserOpener default = nil, want host adapter")
	}
}

func TestFactoryWebhooksDefaultClientDoesNotFollowRedirects(t *testing.T) {
	var targetMu sync.Mutex
	targetRequests := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetMu.Lock()
		targetRequests++
		targetMu.Unlock()
	}))
	defer target.Close()

	configuredRequests := make(chan struct{}, 1)
	configured := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		configuredRequests <- struct{}{}
		writer.Header().Set("Location", target.URL+"/redirect-target")
		writer.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer configured.Close()

	deadLetters := make(chan []byte, 1)
	service := provideFactoryWebhooksService(serviceedges.Edges{
		FactoryWebhookSecretResolver: func(context.Context, factorydefinitions.LoadedFactorySource, string) (string, error) {
			return "redirect-secret", nil
		},
		FactoryWebhookDeadLetterAppender: func(_ string, line []byte) error {
			deadLetters <- append([]byte(nil), line...)
			return nil
		},
	}, logging.NoopLogger{})
	events := &wireWebhookEvents{event: wireWebhookEvent()}
	subscription, err := service.Start(context.Background(), webhooks.StartRequest{
		Definitions: []factorydefinitions.FactoryWebhookConfig{{
			Name:             "redirect-endpoint",
			Enabled:          true,
			URL:              configured.URL + "/configured",
			SigningSecretRef: "secrets/redirect",
			Filter: factorydefinitions.FactoryWebhookFilterConfig{
				EventTypes: []string{factorydefinitions.FactoryWebhookEventTypeWorkStateChange},
			},
		}},
		Events:         events,
		RuntimeSource:  wireLoadedFactorySource{},
		DeadLetterPath: "runtime/dead-letter.jsonl",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		if err := subscription(context.Background()); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()
	if _, ok := <-configuredRequests; !ok {
		t.Fatal("configured endpoint did not receive the request")
	}
	select {
	case <-configuredRequests:
		t.Fatal("configured endpoint received more than one request")
	default:
	}
	line := receiveWireDeadLetter(t, deadLetters)
	var record struct {
		AttemptCount   int    `json:"attemptCount"`
		StatusCode     int    `json:"statusCode"`
		TerminalReason string `json:"terminalReason"`
	}
	if err := json.Unmarshal(line, &record); err != nil {
		t.Fatalf("decode dead-letter record: %v", err)
	}
	if record.AttemptCount != 1 || record.StatusCode != http.StatusTemporaryRedirect || record.TerminalReason != "non_retryable_http_status" {
		t.Fatalf("redirect dead-letter = %#v, want one terminal 307 attempt", record)
	}
	targetMu.Lock()
	defer targetMu.Unlock()
	if targetRequests != 0 {
		t.Fatalf("redirect target received %d requests, want none", targetRequests)
	}
}

func receiveWireDeadLetter(t *testing.T, records <-chan []byte) []byte {
	t.Helper()
	select {
	case record := <-records:
		return record
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for webhook dead-letter record")
		return nil
	}
}

type wireWebhookEvents struct {
	recordings.Service
	event recordings.CanonicalEvent
}

func (events *wireWebhookEvents) SubscribeFrom(context.Context, recordings.SubscribeRequest) (recordings.SubscribeResult, error) {
	outcomes := make(chan recordings.SubscriptionOutcome, 1)
	outcomes <- recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: events.event}
	return recordings.SubscribeResult{Subscription: func(ctx context.Context) recordings.SubscriptionOutcome {
		select {
		case outcome := <-outcomes:
			return outcome
		case <-ctx.Done():
			return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
		}
	}}, nil
}

type wireLoadedFactorySource struct {
	factorydefinitions.RuntimeDefinitionLookup
}

func (wireLoadedFactorySource) FactoryDir() string { return "/factories/test" }
func (wireLoadedFactorySource) FactoryConfig() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{}
}
func (wireLoadedFactorySource) RuntimeBaseDir() string { return "/runtime/test" }

func wireWebhookEvent() recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID:         "wire-event-1",
		Sequence:   1,
		RecordedAt: time.Date(2026, time.August, 11, 9, 0, 0, 0, time.UTC),
		Kind:       recordings.CanonicalEventKind(factorydefinitions.FactoryWebhookEventTypeWorkStateChange),
		Payload:    `{"workId":"work-1","fromState":"queued","toState":"done"}`,
	}
}

var _ factorydefinitions.LoadedFactorySource = wireLoadedFactorySource{}
var _ recordings.Service = (*wireWebhookEvents)(nil)
