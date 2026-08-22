package linear

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxRetryAfterSeconds = int64((1<<63 - 1) / int64(time.Second))

type rateLimitExtensions struct {
	Code              string          `json:"code"`
	RetryAfter        json.RawMessage `json:"retryAfter"`
	RetryAfterSeconds json.RawMessage `json:"retryAfterSeconds"`
}

func newRateLimitError(headers http.Header, extensions rateLimitExtensions, now time.Time) error {
	delay, ok := retryAfterFromHeaders(headers, now)
	if !ok {
		delay, ok = retryAfterFromExtensions(extensions, now)
	}
	return &RateLimitError{RetryAfter: delay, HasRetryAfter: ok}
}

func retryAfterFromHeaders(headers http.Header, now time.Time) (time.Duration, bool) {
	if headers == nil {
		return 0, false
	}
	return parseRetryAfterValue(headers.Get("Retry-After"), now)
}

func retryAfterFromExtensions(extensions rateLimitExtensions, now time.Time) (time.Duration, bool) {
	for _, value := range []json.RawMessage{extensions.RetryAfter, extensions.RetryAfterSeconds} {
		if len(value) == 0 {
			continue
		}
		var raw string
		if err := json.Unmarshal(value, &raw); err == nil {
			if delay, ok := parseRetryAfterValue(raw, now); ok {
				return delay, true
			}
			continue
		}
		var seconds int64
		if err := json.Unmarshal(value, &seconds); err != nil || seconds < 0 || seconds > maxRetryAfterSeconds {
			continue
		}
		return time.Duration(seconds) * time.Second, true
	}
	return 0, false
}

func parseRetryAfterValue(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if isDecimalSeconds(value) {
		seconds, err := strconv.ParseInt(value, 10, 64)
		if err != nil || seconds < 0 || seconds > maxRetryAfterSeconds {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}

	retryAt, err := http.ParseTime(value)
	if err != nil || now.IsZero() || retryAt.Before(now) {
		return 0, false
	}
	return retryAt.Sub(now), true
}

func isDecimalSeconds(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
