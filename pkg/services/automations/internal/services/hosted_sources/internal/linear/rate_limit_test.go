package linear

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
)

func TestClientFetchIssuesPage_PreservesSanitizedRetryAfter(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(now)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"errors":[{"message":"private provider response"}]}`))
	}))
	defer server.Close()

	_, err := (Client{Endpoint: server.URL, HTTPClient: server.Client(), Clock: fakeClock}).fetchIssuesPage(
		context.Background(), "secret", "", linearIssueFilter{},
	)
	if err == nil {
		t.Fatal("fetchIssuesPage() error = nil, want rate-limit error")
	}
	var rateLimitErr *RateLimitError
	if !errors.As(err, &rateLimitErr) {
		t.Fatalf("fetchIssuesPage() error = %v, want RateLimitError", err)
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("fetchIssuesPage() error = %v, want ErrRateLimited", err)
	}
	if delay, ok := rateLimitErr.RetryDelay(); !ok || delay != 7*time.Second {
		t.Fatalf("retry delay = (%s, %t), want (7s, true)", delay, ok)
	}
	if strings.Contains(err.Error(), "private provider response") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("rate-limit error = %q, want sanitized provider metadata", err)
	}
}

func TestDecodeIssuesPageResponse_RetryAfterSemantics(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		header     string
		wantDelay  time.Duration
		wantUsable bool
	}{
		{name: "seconds", header: "5", wantDelay: 5 * time.Second, wantUsable: true},
		{name: "http date", header: now.Add(9 * time.Second).Format(http.TimeFormat), wantDelay: 9 * time.Second, wantUsable: true},
		{name: "zero seconds", header: "0", wantUsable: true},
		{name: "missing", wantUsable: false},
		{name: "malformed", header: "later", wantUsable: false},
		{name: "fractional", header: "1.5", wantUsable: false},
		{name: "negative", header: "-1", wantUsable: false},
		{name: "expired date", header: now.Add(-time.Second).Format(http.TimeFormat), wantUsable: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			headers := http.Header{}
			if tc.header != "" {
				headers.Set("Retry-After", tc.header)
			}
			_, err := decodeIssuesPageResponseWithHeaders(http.StatusTooManyRequests, headers, []byte("not logged"), now)
			var rateLimitErr *RateLimitError
			if !errors.As(err, &rateLimitErr) {
				t.Fatalf("decode error = %v, want RateLimitError", err)
			}
			gotDelay, gotUsable := rateLimitErr.RetryDelay()
			if gotUsable != tc.wantUsable || gotDelay != tc.wantDelay {
				t.Fatalf("retry delay = (%s, %t), want (%s, %t)", gotDelay, gotUsable, tc.wantDelay, tc.wantUsable)
			}
		})
	}
}

func TestDecodeIssuesPageResponse_GraphQLRateLimitUsesRetryAfter(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	header := http.Header{"Retry-After": []string{"11"}}
	data := []byte(`{"errors":[{"message":"private provider response","extensions":{"code":"RATELIMITED"}}]}`)

	_, err := decodeIssuesPageResponseWithHeaders(http.StatusOK, header, data, now)
	var rateLimitErr *RateLimitError
	if !errors.As(err, &rateLimitErr) {
		t.Fatalf("decode error = %v, want RateLimitError", err)
	}
	if delay, ok := rateLimitErr.RetryDelay(); !ok || delay != 11*time.Second {
		t.Fatalf("retry delay = (%s, %t), want (11s, true)", delay, ok)
	}
	if strings.Contains(err.Error(), "private provider response") {
		t.Fatalf("GraphQL rate-limit error = %q, want sanitized error", err)
	}
}

func TestDecodeIssuesPageResponse_GraphQLRateLimitUsesExtensionGuidance(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	data := []byte(`{"errors":[{"message":"private provider response","extensions":{"code":"RATE_LIMITED","retryAfter":4}}]}`)

	_, err := decodeIssuesPageResponseWithHeaders(http.StatusOK, nil, data, now)
	var rateLimitErr *RateLimitError
	if !errors.As(err, &rateLimitErr) {
		t.Fatalf("decode error = %v, want RateLimitError", err)
	}
	if delay, ok := rateLimitErr.RetryDelay(); !ok || delay != 4*time.Second {
		t.Fatalf("retry delay = (%s, %t), want (4s, true)", delay, ok)
	}
}
