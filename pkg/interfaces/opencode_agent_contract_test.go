package interfaces

import (
	"strings"
	"testing"
)

func TestValidateOpenCodeAgentForRunnerSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		workstationAgent  string
		workerAgent       string
		selection         ResolvedRunnerSelection
		wantErrSubstrings []string
	}{
		{
			name:      "AllowsOpenCodeRunner",
			workerAgent: "reviewer",
			selection: ResolvedRunnerSelection{
				RunnerID: RunnerIDOpenCode,
				Source:   RunnerSelectionSourceFactory,
			},
		},
		{
			name:      "AllowsUnsetAgent",
			selection: ResolvedRunnerSelection{RunnerID: RunnerIDCodex, Source: RunnerSelectionSourceDefault},
		},
		{
			name:              "RejectsWorkerAgentOnCodexRunner",
			workerAgent:       "reviewer",
			selection:         ResolvedRunnerSelection{RunnerID: RunnerIDCodex, Source: RunnerSelectionSourceDefault},
			wantErrSubstrings: []string{"openCodeAgent", "reviewer", RunnerIDOpenCode, RunnerIDCodex},
		},
		{
			name:              "RejectsWorkstationAgentOverrideOnNonOpenCodeRunner",
			workstationAgent:  "implementer",
			workerAgent:       "reviewer",
			selection:         ResolvedRunnerSelection{RunnerID: RunnerIDGemini, Source: RunnerSelectionSourceWorkstation},
			wantErrSubstrings: []string{"openCodeAgent", "implementer", RunnerIDOpenCode, RunnerIDGemini},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateOpenCodeAgentForRunnerSelection(tt.workstationAgent, tt.workerAgent, tt.selection)
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

			got := ResolveOpenCodeAgent(tt.workstationAgent, tt.workerAgent)
			if got != tt.wantOpenCodeAgent {
				t.Fatalf("ResolveOpenCodeAgent(...) = %q, want %q", got, tt.wantOpenCodeAgent)
			}
		})
	}
}
