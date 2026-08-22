package authored

import (
	"strings"
	"testing"
)

func TestParseWorkerConfig_StrictFrontmatter(t *testing.T) {
	tests := []struct {
		name        string
		frontmatter string
		wantError   string
		wantModel   string
	}{
		{
			name: "canonical fields",
			frontmatter: `type: INFERENCE_WORKER
model: gpt-5.4
modelProvider: openai
resources:
  - name: inference-slot
    capacity: 1`,
			wantModel: "gpt-5.4",
		},
		{
			name: "retired top-level alias",
			frontmatter: `type: MODEL_WORKER
model_provider: openai`,
			wantError: "frontmatter.model_provider is not supported; use modelProvider",
		},
		{
			name:        "unknown top-level field",
			frontmatter: "type: MODEL_WORKER\nunexpected: true",
			wantError:   "frontmatter.unexpected is not supported",
		},
		{
			name: "unknown nested resource field",
			frontmatter: `type: MODEL_WORKER
resources:
  - name: inference-slot
    capacity: 1
    unexpected: true`,
			wantError: "frontmatter.resources[0].unexpected is not supported",
		},
		{
			name: "unknown nested auth field",
			frontmatter: `type: POLLER_WORKER
auth:
  secretRef: linear-token
  unexpected: true`,
			wantError: "frontmatter.auth.unexpected is not supported",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, err := ParseWorkerConfig(agentsConfigBytes(test.frontmatter), "workers/reviewer/AGENTS.md")
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("ParseWorkerConfig() error = %v, want %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseWorkerConfig() error = %v", err)
			}
			if config.Model != test.wantModel {
				t.Fatalf("model = %q, want %q", config.Model, test.wantModel)
			}
		})
	}
}

func TestParseWorkstationConfig_StrictFrontmatter(t *testing.T) {
	tests := []struct {
		name        string
		frontmatter string
		wantError   string
	}{
		{
			name: "canonical nested fields",
			frontmatter: `behavior: cron
type: INFERENCE_RUN
worker: cron-worker
cron:
  schedule: "0 * * * *"
  triggerAtStart: true
limits:
  maxRetries: 2`,
		},
		{
			name: "retired top-level alias",
			frontmatter: `behavior: repeater
runtime_type: MODEL_WORKSTATION
worker: reviewer`,
			wantError: "frontmatter.runtime_type is not supported; use type",
		},
		{
			name:        "unknown top-level field",
			frontmatter: "type: MODEL_WORKSTATION\nunexpected: true",
			wantError:   "frontmatter.unexpected is not supported",
		},
		{
			name: "retired nested cron alias",
			frontmatter: `behavior: cron
type: MODEL_WORKSTATION
worker: cron-worker
cron:
  schedule: "0 * * * *"
  trigger_at_start: true`,
			wantError: "frontmatter.cron.trigger_at_start is not supported; use triggerAtStart",
		},
		{
			name: "unknown nested cron field",
			frontmatter: `behavior: cron
type: MODEL_WORKSTATION
worker: cron-worker
cron:
  schedule: "0 * * * *"
  unexpected: true`,
			wantError: "frontmatter.cron.unexpected is not supported",
		},
		{
			name: "unknown nested input guard field",
			frontmatter: `type: LOGICAL_MOVE
inputs:
  - workType: task
    state: ready
    guard:
      type: SAME_NAME
      unexpected: task`,
			wantError: "frontmatter.inputs[0].guard.unexpected is not supported",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, err := ParseWorkstationConfig(agentsConfigBytes(test.frontmatter), "workstations/review/AGENTS.md")
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("ParseWorkstationConfig() error = %v, want %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseWorkstationConfig() error = %v", err)
			}
			if config.Cron == nil || !config.Cron.TriggerAtStart {
				t.Fatalf("cron config = %#v, want triggerAtStart", config.Cron)
			}
		})
	}
}

func agentsConfigBytes(frontmatter string) []byte {
	return []byte("---\n" + frontmatter + "\n---\nPrompt body.\n")
}
