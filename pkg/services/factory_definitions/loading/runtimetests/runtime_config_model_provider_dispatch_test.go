package runtimetests

import (
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestLoadRuntimeConfig_ResolvesPublicWorkerModelProviderToCLICommand(t *testing.T) {
	cases := []struct {
		name     string
		public   string
		internal modelprovider.Provider
	}{
		{name: "CLAUDE", public: "CLAUDE", internal: modelprovider.ProviderClaude},
		{name: "CODEX", public: "CODEX", internal: modelprovider.ProviderCodex},
		{name: "CURSOR", public: "CURSOR", internal: modelprovider.ProviderCursor},
		{name: "GEMINI", public: "GEMINI", internal: modelprovider.ProviderGemini},
		{name: "KIRO", public: "KIRO", internal: modelprovider.ProviderKiro},
		{name: "OPENCODE", public: "OPENCODE", internal: modelprovider.ProviderOpenCode},
		{name: "PI", public: "PI", internal: modelprovider.ProviderPi},
		{name: "AGY", public: "AGY", internal: modelprovider.ProviderAgy},
		{name: "anthropic alias", public: "anthropic", internal: modelprovider.ProviderClaude},
		{name: "openai alias", public: "openai", internal: modelprovider.ProviderCodex},
		{name: "cursor-agent alias", public: "cursor-agent", internal: modelprovider.ProviderCursor},
		{name: "kiro-cli alias", public: "kiro-cli", internal: modelprovider.ProviderKiro},
		{name: "antigravity alias", public: "antigravity", internal: modelprovider.ProviderAgy},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			factoryDir := t.TempDir()
			writeRuntimeFactoryJSON(t, factoryDir, map[string]any{
				"name": "factory",
				"workTypes": []map[string]any{
					{
						"name": "story",
						"states": []map[string]string{
							{"name": "init", "type": "INITIAL"},
							{"name": "complete", "type": "TERMINAL"},
						},
					},
				},
				"resources": []map[string]any{},
				"workers": []map[string]any{
					{
						"name":          "executor",
						"type":          "MODEL_WORKER",
						"model":         "test-model",
						"modelProvider": tc.public,
						"stopToken":     "COMPLETE",
						"body":          "You are the executor.",
					},
				},
				"workstations": []map[string]any{
					{
						"id":      "execute-story",
						"name":    "execute-story",
						"worker":  "executor",
						"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
						"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
						"type":    "MODEL_WORKSTATION",
						"body":    "Implement {{ .WorkID }}.",
					},
				},
			})

			loaded, err := LoadRuntimeConfig(factoryDir, nil)
			if err != nil {
				t.Fatalf("LoadRuntimeConfig: %v", err)
			}

			workerDef, ok := loaded.Worker("executor")
			if !ok {
				t.Fatal("expected inline executor worker definition")
			}
			if got := modelprovider.Provider(workerDef.ModelProvider); got != tc.internal {
				t.Fatalf("runtime modelProvider = %q, want %q", got, tc.internal)
			}
		})
	}
}
