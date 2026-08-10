package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
)

type deliveryAttempt struct {
	success         bool
	retryable       bool
	statusCode      int
	reason          string
	retryAfter      time.Duration
	hasRetryAfter   bool
	requestCanceled bool
}

func (service *Service) deliver(
	parent context.Context,
	request webhooks.StartRequest,
	definition factorydefinitions.FactoryWebhookConfig,
	event recordings.CanonicalEvent,
	secret string,
	policy factorydefinitions.FactoryWebhookEffectiveDeliveryPolicy,
) {
	body, err := marshalCanonicalEvent(event)
	if err != nil {
		service.logger.Error(
			"factory webhook event encoding failed",
			"endpoint", definition.Name,
			"event_id", string(event.ID),
			"terminal_reason", "event_encoding_error",
		)
		return
	}

	var firstAttemptAt time.Time
	var lastAttemptAt time.Time
	nextBackoff := policy.InitialBackoff
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if parent.Err() != nil {
			return
		}
		attemptAt := service.clock.Now()
		if firstAttemptAt.IsZero() {
			firstAttemptAt = attemptAt
		}
		lastAttemptAt = attemptAt
		result := service.sendAttempt(
			parent,
			definition,
			event,
			body,
			secret,
			policy.RequestTimeout,
			attemptAt,
		)
		service.logAttempt(definition, event, attempt, policy.MaxAttempts, result)
		if result.success || result.requestCanceled || parent.Err() != nil {
			return
		}
		if !result.retryable || attempt == policy.MaxAttempts {
			reason := result.reason
			if result.retryable {
				reason = "retry_exhausted"
			}
			service.appendDeadLetter(
				request,
				definition,
				event,
				body,
				attempt,
				firstAttemptAt,
				lastAttemptAt,
				result.statusCode,
				reason,
			)
			return
		}

		delay := retryDelay(result, nextBackoff, policy.MaxBackoff)
		if err := service.wait(parent, delay); err != nil {
			if parent.Err() != nil {
				return
			}
			service.appendDeadLetter(
				request,
				definition,
				event,
				body,
				attempt,
				firstAttemptAt,
				lastAttemptAt,
				result.statusCode,
				"retry_scheduler_error",
			)
			return
		}
		nextBackoff = increaseBackoff(nextBackoff, policy.BackoffMultiplier, policy.MaxBackoff)
	}
}

func (service *Service) sendAttempt(
	parent context.Context,
	definition factorydefinitions.FactoryWebhookConfig,
	event recordings.CanonicalEvent,
	body []byte,
	secret string,
	requestTimeout time.Duration,
	attemptAt time.Time,
) deliveryAttempt {
	ctx, cancel := context.WithTimeout(parent, requestTimeout)
	defer cancel()
	timestamp := strconv.FormatInt(attemptAt.Unix(), 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, definition.URL, bytes.NewReader(body))
	if err != nil {
		return deliveryAttempt{reason: "request_construction_error"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(webhooks.EventIDHeader, string(event.ID))
	req.Header.Set(webhooks.TimestampHeader, timestamp)
	req.Header.Set(webhooks.SignatureHeader, webhooks.SignatureVersionV1+"="+sign(secret, timestamp, body))
	response, err := service.httpClient.Do(req)
	if err != nil {
		if response != nil {
			consumeResponseBody(response)
		}
		return deliveryAttempt{
			retryable:       true,
			reason:          "transport_error",
			requestCanceled: parent.Err() != nil,
		}
	}
	if response == nil {
		return deliveryAttempt{retryable: true, reason: "empty_http_response"}
	}
	statusCode := response.StatusCode
	retryAfter, hasRetryAfter := parseRetryAfter(response, attemptAt)
	consumeResponseBody(response)
	if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices {
		return deliveryAttempt{success: true, statusCode: statusCode}
	}
	if retryableStatus(statusCode) {
		return deliveryAttempt{
			retryable:     true,
			statusCode:    statusCode,
			reason:        "retryable_http_status",
			retryAfter:    retryAfter,
			hasRetryAfter: hasRetryAfter,
		}
	}
	return deliveryAttempt{
		statusCode: statusCode,
		reason:     "non_retryable_http_status",
	}
}

func (service *Service) logAttempt(
	definition factorydefinitions.FactoryWebhookConfig,
	event recordings.CanonicalEvent,
	attempt, maxAttempts int,
	result deliveryAttempt,
) {
	outcome := "terminal"
	if result.success {
		outcome = "success"
	} else if result.retryable && attempt < maxAttempts {
		outcome = "retrying"
	}
	service.logger.Info(
		"factory webhook delivery attempt",
		"endpoint", definition.Name,
		"event_id", string(event.ID),
		"attempt", attempt,
		"max_attempts", maxAttempts,
		"status", result.statusCode,
		"outcome", outcome,
		"reason", result.reason,
	)
}

func retryableStatus(statusCode int) bool {
	return statusCode == http.StatusRequestTimeout ||
		statusCode == http.StatusTooManyRequests ||
		statusCode >= http.StatusInternalServerError
}

func retryDelay(result deliveryAttempt, exponential, maximum time.Duration) time.Duration {
	delay := exponential
	if result.hasRetryAfter {
		delay = result.retryAfter
	}
	if delay < 0 || delay > maximum {
		return maximum
	}
	return delay
}

func increaseBackoff(current time.Duration, multiplier float64, maximum time.Duration) time.Duration {
	if current >= maximum || multiplier <= 1 {
		return minDuration(current, maximum)
	}
	scaled := float64(current) * multiplier
	if scaled >= float64(maximum) {
		return maximum
	}
	next := time.Duration(scaled)
	if next < current {
		return maximum
	}
	return next
}

func minDuration(left, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}

func (service *Service) wait(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	if clock, ok := service.clock.(interface {
		After(time.Duration) <-chan time.Time
	}); ok {
		select {
		case <-clock.After(delay):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func parseRetryAfter(response *http.Response, now time.Time) (time.Duration, bool) {
	if response == nil || (response.StatusCode != http.StatusTooManyRequests && response.StatusCode != http.StatusServiceUnavailable) {
		return 0, false
	}
	value := strings.TrimSpace(response.Header.Get("Retry-After"))
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		if seconds > int64((time.Duration(1<<63-1))/time.Second) {
			return time.Duration(1<<63 - 1), true
		}
		return time.Duration(seconds) * time.Second, true
	}
	retryAt, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	if !retryAt.After(now) {
		return 0, true
	}
	return retryAt.Sub(now), true
}

func sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func consumeResponseBody(response *http.Response) {
	if response == nil || response.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, webhooks.MaxResponseBodySize))
	_ = response.Body.Close()
}
