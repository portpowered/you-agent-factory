package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	deadLetters   webhooks.DeadLetterAppender
	deadLetterMu  sync.Mutex
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
	return NewWithDeadLetterAppender(httpClient, secretResolve, clock, nil, logger)
}

// NewWithDeadLetterAppender constructs the Webhooks service with the exact
// runtime-storage effect used for terminal delivery records.
func NewWithDeadLetterAppender(
	httpClient interface {
		Do(*http.Request) (*http.Response, error)
	},
	secretResolve webhooks.SecretResolver,
	clock interface{ Now() time.Time },
	deadLetters webhooks.DeadLetterAppender,
	logger logging.Logger,
) *Service {
	if httpClient == nil || clock == nil {
		return nil
	}
	return &Service{
		httpClient:    httpClient,
		secretResolve: secretResolve,
		clock:         clock,
		deadLetters:   deadLetters,
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
		// A webhook remains a live subscriber after activation. The generic
		// reconnect contract may close immediately when a cursor is exactly at
		// the retained tail, so activation filtering is applied below while the
		// underlying scoped subscription remains live.
		Scope: request.Scope,
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
			if atOrBeforeActivationCursor(outcome.Event, request.ActivationCursor) {
				continue
			}
			if !matchesEventFilter(definition, outcome.Event) {
				continue
			}
			service.deliver(ctx, request, definition, outcome.Event, secret, policy)
		case recordings.SubscriptionGap:
			if outcome.Gap == nil {
				service.logger.Error("factory webhook subscription gap has no reconnect cursor", "endpoint", definition.Name)
				return
			}
			reconnectFrom := outcome.Gap.ReconnectFrom
			reconnected, reconnectErr := request.Events.SubscribeFrom(ctx, recordings.SubscribeRequest{
				Cursor: &reconnectFrom,
				Scope:  request.Scope,
			})
			if reconnectErr != nil {
				service.logger.Error(
					"factory webhook subscription reconnect failed",
					"endpoint", definition.Name,
					"cause", outcome.Gap.Cause,
					"error", reconnectErr,
				)
				return
			}
			subscribed = reconnected
		default:
			return
		}
	}
}

func atOrBeforeActivationCursor(
	event recordings.CanonicalEvent,
	cursor *recordings.CanonicalEventCursor,
) bool {
	if cursor == nil || cursor.StreamGenerationID == "" ||
		event.Cursor.StreamGenerationID != cursor.StreamGenerationID {
		return false
	}
	return event.Cursor.Sequence <= cursor.Sequence
}

func matchesEventFilter(
	definition factorydefinitions.FactoryWebhookConfig,
	event recordings.CanonicalEvent,
) bool {
	switch event.Kind {
	case recordings.CanonicalEventKind(factorydefinitions.FactoryWebhookEventTypeWorkStateChange):
		return containsWebhookValue(definition.Filter.EventTypes, string(event.Kind))
	case recordings.CanonicalEventKind(factorydefinitions.FactoryWebhookEventTypeDispatchResponse):
		return matchesDispatchFailureFilter(definition, event, "outcome", factorydefinitions.FactoryWebhookDispatchStatusFailed)
	case recordings.CanonicalEventKind(factorydefinitions.FactoryWebhookEventTypeDispatchReconciled):
		return matchesDispatchFailureFilter(definition, event, "reconciledStatus", factorydefinitions.FactoryWebhookDispatchStatusFailed)
	case recordings.CanonicalEventKind(factorydefinitions.FactoryWebhookEventTypeDispatchInterrupted):
		return matchesDispatchFailureFilter(definition, event, "observedStatus", factorydefinitions.FactoryWebhookDispatchStatusInterrupted)
	default:
		return false
	}
}

func matchesDispatchFailureFilter(
	definition factorydefinitions.FactoryWebhookConfig,
	event recordings.CanonicalEvent,
	statusField string,
	expectedStatus string,
) bool {
	if !containsWebhookValue(definition.Filter.EventTypes, string(event.Kind)) {
		return false
	}
	status, ok := canonicalDispatchStatus(event.Payload, statusField)
	if !ok || status != expectedStatus {
		return false
	}
	if len(definition.Filter.DispatchStatuses) == 0 {
		return true
	}
	return containsWebhookValue(definition.Filter.DispatchStatuses, status)
}

func canonicalDispatchStatus(payload, field string) (string, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &fields); err != nil {
		return "", false
	}
	value, ok := fields[field]
	if !ok {
		return "", false
	}
	var status string
	if err := json.Unmarshal(value, &status); err != nil {
		return "", false
	}
	return status, true
}

func containsWebhookValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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
