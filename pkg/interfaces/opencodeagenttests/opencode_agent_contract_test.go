package opencodeagenttests

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestValidateOpenCodeAgentForRunnerSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		workstationAgent  string
		workerAgent       string
		selection         interfaces.ResolvedRunnerSelection
		wantErrSubstrings []string
	}{
		{
			name:      "AllowsOpenCodeRunner",
			workerAgent: "reviewer",
			selection: interfaces.ResolvedRunnerSelection{
				RunnerID: interfaces.RunnerIDOpenCode,
				Source:   interfaces.RunnerSelectionSourceFactory,
			},
		},
		{
			name:      "AllowsUnsetAgent",
			selection: interfaces.ResolvedRunnerSelection{RunnerID: interfaces.RunnerIDCodex, Source: interfaces.RunnerSelectionSourceDefault},
		},
		{
			name:              "RejectsWorkerAgentOnCodexRunner",
			workerAgent:       "reviewer",
			selection:         interfaces.ResolvedRunnerSelection{RunnerID: interfaces.RunnerIDCodex, Source: interfaces.RunnerSelectionSourceDefault},
			wantErrSubstrings: []string{"openCodeAgent", "reviewer", interfaces.RunnerIDOpenCode, interfaces.RunnerIDCodex},
		},
		{
			name:              "RejectsWorkstationAgentOverrideOnNonOpenCodeRunner",
			workstationAgent:  "implementer",
			workerAgent:       "reviewer",
			selection:         interfaces.ResolvedRunnerSelection{RunnerID: interfaces.RunnerIDGemini, Source: interfaces.RunnerSelectionSourceWorkstation},
			wantErrSubstrings: []string{"openCodeAgent", "implementer", interfaces.RunnerIDOpenCode, interfaces.RunnerIDGemini},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := interfaces.ValidateOpenCodeAgentForRunnerSelection(tt.workstationAgent, tt.workerAgent, tt.selection)
			if len(tt.wantErrSubstrings) == 0 {
				if err != nil {
					t.Fatalf("ValidateOpenCodeAgentForRunnerSelection(...) = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatal("ValidateOpenCodeAgentForRunnerSelection(...) = nil, want error")
			}
			for _, want := range tt.wantErrSubstrings {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %q, want substring %q", err.Error(), want)
				}
			}
		})
	}
}

func TestResolveOpenCodeAgent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		workstationAgent  string
		workerAgent       string
		wantOpenCodeAgent string
	}{
		{
			name:              "WorkstationOverridesWorker",
			workstationAgent:  " implementer ",
			workerAgent:       "reviewer",
			wantOpenCodeAgent: "implementer",
		},
		{
			name:              "WorkerDefaultWhenWorkstationUnset",
			workerAgent:       "reviewer",
			wantOpenCodeAgent: "reviewer",
		},
		{
			name: "EmptyWhenUnset",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := interfaces.ResolveOpenCodeAgent(tt.workstationAgent, tt.workerAgent)
			if got != tt.wantOpenCodeAgent {
				t.Fatalf("ResolveOpenCodeAgent(...) = %q, want %q", got, tt.wantOpenCodeAgent)
			}
		})
	}
}
