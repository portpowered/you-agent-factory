package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
)

type Service struct {
	httpClient interface {
		Do(*http.Request) (*http.Response, error)
	}
	secretResolve webhooks.SecretResolver
	clock         interface{ Now() time.Time }
	logger        logging.Logger
}

var _ webhooks.Service = (*Service)(nil)

func New(
	httpClient interface {
		Do(*http.Request) (*http.Response, error)
	},
	secretResolve webhooks.SecretResolver,
	clock interface{ Now() time.Time },
	logger logging.Logger,
) *Service {
	if httpClient == nil || clock == nil {
		return nil
	}
	return &Service{
		httpClient:    httpClient,
		secretResolve: secretResolve,
		clock:         clock,
		logger:        logging.EnsureLogger(logger),
	}
}

func (service *Service) Start(
	parent context.Context,
	request webhooks.StartRequest,
) (webhooks.Subscription, error) {
	if service == nil {
		return nil, fmt.Errorf("start Factory Webhooks: service is nil")
	}
	if parent == nil {
		parent = context.Background()
	}
	configs := enabledDefinitions(request.Definitions)
	if len(configs) == 0 {
		return webhooks.Subscription(func(context.Context) error { return nil }), nil
	}
	if request.Events == nil {
		return nil, fmt.Errorf("start Factory Webhooks: Recordings service is required")
	}

	ctx, cancel := context.WithCancel(parent)
	subscription := newSubscription(cancel, make(chan struct{}))
	var waitGroup sync.WaitGroup
	waitGroup.Add(len(configs))
	for _, config := range configs {
		go func(config factorydefinitions.FactoryWebhookConfig) {
			defer waitGroup.Done()
			service.runEndpoint(ctx, request, config)
		}(config)
	}
	go func() {
		waitGroup.Wait()
		close(subscription.done)
	}()
	return webhooks.Subscription(subscription.Close), nil
}

func enabledDefinitions(
	definitions []factorydefinitions.FactoryWebhookConfig,
) []factorydefinitions.FactoryWebhookConfig {
	result := make([]factorydefinitions.FactoryWebhookConfig, 0, len(definitions))
	for _, definition := range definitions {
		if definition.Enabled {
			result = append(result, definition)
		}
	}
	return result
}

func (service *Service) runEndpoint(
	ctx context.Context,
	request webhooks.StartRequest,
	definition factorydefinitions.FactoryWebhookConfig,
) {
	subscribed, err := request.Events.SubscribeFrom(ctx, recordings.SubscribeRequest{
		Cursor: request.ActivationCursor,
		Scope:  request.Scope,
	})
	if err != nil {
		service.logger.Error("factory webhook subscription failed", "endpoint", definition.Name, "error", err)
		return
	}
	if service.secretResolve == nil {
		service.logger.Error("factory webhook secret resolver is unavailable", "endpoint", definition.Name)
		return
	}
	if request.RuntimeSource == nil {
		service.logger.Error("factory webhook runtime secret source is unavailable", "endpoint", definition.Name)
		return
	}
	secret, err := service.secretResolve(ctx, request.RuntimeSource, definition.SigningSecretRef)
	if err != nil || strings.TrimSpace(secret) == "" {
		service.logger.Error("factory webhook secret resolution failed", "endpoint", definition.Name, "secret_ref", definition.SigningSecretRef)
		return
	}
	policy, err := factorydefinitions.ResolveFactoryWebhookDeliveryPolicy(definition.DeliveryPolicy)
	if err != nil {
		service.logger.Error("factory webhook delivery policy failed", "endpoint", definition.Name, "error", err)
		return
	}
	for {
		outcome := subscribed.Subscription.Next(ctx)
		switch outcome.Kind {
		case recordings.SubscriptionEvent:
			if !matchesWorkStateFilter(definition, outcome.Event) {
				continue
			}
			service.deliver(ctx, definition, outcome.Event, secret, policy.RequestTimeout)
		case recordings.SubscriptionGap:
			service.logger.Error("factory webhook subscription gap", "endpoint", definition.Name)
			return
		default:
			return
		}
	}
}

