package runtimetests

import (
	. "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"testing"
)

func TestLoadRuntimeConfig_ResolvesPublicWorkerModelProviderToCLICommand(t *testing.T) {
	cases := []struct {
		name     string
		public   string
		internal interfaces.ModelProvider
	}{
		{name: "CLAUDE", public: "CLAUDE", internal: interfaces.ModelProviderClaude},
		{name: "CODEX", public: "CODEX", internal: interfaces.ModelProviderCodex},
		{name: "CURSOR", public: "CURSOR", internal: interfaces.ModelProviderCursor},
		{name: "GEMINI", public: "GEMINI", internal: interfaces.ModelProviderGemini},
		{name: "KIRO", public: "KIRO", internal: interfaces.ModelProviderKiro},
		{name: "OPENCODE", public: "OPENCODE", internal: interfaces.ModelProviderOpenCode},
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
			if got := interfaces.ModelProvider(workerDef.ModelProvider); got != tc.internal {
				t.Fatalf("runtime modelProvider = %q, want %q", got, tc.internal)
			}
		})
	}
}
