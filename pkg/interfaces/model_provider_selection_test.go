package interfaces

import "testing"

func TestResolveModelProviderSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		workstationModelProvider string
		factoryModelProvider     string
		workerModelProvider      string
		wantProvider             ModelProvider
		wantSource               ModelProviderSelectionSource
	}{
		{
			name:                     "WorkstationWins",
			workstationModelProvider: "GEMINI",
			factoryModelProvider:     string(ModelProviderCodex),
			workerModelProvider:      string(ModelProviderCodex),
			wantProvider:             ModelProviderGemini,
			wantSource:               ModelProviderSelectionSourceWorkstation,
		},
		{
			name:                 "FactoryWinsWhenWorkstationUnset",
			factoryModelProvider: "cursor-cli",
			wantProvider:         ModelProviderCursor,
			wantSource:           ModelProviderSelectionSourceFactory,
		},
		{
			name:                "WorkerProviderCompatibility",
			workerModelProvider: string(ModelProviderCodex),
			wantProvider:        ModelProviderCodex,
			wantSource:          ModelProviderSelectionSourceWorker,
		},
		{
			name:                "WorkerClaudeProvider",
			workerModelProvider: string(ModelProviderClaude),
			wantProvider:        ModelProviderClaude,
			wantSource:          ModelProviderSelectionSourceWorker,
		},
		{
			name:                "OperatorDefaultFallsBackToCodex",
			workerModelProvider: "unknown-provider",
			wantProvider:        OperatorDefaultModelProvider,
			wantSource:          ModelProviderSelectionSourceOperatorDefault,
		},
		{
			name:                     "DefaultDefersFromWorkstationToFactory",
			workstationModelProvider: FactoryModelProviderDefault,
			factoryModelProvider:     "CODEX",
			workerModelProvider:      string(ModelProviderClaude),
			wantProvider:             ModelProviderCodex,
			wantSource:               ModelProviderSelectionSourceFactory,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ResolveModelProviderSelection(tt.workstationModelProvider, tt.factoryModelProvider, tt.workerModelProvider)
			if got.Provider != tt.wantProvider || got.Source != tt.wantSource {
				t.Fatalf("ResolveModelProviderSelection(...) = %#v, want provider=%q source=%q", got, tt.wantProvider, tt.wantSource)
			}
		})
	}
}

func TestResolveRunnerSelection_AdapterMapsProviderSelection(t *testing.T) {
	t.Parallel()

	selection := ResolveRunnerSelection("GEMINI", "CODEX", string(ModelProviderClaude))
	if selection.RunnerID != RunnerIDGemini {
		t.Fatalf("runner = %q, want %q", selection.RunnerID, RunnerIDGemini)
	}
	if selection.Source != RunnerSelectionSourceWorkstation {
		t.Fatalf("source = %q, want %q", selection.Source, RunnerSelectionSourceWorkstation)
	}
}

func TestBuiltInModelProviderMetadata_MatchesRunnerCapabilities(t *testing.T) {
	t.Parallel()

	metadata, ok := BuiltInModelProviderMetadata(ModelProviderCodex)
	if !ok {
		t.Fatal("expected codex provider metadata")
	}
	if metadata.Provider != ModelProviderCodex {
		t.Fatalf("provider = %q, want %q", metadata.Provider, ModelProviderCodex)
	}
	if metadata.DisplayName != "Codex" {
		t.Fatalf("display name = %q, want Codex", metadata.DisplayName)
	}
}

func TestValidateOpenCodeAgentForModelProviderSelection(t *testing.T) {
	t.Parallel()

	err := ValidateOpenCodeAgentForModelProviderSelection(
		"reviewer",
		"",
		ResolvedModelProviderSelection{Provider: ModelProviderGemini, Source: ModelProviderSelectionSourceFactory},
	)
	if err == nil {
		t.Fatal("expected openCodeAgent validation error for non-opencode provider")
	}

	err = ValidateOpenCodeAgentForModelProviderSelection(
		"reviewer",
		"",
		ResolvedModelProviderSelection{Provider: ModelProviderOpenCode, Source: ModelProviderSelectionSourceWorkstation},
	)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}
