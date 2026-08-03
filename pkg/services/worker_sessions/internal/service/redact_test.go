package service

import (
	"errors"
	"strings"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRedactDetail_SensitiveContent_IsReplacedWithFixedPlaceholder(t *testing.T) {
	cases := map[string]string{
		"api key":        "request failed: api_key=sk-live-abc123 rejected",
		"bearer token":   "unauthorized: Bearer eyJhbGciOiJIUzI1NiJ9.secretpayload",
		"password":       "connection refused, password=hunter2",
		"env assignment": "process exited with GITHUB_TOKEN=ghp_abcdefghijklmnop set",
		"windows path":   `failed reading C:\Users\andre\.ssh\id_rsa`,
		"unc path":       `failed reading \\build-host\secrets\deploy.pem`,
		"unix path":      "failed loading /etc/secrets/deploy.key",
		"url":            "request to https://internal.example.com/token?key=abc123 failed",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			got := redactDetail(raw)
			if got != redactedFailureDetail {
				t.Fatalf("redactDetail(%q) = %q, want fixed placeholder %q", raw, got, redactedFailureDetail)
			}
			if strings.Contains(got, "abc123") || strings.Contains(got, "hunter2") || strings.Contains(got, "id_rsa") {
				t.Fatalf("redactDetail(%q) leaked sensitive content: %q", raw, got)
			}
		})
	}
}

func TestRedactDetail_OrdinaryText_PassesThroughUnchanged(t *testing.T) {
	raw := "the business rule rejected this attempt"
	if got := redactDetail(raw); got != raw {
		t.Fatalf("redactDetail(%q) = %q, want unchanged", raw, got)
	}
}

func TestRedactDetail_BlankText_ReturnsEmpty(t *testing.T) {
	if got := redactDetail("   "); got != "" {
		t.Fatalf("redactDetail(blank) = %q, want empty", got)
	}
}

func TestRedactDetail_OverlongText_IsBounded(t *testing.T) {
	raw := strings.Repeat("x", maxFailureDetailLength+50)
	got := redactDetail(raw)
	if len(got) != maxFailureDetailLength {
		t.Fatalf("redactDetail() length = %d, want %d", len(got), maxFailureDetailLength)
	}
}

// TestClassifyTerminal_ExecutorPanicWithSecretBearingCause_RedactsDetailButKeepsKind
// proves the review's exact ask: a failed result whose panic evidence
// carries secret-bearing text still classifies as EXECUTOR_PANIC (Kind is
// never affected by redaction), while Detail never reproduces the secret.
func TestClassifyTerminal_ExecutorPanicWithSecretBearingCause_RedactsDetailButKeepsKind(t *testing.T) {
	dispatchResult := workers.WorkstationDispatchResult{
		Result: workers.WorkResult{
			Outcome: workers.OutcomeFailed,
			Error:   "executor panic: authorization Bearer sk-live-abc123 rejected",
		},
	}

	terminal := classifyTerminal(nil, dispatchResult)

	if terminal.Outcome != workersessions.TerminalOutcomeFailed {
		t.Fatalf("terminal outcome = %q, want FAILED", terminal.Outcome)
	}
	if terminal.Cause == nil {
		t.Fatal("terminal cause = nil, want non-nil")
	}
	if terminal.Cause.Kind != workersessions.FailureCauseExecutorPanic {
		t.Fatalf("terminal cause kind = %q, want EXECUTOR_PANIC", terminal.Cause.Kind)
	}
	if strings.Contains(terminal.Cause.Detail, "sk-live-abc123") {
		t.Fatalf("terminal cause detail leaked secret: %q", terminal.Cause.Detail)
	}
	if terminal.Cause.Detail != redactedFailureDetail {
		t.Fatalf("terminal cause detail = %q, want fixed placeholder %q", terminal.Cause.Detail, redactedFailureDetail)
	}
}

func TestClassifyTerminal_StartFailureWithSecretBearingAdapterError_RedactsDetail(t *testing.T) {
	dispatchErr := errors.New("dial tcp failed: password=hunter2")

	terminal := classifyTerminal(dispatchErr, workers.WorkstationDispatchResult{})

	if terminal.Cause == nil {
		t.Fatal("terminal cause = nil, want non-nil")
	}
	if strings.Contains(terminal.Cause.Detail, "hunter2") {
		t.Fatalf("terminal cause detail leaked secret: %q", terminal.Cause.Detail)
	}
}
