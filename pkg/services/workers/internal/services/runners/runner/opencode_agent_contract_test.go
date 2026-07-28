package runner_test

import (
	"strings"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerrunner "github.com/portpowered/infinite-you/pkg/services/workers/runner"
)

func TestValidateOpenCodeAgentForRunnerSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		workstationAgent  string
		workerAgent       string
		selection         workerexecution.ResolvedRunnerSelection
		wantErrSubstrings []string
	}{
		{
			name:        "AllowsOpenCodeRunner",
			workerAgent: "reviewer",
			selection: workerexecution.ResolvedRunnerSelection{
				RunnerID: workerexecution.RunnerIDOpenCode,
				Source:   workerexecution.RunnerSelectionSourceFactory,
			},
		},
		{
			name:      "AllowsUnsetAgent",
			selection: workerexecution.ResolvedRunnerSelection{RunnerID: workerexecution.RunnerIDCodex, Source: workerexecution.RunnerSelectionSourceDefault},
		},
		{
			name:              "RejectsWorkerAgentOnCodexRunner",
			workerAgent:       "reviewer",
			selection:         workerexecution.ResolvedRunnerSelection{RunnerID: workerexecution.RunnerIDCodex, Source: workerexecution.RunnerSelectionSourceDefault},
			wantErrSubstrings: []string{"openCodeAgent", "reviewer", workerexecution.RunnerIDOpenCode, workerexecution.RunnerIDCodex},
		},
		{
			name:              "RejectsWorkstationAgentOverrideOnNonOpenCodeRunner",
			workstationAgent:  "implementer",
			workerAgent:       "reviewer",
			selection:         workerexecution.ResolvedRunnerSelection{RunnerID: workerexecution.RunnerIDGemini, Source: workerexecution.RunnerSelectionSourceWorkstation},
			wantErrSubstrings: []string{"openCodeAgent", "implementer", workerexecution.RunnerIDOpenCode, workerexecution.RunnerIDGemini},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := workerrunner.ValidateOpenCodeAgentForRunnerSelection(tt.workstationAgent, tt.workerAgent, tt.selection)
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

			got := workerrunner.ResolveOpenCodeAgent(tt.workstationAgent, tt.workerAgent)
			if got != tt.wantOpenCodeAgent {
				t.Fatalf("ResolveOpenCodeAgent(...) = %q, want %q", got, tt.wantOpenCodeAgent)
			}
		})
	}
}