func matchesWorkStateFilter(
	definition factorydefinitions.FactoryWebhookConfig,
	event recordings.CanonicalEvent,
) bool {
	if event.Kind != recordings.CanonicalEventKind(factorydefinitions.FactoryWebhookEventTypeWorkStateChange) {
		return false
	}
	for _, eventType := range definition.Filter.EventTypes {
		if eventType == factorydefinitions.FactoryWebhookEventTypeWorkStateChange {
			return true
		}
	}
	return false
}

func (service *Service) deliver(
	parent context.Context,
	definition factorydefinitions.FactoryWebhookConfig,
	event recordings.CanonicalEvent,
	secret string,
	requestTimeout time.Duration,
) {
	body, err := marshalCanonicalEvent(event)
	if err != nil {
		service.logger.Error("factory webhook event encoding failed", "endpoint", definition.Name, "event_id", string(event.ID), "error", err)
		return
	}
	timestamp := strconv.FormatInt(service.clock.Now().Unix(), 10)
	signature := sign(secret, timestamp, body)
	ctx, cancel := context.WithTimeout(parent, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, definition.URL, bytes.NewReader(body))
	if err != nil {
		service.logger.Error("factory webhook request construction failed", "endpoint", definition.Name, "event_id", string(event.ID), "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(webhooks.EventIDHeader, string(event.ID))
	req.Header.Set(webhooks.TimestampHeader, timestamp)
	req.Header.Set(webhooks.SignatureHeader, webhooks.SignatureVersionV1+"="+signature)
	response, err := service.httpClient.Do(req)
	if err != nil {
		if response != nil {
			consumeResponseBody(response)
		}
		service.logger.Error("factory webhook delivery failed", "endpoint", definition.Name, "event_id", string(event.ID), "error", err)
		return
	}
	if response == nil {
		service.logger.Error("factory webhook delivery failed", "endpoint", definition.Name, "event_id", string(event.ID), "error", "empty HTTP response")
		return
	}
	consumeResponseBody(response)
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		service.logger.Error("factory webhook receiver rejected event", "endpoint", definition.Name, "event_id", string(event.ID), "status", response.StatusCode)
	}
}

func marshalCanonicalEvent(event recordings.CanonicalEvent) ([]byte, error) {
	legacyContext := factorydefinitions.FactoryEventContext{
		EventTime: event.RecordedAt,
		Sequence:  int(event.Sequence),
		Tick:      event.FactoryTick,
	}
	if json.Valid([]byte(event.SourceContext)) {
		if err := json.Unmarshal([]byte(event.SourceContext), &legacyContext); err != nil {
			return nil, err
		}
	}
	if legacyContext.SessionID == nil && event.Scope.FactorySessionID != "" {
		sessionID := event.Scope.FactorySessionID
		legacyContext.SessionID = &sessionID
	}
	return json.Marshal(factorydefinitions.FactoryEvent{
		Context:       legacyContext,
		Id:            string(event.ID),
		Payload:       json.RawMessage(event.Payload),
		SchemaVersion: factorydefinitions.FactoryEventSchemaVersionV1,
		Type:          factorydefinitions.FactoryEventType(event.Kind),
	})
}

func sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func consumeResponseBody(response *http.Response) {
	if response.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, webhooks.MaxResponseBodySize))
	_ = response.Body.Close()
}

type subscription struct {
	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
}

func newSubscription(cancel context.CancelFunc, done chan struct{}) *subscription {
	if done == nil {
		done = make(chan struct{})
		close(done)
	}
	return &subscription{cancel: cancel, done: done}
}

func (subscription *subscription) Close(ctx context.Context) error {
	if subscription == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	subscription.closeOnce.Do(func() {
		if subscription.cancel != nil {
			subscription.cancel()
		}
	})
	select {
	case <-subscription.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
