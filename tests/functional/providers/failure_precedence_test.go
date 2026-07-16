package providers

import (
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"github.com/portpowered/infinite-you/pkg/workers/provider"
)

func TestSelectFailureByPrecedence_Table(t *testing.T) {
	auth := provider.ProviderFailureResult{Reason: workerexecution.WorkFailureTypeAuthFailure, Message: "auth"}
	throttle := provider.ProviderFailureResult{Reason: workerexecution.WorkFailureTypeThrottled, Message: "throttle"}
	timeout := provider.ProviderFailureResult{Reason: workerexecution.WorkFailureTypeTimeout, Message: "timeout"}
	canceled := provider.ProviderFailureResult{Reason: workerexecution.WorkFailureTypeUnknown, Message: "canceled"}
	structuredUnknown := provider.ProviderFailureResult{Reason: workerexecution.WorkFailureTypeUnknown, Message: "structured unknown"}
	exit := provider.ProviderFailureResult{Reason: workerexecution.WorkFailureTypeUnknown, Message: "exit fallback"}

	testCases := []struct {
		name   string
		input  []provider.CompetingFailureSignal
		want   provider.ProviderFailureResult
		wantOK bool
	}{
		{
			name: "cancel_wins_over_structured_stderr_and_exit",
			input: []provider.CompetingFailureSignal{
				{Tier: provider.FailureSignalTierStructured, Recognized: true, Result: throttle},
				{Tier: provider.FailureSignalTierStderr, Recognized: true, Result: auth},
				{Tier: provider.FailureSignalTierExit, Result: exit},
				{Tier: provider.FailureSignalTierCancelTimeout, Result: canceled},
			},
			want:   canceled,
			wantOK: true,
		},
		{
			name: "timeout_wins_over_structured_stderr_and_exit",
			input: []provider.CompetingFailureSignal{
				{Tier: provider.FailureSignalTierCancelTimeout, Recognized: true, Result: timeout},
				{Tier: provider.FailureSignalTierStructured, Recognized: true, Result: throttle},
				{Tier: provider.FailureSignalTierStderr, Recognized: true, Result: auth},
				{Tier: provider.FailureSignalTierExit, Result: exit},
			},
			want:   timeout,
			wantOK: true,
		},
		{
			name: "structured_wins_over_stderr_and_exit",
			input: []provider.CompetingFailureSignal{
				{Tier: provider.FailureSignalTierStructured, Recognized: true, Result: throttle},
				{Tier: provider.FailureSignalTierStderr, Recognized: true, Result: auth},
				{Tier: provider.FailureSignalTierExit, Result: exit},
			},
			want:   throttle,
			wantOK: true,
		},
		{
			name: "stderr_wins_when_no_recognized_structured",
			input: []provider.CompetingFailureSignal{
				{Tier: provider.FailureSignalTierStructured, Recognized: false, Result: structuredUnknown},
				{Tier: provider.FailureSignalTierStderr, Recognized: true, Result: auth},
				{Tier: provider.FailureSignalTierExit, Result: exit},
			},
			want:   auth,
			wantOK: true,
		},
		{
			name: "exit_fallback_when_only_exit_signal",
			input: []provider.CompetingFailureSignal{
				{Tier: provider.FailureSignalTierExit, Result: exit},
			},
			want:   exit,
			wantOK: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := provider.SelectFailureByPrecedence(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("SelectFailureByPrecedence() ok = %t, want %t", ok, tc.wantOK)
			}
			if got.Result != tc.want {
				t.Fatalf("SelectFailureByPrecedence() = %#v, want %#v", got.Result, tc.want)
			}
		})
	}
}
