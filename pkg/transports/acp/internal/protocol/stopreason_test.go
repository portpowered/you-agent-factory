package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
)

func TestMapStopReason_SupportedOutcomes(t *testing.T) {
	tests := []struct {
		outcome TerminalOutcome
		want    acpsdk.StopReason
	}{
		{TerminalCompleted, acpsdk.StopReasonEndTurn},
		{TerminalCancelled, acpsdk.StopReasonCancelled},
		{TerminalTokensExhausted, acpsdk.StopReasonMaxTokens},
		{TerminalRefused, acpsdk.StopReasonRefusal},
	}
	for _, tt := range tests {
		t.Run(string(tt.outcome), func(t *testing.T) {
			got := MapStopReason(tt.outcome, nil)
			if got.StopReason != tt.want {
				t.Errorf("MapStopReason(%q) = %q, want %q", tt.outcome, got.StopReason, tt.want)
			}
		})
	}
}

func TestMapStopReason_FailedAndUnknownOutcomesFallBackToEndTurn(t *testing.T) {
	tests := []TerminalOutcome{
		TerminalFailed,
		TerminalOutcome("some_future_outcome_this_transport_does_not_know_about"),
		TerminalOutcome(""),
	}
	for _, outcome := range tests {
		t.Run(string(outcome), func(t *testing.T) {
			got := MapStopReason(outcome, fmt.Errorf("internal failure detail"))
			if got.StopReason != acpsdk.StopReasonEndTurn {
				t.Errorf("MapStopReason(%q) = %q, want end_turn safe fallback", outcome, got.StopReason)
			}
		})
	}
}

// TestMapStopReason_NeverSerializesTheCause seeds a sensitive internal
// cause into a failed/unmapped terminal outcome and proves the resulting
// StopResult's JSON encoding never contains it, matching the AC that
// failures and unknown terminal outcomes must never serialize an internal
// cause.
func TestMapStopReason_NeverSerializesTheCause(t *testing.T) {
	sentinel := "provider command: /usr/local/bin/agent --token=sk-live-ABC123"
	cause := fmt.Errorf("factory turn failed: %s", sentinel)

	result := MapStopReason(TerminalFailed, cause)

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(encoded), sentinel) {
		t.Errorf("MapStopReason() leaked the internal cause into %s", encoded)
	}
	if strings.Contains(string(encoded), cause.Error()) {
		t.Errorf("MapStopReason() leaked the internal cause into %s", encoded)
	}
}

func TestMapStopReason_IsDeterministic(t *testing.T) {
	first := MapStopReason(TerminalCompleted, nil)
	second := MapStopReason(TerminalCompleted, nil)
	if first != second {
		t.Fatalf("MapStopReason() is not deterministic: %+v vs %+v", first, second)
	}
}
