package provider

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestSelectFailureByPrecedence_Table(t *testing.T) {
	auth := ProviderFailureResult{Reason: interfaces.WorkFailureTypeAuthFailure, Message: "auth"}
	throttle := ProviderFailureResult{Reason: interfaces.WorkFailureTypeThrottled, Message: "throttle"}
	timeout := ProviderFailureResult{Reason: interfaces.WorkFailureTypeTimeout, Message: "timeout"}
	canceled := ProviderFailureResult{Reason: interfaces.WorkFailureTypeUnknown, Message: "canceled"}
	structuredUnknown := ProviderFailureResult{Reason: interfaces.WorkFailureTypeUnknown, Message: "structured unknown"}
	stderrUnknown := ProviderFailureResult{Reason: interfaces.WorkFailureTypeUnknown, Message: "stderr unknown"}
	exit := ProviderFailureResult{Reason: interfaces.WorkFailureTypeUnknown, Message: "exit fallback"}

	testCases := []struct {
		name   string
		input  []CompetingFailureSignal
		want   ProviderFailureResult
		wantOK bool
	}{
		{
			name: "cancel_wins_over_structured_stderr_and_exit",
			input: []CompetingFailureSignal{
				{Tier: FailureSignalTierStructured, Recognized: true, Result: throttle},
				{Tier: FailureSignalTierStderr, Recognized: true, Result: auth},
				{Tier: FailureSignalTierExit, Result: exit},
				{Tier: FailureSignalTierCancelTimeout, Result: canceled},
			},
			want:   canceled,
			wantOK: true,
		},
		{
			name: "timeout_wins_over_structured_stderr_and_exit",
			input: []CompetingFailureSignal{
				{Tier: FailureSignalTierCancelTimeout, Recognized: true, Result: timeout},
				{Tier: FailureSignalTierStructured, Recognized: true, Result: throttle},
				{Tier: FailureSignalTierStderr, Recognized: true, Result: auth},
				{Tier: FailureSignalTierExit, Result: exit},
			},
			want:   timeout,
			wantOK: true,
		},
		{
			name: "structured_wins_over_stderr_and_exit",
			input: []CompetingFailureSignal{
				{Tier: FailureSignalTierStructured, Recognized: true, Result: throttle},
				{Tier: FailureSignalTierStderr, Recognized: true, Result: auth},
				{Tier: FailureSignalTierExit, Result: exit},
			},
			want:   throttle,
			wantOK: true,
		},
		{
			name: "stderr_wins_when_no_recognized_structured",
			input: []CompetingFailureSignal{
				{Tier: FailureSignalTierStructured, Recognized: false, Result: structuredUnknown},
				{Tier: FailureSignalTierStderr, Recognized: true, Result: auth},
				{Tier: FailureSignalTierExit, Result: exit},
			},
			want:   auth,
			wantOK: true,
		},
		{
			name: "exit_fallback_when_only_exit_signal",
			input: []CompetingFailureSignal{
				{Tier: FailureSignalTierExit, Result: exit},
			},
			want:   exit,
			wantOK: true,
		},
		{
			name: "unrecognized_structured_before_unrecognized_stderr",
			input: []CompetingFailureSignal{
				{Tier: FailureSignalTierStructured, Recognized: false, Result: structuredUnknown},
				{Tier: FailureSignalTierStderr, Recognized: false, Result: stderrUnknown},
				{Tier: FailureSignalTierExit, Result: exit},
			},
			want:   structuredUnknown,
			wantOK: true,
		},
		{
			name: "unrecognized_stderr_before_exit",
			input: []CompetingFailureSignal{
				{Tier: FailureSignalTierStderr, Recognized: false, Result: stderrUnknown},
				{Tier: FailureSignalTierExit, Result: exit},
			},
			want:   stderrUnknown,
			wantOK: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := SelectFailureByPrecedence(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("SelectFailureByPrecedence() ok = %t, want %t", ok, tc.wantOK)
			}
			if got.Result != tc.want {
				t.Fatalf("SelectFailureByPrecedence() = %#v, want %#v", got.Result, tc.want)
			}
		})
	}
}
