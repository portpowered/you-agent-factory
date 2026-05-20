package interfaces

import "testing"

func TestResolveRunnerSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		workstationRunner string
		factoryRunner     string
		modelProvider     string
		wantRunner        string
		wantSource        RunnerSelectionSource
	}{
		{
			name:              "WorkstationWins",
			workstationRunner: "  GEMINI ",
			factoryRunner:     RunnerIDCodex,
			modelProvider:     RunnerIDCodex,
			wantRunner:        RunnerIDGemini,
			wantSource:        RunnerSelectionSourceWorkstation,
		},
		{
			name:          "FactoryWinsWhenWorkstationUnset",
			factoryRunner: "cursor-cli",
			wantRunner:    RunnerIDCursorCLI,
			wantSource:    RunnerSelectionSourceFactory,
		},
		{
			name:          "LegacyModelProviderCompatibility",
			modelProvider: "codex",
			wantRunner:    RunnerIDCodex,
			wantSource:    RunnerSelectionSourceLegacyProvider,
		},
		{
			name:          "DefaultFallsBackToCodex",
			modelProvider: "claude",
			wantRunner:    RunnerIDCodex,
			wantSource:    RunnerSelectionSourceDefault,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ResolveRunnerSelection(tt.workstationRunner, tt.factoryRunner, tt.modelProvider)
			if got.RunnerID != tt.wantRunner || got.Source != tt.wantSource {
				t.Fatalf("ResolveRunnerSelection(...) = %#v, want runner=%q source=%q", got, tt.wantRunner, tt.wantSource)
			}
		})
	}
}

func TestBuiltInRunnerMetadata(t *testing.T) {
	t.Parallel()

	metadata, ok := BuiltInRunnerMetadata("  CURSOR-CLI ")
	if !ok {
		t.Fatal("expected cursor-cli metadata")
	}
	if metadata.ID != RunnerIDCursorCLI {
		t.Fatalf("metadata.ID = %q, want %q", metadata.ID, RunnerIDCursorCLI)
	}
}
